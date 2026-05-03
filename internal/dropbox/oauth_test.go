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

func TestPKCEVerifierAndChallenge(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Fatalf("verifier length=%d", len(verifier))
	}
	challenge := DeriveCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if challenge != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge=%q", challenge)
	}
}

func TestStartPKCEAuthBuildsAuthorizeURL(t *testing.T) {
	got, err := NewOAuthClient("app-key").StartPKCEAuth("http://127.0.0.1:17653/callback", "state-1", []string{"account_info.read", "files.metadata.read", "files.content.write"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "https" || u.Host != "www.dropbox.com" || u.Path != "/oauth2/authorize" {
		t.Fatalf("url=%s", got.AuthorizeURL)
	}
	if q.Get("client_id") != "app-key" || q.Get("response_type") != "code" || q.Get("redirect_uri") != "http://127.0.0.1:17653/callback" || q.Get("state") != "state-1" {
		t.Fatalf("query=%#v", q)
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || got.CodeVerifier == "" {
		t.Fatalf("pkce missing query=%#v verifier=%q", q, got.CodeVerifier)
	}
	if q.Get("scope") != "account_info.read files.metadata.read files.content.write" || q.Get("token_access_type") != "offline" {
		t.Fatalf("scope/offline query=%#v", q)
	}
}

func TestExchangePKCECodePostsVerifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		mustParseForm(t, r)
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "auth-code" || r.Form.Get("code_verifier") != "verifier" || r.Form.Get("client_id") != "app-key" || r.Form.Get("redirect_uri") != "http://127.0.0.1:17653/callback" {
			t.Fatalf("form=%#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(OAuthToken{AccessToken: "access", RefreshToken: "refresh", TokenType: "bearer", ExpiresIn: 14400, Scope: "files.metadata.read", AccountID: "acct"})
	}))
	defer srv.Close()

	tok, err := NewOAuthClientWithHTTP("app-key", srv.Client(), srv.URL+"/oauth2").ExchangePKCECode(context.Background(), "auth-code", "verifier", "http://127.0.0.1:17653/callback")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" || tok.AccountID != "acct" {
		t.Fatalf("token=%#v", tok)
	}
}
