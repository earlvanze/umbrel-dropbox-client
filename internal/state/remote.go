package state

import (
	"strings"
	"time"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
)

func (s *Store) ApplyRemoteMetadata(entries []dropbox.Metadata) (int, error) {
	return s.ApplyRemoteMetadataWithBaseFilter(entries, "", nil)
}

func (s *Store) ApplyRemoteMetadataWithBase(entries []dropbox.Metadata, remoteBase string) (int, error) {
	return s.ApplyRemoteMetadataWithBaseFilter(entries, remoteBase, nil)
}

func (s *Store) ApplyRemoteMetadataWithBaseFilter(entries []dropbox.Metadata, remoteBase string, filter func(string) bool) (int, error) {
	if filter == nil {
		filter = func(string) bool { return true }
	}
	applied := 0
	for _, e := range entries {
		if e.Tag != "file" {
			continue
		}
		path := e.PathLower
		if path == "" {
			path = e.PathDisplay
		}
		path = stripRemoteBase(path, remoteBase)
		if path == "" {
			continue
		}
		if !filter(path) {
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

func stripRemoteBase(path, base string) string {
	path = normalizeEntryPath(path)
	base = normalizeEntryPath(base)
	if base == "" {
		return path
	}
	if path == base {
		return ""
	}
	prefix := strings.TrimSuffix(base, "/") + "/"
	if strings.HasPrefix(path, prefix) {
		return normalizeEntryPath(strings.TrimPrefix(path, prefix))
	}
	return path
}
