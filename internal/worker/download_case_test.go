package worker

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

type displayDownloadClient struct {
	fakeTransferClient
	metadata   *dropbox.Metadata
	downloaded int
	during     func()
}

func (c *displayDownloadClient) GetMetadata(context.Context, string) (*dropbox.Metadata, error) {
	return c.metadata, nil
}
func (c *displayDownloadClient) Download(_ context.Context, path string, out io.Writer) (*dropbox.Metadata, error) {
	c.downloaded++
	if c.during != nil {
		c.during()
	}
	if _, err := io.WriteString(out, "cloud bytes"); err != nil {
		return nil, err
	}
	return &dropbox.Metadata{Tag: "file", Rev: "r1", ID: "id:1", PathLower: strings.ToLower(path), Size: 11}, nil
}
func runCaseDownload(t *testing.T, root string, plan reconcile.PlannedOp, c *displayDownloadClient) (Result, *state.Store) {
	t.Helper()
	s := testStore(t)
	if _, err := s.EnqueueOp("download_remote", plan.Path, plan); err != nil {
		t.Fatal(err)
	}
	p := Processor{Store: s, Handler: TransferHandler{Store: s, Client: c, Root: root, AllowLive: true}}
	res, err := p.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res, s
}

func TestDownloadPreservesDisplayCaseAndExistingParent(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "display payload"
		if legacy {
			name = "legacy metadata"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "MOVIES"), 0700); err != nil {
				t.Fatal(err)
			}
			display := "/Movies/Example.Feature.2026/Movie.MKV"
			remote := "/Library" + display
			plan := reconcile.PlannedOp{Op: "download_remote", Path: strings.ToLower(display), RemotePath: strings.ToLower(remote), DisplayPath: display, Rev: "r1"}
			if legacy {
				plan.DisplayPath = ""
			}
			c := &displayDownloadClient{metadata: &dropbox.Metadata{Tag: "file", Rev: "r1", PathLower: strings.ToLower(remote), PathDisplay: remote}}
			res, _ := runCaseDownload(t, root, plan, c)
			if !res.Completed {
				t.Fatalf("result=%#v", res)
			}
			want := filepath.Join(root, "MOVIES", "Example.Feature.2026", "Movie.MKV")
			data, err := os.ReadFile(want)
			if err != nil || string(data) != "cloud bytes" {
				t.Fatalf("data=%q err=%v", data, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != "MOVIES" {
				t.Fatalf("root entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestLegacyDownloadMissingDisplayMetadataReusesExistingCase(t *testing.T) {
	for _, meta := range []*dropbox.Metadata{nil, {Tag: "file", Rev: "r1", PathLower: "/folder/file.txt"}} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "Folder"), 0700); err != nil {
			t.Fatal(err)
		}
		c := &displayDownloadClient{metadata: meta}
		res, _ := runCaseDownload(t, root, reconcile.PlannedOp{Op: "download_remote", Path: "/folder/file.txt", Rev: "r1"}, c)
		if !res.Completed {
			t.Fatalf("result=%#v", res)
		}
		if _, err := os.Stat(filepath.Join(root, "Folder", "file.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, "folder")); !os.IsNotExist(err) {
			t.Fatalf("created duplicate parent: %v", err)
		}
	}
}

func TestDownloadHoldsExistingVariantsAmbiguityAndSymlinks(t *testing.T) {
	for _, kind := range []string{"existing leaf", "ambiguous parents", "ambiguous leaves", "symlink parent", "symlink leaf"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "Folder"), 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "existing leaf":
				if err := os.WriteFile(filepath.Join(root, "Folder", "FILE.TXT"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "ambiguous parents":
				if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
					t.Fatal(err)
				}
			case "ambiguous leaves":
				for _, name := range []string{"File.txt", "file.txt"} {
					if err := os.WriteFile(filepath.Join(root, "Folder", name), []byte("keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "symlink parent":
				if err := os.Remove(filepath.Join(root, "Folder")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, "Folder")); err != nil {
					t.Fatal(err)
				}
			case "symlink leaf":
				if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(root, "Folder", "FILE.TXT")); err != nil {
					t.Fatal(err)
				}
			}
			c := &displayDownloadClient{}
			plan := reconcile.PlannedOp{Op: "download_remote", Path: "/folder/file.txt", DisplayPath: "/Folder/File.txt", Rev: "r1"}
			res, s := runCaseDownload(t, root, plan, c)
			if res.Completed || res.Err == nil || c.downloaded != 0 {
				t.Fatalf("result=%#v downloads=%d", res, c.downloaded)
			}
			conflicts, err := s.ListConflicts(10)
			if err != nil || len(conflicts) != 1 {
				t.Fatalf("conflicts=%#v err=%v", conflicts, err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("external writes: %v %v", entries, err)
			}
			if kind == "existing leaf" {
				b, err := os.ReadFile(filepath.Join(root, "Folder", "FILE.TXT"))
				if err != nil || string(b) != "keep" {
					t.Fatalf("existing data=%q err=%v", b, err)
				}
			}
		})
	}
}

