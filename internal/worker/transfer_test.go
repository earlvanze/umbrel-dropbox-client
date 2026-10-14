package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
)

type fakeTransferClient struct {
	uploadPath  string
	uploadLocal string
	uploadRev   string
	downloadFn  func(path, local string) (*dropbox.Metadata, error)
}

func (f *fakeTransferClient) UploadFileAtRevision(ctx context.Context, dropboxPath, localPath, rev string) (*dropbox.Metadata, error) {
	f.uploadRev = rev
	return f.UploadFile(ctx, dropboxPath, localPath)
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

func TestTransferHandlerUsesRevisionConditionalUpload(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	_, err := s.EnqueueOp("upload_local", "/a.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/a.txt", LocalPath: local, Rev: "r0", ContentHash: mustHashPath(local)})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeTransferClient{}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil || !res.Completed || client.uploadRev != "r0" {
		t.Fatalf("result=%#v client=%#v err=%v", res, client, err)
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

func TestTransferHandlerUsesRemotePathForScopedUploadAndDownload(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "note.md")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	uploadHash := mustHashPath(local)
	s := testStore(t)
	client := &fakeTransferClient{}
	if _, err := s.EnqueueOp("upload_local", "/note.md", reconcile.PlannedOp{Op: "upload_local", Path: "/note.md", RemotePath: "/obsidian/note.md", LocalPath: local, ContentHash: uploadHash}); err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}, Now: fixedNow}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed || client.uploadPath != "/obsidian/note.md" || client.uploadLocal != local {
		t.Fatalf("result=%#v client=%#v", res, client)
	}

	downloadLocal := filepath.Join(root, "remote.md")
	client.downloadFn = func(path, local string) (*dropbox.Metadata, error) {
		if path != "/obsidian/remote.md" || local != downloadLocal {
			t.Fatalf("path=%s local=%s", path, local)
		}
		if err := os.WriteFile(local, []byte("payload"), 0600); err != nil {
			return nil, err
		}
		return &dropbox.Metadata{Tag: "file", ID: "id:3", Rev: "r3", PathLower: path, Size: 7, ServerMtime: fixedNow()}, nil
	}
	downloadHash := mustHashBytes(t, []byte("payload"))
	if _, err := s.EnqueueOp("download_remote", "/remote.md", reconcile.PlannedOp{Op: "download_remote", Path: "/remote.md", RemotePath: "/obsidian/remote.md", Rev: "r3", ContentHash: downloadHash}); err != nil {
		t.Fatal(err)
	}
	res, err = p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Completed {
		t.Fatalf("result=%#v", res)
	}
}

func TestTransferHandlerHoldsPreviouslyQueuedUploadAfterConflict(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "Report.txt")
	if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	id, err := s.EnqueueOp("upload_local", "/report.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/report.txt", LocalPath: local, ContentHash: mustHashPath(local)})
	if err != nil {
		t.Fatal(err)
	}
	cid, _, err := s.AddConflictIfMissing("/report.txt", "local paths differ only by case", local, "")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeTransferClient{}
	h := TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}
	p := Processor{Store: s, Handler: h, Now: fixedNow}
	for i := 0; i < 7; i++ {
		res, err := p.ProcessOne(context.Background())
		if err != nil || res.Processed || client.uploadPath != "" {
			t.Fatalf("result=%#v err=%v", res, err)
		}
	}
	op, err := s.PendingOpByID(id)
	if err != nil || op == nil || op.Attempts != 0 || op.Status != "pending" {
		t.Fatalf("op=%#v err=%v", op, err)
	}
	if err := h.HandleOp(context.Background(), *op); err == nil {
		t.Fatal("direct handler ignored conflict")
	}
	if ok, err := s.ResolveConflict(cid, "resolved"); err != nil || !ok {
		t.Fatalf("resolved=%v err=%v", ok, err)
	}
	res, err := p.ProcessOne(context.Background())
	if err != nil || !res.Completed || client.uploadPath == "" {
		t.Fatalf("result=%#v err=%v", res, err)
	}
}

func TestTransferHandlerDetectsCaseCollisionOutsideScannedDirectory(t *testing.T) {
	for _, directoryCollision := range []bool{true, false} {
		t.Run(fmt.Sprint(directoryCollision), func(t *testing.T) {
			root := t.TempDir()
			local := filepath.Join(root, "Folder", "Report.txt")
			if err := os.MkdirAll(filepath.Dir(local), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(local, []byte("hello"), 0600); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(root, "Folder", "report.txt")
			if directoryCollision {
				other = filepath.Join(root, "folder", "different.txt")
			}
			if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(other, []byte("other"), 0600); err != nil {
				t.Fatal(err)
			}
			s := testStore(t)
			id, err := s.EnqueueOp("upload_local", "/folder/report.txt", reconcile.PlannedOp{Op: "upload_local", Path: "/folder/report.txt", LocalPath: local, ContentHash: mustHashPath(local)})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeTransferClient{}
			p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: client, Root: root, AllowLive: true}, Now: fixedNow}
			res, err := p.ProcessOne(context.Background())
			if err != nil || !res.Processed || res.Completed || res.Err == nil || client.uploadPath != "" {
				t.Fatalf("result=%#v err=%v", res, err)
			}
			op, err := s.PendingOpByID(id)
			if err != nil || op == nil || op.Attempts != 0 || op.Status != "pending" {
				t.Fatalf("op=%#v err=%v", op, err)
			}
			cs, err := s.ListConflicts(10)
			if err != nil || len(cs) != 1 {
				t.Fatalf("conflicts=%#v err=%v", cs, err)
			}
			res, err = p.ProcessOne(context.Background())
			if err != nil || res.Processed {
				t.Fatalf("result=%#v err=%v", res, err)
			}
			// Fix the directory/name ambiguity, then acknowledge the conflict.
			if directoryCollision {
				if err := os.Rename(filepath.Dir(other), filepath.Join(root, "Other")); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Rename(other, filepath.Join(root, "Folder", "Other.txt")); err != nil {
				t.Fatal(err)
			}
			if ok, err := s.ResolveConflict(cs[0].ID, "renamed sibling"); err != nil || !ok {
				t.Fatalf("resolved=%v err=%v", ok, err)
			}
			res, err = p.ProcessOne(context.Background())
			if err != nil || !res.Completed || client.uploadPath == "" {
				t.Fatalf("result=%#v err=%v", res, err)
			}
		})
	}
}
