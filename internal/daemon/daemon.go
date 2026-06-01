package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	slashpath "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/auth"
	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
	"github.com/earlvanze/umbrel-dropbox-client/internal/watch"
	"github.com/earlvanze/umbrel-dropbox-client/internal/worker"
)

type Daemon struct {
	cfg          config.Config
	store        *state.Store
	log          *slog.Logger
	remoteClient state.RemoteDeltaClient
	dirty        *watch.DirtySet
	lastFullScan time.Time
}

type CycleStats struct {
	Root               string
	LocalFiles         int
	LocalChanged       int
	WorkerProcessed    int
	WorkerCompleted    int
	WorkerFailed       int
	WorkerProcessLimit int
	RemotePages        int
	RemoteEntries      int
	RemoteAppliedFiles int
	LocalMissing       int
	Incremental        bool
}

func New(cfg config.Config, store *state.Store, logger *slog.Logger) *Daemon {
	if logger == nil {
		logger = slog.Default()
	}
	d := &Daemon{cfg: cfg, store: store, log: logger}
	if cfg.Watch {
		abs, err := filepath.Abs(cfg.Root)
		if err == nil {
			d.dirty = watch.NewDirtySet(abs)
		}
	}
	return d
}

func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("daemon started", "root", d.cfg.Root, "dry_run", d.cfg.DryRun)
	_ = d.store.Event("daemon.start", d.cfg.Root)
	shutdownHealth := d.startHealth(ctx)
	defer shutdownHealth()
	watchEvents, stopWatch := d.startWatcher(ctx)
	defer stopWatch()
	// Watch events are collected into dirty set in the main select loop
	// Initial full scan
	if _, err := d.RunCycle(ctx); err != nil {
		return err
	}
	d.lastFullScan = time.Now()
	interval := time.Duration(d.cfg.ScanIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	fullScanInterval := time.Duration(d.cfg.FullScanInterval()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	debounce := d.watchDebounce()
	var debounceTimer *time.Timer
	var debounceC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			_ = d.store.Event("daemon.stop", ctx.Err().Error())
			return ctx.Err()
		case <-ticker.C:
			// Periodic scan: full if interval elapsed, otherwise incremental
			incremental := false
			if d.dirty != nil && d.dirty.Len() == 0 && time.Since(d.lastFullScan) < fullScanInterval {
				// No dirty paths and full scan not due; skip periodic cycle
				continue
			}
			if d.dirty != nil && d.dirty.Len() > 0 {
				incremental = true
			} else if time.Since(d.lastFullScan) >= fullScanInterval {
				incremental = false
			}
			if _, err := d.RunCycleIncremental(ctx, incremental); err != nil {
				_ = d.store.Event("daemon.error", err.Error())
				d.log.Error("sync cycle failed", "error", err)
			}
		case ev, ok := <-watchEvents:
			if !ok {
				watchEvents = nil
				continue
			}
			_ = d.store.Event("daemon.watch", ev.Path)
			if d.dirty != nil {
				d.dirty.Add(ev.Path)
			}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(debounce)
				debounceC = debounceTimer.C
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(debounce)
			}
		case <-debounceC:
			debounceC = nil
			debounceTimer = nil
			// Watch-triggered cycles are always incremental
			if _, err := d.RunCycleIncremental(ctx, true); err != nil {
				_ = d.store.Event("daemon.error", err.Error())
				d.log.Error("watch-triggered sync cycle failed", "error", err)
			}
		}
	}
}


func (d *Daemon) RunCycle(ctx context.Context) (CycleStats, error) {
	return d.RunCycleIncremental(ctx, false)
}

