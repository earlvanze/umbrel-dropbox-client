package worker

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/hash"
	"github.com/earlvanze/umbrel-dropbox-client/internal/localpath"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

type metadataGetter interface {
	GetMetadata(context.Context, string) (*dropbox.Metadata, error)
}
type streamingDownloader interface {
	Download(context.Context, string, io.Writer) (*dropbox.Metadata, error)
}

func (h TransferHandler) holdDownload(planned reconcile.PlannedOp, reason, local string) error {
	if _, _, err := h.Store.AddConflictIfMissing(planned.Path, reason, local, planned.Rev); err != nil {
		return err
	}
	return conflictHoldError{path: planned.Path}
}

func (h TransferHandler) download(ctx context.Context, planned reconcile.PlannedOp, root string) error {
	remote := planned.RemotePath
	if remote == "" {
		remote = planned.Path
	}
	display := planned.DisplayPath
	// Old queue payloads have no display spelling. Recover it from current
	// metadata where supported, validating identity and the queued revision.
	if display == "" {
		if client, ok := h.Client.(metadataGetter); ok {
			meta, err := client.GetMetadata(ctx, remote)
			if err != nil {
				return err
			}
			if meta != nil {
				if meta.Tag != "file" || (meta.PathLower != "" && strings.ToLower(meta.PathLower) != strings.ToLower(remote)) || (planned.DropboxID != "" && meta.ID != "" && planned.DropboxID != meta.ID) || (planned.Rev != "" && meta.Rev != planned.Rev) {
					return h.holdDownload(planned, "remote identity or revision changed before download", "")
				}
				display = reconcile.RelativeDisplayPath(meta.PathDisplay, remote, planned.Path)
				if meta.PathDisplay != "" && display == "" {
					return h.holdDownload(planned, "remote display path does not match queued identity", "")
				}
			}
		}
		if display == "" {
			display = planned.Path
		}
	}
	if strings.ToLower(display) != strings.ToLower(planned.Path) {
		return h.holdDownload(planned, "download display path does not match queued identity", "")
	}
	resolve := func() (string, error) {
		local, err := localpath.Resolve(root, display)
		if errors.Is(err, localpath.ErrUnsafe) {
			return "", h.holdDownload(planned, "unsafe or ambiguous download destination", local)
		}
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(local); err == nil {
			return "", h.holdDownload(planned, "remote changed while local file exists", local)
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return local, nil
	}
	local, err := resolve()
	if err != nil {
		return err
	}
	// Anchor staging, directory creation and the final no-clobber link to the
	// configured root. An external symlink replacement cannot escape this root.
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return h.holdDownload(planned, "sync root became a symlink", "")
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer fs.Close()
	openedInfo, err := fs.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(rootInfo, openedInfo) {
		return h.holdDownload(planned, "sync root changed before download", "")
	}
	stage := ".download-" + rand.Text() + ".tmp"
	f, err := fs.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	defer func() { _ = fs.Remove(stage) }()
	var meta *dropbox.Metadata
	if client, ok := h.Client.(streamingDownloader); ok {
		meta, err = client.Download(ctx, remote, f)
	} else {
		// Path-only clients stage in a private temporary directory. Copy
		// their result into the anchored file instead of exposing a path
		// through the mutable sync tree to the client.
		meta, err = downloadFileInto(ctx, h.Client, remote, f)
	}
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return err
	}
	contentHash, err := hash.DropboxContentHashReader(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if meta == nil {
		return fmt.Errorf("download_remote %s returned no metadata", planned.Path)
	}
	if planned.ContentHash != "" && contentHash != planned.ContentHash {
		return fmt.Errorf("download_remote %s content hash mismatch", planned.Path)
	}
	if planned.Rev != "" && meta.Rev != planned.Rev {
		return h.holdDownload(planned, "remote revision changed during download", local)
	}
	if planned.DropboxID != "" && meta.ID != "" && planned.DropboxID != meta.ID {
		return h.holdDownload(planned, "remote identity changed during download", local)
	}
	if meta.PathLower != "" && strings.ToLower(meta.PathLower) != strings.ToLower(remote) {
		return h.holdDownload(planned, "remote path changed during download", local)
	}
	if meta.PathDisplay != "" {
		currentDisplay := reconcile.RelativeDisplayPath(meta.PathDisplay, remote, planned.Path)
		if currentDisplay == "" {
			return h.holdDownload(planned, "remote display path changed during download", local)
		}
		display = currentDisplay
	}
	// Re-resolve after the network transfer: local names may have appeared or
	// changed while downloading. Never publish over an existing case variant.
	local, err = resolve()
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, local)
	if err != nil {
		return err
	}
	if err = fs.MkdirAll(filepath.Dir(rel), 0700); err != nil {
		return err
	}
	local, err = resolve()
	if err != nil {
		return err
	}
	rel, err = filepath.Rel(root, local)
	if err != nil {
		return err
	}
	if err = fs.Link(stage, rel); err != nil {
		if os.IsExist(err) {
			return h.holdDownload(planned, "local file appeared during download", local)
		}
		return err
	}
	return h.Store.UpsertEntry(state.Entry{Path: planned.Path, DropboxID: meta.ID, Rev: meta.Rev, ContentHash: contentHash, Size: meta.Size, MTime: meta.ServerMtime, State: "clean"})
}

func downloadFileInto(ctx context.Context, client TransferClient, remote string, out io.Writer) (*dropbox.Metadata, error) {
	dir, err := os.MkdirTemp("", "syncnest-download-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	name := filepath.Join(dir, "content")
	meta, err := client.DownloadFile(ctx, remote, name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(out, f); err != nil {
		return nil, err
	}
	return meta, nil
}
