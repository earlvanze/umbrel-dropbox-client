package watch

import (
	"path/filepath"
	"strings"
	"sync"
)

// DirtySet collects filesystem paths from watch events and reduces them to a
// deduplicated set of parent directories suitable for incremental scanning.
// It is safe for concurrent use.
type DirtySet struct {
	mu    sync.Mutex
	paths map[string]bool
	dirs  map[string]bool
	root  string
}

// NewDirtySet creates a DirtySet rooted at root.
func NewDirtySet(root string) *DirtySet {
	return &DirtySet{
		paths: make(map[string]bool),
		dirs:  make(map[string]bool),
		root:  root,
	}
}

// Add records a filesystem path as dirty. The path should be an absolute path.
// It computes the parent directory relative to root and adds it to the
// directory set.
func (d *DirtySet) Add(absPath string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rel, err := filepath.Rel(d.root, absPath)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return
	}
	d.paths[rel] = true
	dir := filepath.Dir(rel)
	if dir == "." || dir == "" {
		d.dirs[""] = true
	} else {
		d.dirs[dir] = true
	}
}

// Dirs returns the deduplicated set of dirty directory paths (relative to
// root) and clears the dirty set. An empty string means the root directory.
func (d *DirtySet) Dirs() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.dirs) == 0 && len(d.paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(d.dirs))
	for dir := range d.dirs {
		out = append(out, dir)
	}
	d.paths = make(map[string]bool)
	d.dirs = make(map[string]bool)
	return out
}

// Len returns the number of unique dirty paths recorded since last Dirs call.
func (d *DirtySet) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.paths)
}

// Reset clears the dirty set.
func (d *DirtySet) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.paths = make(map[string]bool)
	d.dirs = make(map[string]bool)
}

// DefaultIgnoreDirNames returns the set of directory names that should be
// excluded from dirty path collection.
func DefaultIgnoreDirNames() map[string]bool {
	return map[string]bool{
		".git":                   true,
		".umbrel-dropbox-client": true,
		"node_modules":           true,
		".cache":                 true,
		".dropbox":               true,
		".dropbox.cache":         true,
	}
}

// IsIgnoredDirName checks if a directory basename should be skipped.
func IsIgnoredDirName(name string) bool {
	return DefaultIgnoreDirNames()[name]
}

// SplitParentDirs returns the unique parent directory paths of the given
// relative paths, suitable for WalkDirs. Results are relative to root
// using forward slashes, deduplicated, and filtered against ignore patterns.
func SplitParentDirs(relPaths []string) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, p := range relPaths {
		p = strings.TrimPrefix(filepath.ToSlash(p), "/")
		dir := filepath.Dir(p)
		if dir == "." || dir == "" {
			// No path separator means the input is a directory itself,
			// not a file. Use the path as-is (e.g., a top-level subdir).
			dir = p
		} else {
			dir = filepath.ToSlash(dir)
		}
		skip := false
		for _, part := range strings.Split(dir, "/") {
			if IsIgnoredDirName(part) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
