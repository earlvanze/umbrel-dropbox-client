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
}

func DefaultOptions() Options {
	return Options{IgnoreDirs: map[string]bool{".git": true, ".umbrel-dropbox-client": true}}
}

func DefaultIgnoreDirs() map[string]bool {
	return map[string]bool{
		".git":                    true,
		".umbrel-dropbox-client": true,
		"node_modules":            true,
		".cache":                  true,
		".dropbox":                true,
		".dropbox.cache":          true,
		"__pycache__":             true,
		".venv":                   true,
		"venv":                    true,
		".tox":                    true,
		".mypy_cache":             true,
		".pytest_cache":           true,
		".next":                   true,
		".nuxt":                   true,
		"dist":                    true,
		"build":                   true,
		".gradle":                 true,
		".idea":                   true,
		".vscode":                 true,
		".DS_Store":               true,
		"Thumbs.db":               true,
	}
}

// Walk performs a full recursive scan of root, applying ignore dirs and
// hash reuse from KnownFiles.
func Walk(root string, opts Options) ([]File, error) {
	if opts.IgnoreDirs == nil {
		opts.IgnoreDirs = DefaultIgnoreDirs()
	}
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && opts.IgnoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".tmp") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		h := ""
		if known, ok := opts.KnownFiles[DropboxPath(rel)]; ok && known.ContentHash != "" && known.Size == info.Size() && known.ModTime.Unix() == info.ModTime().Unix() {
			h = known.ContentHash
		} else {
			h, err = hash.DropboxContentHash(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
		}
		out = append(out, File{Path: rel, AbsPath: path, Size: info.Size(), ModTime: info.ModTime(), ContentHash: h})
		return nil
	})
	return out, err
}

// WalkDirs scans only the listed subdirectories (relative to root) plus the
// root itself for immediate files. It skips ignore dirs and reuses hashes from
// KnownFiles. This is the incremental counterpart to Walk for use after
// watch events.
func WalkDirs(root string, dirs []string, opts Options) ([]File, error) {
	if opts.IgnoreDirs == nil {
		opts.IgnoreDirs = DefaultIgnoreDirs()
	}
	var out []File
	seen := make(map[string]bool)

	walkOne := func(start string) error {
		absStart := start
		if !filepath.IsAbs(start) {
			absStart = filepath.Join(root, start)
		}
		return filepath.WalkDir(absStart, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if path != absStart && opts.IgnoreDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".tmp") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			dp := DropboxPath(rel)
			if dp == "" || seen[dp] {
				return nil
			}
			seen[dp] = true
			info, err := d.Info()
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			h := ""
			if known, ok := opts.KnownFiles[dp]; ok && known.ContentHash != "" && known.Size == info.Size() && known.ModTime.Unix() == info.ModTime().Unix() {
				h = known.ContentHash
			} else {
				h, err = hash.DropboxContentHash(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
			}
			out = append(out, File{Path: rel, AbsPath: path, Size: info.Size(), ModTime: info.ModTime(), ContentHash: h})
			return nil
		})
	}

	// Always scan immediate files at root level
	rootFiles, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, d := range rootFiles {
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			continue
		}
		name := d.Name()
		if strings.HasPrefix(name, ".download-") && strings.HasSuffix(name, ".tmp") {
			continue
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		rel := name
		dp := DropboxPath(rel)
		if dp == "" || seen[dp] {
			continue
		}
		seen[dp] = true
		absPath := filepath.Join(root, name)
		h := ""
		if known, ok := opts.KnownFiles[dp]; ok && known.ContentHash != "" && known.Size == info.Size() && known.ModTime.Unix() == info.ModTime().Unix() {
			h = known.ContentHash
		} else {
			h, err = hash.DropboxContentHash(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		}
		out = append(out, File{Path: rel, AbsPath: absPath, Size: info.Size(), ModTime: info.ModTime(), ContentHash: h})
	}

	for _, dir := range dirs {
		if err := walkOne(dir); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func DropboxPath(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return ""
	}
	return "/" + rel
}
