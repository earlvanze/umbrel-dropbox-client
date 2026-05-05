package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

func TestDryRunFixtureProducesDeterministicPlanAndQueueCounts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "local-only.txt"), "local only")
	writeFile(t, filepath.Join(root, "same.txt"), "same")
	writeFile(t, filepath.Join(root, "conflict.txt"), "local conflict")
	writeFile(t, filepath.Join(root, ".umbrel-dropbox-client", "ignored.db"), "ignored")

	files, err := scan.Walk(root, scan.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("local files=%d files=%#v", len(files), files)
	}

	remote := []dropbox.Metadata{
		{Tag: "file", PathLower: "/same.txt", Rev: "same-rev", ContentHash: contentHash(t, filepath.Join(root, "same.txt")), Size: 4, ServerMtime: fixedTime()},
		{Tag: "file", PathLower: "/remote-only.txt", ID: "id:remote", Rev: "remote-rev", ContentHash: hashString(t, "remote only"), Size: int64(len("remote only")), ServerMtime: fixedTime()},
		{Tag: "file", PathLower: "/conflict.txt", ID: "id:conflict", Rev: "remote-conflict-rev", ContentHash: hashString(t, "remote conflict"), Size: int64(len("remote conflict")), ServerMtime: fixedTime()},
		{Tag: "folder", PathLower: "/ignored-folder"},
	}

	plan := reconcile.BuildDryRunPlan(files, remote)
	if plan.Noop != 1 || len(plan.Ops) != 2 || len(plan.Conflicts) != 1 {
		t.Fatalf("plan counts noop=%d ops=%d conflicts=%d plan=%#v", plan.Noop, len(plan.Ops), len(plan.Conflicts), plan)
	}
	if plan.Ops[0].Op != "upload_local" || plan.Ops[0].Path != "/local-only.txt" {
		t.Fatalf("upload op=%#v", plan.Ops[0])
	}
	if plan.Ops[1].Op != "download_remote" || plan.Ops[1].Path != "/remote-only.txt" || plan.Ops[1].Rev != "remote-rev" {
		t.Fatalf("download op=%#v", plan.Ops[1])
	}
	if plan.Conflicts[0].Path != "/conflict.txt" || plan.Conflicts[0].RemoteRev != "remote-conflict-rev" {
		t.Fatalf("conflict=%#v", plan.Conflicts[0])
	}

	s, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := s.UpsertEntry(state.Entry{Path: scan.DropboxPath(f.Path), ContentHash: f.ContentHash, Size: f.Size, MTime: f.ModTime, State: "local_scanned"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, op := range plan.Ops {
		if _, created, err := s.EnqueueOpIfMissing(op.Op, op.Path, op); err != nil || !created {
			t.Fatalf("enqueue op=%#v created=%v err=%v", op, created, err)
		}
	}
	for _, c := range plan.Conflicts {
		if _, created, err := s.AddConflictIfMissing(c.Path, c.Reason, c.LocalPath, c.RemoteRev); err != nil || !created {
			t.Fatalf("conflict=%#v created=%v err=%v", c, created, err)
		}
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 3 || st.PendingOps != 2 || st.Conflicts != 1 {
		t.Fatalf("status=%#v", st)
	}
}

func writeFile(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func contentHash(t testing.TB, path string) string {
	t.Helper()
	h, err := hash.DropboxContentHash(path)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func hashString(t testing.TB, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blob")
	writeFile(t, path, body)
	return contentHash(t, path)
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 2, 15, 30, 0, 0, time.UTC)
}
