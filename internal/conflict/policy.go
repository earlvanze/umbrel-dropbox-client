package conflict

import "time"

type Side struct {
	Exists      bool
	ContentHash string
	Rev         string
	MTime       time.Time
}

type Decision string

const (
	Noop           Decision = "noop"
	UploadLocal    Decision = "upload_local"
	DownloadRemote Decision = "download_remote"
	DeleteLocal    Decision = "delete_local"
	RecordConflict Decision = "record_conflict"
)

type Result struct {
	Decision Decision
	Reason   string
}

func Decide(base, local, remote Side) Result {
	if local.Exists && remote.Exists && local.ContentHash == remote.ContentHash && local.ContentHash != "" {
		return Result{Decision: Noop, Reason: "local and remote hashes match"}
	}
	if !base.Exists {
		switch {
		case local.Exists && !remote.Exists:
			return Result{Decision: UploadLocal, Reason: "new local file"}
		case !local.Exists && remote.Exists:
			return Result{Decision: DownloadRemote, Reason: "new remote file"}
		case local.Exists && remote.Exists:
			return Result{Decision: RecordConflict, Reason: "concurrent local and remote create"}
		default:
			return Result{Decision: Noop, Reason: "nothing exists"}
		}
	}
	localChanged := local.Exists && local.ContentHash != base.ContentHash
	remoteChanged := remote.Exists && remote.Rev != base.Rev
	localDeleted := !local.Exists
	remoteDeleted := !remote.Exists
	if localChanged && !remoteChanged && !remoteDeleted {
		return Result{Decision: UploadLocal, Reason: "local changed"}
	}
	if remoteChanged && !localChanged && !localDeleted {
		return Result{Decision: DownloadRemote, Reason: "remote changed"}
	}
	if localDeleted && !remoteChanged {
		return Result{Decision: DeleteLocal, Reason: "local deleted; remote unchanged"}
	}
	if remoteDeleted && !localChanged {
		return Result{Decision: DeleteLocal, Reason: "remote deleted; local unchanged"}
	}
	if localChanged && remoteChanged {
		return Result{Decision: RecordConflict, Reason: "concurrent edits"}
	}
	if localDeleted && remoteChanged {
		return Result{Decision: RecordConflict, Reason: "local delete conflicts with remote edit"}
	}
	if remoteDeleted && localChanged {
		return Result{Decision: RecordConflict, Reason: "remote delete conflicts with local edit"}
	}
	return Result{Decision: Noop, Reason: "unchanged"}
}
