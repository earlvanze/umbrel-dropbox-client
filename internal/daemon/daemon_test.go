package daemon

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/earl/umbrel-dropbox-sync/internal/config"
	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

func TestRunCycleScansLocalFilesAndProcessesDryRunQueue(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	if _, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: local, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: root, DryRun: true, UploadWorkers: 1, DownloadWorkers: 1}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := d.RunCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalFiles != 1 || stats.WorkerProcessed != 1 || stats.WorkerCompleted != 1 || stats.WorkerFailed != 0 {
		t.Fatalf("stats=%#v", stats)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 1 || st.PendingOps != 0 || st.LastEvent == "" {
		t.Fatalf("status=%#v", st)
	}
}

func TestRunCycleRejectsLiveDaemonMode(t *testing.T) {
	s := testStore(t)
	d := New(config.Config{Root: t.TempDir(), DryRun: false}, s, nil)
	_, err := d.RunCycle(context.Background())
	if err == nil {
		t.Fatal("expected live mode error")
	}
}

func testStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRunCycleSkipsWhenPaused(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	if err := s.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: root, DryRun: true}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := d.RunCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalFiles != 0 || stats.WorkerProcessed != 0 {
		t.Fatalf("stats=%#v", stats)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 0 || !st.Paused || st.LastEvent == "" {
		t.Fatalf("status=%#v", st)
	}
}

func TestHealthHandlerReturnsStatusJSON(t *testing.T) {
	s := testStore(t)
	if err := s.SetConfig("root", "/tmp/root"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: "/tmp/root", DryRun: true}, s, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"paused":true`) || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

type fakeDaemonRemoteClient struct {
	pages map[string]*dropbox.ListFolderResult
}

func (f fakeDaemonRemoteClient) ListFolder(ctx context.Context, path string, recursive bool) (*dropbox.ListFolderResult, error) {
	return f.pages[""], nil
}

func (f fakeDaemonRemoteClient) ListFolderContinue(ctx context.Context, cursor string) (*dropbox.ListFolderResult, error) {
	return f.pages[cursor], nil
}

func TestRunCycleIngestsRemoteDeltaWhenConfigured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	d := New(config.Config{Root: root, DryRun: true, RemoteDelta: true}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.remoteClient = fakeDaemonRemoteClient{pages: map[string]*dropbox.ListFolderResult{
		"": {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/remote.txt", Rev: "r1"}}, Cursor: "c1", HasMore: false},
	}}
	stats, err := d.RunCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.RemoteEntries != 1 || stats.RemoteAppliedFiles != 1 || stats.RemotePages != 1 || stats.LocalFiles != 1 {
		t.Fatalf("stats=%#v", stats)
	}
	cursor, err := s.GetConfig(state.DropboxCursorKey)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "c1" {
		t.Fatalf("cursor=%q", cursor)
	}
}
