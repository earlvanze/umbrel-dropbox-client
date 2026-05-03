package dropbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	token      string
	http       *http.Client
	apiURL     string
	contentURL string
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
	return &Client{token: token, http: httpClient, apiURL: strings.TrimRight(apiURL, "/"), contentURL: contentURLFor(apiURL)}
}

func contentURLFor(apiURL string) string {
	apiURL = strings.TrimRight(apiURL, "/")
	if strings.Contains(apiURL, "api.dropboxapi.com") {
		return strings.Replace(apiURL, "api.dropboxapi.com", "content.dropboxapi.com", 1)
	}
	return apiURL
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
	if err := c.rpc(ctx, c.apiURL+"/users/get_current_account", nil, &out); err != nil {
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

func (c *Client) ListFolderLatestCursor(ctx context.Context, path string, recursive bool) (string, error) {
	var out struct {
		Cursor string `json:"cursor"`
	}
	in := map[string]any{
		"path":                                path,
		"recursive":                           recursive,
		"include_media_info":                  false,
		"include_deleted":                     false,
		"include_has_explicit_shared_members": false,
		"include_mounted_folders":             true,
		"include_non_downloadable_files":      false,
	}
	if err := c.rpc(ctx, c.apiURL+"/files/list_folder/get_latest_cursor", in, &out); err != nil {
		return "", err
	}
	return out.Cursor, nil
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

func (c *Client) UploadFile(ctx context.Context, dropboxPath, localPath string) (*Metadata, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return c.Upload(ctx, dropboxPath, f)
}

func (c *Client) Upload(ctx context.Context, dropboxPath string, body io.Reader) (*Metadata, error) {
	arg := map[string]any{
		"path":       dropboxPath,
		"mode":       "add",
		"autorename": false,
		"mute":       false,
	}
	var out Metadata
	if err := c.content(ctx, c.contentURL+"/files/upload", arg, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DownloadFile(ctx context.Context, dropboxPath, localPath string) (*Metadata, error) {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".download-*.tmp")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	meta, err := c.Download(ctx, dropboxPath, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, localPath); err != nil {
		return nil, err
	}
	return meta, nil
}

func (c *Client) Download(ctx context.Context, dropboxPath string, out io.Writer) (*Metadata, error) {
	arg := map[string]any{"path": dropboxPath}
	var meta Metadata
	if err := c.content(ctx, c.contentURL+"/files/download", arg, nil, &meta, out); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (c *Client) DeleteFile(ctx context.Context, dropboxPath string) (*Metadata, error) {
	var out struct {
		Metadata Metadata `json:"metadata"`
	}
	if err := c.rpc(ctx, c.apiURL+"/files/delete_v2", map[string]any{"path": dropboxPath}, &out); err != nil {
		return nil, err
	}
	return &out.Metadata, nil
}

func (c *Client) content(ctx context.Context, url string, arg any, body io.Reader, metaOut any, writers ...io.Writer) error {
	if body == nil {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	argRaw, err := json.Marshal(arg)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Dropbox-API-Arg", string(argRaw))
	if len(writers) == 0 {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("dropbox content %s: %s", url, res.Status)
	}
	if len(writers) > 0 {
		if header := res.Header.Get("Dropbox-API-Result"); header != "" && metaOut != nil {
			if err := json.Unmarshal([]byte(header), metaOut); err != nil {
				return err
			}
		}
		_, err = io.Copy(writers[0], res.Body)
		return err
	}
	if metaOut == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(metaOut)
}

func (c *Client) rpc(ctx context.Context, url string, in any, out any) error {
	if in == nil {
		in = json.RawMessage("null")
	}
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
