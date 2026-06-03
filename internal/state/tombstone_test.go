package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMarkMissingLocalAndList(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []Entry{
		{Path: "/kept.txt", ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"},
		{Path: "/missing.txt", DropboxID: "id:m", Rev: "r1", ContentHash: "h2", Size: 2, MTime: time.Now(), State: "clean"},
		{Path: "/remote.txt", ContentHash: "h3", Size: 3, MTime: time.Now(), State: "remote_scanned"},
	} {
		if err := s.UpsertEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	count, err := s.MarkMissingLocal(map[string]bool{"/kept.txt": true})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
	items, err := s.ListMissingLocal(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "/missing.txt" || items[0].Rev != "r1" || items[0].State != "local_missing" {
		t.Fatalf("items=%#v", items)
	}
}

func TestMarkMissingLocalNormalizesSeenPaths(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/inbox/note.md", ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"}); err != nil {
		t.Fatal(err)
	}
	count, err := s.MarkMissingLocal(map[string]bool{"/Inbox/Note.md": true})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count=%d", count)
	}
	items, err := s.ListMissingLocal(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items=%#v", items)
	}
}

func TestMarkMissingLocalInDirs(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Insert entries in different directories
	for _, e := range []Entry{
		{Path: "/docs/readme.md", ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"},
		{Path: "/docs/notes.txt", ContentHash: "h2", Size: 2, MTime: time.Now(), State: "local_scanned"},
		{Path: "/src/main.go", ContentHash: "h3", Size: 3, MTime: time.Now(), State: "clean"},
		{Path: "/src/util.go", ContentHash: "h4", Size: 4, MTime: time.Now(), State: "clean"},
	} {
		if err := s.UpsertEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	// Scan /docs with both files present, missing /src entries
	seen := map[string]bool{"/docs/readme.md": true, "/docs/notes.txt": true}
	count, err := s.MarkMissingLocalInDirs(seen, []string{"/docs", "/src"})
	if err != nil {
		t.Fatal(err)
	}
	// /src/main.go and /src/util.go should be missing = 2
	if count != 2 {
		t.Fatalf("expected 2 missing in dirs, got %d", count)
	}
	items, err := s.ListMissingLocal(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 missing items, got %d", len(items))
	}
}

func TestMarkMissingLocalInDirsEmptyPrefixScansAll(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/kept.txt", ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/missing.txt", ContentHash: "h2", Size: 1, MTime: time.Now(), State: "clean"}); err != nil {
		t.Fatal(err)
	}
	// Empty string prefix should match all paths
	count, err := s.MarkMissingLocalInDirs(map[string]bool{"/kept.txt": true}, []string{""})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 missing, got %d", count)
	}
}

func TestMarkMissingLocalInDirsEmptyPrefixIsRootOnly(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/root.txt", ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/sub/inner.txt", ContentHash: "h2", Size: 1, MTime: time.Now(), State: "local_scanned"}); err != nil {
		t.Fatal(err)
	}
	// Empty prefix should NOT match /sub/inner.txt
	count, err := s.MarkMissingLocalInDirs(map[string]bool{"/root.txt": true}, []string{""})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 (root.txt seen, only root-level), got %d", count)
	}
	// Now mark inner.txt missing by not seeing it under a real prefix
	count, err = s.MarkMissingLocalInDirs(map[string]bool{"/root.txt": true}, []string{"/sub"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 (inner.txt not seen under /sub), got %d", count)
	}
}
