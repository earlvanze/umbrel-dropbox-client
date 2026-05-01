package scan

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/hash"
)

type File struct {
	Path        string
	AbsPath     string
	Size        int64
	ModTime     time.Time
	ContentHash string
}

type Options struct {
	IgnoreDirs map[string]bool
}

func DefaultOptions() Options {
	return Options{IgnoreDirs: map[string]bool{".git": true, ".umbrel-dropbox-sync": true}}
}

func Walk(root string, opts Options) ([]File, error) {
	if opts.IgnoreDirs == nil {
		opts = DefaultOptions()
	}
	var out []File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && opts.IgnoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		h, err := hash.DropboxContentHash(path)
		if err != nil {
			return err
		}
		out = append(out, File{Path: filepath.ToSlash(rel), AbsPath: path, Size: info.Size(), ModTime: info.ModTime(), ContentHash: h})
		return nil
	})
	return out, err
}

func DropboxPath(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return ""
	}
	return "/" + rel
}
