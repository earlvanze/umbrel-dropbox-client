package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/config"
	"github.com/earl/umbrel-dropbox-sync/internal/scan"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
	"github.com/earl/umbrel-dropbox-sync/internal/worker"
)

type Daemon struct {
	cfg   config.Config
	store *state.Store
	log   *slog.Logger
}

type CycleStats struct {
	Root               string
	LocalFiles         int
	WorkerProcessed    int
	WorkerCompleted    int
	WorkerFailed       int
	WorkerProcessLimit int
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

func (d *Daemon) RunCycle(ctx context.Context) (CycleStats, error) {
	if d.store == nil {
		return CycleStats{}, fmt.Errorf("daemon missing store")
	}
	if d.cfg.Root == "" {
		return CycleStats{}, fmt.Errorf("daemon missing root")
	}
	if !d.cfg.DryRun {
		return CycleStats{}, fmt.Errorf("daemon live mode is not enabled; use CLI worker --live for guarded transfers")
	}
	files, err := scan.Walk(d.cfg.Root, scan.DefaultOptions())
	if err != nil {
		return CycleStats{}, err
	}
	for _, f := range files {
		if err := d.store.UpsertEntry(state.Entry{Path: scan.DropboxPath(f.Path), ContentHash: f.ContentHash, Size: f.Size, MTime: f.ModTime, State: "local_scanned"}); err != nil {
			return CycleStats{}, err
		}
	}
	limit := d.workerLimit()
	p := worker.Processor{Store: d.store, Handler: worker.DryRunHandler{Store: d.store}}
	stats := CycleStats{Root: d.cfg.Root, LocalFiles: len(files), WorkerProcessLimit: limit}
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
	if err := d.store.Event("daemon.cycle", fmt.Sprintf("root=%s local_files=%d worker_processed=%d worker_completed=%d worker_failed=%d", stats.Root, stats.LocalFiles, stats.WorkerProcessed, stats.WorkerCompleted, stats.WorkerFailed)); err != nil {
		return stats, err
	}
	d.log.Info("sync cycle complete", "root", stats.Root, "local_files", stats.LocalFiles, "worker_processed", stats.WorkerProcessed, "worker_completed", stats.WorkerCompleted, "worker_failed", stats.WorkerFailed)
	return stats, nil
}

func (d *Daemon) workerLimit() int {
	limit := d.cfg.UploadWorkers + d.cfg.DownloadWorkers
	if limit <= 0 {
		return 1
	}
	return limit
}
