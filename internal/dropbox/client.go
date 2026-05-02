package dropbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	token  string
	http   *http.Client
	apiURL string
}

func New(token string) *Client {
	return NewWithHTTP(token, &http.Client{Timeout: 60 * time.Second}, "https://api.dropboxapi.com/2")
}

func NewWithHTTP(token string, httpClient *http.Client, apiURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if apiURL == "" {
		apiURL = "https://api.dropboxapi.com/2"
	}
	return &Client{token: token, http: httpClient, apiURL: apiURL}
}

type Account struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
	Name      struct {
		DisplayName string `json:"display_name"`
	} `json:"name"`
}

type Metadata struct {
	Tag         string    `json:".tag"`
	Name        string    `json:"name"`
	PathLower   string    `json:"path_lower"`
	PathDisplay string    `json:"path_display"`
	ID          string    `json:"id"`
	ClientMtime time.Time `json:"client_modified"`
	ServerMtime time.Time `json:"server_modified"`
	Rev         string    `json:"rev"`
	Size        int64     `json:"size"`
	ContentHash string    `json:"content_hash"`
}

type ListFolderResult struct {
	Entries []Metadata `json:"entries"`
	Cursor  string     `json:"cursor"`
	HasMore bool       `json:"has_more"`
}

func (c *Client) CurrentAccount(ctx context.Context) (*Account, error) {
	var out Account
	if err := c.rpc(ctx, c.apiURL+"/users/get_current_account", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListFolder(ctx context.Context, path string, recursive bool) (*ListFolderResult, error) {
	var out ListFolderResult
	in := map[string]any{
		"path":                                path,
		"recursive":                           recursive,
		"include_media_info":                  false,
		"include_deleted":                     false,
		"include_has_explicit_shared_members": false,
		"include_mounted_folders":             true,
		"include_non_downloadable_files":      false,
	}
	if err := c.rpc(ctx, c.apiURL+"/files/list_folder", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListFolderContinue(ctx context.Context, cursor string) (*ListFolderResult, error) {
	var out ListFolderResult
	if err := c.rpc(ctx, c.apiURL+"/files/list_folder/continue", map[string]any{"cursor": cursor}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListFolderAll(ctx context.Context, path string, recursive bool) ([]Metadata, string, error) {
	page, err := c.ListFolder(ctx, path, recursive)
	if err != nil {
		return nil, "", err
	}
	entries := append([]Metadata{}, page.Entries...)
	cursor := page.Cursor
	for page.HasMore {
		page, err = c.ListFolderContinue(ctx, cursor)
		if err != nil {
			return nil, cursor, err
		}
		entries = append(entries, page.Entries...)
		cursor = page.Cursor
	}
	return entries, cursor, nil
}

func (c *Client) rpc(ctx context.Context, url string, in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("dropbox rpc %s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
