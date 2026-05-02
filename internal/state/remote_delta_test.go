package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
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
	cursor, err := s.GetConfig(DropboxCursorKey)
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
	if err := s.SetConfig(DropboxCursorKey, "stored"); err != nil {
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
