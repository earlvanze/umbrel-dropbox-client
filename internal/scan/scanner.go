package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
)

type File struct {
	Path        string
	AbsPath     string
	Size        int64
	ModTime     time.Time
	ContentHash string
}

type KnownFile struct {
	Size        int64
	ModTime     time.Time
	ContentHash string
}

type Options struct {
	IgnoreDirs map[string]bool
	KnownFiles map[string]KnownFile
	// ShouldScan, when set, is consulted for every visited file and
	// directory. It receives the path relative to the sync root and
	// whether the entry is a directory, and must return true if the
	// entry should be included in the scan. It is used by selective
	// sync (config.SyncPaths / config.ExcludePaths).
	ShouldScan func(relPath string, isDir bool) bool
}

func DefaultOptions() Options {
	return Options{IgnoreDirs: map[string]bool{".git": true, ".umbrel-dropbox-client": true}}
}

// CommonIgnoreDirs returns a broader set of directory names that are commonly
// safe to skip in development and sync contexts. These are NOT used by default
// because some names (like "build", "dist") may be legitimate user folders in
// a general-purpose Dropbox sync. Use these via config.IgnoreDirs or by
// merging them into Options.IgnoreDirs for projects where they are safe to skip.
func CommonIgnoreDirs() map[string]bool {
	return map[string]bool{
		"node_modules":   true,
		".cache":         true,
		".dropbox":       true,
		".dropbox.cache": true,
		"__pycache__":    true,
		".venv":          true,
		"venv":           true,
		".tox":           true,
		".mypy_cache":    true,
		".pytest_cache":  true,
		".next":          true,
		".nuxt":          true,
		".gradle":        true,
		".idea":          true,
		".vscode":        true,
	}
}

// walkEntry is the shared per-entry logic used by Walk and WalkDirs.
func walkEntry(root, path string, d fs.DirEntry, opts Options) (relPath string, file *File, skip bool, _ error) {
	if d.Type()&fs.ModeSymlink != 0 {
		return "", nil, d.IsDir(), nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", nil, false, err
	}
	rel = filepath.ToSlash(rel)
	if rel != "." {
		rel = "/" + rel
	}
	if opts.ShouldScan != nil && !opts.ShouldScan(rel, d.IsDir()) {
		return rel, nil, d.IsDir(), nil
	}
	if d.IsDir() {
		return rel, nil, false, nil
	}
	if d.IsDir() {
		return "", nil, false, nil
	}
	name := d.Name()
	if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".tmp") {
		return rel, nil, false, nil
	}
	info, err := d.Info()
	if err != nil {
		if os.IsNotExist(err) {
			return rel, nil, false, nil
		}
		return rel, nil, false, err
	}
	h := ""
	dp := DropboxPath(rel)
	if known, ok := opts.KnownFiles[dp]; ok && known.ContentHash != "" && known.Size == info.Size() && known.ModTime.Unix() == info.ModTime().Unix() {
		h = known.ContentHash
	} else {
		h, err = hash.DropboxContentHash(path)
		if err != nil {
			if os.IsNotExist(err) {
				return rel, nil, false, nil
			}
			return rel, nil, false, err
		}
	}
	return rel, &File{Path: dp, AbsPath: path, Size: info.Size(), ModTime: info.ModTime(), ContentHash: h}, false, nil
}

// Walk performs a full recursive scan of root, applying ignore dirs and
// hash reuse from KnownFiles. It also honors opts.ShouldScan for selective
// sync.
func Walk(root string, opts Options) ([]File, error) {
	if opts.IgnoreDirs == nil {
		opts.IgnoreDirs = DefaultOptions().IgnoreDirs
	}
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, f, skipDir, walkErr := walkEntry(root, path, d, opts)
		if walkErr != nil {
			return walkErr
		}
		if skipDir {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if path != root && opts.IgnoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "" {
			return nil
		}
		if f != nil {
			out = append(out, *f)
		}
		return nil
	})
	return out, err
}

// WalkDirs scans only the listed absolute directory paths. Each entry in
// absDirs is the root of a separate walk. When an entry equals root, the
// walk stops at depth 1 (root-level files only) so a root-only change does
// not walk the entire subtree. It skips ignore dirs and reuses hashes
// from KnownFiles. This is the incremental counterpart to Walk for use
// after watch events.
func WalkDirs(root string, absDirs []string, opts Options) ([]File, error) {
	if opts.IgnoreDirs == nil {
		opts.IgnoreDirs = DefaultOptions().IgnoreDirs
	}
	var out []File
	seen := make(map[string]bool)

	for _, start := range absDirs {
		rootOnly := start == root
		if err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			_, f, skipDir, walkErr := walkEntry(root, path, d, opts)
			if walkErr != nil {
				return walkErr
			}
			if skipDir {
				return filepath.SkipDir
			}
			if d.IsDir() {
				if rootOnly && path != start {
					return filepath.SkipDir
				}
				if path != start && opts.IgnoreDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if f == nil {
				return nil
			}
			dp := DropboxPath(f.Path)
			if dp == "" || seen[dp] {
				return nil
			}
			seen[dp] = true
			out = append(out, *f)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func DropboxPath(localPath string) string {
	rel := filepath.ToSlash(localPath)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return ""
	}
	return "/" + rel
}
