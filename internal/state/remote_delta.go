package state

import (
	"context"
	"fmt"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
)

const DropboxCursorKey = "dropbox_cursor"

func DropboxCursorKeyForPath(remotePath string) string {
	p := normalizeEntryPath(remotePath)
	if p == "" {
		return DropboxCursorKey + ":root"
	}
	return DropboxCursorKey + ":" + p
}

type RemoteDeltaClient interface {
	ListFolder(ctx context.Context, path string, recursive bool) (*dropbox.ListFolderResult, error)
	ListFolderContinue(ctx context.Context, cursor string) (*dropbox.ListFolderResult, error)
}

type RemoteDeltaStats struct {
	PreviousCursor string
	Cursor         string
	Pages          int
	Entries        int
	AppliedFiles   int
	DeletedEntries int
	// Metadata is the exact delta received during this cycle. The daemon uses
	// it to queue only newly observed remote downloads, rather than walking the
	// entire persisted remote-scanned backlog on every cycle.
	Metadata []dropbox.Metadata
}

func (s *Store) IngestRemoteDelta(ctx context.Context, client RemoteDeltaClient, remotePath string) (RemoteDeltaStats, error) {
	return s.IngestRemoteDeltaFilter(ctx, client, remotePath, nil)
}

func (s *Store) IngestRemoteDeltaFilter(ctx context.Context, client RemoteDeltaClient, remotePath string, filter func(string) bool) (RemoteDeltaStats, error) {
	cursorKey := DropboxCursorKeyForPath(remotePath)
	cursor, err := s.GetConfig(cursorKey)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	delta, err := dropbox.FetchDelta(ctx, client, cursor, remotePath, true)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	accepted := make([]dropbox.Metadata, 0, len(delta.Entries))
	for _, entry := range delta.Entries {
		path := entry.PathLower
		if path == "" {
			path = entry.PathDisplay
		}
		path = stripRemoteBase(path, remotePath)
		if path != "" && (filter == nil || filter(path)) {
			accepted = append(accepted, entry)
		}
	}
	applied, deleted, err := s.applyRemoteDeltaMetadata(accepted, remotePath, nil)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	if err := s.SetConfig(cursorKey, delta.Cursor); err != nil {
		return RemoteDeltaStats{}, err
	}
	stats := RemoteDeltaStats{PreviousCursor: cursor, Cursor: delta.Cursor, Pages: delta.Pages, Entries: len(delta.Entries), AppliedFiles: applied, DeletedEntries: deleted, Metadata: accepted}
	if err := s.Event("remote.delta", fmt.Sprintf("previous_cursor=%s cursor=%s pages=%d entries=%d applied_files=%d deleted_entries=%d", stats.PreviousCursor, stats.Cursor, stats.Pages, stats.Entries, stats.AppliedFiles, stats.DeletedEntries)); err != nil {
		return stats, err
	}
	return stats, nil
}

func (s *Store) applyRemoteDeltaMetadata(entries []dropbox.Metadata, remoteBase string, filter func(string) bool) (int, int, error) {
	if filter == nil {
		filter = func(string) bool { return true }
	}
	applied := 0
	deleted := 0
	for _, entry := range entries {
		path := entry.PathLower
		if path == "" {
			path = entry.PathDisplay
		}
		path = stripRemoteBase(path, remoteBase)
		if path == "" || !filter(path) {
			continue
		}
		if entry.Tag == "deleted" {
			count, err := s.DeleteEntriesUnder(path)
			if err != nil {
				return applied, deleted, err
			}
			deleted += count
			continue
		}
		if entry.Tag != "file" {
			continue
		}
		count, err := s.ApplyRemoteMetadataWithBaseFilter([]dropbox.Metadata{entry}, remoteBase, filter)
		if err != nil {
			return applied, deleted, err
		}
		applied += count
	}
	return applied, deleted, nil
}
