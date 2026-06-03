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
	if files[0].Path != "/a.txt" {
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
	if got := DropboxPath("a.txt"); got != "/a.txt" {
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
	for _, want := range []string{"/.download-123", "/real.txt", "/work.tmp"} {
		if !seen[want] {
			t.Fatalf("missing %s in files=%#v", want, files)
		}
	}
	if seen["/.download-123.tmp"] || len(files) != 3 {
		t.Fatalf("files=%#v", files)
	}
}

func TestWalkDirsScansOnlySpecifiedDirectories(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Mkdir(filepath.Join(root, "sub1"), 0700))
	must(os.Mkdir(filepath.Join(root, "sub2"), 0700))
	must(os.WriteFile(filepath.Join(root, "root.txt"), []byte("root"), 0600))
	must(os.WriteFile(filepath.Join(root, "sub1", "a.txt"), []byte("a"), 0600))
	must(os.WriteFile(filepath.Join(root, "sub2", "b.txt"), []byte("b"), 0600))

	// Walk only sub1 directory — WalkDirs does NOT scan root-level files
	files, err := WalkDirs(root, []string{filepath.Join(root, "sub1")}, DefaultOptions())
	must(err)
	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["/sub1/a.txt"] {
		t.Fatal("expected /sub1/a.txt from directory scan")
	}
	if paths["/root.txt"] {
		t.Fatal("WalkDirs should not scan root-level files; that is Walk's job")
	}
	if paths["/sub2/b.txt"] {
		t.Fatal("did not expect /sub2/b.txt since sub2 was not requested")
	}
}

func TestWalkDirsRootOnlyScansRootLevelFiles(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "root.txt"), []byte("root"), 0600))
	must(os.Mkdir(filepath.Join(root, "sub"), 0700))
	must(os.WriteFile(filepath.Join(root, "sub", "deep.txt"), []byte("deep"), 0600))
	files, err := WalkDirs(root, []string{root}, DefaultOptions())
	must(err)
	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["/root.txt"] {
		t.Fatal("expected /root.txt at root level")
	}
	if paths["/sub/deep.txt"] {
		t.Fatal("WalkDirs with root should not recurse into subdirs during incremental scans")
	}
}

func TestWalkDirsIgnoresStateDir(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Mkdir(filepath.Join(root, "docs"), 0700))
	must(os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("hello"), 0600))
	must(os.Mkdir(filepath.Join(root, ".umbrel-dropbox-client"), 0700))
	must(os.WriteFile(filepath.Join(root, ".umbrel-dropbox-client", "state.db"), []byte("ignore"), 0600))
	files, err := WalkDirs(root, []string{filepath.Join(root, "docs")}, DefaultOptions())
	must(err)
	if len(files) != 1 || files[0].Path != "/docs/a.txt" {
		t.Fatalf("expected 1 file (/docs/a.txt), got %d: %#v", len(files), files)
	}
}

func TestWalkHonorsShouldScan(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "kept.txt"), []byte("k"), 0600))
	must(os.Mkdir(filepath.Join(root, "excluded"), 0700))
	must(os.WriteFile(filepath.Join(root, "excluded", "skip.txt"), []byte("x"), 0600))
	opts := DefaultOptions()
	opts.ShouldScan = func(relPath string, isDir bool) bool {
		return relPath != "/excluded" && relPath != "/excluded/skip.txt"
	}
	files, err := Walk(root, opts)
	must(err)
	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["/kept.txt"] {
		t.Fatal("expected /kept.txt")
	}
	if paths["/excluded/skip.txt"] {
		t.Fatal("ShouldScan=false should skip excluded subtree")
	}
}
