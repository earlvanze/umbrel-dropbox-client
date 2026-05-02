package state

import (
	"path/filepath"
	"testing"
)

func TestListAndResolveConflicts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	id1, err := s.AddConflict("/a.txt", "changed both", "/tmp/a.txt", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddConflict("/b.txt", "changed both", "/tmp/b.txt", "r2"); err != nil {
		t.Fatal(err)
	}
	items, err := s.ListConflicts(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != id1 || items[0].Path != "/a.txt" || items[0].RemoteRev != "r1" {
		t.Fatalf("items=%#v", items)
	}
	ok, err := s.ResolveConflict(id1, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected conflict resolved")
	}
	items, err = s.ListConflicts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "/b.txt" {
		t.Fatalf("items after resolve=%#v", items)
	}
	ok, err = s.ResolveConflict(99999, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("missing conflict should not resolve")
	}
}
