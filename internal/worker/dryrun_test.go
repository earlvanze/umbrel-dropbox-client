package worker

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
)

func TestDryRunHandlerCompletesValidUploadAndRecordsEvent(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: filepath.Join(t.TempDir(), "a.txt"), Reason: "local only"})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: DryRunHandler{Store: s}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || res.OpID != id {
		t.Fatalf("result=%#v", res)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.PendingOps != 0 || st.LastEvent == "" {
		t.Fatalf("status=%#v", st)
	}
}

func TestDryRunHandlerRetriesInvalidDownload(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("download_remote", "/bad.txt", reconcile.PlannedOp{Op: "download_remote", Path: "/bad.txt"})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: DryRunHandler{Store: s}, MaxAttempts: 2, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || res.Completed || res.Failed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts != 1 || got.LastError == "" {
		t.Fatalf("op=%#v", got)
	}
}

func TestDryRunHandlerRejectsUnsupportedOp(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("delete_local", "/danger.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: DryRunHandler{Store: s}, MaxAttempts: 1, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatalf("result=%#v", res)
	}
	got, err := s.PendingOpByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" {
		t.Fatalf("op=%#v", got)
	}
}
