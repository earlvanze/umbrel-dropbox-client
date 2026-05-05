package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
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
}

type CycleStats struct {
	Root               string
	LocalFiles         int
	WorkerProcessed    int
	WorkerCompleted    int
	WorkerFailed       int
	WorkerProcessLimit int
	RemotePages        int
	RemoteEntries      int
	RemoteAppliedFiles int
	LocalMissing       int
}

func New(cfg config.Config, store *state.Store, logger *slog.Logger) *Daemon {
	if logger == nil {
		logger = slog.Default()
	}
	return &Daemon{cfg: cfg, store: store, log: logger}
}

func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("daemon started", "root", d.cfg.Root, "dry_run", d.cfg.DryRun)
	_ = d.store.Event("daemon.start", d.cfg.Root)
	shutdownHealth := d.startHealth(ctx)
	defer shutdownHealth()
	if _, err := d.RunCycle(ctx); err != nil {
		return err
	}
	interval := time.Duration(d.cfg.ScanIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	watchEvents, stopWatch := d.startWatcher(ctx)
	defer stopWatch()
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
			if _, err := d.RunCycle(ctx); err != nil {
				_ = d.store.Event("daemon.error", err.Error())
				d.log.Error("sync cycle failed", "error", err)
			}
		case ev, ok := <-watchEvents:
			if !ok {
				watchEvents = nil
				continue
			}
			_ = d.store.Event("daemon.watch", ev.Path)
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
			if _, err := d.RunCycle(ctx); err != nil {
				_ = d.store.Event("daemon.error", err.Error())
				d.log.Error("watch-triggered sync cycle failed", "error", err)
			}
		}
	}
}

func (d *Daemon) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/ui":
			d.serveDashboard(w, r)
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

func (d *Daemon) serveDashboard(w http.ResponseWriter, _ *http.Request) {
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
	fmt.Fprint(w, `<h2>Auth</h2><p>Authenticate from CLI: <code>umbrel-dropbox-client auth pkce --client-id APP_KEY</code></p><p><a href="/status">status JSON</a> · <a href="/conflicts">conflicts JSON</a></p></body></html>`)
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

func (d *Daemon) RunCycle(ctx context.Context) (CycleStats, error) {
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
	if !d.cfg.DryRun {
		return CycleStats{}, fmt.Errorf("daemon live mode is not enabled; use CLI worker --live for guarded transfers")
	}
	remoteStats, err := d.ingestRemoteDelta(ctx)
	if err != nil {
		return CycleStats{}, err
	}
	files, err := scan.Walk(d.cfg.Root, scan.DefaultOptions())
	if err != nil {
		return CycleStats{}, err
	}
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		seen[scan.DropboxPath(f.Path)] = true
		if err := d.store.UpsertEntry(state.Entry{Path: scan.DropboxPath(f.Path), ContentHash: f.ContentHash, Size: f.Size, MTime: f.ModTime, State: "local_scanned"}); err != nil {
			return CycleStats{}, err
		}
	}
	missing, err := d.store.MarkMissingLocal(seen)
	if err != nil {
		return CycleStats{}, err
	}
	limit := d.workerLimit()
	p := worker.Processor{Store: d.store, Handler: worker.DryRunHandler{Store: d.store}}
	stats := CycleStats{Root: d.cfg.Root, LocalFiles: len(files), WorkerProcessLimit: limit, RemotePages: remoteStats.Pages, RemoteEntries: remoteStats.Entries, RemoteAppliedFiles: remoteStats.AppliedFiles, LocalMissing: missing}
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
	if err := d.store.Event("daemon.cycle", fmt.Sprintf("root=%s local_files=%d local_missing=%d remote_entries=%d remote_applied_files=%d worker_processed=%d worker_completed=%d worker_failed=%d", stats.Root, stats.LocalFiles, stats.LocalMissing, stats.RemoteEntries, stats.RemoteAppliedFiles, stats.WorkerProcessed, stats.WorkerCompleted, stats.WorkerFailed)); err != nil {
		return stats, err
	}
	d.log.Info("sync cycle complete", "root", stats.Root, "local_files", stats.LocalFiles, "local_missing", stats.LocalMissing, "remote_entries", stats.RemoteEntries, "remote_applied_files", stats.RemoteAppliedFiles, "worker_processed", stats.WorkerProcessed, "worker_completed", stats.WorkerCompleted, "worker_failed", stats.WorkerFailed)
	return stats, nil
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
	return d.store.IngestRemoteDelta(ctx, client, d.cfg.RemotePath)
}

func (d *Daemon) loadDropboxAccessToken(ctx context.Context) (string, error) {
	if d.cfg.TokenFile == "" {
		return "", fmt.Errorf("remote_delta requires token_file")
	}
	tok, err := auth.LoadToken(d.cfg.TokenFile)
	if err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("remote_delta token_file has no access token")
	}
	if tok.ExpiresAt.IsZero() || time.Now().Before(tok.ExpiresAt.Add(-1*time.Minute)) {
		return tok.AccessToken, nil
	}
	clientID := tok.ClientID
	if clientID == "" {
		clientID = os.Getenv("DROPBOX_CLIENT_ID")
	}
	if clientID == "" || tok.RefreshToken == "" {
		return "", fmt.Errorf("remote_delta token expired and cannot refresh; run auth refresh --client-id APP_KEY")
	}
	refreshed, err := dropbox.NewOAuthClient(clientID).RefreshToken(ctx, tok.RefreshToken)
	if err != nil {
		return "", err
	}
	next := auth.TokenFromDropbox(refreshed.AccessToken, refreshed.RefreshToken, refreshed.TokenType, refreshed.ExpiresIn, refreshed.AccountID, refreshed.Scope, time.Now())
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
