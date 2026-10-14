package reconcile

import (
	"github.com/earlvanze/umbrel-dropbox-client/internal/dropbox"
	"testing"
)

func TestDownloadPlanKeepsScopedDisplayCase(t *testing.T) {
	plan := BuildDryRunPlanWithRemoteBase(nil, []dropbox.Metadata{{Tag: "file", PathLower: "/library/movies/example/movie.mkv", PathDisplay: "/Library/Movies/Example/Movie.MKV", Rev: "r1"}}, "/LIBRARY")
	if len(plan.Ops) != 1 {
		t.Fatalf("plan=%#v", plan)
	}
	op := plan.Ops[0]
	if op.Path != "/movies/example/movie.mkv" || op.RemotePath != "/library/movies/example/movie.mkv" || op.DisplayPath != "/Movies/Example/Movie.MKV" {
		t.Fatalf("op=%#v", op)
	}
}

func TestRelativeDisplayPathRejectsMismatchedIdentity(t *testing.T) {
	for _, display := range []string{"", "/Library/Other.txt", "/Other/File.txt"} {
		if got := RelativeDisplayPath(display, "/library/file.txt", "/file.txt"); got != "" {
			t.Fatalf("display=%q result=%q", display, got)
		}
	}
}
