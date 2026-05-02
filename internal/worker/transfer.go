package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/earl/umbrel-dropbox-sync/internal/conflict"
	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/hash"
	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

type TransferClient interface {
	UploadFile(ctx context.Context, dropboxPath, localPath string) (*dropbox.Metadata, error)
	DownloadFile(ctx context.Context, dropboxPath, localPath string) (*dropbox.Metadata, error)
}

type TransferHandler struct {
	Store     *state.Store
	Client    TransferClient
	Root      string
	AllowLive bool
}

func (h TransferHandler) HandleOp(ctx context.Context, op state.PendingOp) error {
	if !h.AllowLive {
		return fmt.Errorf("live transfer handler requires AllowLive=true")
	}
	if h.Store == nil {
		return fmt.Errorf("transfer handler missing store")
	}
	if h.Client == nil {
		return fmt.Errorf("transfer handler missing client")
	}
	root, err := filepath.Abs(h.Root)
	if err != nil {
		return err
	}
	if root == "" || root == string(filepath.Separator) {
		return fmt.Errorf("transfer handler requires non-root sync root")
	}
	planned, err := decodePlannedOp(op)
	if err != nil {
		return err
	}
	localPath, err := h.localPath(root, planned.Path, planned.LocalPath)
	if err != nil {
		return err
	}
	switch planned.Op {
	case string(conflict.UploadLocal):
		return h.upload(ctx, planned, localPath)
	case string(conflict.DownloadRemote):
		return h.download(ctx, planned, localPath)
	default:
		return fmt.Errorf("unsupported pending op %q for transfer", planned.Op)
	}
}

func (h TransferHandler) upload(ctx context.Context, planned reconcile.PlannedOp, localPath string) error {
	if planned.ContentHash != "" {
		got, err := hash.DropboxContentHash(localPath)
		if err != nil {
			return err
		}
		if got != planned.ContentHash {
			return fmt.Errorf("upload_local %s content hash changed", planned.Path)
		}
	}
	meta, err := h.Client.UploadFile(ctx, planned.Path, localPath)
	if err != nil {
		return err
	}
	return h.Store.UpsertEntry(state.Entry{Path: planned.Path, DropboxID: meta.ID, Rev: meta.Rev, ContentHash: meta.ContentHash, Size: meta.Size, MTime: meta.ServerMtime, State: "clean"})
}

func (h TransferHandler) download(ctx context.Context, planned reconcile.PlannedOp, localPath string) error {
	if _, err := os.Stat(localPath); err == nil {
		return fmt.Errorf("download_remote %s refused to overwrite existing local file", planned.Path)
	} else if !os.IsNotExist(err) {
		return err
	}
	meta, err := h.Client.DownloadFile(ctx, planned.Path, localPath)
	if err != nil {
		return err
	}
	contentHash, err := hash.DropboxContentHash(localPath)
	if err != nil {
		return err
	}
	if planned.ContentHash != "" && contentHash != planned.ContentHash {
		return fmt.Errorf("download_remote %s content hash mismatch", planned.Path)
	}
	return h.Store.UpsertEntry(state.Entry{Path: planned.Path, DropboxID: meta.ID, Rev: meta.Rev, ContentHash: contentHash, Size: meta.Size, MTime: meta.ServerMtime, State: "clean"})
}

func (h TransferHandler) localPath(root, dropboxPath, plannedLocalPath string) (string, error) {
	localPath := plannedLocalPath
	if localPath == "" {
		localPath = filepath.Join(root, strings.TrimPrefix(filepath.FromSlash(dropboxPath), string(filepath.Separator)))
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("local path %s escapes sync root %s", abs, root)
	}
	return abs, nil
}
