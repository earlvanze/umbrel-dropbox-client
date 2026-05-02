package dropbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultOAuthURL = "https://api.dropboxapi.com/oauth2"

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
