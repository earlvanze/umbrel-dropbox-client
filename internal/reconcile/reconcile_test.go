package reconcile

import (
	"testing"

	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"github.com/earlvanze/umbrel-dropbox-client/internal/scan"
)

func TestBuildDryRunPlanUploadDownloadConflictAndNoop(t *testing.T) {
	local := []scan.File{
		{Path: "local-only.txt", AbsPath: "/tmp/local-only.txt", ContentHash: "local", Size: 5},
		{Path: "same.txt", AbsPath: "/tmp/same.txt", ContentHash: "same", Size: 4},
		{Path: "different.txt", AbsPath: "/tmp/different.txt", ContentHash: "local-different", Size: 9},
	}
	remote := []dropbox.Metadata{
		{Tag: "file", PathLower: "/remote-only.txt", Rev: "r1", ContentHash: "remote", Size: 6},
		{Tag: "file", PathLower: "/same.txt", Rev: "r2", ContentHash: "same", Size: 4},
		{Tag: "file", PathLower: "/different.txt", Rev: "r3", ContentHash: "remote-different", Size: 10},
		{Tag: "folder", PathLower: "/ignored"},
	}

	plan := BuildDryRunPlan(local, remote)
	if plan.Noop != 1 {
		t.Fatalf("noop=%d plan=%#v", plan.Noop, plan)
	}
	if len(plan.Ops) != 2 {
		t.Fatalf("ops=%d plan=%#v", len(plan.Ops), plan)
	}
	if plan.Ops[0].Op != "upload_local" || plan.Ops[0].Path != "/local-only.txt" {
		t.Fatalf("unexpected upload op: %#v", plan.Ops[0])
	}
	if plan.Ops[1].Op != "download_remote" || plan.Ops[1].Path != "/remote-only.txt" || plan.Ops[1].Rev != "r1" {
		t.Fatalf("unexpected download op: %#v", plan.Ops[1])
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Path != "/different.txt" {
		t.Fatalf("unexpected conflicts: %#v", plan.Conflicts)
	}
}

func TestBuildDryRunPlanWithRemoteBaseUsesLocalRelativePaths(t *testing.T) {
	local := []scan.File{{Path: "note.md", AbsPath: "/tmp/Obsidian/note.md", ContentHash: "same", Size: 4}}
	remote := []dropbox.Metadata{{Tag: "file", PathLower: "/obsidian/note.md", Rev: "r1", ContentHash: "same", Size: 4}}

	plan := BuildDryRunPlanWithRemoteBase(local, remote, "/Obsidian")
	if plan.Noop != 1 || len(plan.Ops) != 0 || len(plan.Conflicts) != 0 {
		t.Fatalf("plan=%#v", plan)
	}

	plan = BuildDryRunPlanWithRemoteBase(nil, []dropbox.Metadata{{Tag: "file", PathLower: "/obsidian/remote.md", Rev: "r2", ContentHash: "remote", Size: 6}}, "/Obsidian")
	if len(plan.Ops) != 1 || plan.Ops[0].Op != "download_remote" || plan.Ops[0].Path != "/remote.md" || plan.Ops[0].RemotePath != "/obsidian/remote.md" {
		t.Fatalf("download plan=%#v", plan)
	}

	plan = BuildDryRunPlanWithRemoteBase([]scan.File{{Path: "local.md", AbsPath: "/tmp/Obsidian/local.md", ContentHash: "local", Size: 5}}, nil, "/Obsidian")
	if len(plan.Ops) != 1 || plan.Ops[0].Op != "upload_local" || plan.Ops[0].Path != "/local.md" || plan.Ops[0].RemotePath != "/obsidian/local.md" {
		t.Fatalf("upload plan=%#v", plan)
	}
}
