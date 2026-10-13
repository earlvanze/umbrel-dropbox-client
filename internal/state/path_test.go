package state

import "testing"

func TestNormalizeEntryPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{".", ""},
		{"/", ""},
		{"a/b.txt", "/a/b.txt"},
		{"Notes/README.md", "/notes/readme.md"},
		{"trailing ", "/trailing"},
		{`(back\slash).txt`, "/(back/slash).txt"},
		{"/Ledger/README.md", "/ledger/readme.md"},
	}

	for _, tc := range tests {
		if got := normalizeEntryPath(tc.input); got != tc.want {
			t.Fatalf("normalizeEntryPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if normalizeEntryPath("/Ledger/README.md") != normalizeEntryPath("/ledger/readme.md") {
		t.Fatal("case-only display paths must share the Dropbox comparison key")
	}
}
