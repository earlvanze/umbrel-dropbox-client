package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const dirMode os.FileMode = 0700
const fileMode os.FileMode = 0600

type Token struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

type Status struct {
	Path       string
	Present    bool
	TokenType  string
	AccountID  string
	Scope      string
	ExpiresAt  time.Time
	HasRefresh bool
}

func DefaultTokenPath() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "umbrel-dropbox-sync", "token.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "umbrel-dropbox-sync", "token.json"), nil
}

func SaveToken(path string, tok Token) error {
	if path == "" {
		return errors.New("token path is required")
	}
	if tok.AccessToken == "" && tok.RefreshToken == "" {
		return errors.New("token must include access_token or refresh_token")
	}
	if tok.TokenType == "" {
		tok.TokenType = "bearer"
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".token-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func LoadToken(path string) (Token, error) {
	var tok Token
	if path == "" {
		return tok, errors.New("token path is required")
	}
	st, err := os.Stat(path)
	if err != nil {
		return tok, err
	}
	if st.Mode().Perm()&0077 != 0 {
		return tok, fmt.Errorf("token file %s permissions %o are too broad; require 0600", path, st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return tok, err
	}
	if err := json.Unmarshal(b, &tok); err != nil {
		return tok, err
	}
	if tok.AccessToken == "" && tok.RefreshToken == "" {
		return tok, errors.New("token file does not contain access_token or refresh_token")
	}
	return tok, nil
}

func TokenStatus(path string) (Status, error) {
	out := Status{Path: path}
	tok, err := LoadToken(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	out.Present = true
	out.TokenType = tok.TokenType
	out.AccountID = tok.AccountID
	out.Scope = tok.Scope
	out.ExpiresAt = tok.ExpiresAt
	out.HasRefresh = tok.RefreshToken != ""
	return out, nil
}
