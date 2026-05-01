package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/earl/umbrel-dropbox-sync/internal/config"
	"github.com/earl/umbrel-dropbox-sync/internal/daemon"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

func main() {
	cfgPath := flag.String("config", filepath.Join(os.Getenv("HOME"), ".config", "umbrel-dropbox-sync", "config.json"), "config path")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	st, err := state.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	_ = st.Init()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.New(cfg, st, slog.Default()).Run(ctx); err != nil && err != context.Canceled {
		slog.Error("daemon stopped", "error", err)
		os.Exit(1)
	}
}
