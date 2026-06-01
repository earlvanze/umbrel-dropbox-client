package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertEntryNormalizesPaths(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/Inbox/Note.md", ContentHash: "local", Size: 1, MTime: time.Now(), State: "local_scanned"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEntry(Entry{Path: "/inbox/note.md", ContentHash: "remote", Size: 1, MTime: time.Now(), State: "remote_scanned"}); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 1 {
		t.Fatalf("status=%#v", st)
	}
	entry, err := s.EntryByPath("/INBOX/NOTE.md")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Path != "/inbox/note.md" || entry.ContentHash != "remote" || entry.State != "remote_scanned" {
		t.Fatalf("entry=%#v", entry)
	}
	if err := s.DeleteEntry("/Inbox/Note.md"); err != nil {
		t.Fatal(err)
	}
	st, err = s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 0 {
		t.Fatalf("status after delete=%#v", st)
	}
}

func TestUpsertEntryRejectsEmptyNormalizedPath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "/"} {
		if err := s.UpsertEntry(Entry{Path: path, ContentHash: "h1", Size: 1, MTime: time.Now(), State: "local_scanned"}); err == nil {
			t.Fatalf("expected error for path %q", path)
		}
	}
}


func TestUpsertEntryIfChangedSkipsUnchanged(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Insert initial entry
	changed, err := s.UpsertEntryIfChanged(Entry{Path: "/foo/bar.txt", ContentHash: "hash1", Size: 100, MTime: time.Unix(1700000000, 0), State: "local_scanned"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected first upsert to be a change")
	}
	// Same data: should skip
	changed, err = s.UpsertEntryIfChanged(Entry{Path: "/foo/bar.txt", ContentHash: "hash1", Size: 100, MTime: time.Unix(1700000000, 0), State: "local_scanned"})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected second upsert to be skipped (no change)")
	}
	// Different content hash: should update
	changed, err = s.UpsertEntryIfChanged(Entry{Path: "/foo/bar.txt", ContentHash: "hash2", Size: 100, MTime: time.Unix(1700000000, 0), State: "local_scanned"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected third upsert to be a change")
	}
	// Verify final state
	entry, err := s.EntryByPath("/foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ContentHash != "hash2" {
		t.Fatalf("expected hash2, got %s", entry.ContentHash)
	}
}

func TestUpsertBatchReturnsChangedCount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{Path: "/a.txt", ContentHash: "h1", Size: 1, MTime: time.Unix(1700000000, 0), State: "local_scanned"},
		{Path: "/b.txt", ContentHash: "h2", Size: 2, MTime: time.Unix(1700000000, 0), State: "local_scanned"},
	}
	changed, err := s.UpsertBatch(entries)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 2 {
		t.Fatalf("expected 2 changed, got %d", changed)
	}
	// Re-upsert same data: should be 0 changes
	changed, err = s.UpsertBatch(entries)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Fatalf("expected 0 changed on re-upsert, got %d", changed)
	}
}
