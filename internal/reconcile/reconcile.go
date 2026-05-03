package reconcile

import (
	"sort"
	"strings"

	"github.com/earlvanze/umbrel-dropbox-client/internal/conflict"
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
)

type PlannedOp struct {
	Op          string `json:"op"`
	Path        string `json:"path"`
	LocalPath   string `json:"local_path,omitempty"`
	DropboxID   string `json:"dropbox_id,omitempty"`
	Rev         string `json:"rev,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Reason      string `json:"reason"`
}

type PlannedConflict struct {
	Path      string `json:"path"`
	Reason    string `json:"reason"`
	LocalPath string `json:"local_path,omitempty"`
	RemoteRev string `json:"remote_rev,omitempty"`
}

type Plan struct {
	Ops       []PlannedOp       `json:"ops"`
	Conflicts []PlannedConflict `json:"conflicts"`
	Noop      int               `json:"noop"`
}

func BuildDryRunPlan(localFiles []scan.File, remoteEntries []dropbox.Metadata) Plan {
	locals := make(map[string]scan.File, len(localFiles))
	remotes := make(map[string]dropbox.Metadata, len(remoteEntries))
	keys := make(map[string]bool, len(localFiles)+len(remoteEntries))
	for _, f := range localFiles {
		p := normalize(scan.DropboxPath(f.Path))
		if p == "" {
			continue
		}
		locals[p] = f
		keys[p] = true
	}
	for _, e := range remoteEntries {
		if e.Tag != "file" {
			continue
		}
		p := e.PathLower
		if p == "" {
			p = e.PathDisplay
		}
		p = normalize(p)
		if p == "" {
			continue
		}
		remotes[p] = e
		keys[p] = true
	}

	paths := make([]string, 0, len(keys))
	for p := range keys {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var out Plan
	for _, p := range paths {
		lf, hasLocal := locals[p]
		re, hasRemote := remotes[p]
		res := conflict.Decide(conflict.Side{}, conflict.Side{Exists: hasLocal, ContentHash: lf.ContentHash, MTime: lf.ModTime}, conflict.Side{Exists: hasRemote, ContentHash: re.ContentHash, Rev: re.Rev, MTime: re.ServerMtime})
		switch res.Decision {
		case conflict.Noop:
			out.Noop++
		case conflict.UploadLocal:
			out.Ops = append(out.Ops, PlannedOp{Op: string(conflict.UploadLocal), Path: p, LocalPath: lf.AbsPath, ContentHash: lf.ContentHash, Size: lf.Size, Reason: res.Reason})
		case conflict.DownloadRemote:
			out.Ops = append(out.Ops, PlannedOp{Op: string(conflict.DownloadRemote), Path: p, DropboxID: re.ID, Rev: re.Rev, ContentHash: re.ContentHash, Size: re.Size, Reason: res.Reason})
		case conflict.RecordConflict:
			out.Conflicts = append(out.Conflicts, PlannedConflict{Path: p, Reason: res.Reason, LocalPath: lf.AbsPath, RemoteRev: re.Rev})
		}
	}
	return out
}

func normalize(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || path == "." || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.ToLower(path)
}
