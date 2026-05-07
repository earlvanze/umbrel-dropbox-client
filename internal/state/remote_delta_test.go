package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
)

type fakeRemoteDeltaClient struct {
	listCalls     int
	continueCalls []string
	pages         map[string]*dropbox.ListFolderResult
}

func (f *fakeRemoteDeltaClient) ListFolder(ctx context.Context, path string, recursive bool) (*dropbox.ListFolderResult, error) {
	f.listCalls++
	return f.pages[""], nil
}

func (f *fakeRemoteDeltaClient) ListFolderContinue(ctx context.Context, cursor string) (*dropbox.ListFolderResult, error) {
	f.continueCalls = append(f.continueCalls, cursor)
	return f.pages[cursor], nil
}

func TestIngestRemoteDeltaPersistsCursorAfterApplyingFiles(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	client := &fakeRemoteDeltaClient{pages: map[string]*dropbox.ListFolderResult{
		"":   {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/a.txt", Rev: "r1"}}, Cursor: "c1", HasMore: true},
		"c1": {Entries: []dropbox.Metadata{{Tag: "folder", PathLower: "/folder"}, {Tag: "file", PathLower: "/b.txt", Rev: "r2"}}, Cursor: "c2", HasMore: false},
	}}
	stats, err := s.IngestRemoteDelta(context.Background(), client, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.PreviousCursor != "" || stats.Cursor != "c2" || stats.Pages != 2 || stats.Entries != 3 || stats.AppliedFiles != 2 {
		t.Fatalf("stats=%#v", stats)
	}
	cursor, err := s.GetConfig(DropboxCursorKeyForPath(""))
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "c2" {
		t.Fatalf("cursor=%q", cursor)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 2 || st.LastEvent == "" {
		t.Fatalf("status=%#v", st)
	}
}

func TestIngestRemoteDeltaUsesStoredCursor(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConfig(DropboxCursorKeyForPath(""), "stored"); err != nil {
		t.Fatal(err)
	}
	client := &fakeRemoteDeltaClient{pages: map[string]*dropbox.ListFolderResult{
		"stored": {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/changed.txt", Rev: "r3"}}, Cursor: "next", HasMore: false},
	}}
	stats, err := s.IngestRemoteDelta(context.Background(), client, "")
	if err != nil {
		t.Fatal(err)
	}
	if client.listCalls != 0 || len(client.continueCalls) != 1 || client.continueCalls[0] != "stored" {
		t.Fatalf("client=%#v", client)
	}
	if stats.PreviousCursor != "stored" || stats.Cursor != "next" || stats.AppliedFiles != 1 {
		t.Fatalf("stats=%#v", stats)
	}
}

func TestIngestRemoteDeltaStripsRemoteBase(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	client := &fakeRemoteDeltaClient{pages: map[string]*dropbox.ListFolderResult{
		"": {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/obsidian/note.md", Rev: "r1"}}, Cursor: "c1", HasMore: false},
	}}
	stats, err := s.IngestRemoteDelta(context.Background(), client, "/Obsidian")
	if err != nil {
		t.Fatal(err)
	}
	if stats.AppliedFiles != 1 {
		t.Fatalf("stats=%#v", stats)
	}
	entry, err := s.EntryByPath("/note.md")
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Path != "/note.md" || entry.Rev != "r1" {
		t.Fatalf("entry=%#v", entry)
	}
	if full, err := s.EntryByPath("/obsidian/note.md"); err != nil || full != nil {
		t.Fatalf("full=%#v err=%v", full, err)
	}
}

func TestDropboxCursorKeyForPath(t *testing.T) {
	cases := map[string]string{
		"":          "dropbox_cursor:root",
		"/":         "dropbox_cursor:root",
		"Obsidian":  "dropbox_cursor:/obsidian",
		"/Obsidian": "dropbox_cursor:/obsidian",
	}
	for input, want := range cases {
		if got := DropboxCursorKeyForPath(input); got != want {
			t.Fatalf("key(%q)=%q want %q", input, got, want)
		}
	}
}

func TestIngestRemoteDeltaScopesCursorByRemotePath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConfig(DropboxCursorKeyForPath("/other"), "other-cursor"); err != nil {
		t.Fatal(err)
	}
	client := &fakeRemoteDeltaClient{pages: map[string]*dropbox.ListFolderResult{
		"": {Entries: []dropbox.Metadata{{Tag: "file", PathLower: "/obsidian/note.md", Rev: "r1"}}, Cursor: "obsidian-cursor", HasMore: false},
	}}
	stats, err := s.IngestRemoteDelta(context.Background(), client, "/Obsidian")
	if err != nil {
		t.Fatal(err)
	}
	if stats.PreviousCursor != "" || stats.Cursor != "obsidian-cursor" || client.listCalls != 1 || len(client.continueCalls) != 0 {
		t.Fatalf("stats=%#v client=%#v", stats, client)
	}
	got, err := s.GetConfig(DropboxCursorKeyForPath("/Obsidian"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "obsidian-cursor" {
		t.Fatalf("obsidian cursor=%q", got)
	}
	other, err := s.GetConfig(DropboxCursorKeyForPath("/other"))
	if err != nil {
		t.Fatal(err)
	}
	if other != "other-cursor" {
		t.Fatalf("other cursor=%q", other)
	}
}
