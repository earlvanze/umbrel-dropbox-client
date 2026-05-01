package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/config"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

type Daemon struct {
	cfg   config.Config
	store *state.Store
	log   *slog.Logger
}

func New(cfg config.Config, store *state.Store, logger *slog.Logger) *Daemon {
	return &Daemon{cfg: cfg, store: store, log: logger}
}
func (d *Daemon) Run(ctx context.Context) error {
	d.log.Info("daemon started", "root", d.cfg.Root, "dry_run", d.cfg.DryRun)
	_ = d.store.Event("daemon.start", d.cfg.Root)
	ticker := time.NewTicker(time.Duration(d.cfg.ScanIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = d.store.Event("daemon.stop", ctx.Err().Error())
			return ctx.Err()
		case <-ticker.C:
			_ = d.store.Event("daemon.tick", "scan placeholder")
			d.log.Info("sync tick", "dry_run", d.cfg.DryRun)
		}
	}
}
