package watch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path string
	Op   fsnotify.Op
}

type Options struct {
	IgnoreDirs map[string]bool
}

func DefaultOptions() Options {
	return Options{IgnoreDirs: map[string]bool{".git": true, ".umbrel-dropbox-sync": true}}
}

type Watcher struct {
	root string
	opts Options
	w    *fsnotify.Watcher
}

func New(root string, opts Options) (*Watcher, error) {
	if opts.IgnoreDirs == nil {
		opts = DefaultOptions()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	out := &Watcher{root: abs, opts: opts, w: w}
	if err := out.addTree(abs); err != nil {
		_ = w.Close()
		return nil, err
	}
	return out, nil
}

func (w *Watcher) Close() error {
	if w == nil || w.w == nil {
		return nil
	}
	return w.w.Close()
}

func (w *Watcher) Events(ctx context.Context) <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-w.w.Errors:
				if !ok || err == nil {
					continue
				}
			case ev, ok := <-w.w.Events:
				if !ok {
					return
				}
				if w.ignored(ev.Name) {
					continue
				}
				if ev.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
						_ = w.addTree(ev.Name)
					}
				}
				select {
				case out <- Event{Path: ev.Name, Op: ev.Op}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func (w *Watcher) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && w.opts.IgnoreDirs[d.Name()] {
			return filepath.SkipDir
		}
		if err := w.w.Add(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
}

func (w *Watcher) ignored(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if w.opts.IgnoreDirs[part] {
			return true
		}
	}
	return false
}
