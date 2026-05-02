package state

import (
	"path/filepath"
	"testing"
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
