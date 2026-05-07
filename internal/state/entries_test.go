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
