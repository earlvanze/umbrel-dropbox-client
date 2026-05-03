package worker

import (
	"context"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

type fakeDeleteClient struct {
	deletedPath string
}

func (f *fakeDeleteClient) DeleteFile(_ context.Context, dropboxPath string) (*dropbox.Metadata, error) {
	f.deletedPath = dropboxPath
	return &dropbox.Metadata{Tag: "file", PathLower: dropboxPath, Rev: "deleted"}, nil
}

func TestReviewedDeleteHandlerRequiresBothExplicitGates(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		allowLive            bool
		allowReviewedDeletes bool
	}{
		{name: "live disabled", allowLive: false, allowReviewedDeletes: true},
		{name: "reviewed deletes disabled", allowLive: true, allowReviewedDeletes: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			id := enqueueReviewedDelete(t, s, reconcile.OpReviewRemoteDelete)
			p := Processor{Store: s, Handler: ReviewedDeleteHandler{Store: s, AllowLive: tc.allowLive, AllowReviewedDeletes: tc.allowReviewedDeletes}, MaxAttempts: 1, Now: fixedNow}
			res, err := p.ProcessOne(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !res.Failed || res.Err == nil {
				t.Fatalf("result=%#v", res)
			}
			got, err := s.PendingOpByID(id)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.Status != "failed" {
				t.Fatalf("op=%#v", got)
			}
		})
	}
}

func TestReviewedDeleteHandlerRejectsOrdinaryDeleteOps(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("delete_remote", "/danger.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: ReviewedDeleteHandler{Store: s, Client: &fakeDeleteClient{}, AllowLive: true, AllowReviewedDeletes: true}, MaxAttempts: 1, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != "failed" {
		t.Fatalf("op=%#v", got)
	}
}

func TestReviewedDeleteHandlerPrunesRemoteMissingTombstoneOnlyWhenStateStillMatches(t *testing.T) {
	s := testStore(t)
	id := enqueueReviewedDelete(t, s, reconcile.OpReviewRemoteDelete)
	p := Processor{Store: s, Handler: ReviewedDeleteHandler{Store: s, AllowLive: true, AllowReviewedDeletes: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || res.OpID != id {
		t.Fatalf("result=%#v", res)
	}
	entry, err := s.EntryByPath("/missing.txt")
	if err != nil {
		t.Fatal(err)
	}
	if entry != nil {
		t.Fatalf("entry should be pruned: %#v", entry)
	}
}

func TestReviewedDeleteHandlerDeletesRemoteOnlyForReviewedLocalDelete(t *testing.T) {
	s := testStore(t)
	id := enqueueReviewedDelete(t, s, reconcile.OpReviewLocalDelete)
	client := &fakeDeleteClient{}
	p := Processor{Store: s, Handler: ReviewedDeleteHandler{Store: s, Client: client, AllowLive: true, AllowReviewedDeletes: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || res.OpID != id || client.deletedPath != "/missing.txt" {
		t.Fatalf("result=%#v deleted=%q", res, client.deletedPath)
	}
}

func TestReviewedDeleteHandlerRejectsStaleReviewState(t *testing.T) {
	s := testStore(t)
	if err := s.UpsertEntry(state.Entry{Path: "/missing.txt", DropboxID: "id:old", Rev: "r2", ContentHash: "h1", Size: 5, State: "local_missing"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.EnqueueOp(reconcile.OpReviewRemoteDelete, "/missing.txt", reconcile.PlannedOp{Op: reconcile.OpReviewRemoteDelete, Path: "/missing.txt", DropboxID: "id:old", Rev: "r1", ContentHash: "h1"})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: ReviewedDeleteHandler{Store: s, AllowLive: true, AllowReviewedDeletes: true}, MaxAttempts: 1, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
}

func enqueueReviewedDelete(t *testing.T, s *state.Store, op string) int64 {
	t.Helper()
	if err := s.UpsertEntry(state.Entry{Path: "/missing.txt", DropboxID: "id:old", Rev: "r1", ContentHash: "h1", Size: 5, State: "local_missing"}); err != nil {
		t.Fatal(err)
	}
	id, err := s.EnqueueOp(op, "/missing.txt", reconcile.PlannedOp{Op: op, Path: "/missing.txt", DropboxID: "id:old", Rev: "r1", ContentHash: "h1", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
