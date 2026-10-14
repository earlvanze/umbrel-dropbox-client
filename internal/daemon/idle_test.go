package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
	"github.com/earlvanze/umbrel-dropbox-client/internal/watch"
)

func TestIdleCyclePollsRemoteAndDrainsWithoutScanningLocalFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "historical.txt"), []byte("local bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	// A missing historical entry must not be marked missing by an idle cycle.
	if err := s.UpsertEntry(state.Entry{Path: "/previous.txt", ContentHash: "old", State: "clean"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnqueueOp("download_remote", "/queued.txt", reconcile.PlannedOp{Op: "download_remote", Path: "/queued.txt", Rev: "r1"}); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: root, DryRun: true, Watch: true, RemoteDelta: true, DownloadWorkers: 3}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.remoteClient = fakeDaemonRemoteClient{pages: map[string]*dropbox.ListFolderResult{
		"":   {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/fresh.txt", Rev: "r2"}, {Tag: "file", PathLower: "/historical.txt", Rev: "r3", ContentHash: "different remote bytes"}}, Cursor: "c1"},
		"c1": {Cursor: "c2"},
	}}
	stats, err := d.RunCycleIncremental(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalFiles != 0 || stats.LocalChanged != 0 || stats.LocalMissing != 0 || stats.LocalUploadsQueued != 0 || stats.RemoteDownloadsQueued != 1 || stats.WorkerCompleted != 2 {
		t.Fatalf("stats=%#v", stats)
	}
	entry, err := s.EntryByPath("/previous.txt")
	if err != nil || entry == nil || entry.State != "clean" {
		t.Fatalf("entry=%#v err=%v", entry, err)
	}
	conflicts, err := s.ListConflicts(10)
	if err != nil || len(conflicts) != 1 || conflicts[0].Path != "/historical.txt" {
		t.Fatalf("conflicts=%#v err=%v", conflicts, err)
	}
	if !d.lastFullScan.IsZero() {
		t.Fatal("idle cycle performed full scan")
	}
	stats, err = d.RunCycleIncremental(context.Background(), true)
	if err != nil || stats.RemoteDownloadsQueued != 0 || stats.WorkerProcessed != 0 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
}

func TestIdleCycleRespectsPause(t *testing.T) {
	s := testStore(t)
	if err := s.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	id, err := s.EnqueueOp("download_remote", "/queued.txt", reconcile.PlannedOp{Op: "download_remote", Path: "/queued.txt", Rev: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	// No token or remote client: any attempt to poll while paused would fail.
	d := New(config.Config{Root: t.TempDir(), DryRun: true, Watch: true, RemoteDelta: true}, s, nil)
	stats, err := d.RunCycleIncremental(context.Background(), true)
	if err != nil || stats.WorkerProcessed != 0 || stats.RemotePages != 0 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	op, err := s.PendingOpByID(id)
	if err != nil || op == nil || op.Attempts != 0 {
		t.Fatalf("op=%#v err=%v", op, err)
	}
}

func TestPeriodicTickerDrainsQueueWithQuietWatcher(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 3; i++ {
		path := fmt.Sprintf("/queued%d.txt", i)
		if _, err := s.EnqueueOp("download_remote", path, reconcile.PlannedOp{Op: "download_remote", Path: path, Rev: "r1"}); err != nil {
			t.Fatal(err)
		}
	}
	d := New(config.Config{Root: t.TempDir(), DryRun: true, Watch: true, DownloadWorkers: 1, ScanIntervalSeconds: 1}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("daemon did not stop")
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, err := s.Status()
		if err != nil {
			t.Fatal(err)
		}
		if st.PendingOps == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("quiet watcher left queued transfers undrained")
}

func TestStartupWatchDrainHandlesClosedChannelAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprint(canceled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			events := make(chan watch.Event)
			close(events)
			if canceled {
				cancel()
			}
			done := make(chan error, 1)
			go func() { done <- drainPendingWatchEvents(ctx, events) }()
			select {
			case err := <-done:
				if err != ctx.Err() {
					t.Fatalf("err=%v want=%v", err, ctx.Err())
				}
			case <-time.After(time.Second):
				t.Fatal("drain spun on closed watcher channel")
			}
		})
	}
}
