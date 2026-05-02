package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/auth"
	"github.com/earl/umbrel-dropbox-sync/internal/config"
	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/scan"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
	"github.com/earl/umbrel-dropbox-sync/internal/worker"
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
	for {
		select {
		case <-ctx.Done():
			_ = d.store.Event("daemon.stop", ctx.Err().Error())
			return ctx.Err()
		case <-ticker.C:
			if _, err := d.RunCycle(ctx); err != nil {
				_ = d.store.Event("daemon.error", err.Error())
				d.log.Error("sync cycle failed", "error", err)
			}
		}
	}
}

func (d *Daemon) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
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
	})
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
		tok, err := auth.LoadToken(d.cfg.TokenFile)
		if err != nil {
			return state.RemoteDeltaStats{}, err
		}
		if tok.AccessToken == "" {
			return state.RemoteDeltaStats{}, fmt.Errorf("remote_delta token_file has no access token")
		}
		client = dropbox.New(tok.AccessToken)
	}
	return d.store.IngestRemoteDelta(ctx, client, d.cfg.RemotePath)
}
