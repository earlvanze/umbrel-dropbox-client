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
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
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
	if stats.LocalFiles != 1 || stats.LocalMissing != 0 || stats.WorkerProcessed != 1 || stats.WorkerCompleted != 1 || stats.WorkerFailed != 0 {
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

func TestRunCycleMarksPreviouslySeenLocalFilesMissing(t *testing.T) {
	root := t.TempDir()
	s := testStore(t)
	if err := s.UpsertEntry(state.Entry{Path: "/missing.txt", ContentHash: "old", State: "clean"}); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: root, DryRun: true}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	stats, err := d.RunCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.LocalMissing != 1 {
		t.Fatalf("stats=%#v", stats)
	}
	items, err := s.ListMissingLocal(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "/missing.txt" {
		t.Fatalf("items=%#v", items)
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

func TestDashboardHandlerReturnsHTMLAndConflictsJSON(t *testing.T) {
	s := testStore(t)
	if err := s.SetConfig("root", "/tmp/root"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddConflict("/a.txt", "hash_mismatch", "/tmp/root/a.txt", "rev1"); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: "/tmp/root", DryRun: true}, s, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Umbrel Dropbox Client") || !strings.Contains(rr.Body.String(), "/a.txt") {
		t.Fatalf("body=%s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/conflicts", nil)
	rr = httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"Path":"/a.txt"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestFilesHandlerBrowsesConfiguredRootReadOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{Root: root, DryRun: true}, testStore(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/files?path=nested", nil)
	rr := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "a.txt") || !strings.Contains(rr.Body.String(), "download") {
		t.Fatalf("body=%s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/files?path=nested", nil)
	rr = httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"name":"a.txt"`) {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/download?path=nested/a.txt", nil)
	rr = httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "hello" {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFilesHandlerRejectsRootEscape(t *testing.T) {
	d := New(config.Config{Root: t.TempDir(), DryRun: true}, testStore(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/files?path=../../", nil)
	rr := httptest.NewRecorder()
	d.HealthHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cleaned root traversal should resolve to root, code=%d body=%s", rr.Code, rr.Body.String())
	}
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
	cursor, err := s.GetConfig(state.DropboxCursorKeyForPath(""))
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "c1" {
		t.Fatalf("cursor=%q", cursor)
	}
}

func TestRunTriggersCycleFromWatcherEvent(t *testing.T) {
	root := t.TempDir()
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	d := New(config.Config{Root: root, DryRun: true, Watch: true, WatchDebounceMs: 50, ScanIntervalSeconds: 3600, HealthAddr: ""}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitForEntries(t, s, 0)
	if err := os.WriteFile(filepath.Join(root, "watched.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	waitForEntries(t, s, 1)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("run err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func waitForEntries(t *testing.T, s *state.Store, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, err := s.Status()
		if err != nil {
			t.Fatal(err)
		}
		if st.Entries == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	st, _ := s.Status()
	t.Fatalf("entries=%d want=%d", st.Entries, want)
}