func (d *Daemon) RunCycleIncremental(ctx context.Context, incremental bool) (CycleStats, error) {
	if d.store == nil {
		return CycleStats{}, fmt.Errorf("daemon missing store")
	}
	if d.cfg.Root == "" {
		return CycleStats{}, fmt.Errorf("daemon missing root")
	}
	paused, err := d.store.IsPaused()
	if err != nil {
		return CycleStats{}, err
	}
	if paused {
		if err := d.store.Event("daemon.paused", "cycle skipped"); err != nil {
			return CycleStats{}, err
		}
		return CycleStats{Root: d.cfg.Root}, nil
	}
	if !d.cfg.DryRun && !d.cfg.AllowLive {
		return CycleStats{}, fmt.Errorf("daemon live mode requires allow_live=true")
	}
	remoteStats, err := d.ingestRemoteDelta(ctx)
	if err != nil {
		return CycleStats{}, err
	}

	// Determine scan scope
	dirtyDirs := []string{}
	if incremental && d.dirty != nil {
		dirtyDirs = d.dirty.Dirs()
		if len(dirtyDirs) == 0 {
			// Debounce fired but no actual dirty paths; skip
			return CycleStats{Root: d.cfg.Root, Incremental: true}, nil
		}
		dirtyDirs = watch.SplitParentDirs(dirtyDirs)
	}

	known, err := d.store.LocalEntries()
	if err != nil {
		return CycleStats{}, err
	}
	scanOpts := d.buildScanOpts(known)

	var files []scan.File
	if incremental && len(dirtyDirs) > 0 {
		// Incremental scan: only scan changed directories
		absDirs := make([]string, 0, len(dirtyDirs))
		root, err := filepath.Abs(d.cfg.Root)
		if err != nil {
			return CycleStats{}, err
		}
		for _, dir := range dirtyDirs {
			if dir == "" {
				absDirs = append(absDirs, root)
			} else {
				absDirs = append(absDirs, filepath.Join(root, dir))
			}
		}
		files, err = scan.WalkDirs(d.cfg.Root, absDirs, scanOpts)
		if err != nil {
			return CycleStats{}, err
		}
	} else {
		// Full scan
		files, err = scan.Walk(d.cfg.Root, scanOpts)
		if err != nil {
			return CycleStats{}, err
		}
		d.lastFullScan = time.Now()
	}

	// Upsert scanned files, skipping unchanged rows
	seen := make(map[string]bool, len(files))
	changed := 0
	for _, f := range files {
		dp := scan.DropboxPath(f.Path)
		seen[dp] = true
		didChange, err := d.store.UpsertEntryIfChanged(state.Entry{Path: dp, ContentHash: f.ContentHash, Size: f.Size, MTime: f.ModTime, State: "local_scanned"})
		if err != nil {
			return CycleStats{}, err
		}
		if didChange {
			changed++
		}
	}

	// Mark missing files
	var missing int
	if incremental && len(dirtyDirs) > 0 {
		// Only check missing in changed directories
		missing, err = d.store.MarkMissingLocalInDirs(seen, dirtyDirs)
		if err != nil {
			return CycleStats{}, err
		}
	} else {
		missing, err = d.store.MarkMissingLocal(seen)
		if err != nil {
			return CycleStats{}, err
		}
	}

	limit := d.workerLimit()
	handler, err := d.workerHandler(ctx)
	if err != nil {
		return CycleStats{}, err
	}
	p := worker.Processor{Store: d.store, Handler: handler}
	stats := CycleStats{Root: d.cfg.Root, LocalFiles: len(files), LocalChanged: changed, WorkerProcessLimit: limit, RemotePages: remoteStats.Pages, RemoteEntries: remoteStats.Entries, RemoteAppliedFiles: remoteStats.AppliedFiles, LocalMissing: missing, Incremental: incremental}
	for stats.WorkerProcessed < limit {
		res, err := p.ProcessOne(ctx)
		if err != nil {
			return stats, err
		}
		if !res.Processed {
			break
		}
		stats.WorkerProcessed++
		if res.Completed {
			stats.WorkerCompleted++
		}
		if res.Failed {
			stats.WorkerFailed++
		}
	}
	scanMode := "full"
	if incremental {
		scanMode = "incremental"
	}
	if err := d.store.Event("daemon.cycle", fmt.Sprintf("root=%s mode=%s local_files=%d local_changed=%d local_missing=%d remote_entries=%d remote_applied_files=%d worker_processed=%d worker_completed=%d worker_failed=%d", stats.Root, scanMode, stats.LocalFiles, stats.LocalChanged, stats.LocalMissing, stats.RemoteEntries, stats.RemoteAppliedFiles, stats.WorkerProcessed, stats.WorkerCompleted, stats.WorkerFailed)); err != nil {
		return stats, err
	}
	d.log.Info("sync cycle complete", "root", stats.Root, "mode", scanMode, "local_files", stats.LocalFiles, "local_changed", stats.LocalChanged, "local_missing", stats.LocalMissing, "remote_entries", stats.RemoteEntries, "remote_applied_files", stats.RemoteAppliedFiles, "worker_processed", stats.WorkerProcessed, "worker_completed", stats.WorkerCompleted, "worker_failed", stats.WorkerFailed)
	return stats, nil
}

