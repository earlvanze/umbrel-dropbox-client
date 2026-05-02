package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadTokenUsesPrivateFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "token.json")
	expires := time.Date(2026, 5, 2, 14, 0, 0, 0, time.UTC)
	if err := SaveToken(path, Token{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: expires, AccountID: "acct", Scope: "files.metadata.read"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0600 {
		t.Fatalf("mode=%o want 0600", got)
	}
	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "access" || loaded.RefreshToken != "refresh" || loaded.TokenType != "bearer" || !loaded.ExpiresAt.Equal(expires) {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestLoadTokenRejectsBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"secret"}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadToken(path)
	if err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("err=%v want permissions error", err)
	}
}

func TestTokenStatusDoesNotExposeSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := SaveToken(path, Token{AccessToken: "access-secret", RefreshToken: "refresh-secret", AccountID: "acct"}); err != nil {
		t.Fatal(err)
	}
	st, err := TokenStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Present || !st.HasRefresh || st.AccountID != "acct" {
		t.Fatalf("status=%#v", st)
	}
	if strings.Contains(st.TokenType+st.AccountID+st.Scope, "secret") {
		t.Fatalf("status leaked secret: %#v", st)
	}
}

func TestTokenStatusMissingFile(t *testing.T) {
	st, err := TokenStatus(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Present {
		t.Fatalf("status=%#v", st)
	}
}
