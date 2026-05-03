package reconcile

import (
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/state"
)

func TestBuildDeleteReviewPlanNeverPlansDestructiveDelete(t *testing.T) {
	missing := []state.MissingLocal{
		{Path: "/remote-still-there.txt", DropboxID: "id:1", Rev: "r1", ContentHash: "h1", Size: 10},
		{Path: "/not-seen-remotely.txt", DropboxID: "id:2", Rev: "r2", ContentHash: "h2", Size: 20},
	}
	remote := RemotePathSet([]dropbox.Metadata{{Tag: "file", PathLower: "/remote-still-there.txt"}})
	plan := BuildDeleteReviewPlan(missing, remote)
	if len(plan.Ops) != 2 {
		t.Fatalf("ops=%#v", plan.Ops)
	}
	if plan.Ops[0].Op != OpReviewLocalDelete || plan.Ops[0].Path != "/remote-still-there.txt" || plan.Ops[0].Rev != "r1" {
		t.Fatalf("first op=%#v", plan.Ops[0])
	}
	if plan.Ops[1].Op != OpReviewRemoteDelete || plan.Ops[1].Path != "/not-seen-remotely.txt" || plan.Ops[1].Rev != "r2" {
		t.Fatalf("second op=%#v", plan.Ops[1])
	}
	for _, op := range plan.Ops {
		if op.Op == "delete_local" || op.Op == "delete_remote" {
			t.Fatalf("destructive delete op must not be planned: %#v", op)
		}
	}
}

func TestRemotePathSetSkipsFoldersAndNormalizes(t *testing.T) {
	set := RemotePathSet([]dropbox.Metadata{
		{Tag: "folder", PathLower: "/Folder"},
		{Tag: "file", PathDisplay: "/Mixed/Case.txt"},
	})
	if set["/folder"] {
		t.Fatal("folder should not be included")
	}
	if !set["/mixed/case.txt"] {
		t.Fatalf("set=%#v", set)
	}
}
