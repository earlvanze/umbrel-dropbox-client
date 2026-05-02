package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEnqueueOpIfMissingDedupesByOpAndPath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	id1, created, err := s.EnqueueOpIfMissing("upload_local", "/a.txt", map[string]string{"first": "yes"})
	if err != nil || !created || id1 == 0 {
		t.Fatalf("first id=%d created=%v err=%v", id1, created, err)
	}
	id2, created, err := s.EnqueueOpIfMissing("upload_local", "/a.txt", nil)
	if err != nil || created || id2 != id1 {
		t.Fatalf("second id=%d created=%v err=%v", id2, created, err)
	}
}

func TestNextReadyPendingOpSkipsFutureRetriesAndFailedOps(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	futureID, err := s.EnqueueOp("upload_local", "/future.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RetryOp(futureID, time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC), "rate limited"); err != nil {
		t.Fatal(err)
	}
	failedID, err := s.EnqueueOp("upload_local", "/failed.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FailOp(failedID, "permanent"); err != nil {
		t.Fatal(err)
	}
	readyID, err := s.EnqueueOp("download_remote", "/ready.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	op, err := s.NextReadyPendingOp(time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if op == nil || op.ID != readyID {
		t.Fatalf("ready op id=%v want %d", op, readyID)
	}

	if err := s.CompleteOp(readyID); err != nil {
		t.Fatal(err)
	}
	op, err = s.NextReadyPendingOp(time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if op != nil {
		t.Fatalf("unexpected op before retry_at: %#v", op)
	}
	op, err = s.NextReadyPendingOp(time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if op == nil || op.ID != futureID || op.Attempts != 1 || op.LastError != "rate limited" {
		t.Fatalf("future op=%#v want id=%d attempts=1 last_error", op, futureID)
	}
}

func TestAddConflictIfMissingDedupesByPathAndReason(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	id1, created, err := s.AddConflictIfMissing("/a.txt", "concurrent create", "/tmp/a.txt", "r1")
	if err != nil || !created || id1 == 0 {
		t.Fatalf("first id=%d created=%v err=%v", id1, created, err)
	}
	id2, created, err := s.AddConflictIfMissing("/a.txt", "concurrent create", "/tmp/a.txt", "r2")
	if err != nil || created || id2 != id1 {
		t.Fatalf("second id=%d created=%v err=%v", id2, created, err)
	}
}

func TestPausedStateRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	paused, err := s.IsPaused()
	if err != nil {
		t.Fatal(err)
	}
	if paused {
		t.Fatal("new store should not be paused")
	}
	if err := s.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Paused {
		t.Fatalf("status=%#v", st)
	}
	if err := s.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	paused, err = s.IsPaused()
	if err != nil {
		t.Fatal(err)
	}
	if paused {
		t.Fatal("store should be resumed")
	}
}
