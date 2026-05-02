package dropbox

import (
	"context"
	"testing"
)

type fakeDeltaClient struct {
	listCalled     bool
	continueCursor []string
	pages          map[string]*ListFolderResult
}

func (f *fakeDeltaClient) ListFolder(ctx context.Context, path string, recursive bool) (*ListFolderResult, error) {
	f.listCalled = true
	return f.pages[""], nil
}

func (f *fakeDeltaClient) ListFolderContinue(ctx context.Context, cursor string) (*ListFolderResult, error) {
	f.continueCursor = append(f.continueCursor, cursor)
	return f.pages[cursor], nil
}

func TestFetchDeltaStartsFromListFolderWhenCursorMissing(t *testing.T) {
	client := &fakeDeltaClient{pages: map[string]*ListFolderResult{
		"":   {Entries: []Metadata{{Tag: "file", PathLower: "/a.txt", Rev: "r1"}}, Cursor: "c1", HasMore: true},
		"c1": {Entries: []Metadata{{Tag: "file", PathLower: "/b.txt", Rev: "r2"}}, Cursor: "c2", HasMore: false},
	}}
	got, err := FetchDelta(context.Background(), client, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !client.listCalled || len(client.continueCursor) != 1 || client.continueCursor[0] != "c1" {
		t.Fatalf("client=%#v", client)
	}
	if got.Cursor != "c2" || got.Pages != 2 || len(got.Entries) != 2 {
		t.Fatalf("result=%#v", got)
	}
}

func TestFetchDeltaStartsFromStoredCursor(t *testing.T) {
	client := &fakeDeltaClient{pages: map[string]*ListFolderResult{
		"stored": {Entries: []Metadata{{Tag: "file", PathLower: "/changed.txt", Rev: "r3"}}, Cursor: "next", HasMore: false},
	}}
	got, err := FetchDelta(context.Background(), client, "stored", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if client.listCalled || len(client.continueCursor) != 1 || client.continueCursor[0] != "stored" {
		t.Fatalf("client=%#v", client)
	}
	if got.Cursor != "next" || got.Pages != 1 || len(got.Entries) != 1 {
		t.Fatalf("result=%#v", got)
	}
}