func (d *Daemon) buildScanOpts(known map[string]state.Entry) scan.Options {
	opts := scan.DefaultOptions()
	opts.KnownFiles = make(map[string]scan.KnownFile, len(known))
	for path, entry := range known {
		opts.KnownFiles[path] = scan.KnownFile{Size: entry.Size, ModTime: entry.MTime, ContentHash: entry.ContentHash}
	}
	// Merge extra ignore dirs from config
	for _, dir := range d.cfg.ExtraIgnoreDirs() {
		opts.IgnoreDirs[dir] = true
	}
	// Apply selective sync scope
	opts.ShouldScan = func(relPath string, isDir bool) bool {
		return d.cfg.IsPathInSyncScope(relPath)
	}
	return opts
}

func (d *Daemon) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/ui":
			d.serveDashboard(w, r)
		case "/setup":
			d.serveSetupHTML(w, r)
		case "/files":
			d.serveFilesHTML(w, r)
		case "/api/files":
			d.serveFilesJSON(w, r)
		case "/api/config":
			d.serveConfigAPI(w, r)
		case "/api/setup":
			d.serveSetupStatus(w, r)
		case "/api/auth/device":
			d.serveAuthDeviceStart(w, r)
		case "/api/auth/device-poll":
			d.serveAuthDevicePoll(w, r)
		case "/api/remote/folders":
			d.serveRemoteFolders(w, r)
		case "/download":
			d.serveDownload(w, r)
		case "/healthz", "/status":
			d.serveStatusJSON(w, r)
		case "/conflicts":
			d.serveConflictsJSON(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (d *Daemon) serveStatusJSON(w http.ResponseWriter, _ *http.Request) {
	st, err := d.store.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"root":        st.Root,
		"paused":      st.Paused,
		"entries":     st.Entries,
		"pending_ops": st.PendingOps,
		"conflicts":   st.Conflicts,
		"last_event":  st.LastEvent,
	})
}

func (d *Daemon) serveConflictsJSON(w http.ResponseWriter, _ *http.Request) {
	items, err := d.store.ListConflicts(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"conflicts": items})
}