func TestDownloadRechecksDestinationAfterTransfer(t *testing.T) {
	root := t.TempDir()
	c := &displayDownloadClient{during: func() {
		if err := os.WriteFile(filepath.Join(root, "FILE.TXT"), []byte("local winner"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	res, _ := runCaseDownload(t, root, reconcile.PlannedOp{Op: "download_remote", Path: "/file.txt", DisplayPath: "/File.txt", Rev: "r1"}, c)
	if res.Completed || res.Err == nil || c.downloaded != 1 {
		t.Fatalf("result=%#v downloads=%d", res, c.downloaded)
	}
	b, err := os.ReadFile(filepath.Join(root, "FILE.TXT"))
	if err != nil || string(b) != "local winner" {
		t.Fatalf("data=%q err=%v", b, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected staged/published file: %v %v", entries, err)
	}
}

func TestLegacyDownloadRejectsStaleOrMismatchedMetadata(t *testing.T) {
	for _, meta := range []*dropbox.Metadata{
		{Tag: "file", Rev: "r2", PathLower: "/file.txt", PathDisplay: "/File.txt"},
		{Tag: "file", Rev: "r1", PathLower: "/different.txt", PathDisplay: "/Different.txt"},
		{Tag: "file", Rev: "r1", PathLower: "/file.txt", PathDisplay: "/Other.txt"},
	} {
		c := &displayDownloadClient{metadata: meta}
		res, _ := runCaseDownload(t, t.TempDir(), reconcile.PlannedOp{Op: "download_remote", Path: "/file.txt", Rev: "r1"}, c)
		if res.Completed || res.Err == nil || c.downloaded != 0 {
			t.Fatalf("result=%#v downloads=%d", res, c.downloaded)
		}
	}
}

func TestDownloadDoesNotPublishHashMismatchOrChangedParent(t *testing.T) {
	for _, kind := range []string{"hash mismatch", "symlink appeared"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			c := &displayDownloadClient{}
			plan := reconcile.PlannedOp{Op: "download_remote", Path: "/folder/file.txt", DisplayPath: "/Folder/File.txt", Rev: "r1"}
			if kind == "hash mismatch" {
				plan.ContentHash = "wrong hash"
			} else {
				c.during = func() {
					if err := os.Symlink(outside, filepath.Join(root, "Folder")); err != nil {
						t.Fatal(err)
					}
				}
			}
			res, _ := runCaseDownload(t, root, plan, c)
			if res.Completed || res.Err == nil {
				t.Fatalf("result=%#v", res)
			}
			if _, err := os.Stat(filepath.Join(root, "Folder", "File.txt")); !os.IsNotExist(err) {
				t.Fatalf("published invalid download: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("external entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestDownloadReusesCaseOnlyParentCreatedDuringTransfer(t *testing.T) {
	root := t.TempDir()
	c := &displayDownloadClient{during: func() {
		if err := os.Mkdir(filepath.Join(root, "FOLDER"), 0700); err != nil {
			t.Fatal(err)
		}
	}}
	res, _ := runCaseDownload(t, root, reconcile.PlannedOp{Op: "download_remote", Path: "/folder/new.txt", DisplayPath: "/Folder/New.txt", Rev: "r1"}, c)
	if !res.Completed {
		t.Fatalf("result=%#v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "FOLDER", "New.txt")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "FOLDER" {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
}

func TestDownloadRejectsParentReplacedBySymlinkDuringTransfer(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Folder"), 0700); err != nil {
		t.Fatal(err)
	}
	c := &displayDownloadClient{during: func() {
		if err := os.Rename(filepath.Join(root, "Folder"), filepath.Join(root, "Original")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "Folder")); err != nil {
			t.Fatal(err)
		}
	}}
	res, _ := runCaseDownload(t, root, reconcile.PlannedOp{Op: "download_remote", Path: "/folder/new.txt", DisplayPath: "/Folder/New.txt", Rev: "r1"}, c)
	if res.Completed || res.Err == nil {
		t.Fatalf("result=%#v", res)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external entries=%v err=%v", entries, err)
	}
}
