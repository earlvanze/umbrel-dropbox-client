package state

import (
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
)

func (s *Store) ApplyRemoteMetadata(entries []dropbox.Metadata) (int, error) {
	applied := 0
	for _, e := range entries {
		if e.Tag != "file" {
			continue
		}
		path := e.PathLower
		if path == "" {
			path = e.PathDisplay
		}
		if path == "" {
			continue
		}
		mtime := e.ServerMtime
		if mtime.IsZero() {
			mtime = time.Now().UTC()
		}
		if err := s.UpsertEntry(Entry{Path: path, DropboxID: e.ID, Rev: e.Rev, ContentHash: e.ContentHash, Size: e.Size, MTime: mtime, State: "remote_scanned"}); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}
