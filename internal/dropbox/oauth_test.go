package dropbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOAuthStartDeviceCodePostsClientID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/authorize" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type=%q", got)
		}
		mustParseForm(t, r)
		if r.Form.Get("client_id") != "app-key" {
			t.Fatalf("form=%#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(DeviceCode{DeviceCode: "dev", UserCode: "ABCD", VerificationURI: "https://dropbox.com/device", Interval: 5})
	}))
	defer srv.Close()

	got, err := NewOAuthClientWithHTTP("app-key", srv.Client(), srv.URL+"/oauth2").StartDeviceCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceCode != "dev" || got.UserCode != "ABCD" || got.Interval != 5 {
		t.Fatalf("device code=%#v", got)
	}
}

func TestOAuthPollDeviceTokenHandlesPendingAndSuccess(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/oauth2/token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		mustParseForm(t, r)
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || r.Form.Get("device_code") != "dev" || r.Form.Get("client_id") != "app-key" {
			t.Fatalf("form=%#v", r.Form)
		}
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"not yet"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(OAuthToken{AccessToken: "access", RefreshToken: "refresh", TokenType: "bearer", ExpiresIn: 14400, Scope: "files.metadata.read", AccountID: "acct"})
	}))
	defer srv.Close()

	client := NewOAuthClientWithHTTP("app-key", srv.Client(), srv.URL+"/oauth2")
	_, err := client.PollDeviceToken(context.Background(), "dev")
	if err == nil {
		t.Fatal("expected authorization_pending")
	}
	oauthErr, ok := err.(OAuthError)
	if !ok || oauthErr.Code != "authorization_pending" {
		t.Fatalf("err=%T %v", err, err)
	}
	tok, err := client.PollDeviceToken(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" || tok.AccountID != "acct" || tok.Scope == "" {
		t.Fatalf("token=%#v", tok)
	}
}

func TestOAuthRefreshTokenPreservesRefreshWhenDropboxOmitsIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mustParseForm(t, r)
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh" || r.Form.Get("client_id") != "app-key" {
			t.Fatalf("form=%#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(OAuthToken{AccessToken: "new-access", TokenType: "bearer", ExpiresIn: 14400})
	}))
	defer srv.Close()

	tok, err := NewOAuthClientWithHTTP("app-key", srv.Client(), srv.URL+"/oauth2").RefreshToken(context.Background(), "refresh")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "new-access" || tok.RefreshToken != "refresh" {
		t.Fatalf("token=%#v", tok)
	}
}

func mustParseForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return r.Form
}
