package worker

import (
	"context"
	"fmt"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/reconcile"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

type DeleteClient interface {
	DeleteFile(ctx context.Context, dropboxPath string) (*dropbox.Metadata, error)
}

type ReviewedDeleteHandler struct {
	Store                *state.Store
	Client               DeleteClient
	AllowLive            bool
	AllowReviewedDeletes bool
}

func (h ReviewedDeleteHandler) HandleOp(ctx context.Context, op state.PendingOp) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if !h.AllowLive {
		return fmt.Errorf("reviewed delete handler requires AllowLive=true")
	}
	if !h.AllowReviewedDeletes {
		return fmt.Errorf("reviewed delete handler requires AllowReviewedDeletes=true")
	}
	if h.Store == nil {
		return fmt.Errorf("reviewed delete handler missing store")
	}
	planned, err := decodePlannedOp(op)
	if err != nil {
		return err
	}
	if op.Status != "" && op.Status != "pending" {
		return fmt.Errorf("%s %s requires pending op status, got %q", planned.Op, planned.Path, op.Status)
	}
	if op.Op != "" && op.Op != planned.Op {
		return fmt.Errorf("%s %s pending op does not match reviewed payload op %q", op.Op, planned.Path, planned.Op)
	}
	switch planned.Op {
	case reconcile.OpReviewLocalDelete:
		if h.Client == nil {
			return fmt.Errorf("review_local_delete %s missing delete client", planned.Path)
		}
		if err := h.requireMatchingLocalMissing(planned); err != nil {
			return err
		}
		if _, err := h.Client.DeleteFile(ctx, planned.Path); err != nil {
			return err
		}
		if err := h.Store.DeleteEntry(planned.Path); err != nil {
			return err
		}
		return h.Store.Event("worker.live.review_delete", fmt.Sprintf("op=%s path=%s", planned.Op, planned.Path))
	case reconcile.OpReviewRemoteDelete:
		if err := h.requireMatchingLocalMissing(planned); err != nil {
			return err
		}
		if err := h.Store.DeleteEntry(planned.Path); err != nil {
			return err
		}
		return h.Store.Event("worker.live.review_delete", fmt.Sprintf("op=%s path=%s", planned.Op, planned.Path))
	default:
		return fmt.Errorf("unsupported pending op %q for reviewed delete", planned.Op)
	}
}

func (h ReviewedDeleteHandler) requireMatchingLocalMissing(planned reconcile.PlannedOp) error {
	entry, err := h.Store.EntryByPath(planned.Path)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("%s %s has no current state entry", planned.Op, planned.Path)
	}
	if entry.State != "local_missing" {
		return fmt.Errorf("%s %s requires current local_missing state, got %q", planned.Op, planned.Path, entry.State)
	}
	if planned.Rev != "" && entry.Rev != planned.Rev {
		return fmt.Errorf("%s %s rev changed", planned.Op, planned.Path)
	}
	if planned.DropboxID != "" && entry.DropboxID != planned.DropboxID {
		return fmt.Errorf("%s %s dropbox id changed", planned.Op, planned.Path)
	}
	if planned.ContentHash != "" && entry.ContentHash != planned.ContentHash {
		return fmt.Errorf("%s %s content hash changed", planned.Op, planned.Path)
	}
	return nil
}

type LiveHandler struct {
	Transfer TransferHandler
	Deletes  ReviewedDeleteHandler
}

func (h LiveHandler) HandleOp(ctx context.Context, op state.PendingOp) error {
	planned, err := decodePlannedOp(op)
	if err != nil {
		return err
	}
	switch planned.Op {
	case reconcile.OpReviewLocalDelete, reconcile.OpReviewRemoteDelete:
		return h.Deletes.HandleOp(ctx, op)
	default:
		return h.Transfer.HandleOp(ctx, op)
	}
}
