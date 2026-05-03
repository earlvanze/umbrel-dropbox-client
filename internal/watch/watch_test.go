package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatcherEmitsCreateAndIgnoresStateDir(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events := w.Events(ctx)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".umbrel-dropbox-client"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".umbrel-dropbox-client", "state.db"), []byte("ignore"), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case ev := <-events:
			if strings.HasSuffix(ev.Path, "a.txt") {
				return
			}
			if strings.Contains(ev.Path, ".umbrel-dropbox-client") {
				t.Fatalf("state dir event leaked: %#v", ev)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for a.txt event")
		}
	}
}

func TestWatcherAddsNewSubdirectories(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events := w.Events(ctx)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	// Give the watcher goroutine a moment to add the newly-created directory.
	time.Sleep(100 * time.Millisecond)
	file := filepath.Join(sub, "nested.txt")
	if err := os.WriteFile(file, []byte("nested"), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case ev := <-events:
			if ev.Path == file {
				return
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for nested file event")
		}
	}
}
