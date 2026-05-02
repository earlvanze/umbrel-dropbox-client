package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/hash"
	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
)

type fakeTransferClient struct {
	uploadPath  string
	uploadLocal string
	downloadFn  func(path, local string) (*dropbox.Metadata, error)
}

func (f *fakeTransferClient) UploadFile(_ context.Context, dropboxPath, localPath string) (*dropbox.Metadata, error) {
	f.uploadPath = dropboxPath
	f.uploadLocal = localPath
	return &dropbox.Metadata{Tag: "file", ID: "id:1", Rev: "r1", PathLower: dropboxPath, ContentHash: mustHashPath(localPath), Size: 5, ServerMtime: fixedNow()}, nil
}

func (f *fakeTransferClient) DownloadFile(_ context.Context, dropboxPath, localPath string) (*dropbox.Metadata, error) {
	if f.downloadFn != nil {
		return f.downloadFn(dropboxPath, localPath)
	}
	return nil, os.ErrInvalid
}

func TestTransferHandlerRequiresExplicitLiveGate(t *testing.T) {
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: filepath.Join(t.TempDir(), "a.txt")})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: &fakeTransferClient{}, Root: t.TempDir()}, MaxAttempts: 1, Now: fixedNow}
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
	if got.Status != "failed" || got.LastError == "" {
		t.Fatalf("op=%#v", got)
	}
}

func TestTransferHandlerUploadsOnlyInsideRootAndUpdatesEntry(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	h := mustHashPath(local)
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: local, ContentHash: h})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeTransferClient{}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || res.OpID != id || client.uploadPath != "/a.txt" || client.uploadLocal != local {
		t.Fatalf("result=%#v client=%#v", res, client)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 1 || st.PendingOps != 0 {
		t.Fatalf("status=%#v", st)
	}
}

func TestTransferHandlerRejectsUploadHashChange(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	_, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: local, ContentHash: "old"})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: &fakeTransferClient{}, Root: root, AllowLive: true}, MaxAttempts: 1, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
}

func TestTransferHandlerDownloadsWithoutOverwriteAndUpdatesEntry(t *testing.T) {
	root := t.TempDir()
	wantLocal := filepath.Join(root, "nested", "a.txt")
	client := &fakeTransferClient{downloadFn: func(path, local string) (*dropbox.Metadata, error) {
		if path != "/nested/a.txt" || local != wantLocal {
			t.Fatalf("path=%s local=%s", path, local)
		}
		if err := os.MkdirAll(filepath.Dir(local), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(local, []byte("payload"), 0600); err != nil {
			return nil, err
		}
		return &dropbox.Metadata{Tag: "file", ID: "id:2", Rev: "r2", PathLower: path, Size: 7, ServerMtime: fixedNow()}, nil
	}}
	s := testStore(t)
	downloadHash := mustHashBytes(t, []byte("payload"))
	_, err := s.EnqueueOp("download_remote", "/nested/a.txt", reconcile.PlannedOp{Op: "download_remote", Path: "/nested/a.txt", Rev: "r2", ContentHash: downloadHash})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed {
		t.Fatalf("result=%#v", res)
	}
	body, err := os.ReadFile(wantLocal)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Fatalf("body=%q", body)
	}
}

func TestTransferHandlerRefusesDownloadOverwrite(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	_, err := s.EnqueueOp("download_remote", "/a.txt", reconcile.PlannedOp{Op: "download_remote", Path: "/a.txt", Rev: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: &fakeTransferClient{}, Root: root, AllowLive: true}, MaxAttempts: 1, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
}

func mustHashPath(path string) string {
	h, err := hash.DropboxContentHash(path)
	if err != nil {
		panic(err)
	}
	return h
}

func mustHashBytes(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return mustHashPath(path)
}