func (d *Daemon) serveDashboard(w http.ResponseWriter, r *http.Request) {
	// Redirect to setup if not configured
	tokStatus, _ := auth.TokenStatus(d.cfg.TokenFile)
	if d.cfg.Root == "" || !tokStatus.Present {
		http.Redirect(w, r, "/setup", http.StatusTemporaryRedirect)
		return
	}
	st, err := d.store.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	conflicts, err := d.store.ListConflicts(10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Umbrel Dropbox Client</title><style>body{font-family:system-ui,sans-serif;max-width:860px;margin:2rem auto;padding:0 1rem;line-height:1.4}code{background:#f4f4f5;padding:.1rem .3rem;border-radius:.25rem}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.75rem}.card{border:1px solid #ddd;border-radius:.75rem;padding:1rem}.muted{color:#666}li{margin:.35rem 0}</style></head><body><h1>Umbrel Dropbox Client</h1><p class="muted">Dry-run-first sync dashboard. Live file transfer remains CLI-gated.</p><div class="grid"><div class="card"><strong>Paused</strong><br>%v</div><div class="card"><strong>Entries</strong><br>%d</div><div class="card"><strong>Pending ops</strong><br>%d</div><div class="card"><strong>Conflicts</strong><br>%d</div></div><p><strong>Root:</strong> <code>%s</code></p><p><strong>Last event:</strong> <code>%s</code></p><h2>Recent conflicts</h2>`, st.Paused, st.Entries, st.PendingOps, st.Conflicts, html.EscapeString(st.Root), html.EscapeString(st.LastEvent))
	if len(conflicts) == 0 {
		fmt.Fprint(w, `<p class="muted">No conflicts recorded.</p>`)
	} else {
		fmt.Fprint(w, "<ul>")
		for _, c := range conflicts {
			fmt.Fprintf(w, `<li><code>#%d</code> %s <span class="muted">%s</span></li>`, c.ID, html.EscapeString(c.Path), html.EscapeString(c.Reason))
		}
		fmt.Fprint(w, "</ul>")
	}
	fmt.Fprint(w, `<h2>Auth</h2><p>Authenticate from CLI: <code>umbrel-dropbox-client auth pkce --client-id APP_KEY</code></p><p><a href="/files">local file manager</a> · <a href="/status">status JSON</a> · <a href="/conflicts">conflicts JSON</a></p></body></html>`)
}

type localFileItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Dir     bool   `json:"dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modified"`
}

type localFileListing struct {
	Root  string          `json:"root"`
	Path  string          `json:"path"`
	Items []localFileItem `json:"items"`
}

func (d *Daemon) serveFilesJSON(w http.ResponseWriter, r *http.Request) {
	listing, err := d.localFileListing(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(listing)
}

func (d *Daemon) serveFilesHTML(w http.ResponseWriter, r *http.Request) {
	listing, err := d.localFileListing(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Local files - Umbrel Dropbox Client</title><style>body{font-family:system-ui,sans-serif;max-width:1040px;margin:2rem auto;padding:0 1rem;line-height:1.4;color:#18181b}a{color:#0061ff;text-decoration:none}a:hover{text-decoration:underline}.muted{color:#71717a}.top{display:flex;justify-content:space-between;align-items:center;gap:1rem;flex-wrap:wrap}.crumbs{margin:1rem 0;padding:.75rem 1rem;background:#f4f4f5;border-radius:.75rem}table{width:100%%;border-collapse:collapse;background:white;border:1px solid #e4e4e7;border-radius:.75rem;overflow:hidden}th,td{padding:.75rem;border-bottom:1px solid #eee;text-align:left}th{font-size:.85rem;color:#52525b;background:#fafafa}tr:last-child td{border-bottom:0}.name{font-weight:600}.size,.modified{white-space:nowrap}.pill{display:inline-block;font-size:.75rem;border:1px solid #d4d4d8;border-radius:999px;padding:.1rem .45rem;color:#52525b}</style></head><body><div class="top"><div><h1>Local files</h1><p class="muted">Read-only browser for the configured sync root.</p></div><p><a href="/">Dashboard</a> · <a href="/api/files?path=%s">JSON</a></p></div>`, html.EscapeString(url.QueryEscape(listing.Path)))
	fmt.Fprintf(w, `<p><strong>Root:</strong> <code>%s</code></p><div class="crumbs">`, html.EscapeString(listing.Root))
	fmt.Fprint(w, `<a href="/files">root</a>`)
	if listing.Path != "" {
		parts := strings.Split(listing.Path, "/")
		for i, part := range parts {
			crumb := strings.Join(parts[:i+1], "/")
			fmt.Fprintf(w, ` / <a href="/files?path=%s">%s</a>`, html.EscapeString(url.QueryEscape(crumb)), html.EscapeString(part))
		}
	}
	fmt.Fprint(w, `</div><table><thead><tr><th>Name</th><th>Kind</th><th>Size</th><th>Modified</th><th></th></tr></thead><tbody>`)
	if listing.Path != "" {
		parent := slashpath.Dir("/" + listing.Path)
		if parent == "/" {
			parent = ""
		} else {
			parent = strings.TrimPrefix(parent, "/")
		}
		fmt.Fprintf(w, `<tr><td class="name"><a href="/files?path=%s">..</a></td><td><span class="pill">folder</span></td><td></td><td></td><td></td></tr>`, html.EscapeString(url.QueryEscape(parent)))
	}
	for _, item := range listing.Items {
		kind := "file"
		link := "/download?path=" + url.QueryEscape(item.Path)
		action := "download"
		if item.Dir {
			kind = "folder"
			link = "/files?path=" + url.QueryEscape(item.Path)
			action = "open"
		}
		fmt.Fprintf(w, `<tr><td class="name"><a href="%s">%s</a></td><td><span class="pill">%s</span></td><td class="size">%s</td><td class="modified">%s</td><td><a href="%s">%s</a></td></tr>`, html.EscapeString(link), html.EscapeString(item.Name), kind, html.EscapeString(formatBytes(item.Size, item.Dir)), html.EscapeString(item.ModTime), html.EscapeString(link), action)
	}
	fmt.Fprint(w, `</tbody></table></body></html>`)
}

func (d *Daemon) serveDownload(w http.ResponseWriter, r *http.Request) {
	full, _, err := d.resolveLocalPath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "cannot download a directory", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, full)
}

func (d *Daemon) localFileListing(rel string) (localFileListing, error) {
	full, clean, err := d.resolveLocalPath(rel)
	if err != nil {
		return localFileListing{}, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return localFileListing{}, err
	}
	items := make([]localFileItem, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		child := name
		if clean != "" {
			child = clean + "/" + name
		}
		items = append(items, localFileItem{Name: name, Path: child, Dir: info.IsDir(), Size: info.Size(), ModTime: info.ModTime().Format("2006-01-02 15:04:05")})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Dir != items[j].Dir {
			return items[i].Dir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return localFileListing{Root: d.cfg.Root, Path: clean, Items: items}, nil
}

func (d *Daemon) resolveLocalPath(rel string) (string, string, error) {
	if d.cfg.Root == "" {
		return "", "", fmt.Errorf("configured root is empty")
	}
	if strings.ContainsRune(rel, '\x00') {
		return "", "", fmt.Errorf("invalid path")
	}
	clean := slashpath.Clean("/" + strings.TrimSpace(rel))
	if clean == "/" || clean == "." {
		clean = ""
	} else {
		clean = strings.TrimPrefix(clean, "/")
	}
	rootAbs, err := filepath.Abs(d.cfg.Root)
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(rootAbs, filepath.FromSlash(clean))
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", "", err
	}
	relToRoot, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil {
		return "", "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("path escapes configured root")
	}
	return fullAbs, clean, nil
}

func formatBytes(size int64, isDir bool) string {
	if isDir {
		return ""
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func (d *Daemon) startHealth(ctx context.Context) func() {
	if d.cfg.HealthAddr == "" {
		return func() {}
	}
	server := &http.Server{Addr: d.cfg.HealthAddr, Handler: d.HealthHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			_ = d.store.Event("daemon.health_error", err.Error())
			d.log.Error("health server stopped", "error", err)
		}
	}()
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
}

func (d *Daemon) workerHandler(ctx context.Context) (worker.Handler, error) {
	if d.cfg.DryRun {
		return worker.DryRunHandler{Store: d.store}, nil
	}
	if !d.cfg.AllowLive {
		return nil, fmt.Errorf("live worker requires allow_live=true")
	}
	if d.cfg.TokenFile == "" {
		return nil, fmt.Errorf("live worker requires token_file")
	}
	accessToken, err := d.loadDropboxAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	client := dropbox.New(accessToken)
	return worker.LiveHandler{
		Transfer: worker.TransferHandler{Store: d.store, Client: client, Root: d.cfg.Root, AllowLive: true},
		Deletes:  worker.ReviewedDeleteHandler{Store: d.store, Client: client, AllowLive: true, AllowReviewedDeletes: false},
	}, nil
}

func (d *Daemon) workerLimit() int {
	limit := d.cfg.UploadWorkers + d.cfg.DownloadWorkers
	if limit <= 0 {
		return 1
	}
	return limit
}

func (d *Daemon) ingestRemoteDelta(ctx context.Context) (state.RemoteDeltaStats, error) {
	if !d.cfg.RemoteDelta {
		return state.RemoteDeltaStats{}, nil
	}
	client := d.remoteClient
	if client == nil {
		if d.cfg.TokenFile == "" {
			return state.RemoteDeltaStats{}, fmt.Errorf("remote_delta requires token_file")
		}
		accessToken, err := d.loadDropboxAccessToken(ctx)
		if err != nil {
			return state.RemoteDeltaStats{}, err
		}
		client = dropbox.New(accessToken)
	}
	return d.store.IngestRemoteDeltaFilter(ctx, client, d.cfg.RemotePath, d.cfg.IsPathInSyncScope)
}

func (d *Daemon) loadDropboxAccessToken(ctx context.Context) (string, error) {
	if d.cfg.TokenFile == "" {
		return "", fmt.Errorf("remote_delta requires token_file")
	}
	tok, err := auth.LoadToken(d.cfg.TokenFile)
	if err != nil {
		return "", err
	}
	if tok.AccessToken != "" && (tok.ExpiresAt.IsZero() || time.Now().Before(tok.ExpiresAt.Add(-1*time.Minute))) {
		return tok.AccessToken, nil
	}
	clientID := tok.ClientID
	if clientID == "" {
		clientID = os.Getenv("DROPBOX_CLIENT_ID")
	}
	if clientID == "" || tok.RefreshToken == "" {
		if tok.AccessToken == "" {
			return "", fmt.Errorf("remote_delta token_file has no access token and cannot refresh; run auth pkce --client-id APP_KEY")
		}
		return "", fmt.Errorf("remote_delta token expired and cannot refresh; run auth refresh --client-id APP_KEY")
	}
	refreshed, err := dropbox.NewOAuthClient(clientID).RefreshToken(ctx, tok.RefreshToken)
	if err != nil {
		return "", err
	}
	next := auth.TokenFromDropbox(refreshed.AccessToken, refreshed.RefreshToken, refreshed.TokenType, refreshed.ExpiresIn, refreshed.AccountID, refreshed.Scope, time.Now())
	if next.RefreshToken == "" {
		next.RefreshToken = tok.RefreshToken
	}
	if next.AccountID == "" {
		next.AccountID = tok.AccountID
	}
	if next.Scope == "" {
		next.Scope = tok.Scope
	}
	next.ClientID = clientID
	if err := auth.SaveToken(d.cfg.TokenFile, next); err != nil {
		return "", err
	}
	return next.AccessToken, nil
}

func (d *Daemon) startWatcher(ctx context.Context) (<-chan watch.Event, func()) {
	if !d.cfg.Watch {
		return nil, func() {}
	}
	w, err := watch.New(d.cfg.Root, watch.DefaultOptions())
	if err != nil {
		_ = d.store.Event("daemon.watch_error", err.Error())
		d.log.Error("filesystem watcher disabled", "error", err)
		return nil, func() {}
	}
	d.log.Info("filesystem watcher started", "root", d.cfg.Root)
	return w.Events(ctx), func() { _ = w.Close() }
}

func (d *Daemon) watchDebounce() time.Duration {
	if d.cfg.WatchDebounceMs <= 0 {
		return 1500 * time.Millisecond
	}
	return time.Duration(d.cfg.WatchDebounceMs) * time.Millisecond
}
