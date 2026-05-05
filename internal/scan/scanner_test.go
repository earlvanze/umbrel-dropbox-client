package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkIgnoresStateDirAndHashesFiles(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600))
	must(os.Mkdir(filepath.Join(root, ".umbrel-dropbox-client"), 0700))
	must(os.WriteFile(filepath.Join(root, ".umbrel-dropbox-client", "state.db"), []byte("ignore"), 0600))
	files, err := Walk(root, DefaultOptions())
	must(err)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %#v", len(files), files)
	}
	if files[0].Path != "a.txt" {
		t.Fatalf("unexpected path %q", files[0].Path)
	}
	if files[0].ContentHash == "" {
		t.Fatal("missing content hash")
	}
}

func TestDropboxPath(t *testing.T) {
	if got := DropboxPath("foo/bar.txt"); got != "/foo/bar.txt" {
		t.Fatalf("got %q", got)
	}
}

func TestWalkIgnoresAtomicDownloadTempFiles(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, ".download-123.tmp"), []byte("partial"), 0600))
	must(os.WriteFile(filepath.Join(root, ".download-123"), []byte("not partial"), 0600))
	must(os.WriteFile(filepath.Join(root, "work.tmp"), []byte("not atomic"), 0600))
	must(os.WriteFile(filepath.Join(root, "real.txt"), []byte("real"), 0600))
	files, err := Walk(root, DefaultOptions())
	must(err)
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
	}
	for _, want := range []string{".download-123", "real.txt", "work.tmp"} {
		if !seen[want] {
			t.Fatalf("missing %s in files=%#v", want, files)
		}
	}
	if seen[".download-123.tmp"] || len(files) != 3 {
		t.Fatalf("files=%#v", files)
	}
}
