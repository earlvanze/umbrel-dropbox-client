package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/earl/umbrel-dropbox-sync/internal/conflict"
	"github.com/earl/umbrel-dropbox-sync/internal/reconcile"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

// DryRunHandler validates planned transfer operations and records what would run
// without mutating local files or Dropbox state.
type DryRunHandler struct {
	Store *state.Store
}

func (h DryRunHandler) HandleOp(ctx context.Context, op state.PendingOp) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if h.Store == nil {
		return fmt.Errorf("dry-run handler missing store")
	}
	planned, err := decodePlannedOp(op)
	if err != nil {
		return err
	}
	switch planned.Op {
	case string(conflict.UploadLocal):
		if planned.LocalPath == "" {
			return fmt.Errorf("upload_local %s missing local_path", planned.Path)
		}
	case string(conflict.DownloadRemote):
		if planned.Rev == "" && planned.DropboxID == "" {
			return fmt.Errorf("download_remote %s missing rev/dropbox_id", planned.Path)
		}
	default:
		return fmt.Errorf("unsupported pending op %q for %s", planned.Op, planned.Path)
	}
	return h.Store.Event("worker.dry_run", fmt.Sprintf("op=%s path=%s reason=%s", planned.Op, planned.Path, planned.Reason))
}

func decodePlannedOp(op state.PendingOp) (reconcile.PlannedOp, error) {
	var planned reconcile.PlannedOp
	if op.Payload != "" {
		if err := json.Unmarshal([]byte(op.Payload), &planned); err != nil {
			return planned, fmt.Errorf("decode pending op payload id=%d: %w", op.ID, err)
		}
	}
	if planned.Op == "" {
		planned.Op = op.Op
	}
	if planned.Path == "" {
		planned.Path = op.Path
	}
	if planned.Path == "" {
		return planned, fmt.Errorf("pending op id=%d missing path", op.ID)
	}
	return planned, nil
}
