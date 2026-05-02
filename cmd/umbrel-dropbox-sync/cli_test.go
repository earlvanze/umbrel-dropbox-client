package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

func TestCLIInitSyncStatusDryRunFixture(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, ".umbrel-dropbox-sync", "state.db")
	cfg := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { cmdInit([]string{"--root", root, "--db", db, "--config", cfg}) })
	if !strings.Contains(out, "initialized root=") || !strings.Contains(out, "db=") || !strings.Contains(out, "config=") {
		t.Fatalf("init output=%q", out)
	}
	out = captureStdout(t, func() { cmdSync([]string{"--once", "--dry-run", "--root", root, "--db", db}) })
	if !strings.Contains(out, "dry-run scan complete") || !strings.Contains(out, "local_files=1") || !strings.Contains(out, "remote_files=0") {
		t.Fatalf("sync output=%q", out)
	}
	out = captureStdout(t, func() { cmdStatus([]string{"--db", db}) })
	for _, want := range []string{"entries: 1", "pending_ops: 0", "conflicts: 0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q: %q", want, out)
		}
	}

	if _, err := os.Stat(cfg); err != nil {
		t.Fatal(err)
	}
	s, err := state.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Root != root || st.Entries != 1 || st.PendingOps != 0 || st.Conflicts != 0 {
		t.Fatalf("status=%#v", st)
	}
}

func TestCLIPauseResumeFixture(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, ".umbrel-dropbox-sync", "state.db")
	captureStdout(t, func() { cmdInit([]string{"--root", root, "--db", db}) })
	out := captureStdout(t, func() { cmdPause([]string{"--db", db}, true) })
	if !strings.Contains(out, "paused db=") {
		t.Fatalf("pause output=%q", out)
	}
	out = captureStdout(t, func() { cmdStatus([]string{"--db", db}) })
	if !strings.Contains(out, "paused: true") {
		t.Fatalf("status paused output=%q", out)
	}
	out = captureStdout(t, func() { cmdPause([]string{"--db", db}, false) })
	if !strings.Contains(out, "resumed db=") {
		t.Fatalf("resume output=%q", out)
	}
	out = captureStdout(t, func() { cmdStatus([]string{"--db", db}) })
	if !strings.Contains(out, "paused: false") {
		t.Fatalf("status resumed output=%q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestCLIConflictsAndResolveFixture(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, ".umbrel-dropbox-sync", "state.db")
	captureStdout(t, func() { cmdInit([]string{"--root", root, "--db", db}) })
	s, err := state.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.AddConflict("/a.txt", "changed both", filepath.Join(root, "a.txt"), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { cmdConflicts([]string{"--db", db}) })
	if !strings.Contains(out, "id=") || !strings.Contains(out, "path=/a.txt") || !strings.Contains(out, "remote_rev=r1") {
		t.Fatalf("conflicts output=%q", out)
	}
	out = captureStdout(t, func() { cmdResolveConflict([]string{"--db", db, "--id", fmt.Sprint(id), "--note", "test"}) })
	if !strings.Contains(out, "resolved conflict id=") {
		t.Fatalf("resolve output=%q", out)
	}
	out = captureStdout(t, func() { cmdConflicts([]string{"--db", db}) })
	if !strings.Contains(out, "conflicts: none") {
		t.Fatalf("empty conflicts output=%q", out)
	}
}

func TestCLIMissingLocalFixture(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, ".umbrel-dropbox-sync", "state.db")
	captureStdout(t, func() { cmdInit([]string{"--root", root, "--db", db}) })
	s, err := state.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(state.Entry{Path: "/missing.txt", Rev: "r1", ContentHash: "h1", Size: 5, State: "local_missing"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { cmdMissingLocal([]string{"--db", db, "--enqueue-review"}) })
	if !strings.Contains(out, "path=/missing.txt") || !strings.Contains(out, "state=local_missing") || !strings.Contains(out, "review_ops_enqueued=1") {
		t.Fatalf("missing-local output=%q", out)
	}
	s2, err := state.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	op, err := s2.NextPendingOp()
	if err != nil {
		t.Fatal(err)
	}
	if op == nil || op.Op != reconcile.OpReviewRemoteDelete || op.Path != "/missing.txt" {
		t.Fatalf("op=%#v", op)
	}
}

func TestCLISmokeTestDryRunFixture(t *testing.T) {
	out := captureStdout(t, func() { cmdSmokeTest([]string{"--dry-run", "--remote-path", "/OpenClaw-Test"}) })
	if !strings.Contains(out, "smoke-test dry-run ok:") || !strings.Contains(out, "path=/OpenClaw-Test/smoke.txt") {
		t.Fatalf("smoke output=%q", out)
	}
	fields := strings.Fields(out)
	var db string
	for _, field := range fields {
		if strings.HasPrefix(field, "db=") {
			db = strings.TrimPrefix(field, "db=")
		}
	}
	if db == "" {
		t.Fatalf("db path missing from output=%q", out)
	}
	s, err := state.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.PendingOps != 0 || st.Entries != 0 {
		t.Fatalf("status=%#v", st)
	}
}
