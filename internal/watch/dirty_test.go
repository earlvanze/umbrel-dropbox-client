
package watch

import (
	"path/filepath"
	"testing"
)

func TestDirtySetAddAndDirs(t *testing.T) {
	root := t.TempDir()
	d := NewDirtySet(root)
	d.Add(filepath.Join(root, "sub", "file.txt"))
	d.Add(filepath.Join(root, "other", "deep", "file.txt"))
	dirs := d.Dirs()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
	dirMap := map[string]bool{}
	for _, dir := range dirs {
		dirMap[dir] = true
	}
	if !dirMap["sub"] {
		t.Fatal("expected 'sub' in dirs")
	}
	if !dirMap["other/deep"] {
		t.Fatal("expected 'other/deep' in dirs")
	}
	// After Dirs(), set should be cleared
	dirs2 := d.Dirs()
	if dirs2 != nil {
		t.Fatalf("expected nil dirs after clear, got %v", dirs2)
	}
}

func TestDirtySetRootLevelFiles(t *testing.T) {
	root := t.TempDir()
	d := NewDirtySet(root)
	d.Add(filepath.Join(root, "rootfile.txt"))
	dirs := d.Dirs()
	if len(dirs) != 1 || dirs[0] != "" {
		t.Fatalf("expected empty string dir for root-level file, got %v", dirs)
	}
}

func TestSplitParentDirs(t *testing.T) {
	dirs := SplitParentDirs([]string{"sub/a.txt", "sub/deep/b.txt", "other/c.txt", "node_modules/x/y.txt"})
	dirMap := map[string]bool{}
	for _, d := range dirs {
		dirMap[d] = true
	}
	if !dirMap["sub"] {
		t.Fatal("expected 'sub'")
	}
	if !dirMap["sub/deep"] {
		t.Fatal("expected 'sub/deep'")
	}
	if !dirMap["other"] {
		t.Fatal("expected 'other'")
	}
	if dirMap["node_modules"] {
		t.Fatal("node_modules should be filtered")
	}
}

func TestDirtySetLen(t *testing.T) {
	root := t.TempDir()
	d := NewDirtySet(root)
	if d.Len() != 0 {
		t.Fatal("expected 0")
	}
	d.Add(filepath.Join(root, "a.txt"))
	if d.Len() != 1 {
		t.Fatal("expected 1")
	}
	d.Add(filepath.Join(root, "a.txt")) // same file
	if d.Len() != 1 {
		t.Fatal("expected 1 after duplicate")
	}
	d.Add(filepath.Join(root, "b.txt"))
	if d.Len() != 2 {
		t.Fatal("expected 2")
	}
}

func TestDirtySetReset(t *testing.T) {
	root := t.TempDir()
	d := NewDirtySet(root)
	d.Add(filepath.Join(root, "a.txt"))
	d.Reset()
	if d.Len() != 0 {
		t.Fatal("expected 0 after reset")
	}
	dirs := d.Dirs()
	if dirs != nil {
		t.Fatalf("expected nil after reset, got %v", dirs)
	}
}
