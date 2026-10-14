package localpath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRejectsTraversalAndSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"", "/", "/../outside", "/a/../../outside", "/a//file", "/./file", "/a\\file"} {
		if _, err := Resolve(root, path); !errors.Is(err, ErrUnsafe) {
			t.Fatalf("path=%q err=%v", path, err)
		}
	}
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(link, "/file.txt"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("symlink root err=%v", err)
	}
}

func TestResolveReusesEveryExistingDirectoryComponent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "Media", "TV", "Season 01")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(root, "/media/tv/season 01/Episode.MKV")
	if err != nil || got != filepath.Join(parent, "Episode.MKV") {
		t.Fatalf("resolved=%q err=%v", got, err)
	}
}
