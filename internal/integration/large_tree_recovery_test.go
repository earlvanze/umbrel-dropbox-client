package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
	"github.com/earlvanze/umbrel-dropbox-client/internal/worker"
)

const (
	largeTreeSameFiles       = 40
	largeTreeLocalOnlyFiles  = 30
	largeTreeRemoteOnlyFiles = 20
	largeTreeConflictFiles   = 10
	largeTreeMissingFiles    = 5
)

func TestLargeTreeDryRunQueueSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "state.db")
	remote := buildLargeTreeFixture(t, root)

	s := openIntegrationStore(t, db)
	seedPreviouslySeenMissingFiles(t, s)
	files, plan := scanAndQueuePlan(t, s, root, remote)
	assertLargeTreePlan(t, files, plan)

	missing, err := s.MarkMissingLocal(pathSet(files))
	if err != nil {
		t.Fatal(err)
	}
	if missing != largeTreeMissingFiles {
		t.Fatalf("missing=%d want=%d", missing, largeTreeMissingFiles)
	}

	processed := processDryRunOps(t, s, 7)
	if processed != 7 {
		t.Fatalf("processed before restart=%d want=7", processed)
	}
	assertStatus(t, s, 85, 43, 10)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = openIntegrationStore(t, db)
	defer s.Close()
	assertStatus(t, s, 85, 43, 10)

	processed = processDryRunOps(t, s, 1000)
	if processed != 43 {
		t.Fatalf("processed after restart=%d want=43", processed)
	}
	assertStatus(t, s, 85, 0, 10)

	missingItems, err := s.ListMissingLocal(largeTreeMissingFiles + 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(missingItems) != largeTreeMissingFiles {
		t.Fatalf("missing items=%d items=%#v", len(missingItems), missingItems)
	}
}

