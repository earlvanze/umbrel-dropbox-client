package reconcile

import (
	"github.com/earl/umbrel-dropbox-sync/internal/dropbox"
	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

const (
	OpReviewLocalDelete  = "review_local_delete"
	OpReviewRemoteDelete = "review_remote_delete"
)

func BuildDeleteReviewPlan(missingLocal []state.MissingLocal, remoteExists map[string]bool) Plan {
	var out Plan
	for _, item := range missingLocal {
		p := normalize(item.Path)
		if p == "" {
			continue
		}
		if remoteExists != nil && remoteExists[p] {
			out.Ops = append(out.Ops, PlannedOp{Op: OpReviewLocalDelete, Path: p, DropboxID: item.DropboxID, Rev: item.Rev, ContentHash: item.ContentHash, Size: item.Size, Reason: "local file missing while remote still exists; review before deleting remote"})
			continue
		}
		out.Ops = append(out.Ops, PlannedOp{Op: OpReviewRemoteDelete, Path: p, DropboxID: item.DropboxID, Rev: item.Rev, ContentHash: item.ContentHash, Size: item.Size, Reason: "local file missing and remote not observed; review tombstone before pruning state"})
	}
	return out
}

func RemotePathSet(entries []dropbox.Metadata) map[string]bool {
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Tag != "file" {
			continue
		}
		p := e.PathLower
		if p == "" {
			p = e.PathDisplay
		}
		p = normalize(p)
		if p != "" {
			out[p] = true
		}
	}
	return out
}
