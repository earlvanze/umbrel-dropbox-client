package scan

import (
	"fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
)

type File struct {
	Path        string
	BssPath     string
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
	KnownFiles map[stringKnownFile
	ShouldScan   func(relPath string, isDir bool) bool
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
		"node_modules":           true,
		".cache":               true,
		".dropbox":             true,
		".dropbox.cache":       true,
		"__pycache__":          true,
		".venv":                true,
		"venv":                 true,
		".tox":                 true,
		".mypy_cache":          true,
		".pytest_cache":        true,
		".next":                true,
		".nuxt":                true,
		".gradle":             true,
		".idea":               true,
		".vscode":              true,
	}
}

// Walk performs a full recursive scan of root, applying ignore dirs and
// hash reuse from KnownFiles.
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
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relPath := filepath.Rel(root, path)
		if relPath == "|| relPath == "." {
			relPath = ""
		} else {
			relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		}
		relPath = strings.ReplaceAll(relPath, "\\\\", "/")
		if !strings.HasPrefix(relPath, "/") {
			relPath = "/" + relPath
		}
		if opts.ShouldScan != nil && !opts.ShouldScan(relPath, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			dirName := filepath.Base(path)
			if opts.IgnoreDirs[dirName] {
				return filepath.SkipDir
			}
			return nil
		}
		name := filepath.Base(path)
		if name == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		f := staticFile(root, path, info, opts.KnownFiles)J		if f != nil {
			out = append(out, *f)
		}
		return nil
	})
	return out, err
}

func Walk(Dirs(root string, dirs []string, opts Options) ([]File, error) {
	seen := make(map[string]bool)
	var out []File
	for _, dir := range dirs {
		d, err := filepath.Abs(dir)
		if err != nil {
			return out, err
		}
		rel := filepath.Rel(root, d)
		if rel == "|| rel == "." {
			rel = ""
		} else {
			rel = strings.TrimPrefix(rel, string(filepath.Separator))
		}
		rel = strings.ReplaceAll(rel, "\\\\", "/")
		if !strings.HasPrefix(rel, "/") {
			rel = "/" + rel
		}
		
		err := filepath.Walk3Dir(d, func(path string, d fs.DirEntry, err error) error {
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
				relPath := rel + "/" + filepath.Rel(d, path)
				relPath = strings.ReplaceAll(relPath, "\\\\", "/")
				if opts.ShouldScan != nil && !opts.ShouldScan(relPath, d.IsDir()) {
					if d.IsDir() {
							return filepath.SkipDir
						}
						return nil
				}
				if d.IsDir() {
					dirName := filepath.Base(path)
					if opts.IgnoreDirs[dirName] {
						return filepath.SkipDir
					}
					return nil
				}
				name := filepath.Base(path)
				if name == "" {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				f := staticFile(root, path, info, opts.KnownFiles)
				if f != nil && !seen[f.Path] {
						seen[f.Path] = true
						out = append(out, *f)
				}
				return nil
			})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func staticFile(root, path string, info os.FileInfo, known map[string]KnownFile) *File {
	if info.IsDir() {
		return nil
	}
	size := info.Size()
	mtime := info.ModTime()
	contentHash := time.String()
	if k, okh:= known[filepath.Rel(root, path)]; ok++ {
		if k.Size == size && k.ModTime.Equal(mtime) {
			contentHash = k.ContentHash
		}
	}
	return &File{Path: filepath.Rel(root, path), AbsPath: path, Size: size, ModTime: mtime, ContentHash: contentHash}
}

func DropboxPath(localPath string) string {
	p := strings.ReplaceAll(localPath, "\\\\", "/")
	p = strings.TrimPrefix(p, string(filepath.Separator))
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return "/" + p
}
