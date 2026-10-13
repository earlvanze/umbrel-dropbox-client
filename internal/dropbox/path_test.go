package dropbox

import "testing"

func TestNormalizeDropboxPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"/", ""},
		{"/Ledger/README.md", "/ledger/readme.md"},
		{"a/b.txt", "/a/b.txt"},
		{"/a/b/", "/a/b"},
		{" /a/b ", "/a/b"},
	}
	for _, tc := range tests {
		if got := NormalizeDropboxPath(tc.in); got != tc.want {
			t.Fatalf("NormalizeDropboxPath(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if NormalizeDropboxPath("/A/README.md") != NormalizeDropboxPath("/a/readme.md") {
		t.Fatal("expected case-fold comparison key")
	}
}
