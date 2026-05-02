package state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
)

func TestApplyRemoteMetadataSkipsNonFilesAndStoresFiles(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	applied, err := s.ApplyRemoteMetadata([]dropbox.Metadata{
		{Tag: "folder", PathLower: "/folder"},
		{Tag: "file", PathLower: "/a.txt", ID: "id:a", Rev: "r1", ContentHash: "h1", Size: 1, ServerMtime: time.Date(2026, 5, 2, 15, 0, 0, 0, time.UTC)},
		{Tag: "file", PathDisplay: "/B.txt", ID: "id:b", Rev: "r2", ContentHash: "h2", Size: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("applied=%d", applied)
	}
	st, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 2 {
		t.Fatalf("status=%#v", st)
	}
}