func BenchmarkLargeTreeDryRunHarness(b *testing.B) {
	for i := 0; i < b.N; i++ {
		root := b.TempDir()
		db := filepath.Join(b.TempDir(), "state.db")
		remote := buildLargeTreeFixture(b, root)
		s := openIntegrationStore(b, db)
		seedPreviouslySeenMissingFiles(b, s)
		files, plan := scanAndQueuePlan(b, s, root, remote)
		if len(files) != 80 || len(plan.Ops) != 50 || len(plan.Conflicts) != 10 {
			b.Fatalf("files=%d ops=%d conflicts=%d", len(files), len(plan.Ops), len(plan.Conflicts))
		}
		if _, err := s.MarkMissingLocal(pathSet(files)); err != nil {
			b.Fatal(err)
		}
		if processed := processDryRunOps(b, s, 1000); processed != 50 {
			b.Fatalf("processed=%d want=50", processed)
		}
		if err := s.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func buildLargeTreeFixture(tb testing.TB, root string) []dropbox.Metadata {
	tb.Helper()
	var remote []dropbox.Metadata
	for i := 0; i < largeTreeSameFiles; i++ {
		path := fixturePath("same", i)
		body := fmt.Sprintf("same-%03d\n", i)
		writeFile(tb, filepath.Join(root, filepath.FromSlash(path)), body)
		remote = append(remote, remoteFile(tb, "/"+path, body, "same-rev", i))
	}
	for i := 0; i < largeTreeLocalOnlyFiles; i++ {
		path := fixturePath("local-only", i)
		writeFile(tb, filepath.Join(root, filepath.FromSlash(path)), fmt.Sprintf("local-only-%03d\n", i))
	}
	for i := 0; i < largeTreeRemoteOnlyFiles; i++ {
		path := fixturePath("remote-only", i)
		body := fmt.Sprintf("remote-only-%03d\n", i)
		remote = append(remote, remoteFile(tb, "/"+path, body, "remote-rev", i))
	}
	for i := 0; i < largeTreeConflictFiles; i++ {
		path := fixturePath("conflict", i)
		writeFile(tb, filepath.Join(root, filepath.FromSlash(path)), fmt.Sprintf("local-conflict-%03d\n", i))
		remote = append(remote, remoteFile(tb, "/"+path, fmt.Sprintf("remote-conflict-%03d\n", i), "conflict-rev", i))
	}
	remote = append(remote, dropbox.Metadata{Tag: "folder", PathLower: "/ignored-folder"})
	return remote
}

func fixturePath(kind string, n int) string {
	return fmt.Sprintf("%s/group-%02d/file-%03d.txt", kind, n%7, n)
}

func remoteFile(tb testing.TB, path, body, revPrefix string, n int) dropbox.Metadata {
	tb.Helper()
	return dropbox.Metadata{
		Tag:         "file",
		ID:          fmt.Sprintf("id:%s-%03d", revPrefix, n),
		PathLower:   path,
		Rev:         fmt.Sprintf("%s-%03d", revPrefix, n),
		ContentHash: hashString(tb, body),
		Size:        int64(len(body)),
		ServerMtime: fixedLargeTreeTime(n),
	}
}

func fixedLargeTreeTime(n int) time.Time {
	return time.Date(2026, 5, 5, 12, 0, n%60, 0, time.UTC)
}

func openIntegrationStore(tb testing.TB, db string) *state.Store {
	tb.Helper()
	s, err := state.Open(db)
	if err != nil {
		tb.Fatal(err)
	}
	if err := s.Init(); err != nil {
		_ = s.Close()
		tb.Fatal(err)
	}
	return s
}

func seedPreviouslySeenMissingFiles(tb testing.TB, s *state.Store) {
	tb.Helper()
	for i := 0; i < largeTreeMissingFiles; i++ {
		if err := s.UpsertEntry(state.Entry{
			Path:        fmt.Sprintf("/deleted/old-%03d.txt", i),
			DropboxID:   fmt.Sprintf("id:old-%03d", i),
			Rev:         fmt.Sprintf("old-rev-%03d", i),
			ContentHash: fmt.Sprintf("old-hash-%03d", i),
			Size:        int64(i + 1),
			MTime:       fixedLargeTreeTime(i),
			State:       "clean",
		}); err != nil {
			tb.Fatal(err)
		}
	}
}

func scanAndQueuePlan(tb testing.TB, s *state.Store, root string, remote []dropbox.Metadata) ([]scan.File, reconcile.Plan) {
	tb.Helper()
	files, err := scan.Walk(root, scan.DefaultOptions())
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range files {
		if err := s.UpsertEntry(state.Entry{Path: scan.DropboxPath(f.Path), ContentHash: f.ContentHash, Size: f.Size, MTime: f.ModTime, State: "local_scanned"}); err != nil {
			tb.Fatal(err)
		}
	}
	plan := reconcile.BuildDryRunPlan(files, remote)
	for _, op := range plan.Ops {
		if _, created, err := s.EnqueueOpIfMissing(op.Op, op.Path, op); err != nil || !created {
			tb.Fatalf("enqueue op=%#v created=%v err=%v", op, created, err)
		}
	}
	for _, c := range plan.Conflicts {
		if _, created, err := s.AddConflictIfMissing(c.Path, c.Reason, c.LocalPath, c.RemoteRev); err != nil || !created {
			tb.Fatalf("conflict=%#v created=%v err=%v", c, created, err)
		}
	}
	return files, plan
}

func pathSet(files []scan.File) map[string]bool {
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		seen[scan.DropboxPath(f.Path)] = true
	}
	return seen
}

func processDryRunOps(tb testing.TB, s *state.Store, limit int) int {
	tb.Helper()
	p := worker.Processor{Store: s, Handler: worker.DryRunHandler{Store: s}}
	processed := 0
	for processed < limit {
		res, err := p.ProcessOne(context.Background())
		if err != nil {
			tb.Fatal(err)
		}
		if !res.Processed {
			return processed
		}
		if !res.Completed || res.Failed {
			tb.Fatalf("unexpected worker result=%#v", res)
		}
		processed++
	}
	return processed
}

func assertLargeTreePlan(tb testing.TB, files []scan.File, plan reconcile.Plan) {
	tb.Helper()
	if len(files) != largeTreeSameFiles+largeTreeLocalOnlyFiles+largeTreeConflictFiles {
		tb.Fatalf("files=%d files=%#v", len(files), files)
	}
	if plan.Noop != largeTreeSameFiles || len(plan.Ops) != largeTreeLocalOnlyFiles+largeTreeRemoteOnlyFiles || len(plan.Conflicts) != largeTreeConflictFiles {
		tb.Fatalf("plan noop=%d ops=%d conflicts=%d plan=%#v", plan.Noop, len(plan.Ops), len(plan.Conflicts), plan)
	}
	if plan.Ops[0].Op != "upload_local" || plan.Ops[0].Path != "/local-only/group-00/file-000.txt" {
		tb.Fatalf("first op=%#v", plan.Ops[0])
	}
	if plan.Ops[len(plan.Ops)-1].Op != "download_remote" || plan.Ops[len(plan.Ops)-1].Path != "/remote-only/group-06/file-013.txt" {
		tb.Fatalf("last op=%#v", plan.Ops[len(plan.Ops)-1])
	}
}

func assertStatus(tb testing.TB, s *state.Store, entries, pendingOps, conflicts int) {
	tb.Helper()
	st, err := s.Status()
	if err != nil {
		tb.Fatal(err)
	}
	if st.Entries != entries || st.PendingOps != pendingOps || st.Conflicts != conflicts {
		tb.Fatalf("status=%#v want entries=%d pending=%d conflicts=%d", st, entries, pendingOps, conflicts)
	}
}
