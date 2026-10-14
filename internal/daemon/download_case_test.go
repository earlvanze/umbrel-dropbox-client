package daemon

import (
	"github.com/earlvanze/umbrel-dropbox-client/internal/config"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteDownloadQueueRespectsExistingCaseOutsideScan(t *testing.T) {
	for _, kind := range []string{"existing leaf", "ambiguous parent", "symlink parent", "new leaf"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "MOVIES"), 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "existing leaf":
				if err := os.WriteFile(filepath.Join(root, "MOVIES", "Example.MKV"), []byte("local"), 0600); err != nil {
					t.Fatal(err)
				}
			case "ambiguous parent":
				if err := os.Mkdir(filepath.Join(root, "movies"), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink parent":
				if err := os.Remove(filepath.Join(root, "MOVIES")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "MOVIES")); err != nil {
					t.Fatal(err)
				}
			}
			s := testStore(t)
			d := New(config.Config{Root: root, RemotePath: "/Library", DryRun: true}, s, nil)
			n, err := d.enqueueRemoteDownloads(nil, []dropbox.Metadata{{Tag: "file", PathLower: "/library/movies/example.mkv", PathDisplay: "/Library/Movies/Example.MKV", Rev: "r1"}})
			want := 0
			if kind == "new leaf" {
				want = 1
			}
			if err != nil || n != want {
				t.Fatalf("queued=%d want=%d err=%v", n, want, err)
			}
			cs, err := s.ListConflicts(10)
			if err != nil || len(cs) != 1-want {
				t.Fatalf("conflicts=%#v err=%v", cs, err)
			}
		})
	}
}
