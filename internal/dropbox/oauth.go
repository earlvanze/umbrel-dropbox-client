package dropbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultOAuthURL = "https://api.dropboxapi.com/oauth2"
const DefaultOAuthAuthorizeURL = "https://www.dropbox.com/oauth2/authorize"

type OAuthClient struct {
	clientID string
	http     *http.Client
	baseURL  string
}

type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccountID    string `json:"account_id"`
}

type PKCEAuth struct {
	AuthorizeURL string
	CodeVerifier string
	State        string
}

type OAuthError struct {
	Code        string
	Description string
}

func (e OAuthError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("dropbox oauth %s: %s", e.Code, e.Description)
	}
	return "dropbox oauth " + e.Code
}

func NewOAuthClient(clientID string) *OAuthClient {
	return NewOAuthClientWithHTTP(clientID, &http.Client{Timeout: 30 * time.Second}, DefaultOAuthURL)
}

func NewOAuthClientWithHTTP(clientID string, httpClient *http.Client, baseURL string) *OAuthClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = DefaultOAuthURL
	}
	return &OAuthClient{clientID: clientID, http: httpClient, baseURL: strings.TrimRight(baseURL, "/")}
}

func GenerateCodeVerifier() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func DeriveCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *OAuthClient) StartPKCEAuth(redirectURI, state string, scopes []string) (*PKCEAuth, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("dropbox oauth client id is required")
	}
	if redirectURI == "" {
		return nil, fmt.Errorf("dropbox oauth redirect uri is required")
	}
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirectURI)
	values.Set("token_access_type", "offline")
	values.Set("code_challenge", DeriveCodeChallenge(verifier))
	values.Set("code_challenge_method", "S256")
	if state != "" {
		values.Set("state", state)
	}
	if len(scopes) > 0 {
		values.Set("scope", strings.Join(scopes, " "))
	}
	return &PKCEAuth{AuthorizeURL: DefaultOAuthAuthorizeURL + "?" + values.Encode(), CodeVerifier: verifier, State: state}, nil
}

func (c *OAuthClient) ExchangePKCECode(ctx context.Context, code, codeVerifier, redirectURI string) (*OAuthToken, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("dropbox oauth client id is required")
	}
	if code == "" {
		return nil, fmt.Errorf("dropbox oauth authorization code is required")
	}
	if codeVerifier == "" {
		return nil, fmt.Errorf("dropbox oauth code verifier is required")
	}
	if redirectURI == "" {
		return nil, fmt.Errorf("dropbox oauth redirect uri is required")
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	values.Set("code", code)
	values.Set("code_verifier", codeVerifier)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectURI)
	var out OAuthToken
	if err := c.form(ctx, c.baseURL+"/token", values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OAuthClient) StartDeviceCode(ctx context.Context) (*DeviceCode, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("dropbox oauth client id is required")
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	var out DeviceCode
	if err := c.form(ctx, c.baseURL+"/authorize", values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OAuthClient) PollDeviceToken(ctx context.Context, deviceCode string) (*OAuthToken, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("dropbox oauth client id is required")
	}
	if deviceCode == "" {
		return nil, fmt.Errorf("dropbox device code is required")
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	values.Set("device_code", deviceCode)
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	var out OAuthToken
	if err := c.form(ctx, c.baseURL+"/token", values, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OAuthClient) RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("dropbox oauth client id is required")
	}
	if refreshToken == "" {
		return nil, fmt.Errorf("dropbox refresh token is required")
	}
	values := url.Values{}
	values.Set("client_id", c.clientID)
	values.Set("refresh_token", refreshToken)
	values.Set("grant_type", "refresh_token")
	var out OAuthToken
	if err := c.form(ctx, c.baseURL+"/token", values, &out); err != nil {
		return nil, err
	}
	if out.RefreshToken == "" {
		out.RefreshToken = refreshToken
	}
	return &out, nil
}

func (c *OAuthClient) form(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var body struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.NewDecoder(res.Body).Decode(&body)
		if body.Error == "" {
			body.Error = res.Status
		}
		return OAuthError{Code: body.Error, Description: body.ErrorDescription}
	}
	return json.NewDecoder(res.Body).Decode(out)
}
