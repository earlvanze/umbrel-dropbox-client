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
}

func (s *Store) IngestRemoteDelta(ctx context.Context, client RemoteDeltaClient, remotePath string) (RemoteDeltaStats, error) {
	cursorKey := DropboxCursorKeyForPath(remotePath)
	cursor, err := s.GetConfig(cursorKey)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	delta, err := dropbox.FetchDelta(ctx, client, cursor, remotePath, true)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	applied, err := s.ApplyRemoteMetadataWithBase(delta.Entries, remotePath)
	if err != nil {
		return RemoteDeltaStats{}, err
	}
	if err := s.SetConfig(cursorKey, delta.Cursor); err != nil {
		return RemoteDeltaStats{}, err
	}
	stats := RemoteDeltaStats{PreviousCursor: cursor, Cursor: delta.Cursor, Pages: delta.Pages, Entries: len(delta.Entries), AppliedFiles: applied}
	if err := s.Event("remote.delta", fmt.Sprintf("previous_cursor=%s cursor=%s pages=%d entries=%d applied_files=%d", stats.PreviousCursor, stats.Cursor, stats.Pages, stats.Entries, stats.AppliedFiles)); err != nil {
		return stats, err
	}
	return stats, nil
}
