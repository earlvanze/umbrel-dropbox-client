package dropbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct { token string; http *http.Client }
func New(token string) *Client { return &Client{token: token, http: &http.Client{Timeout: 60*time.Second}} }

type Account struct {
	AccountID string `json:"account_id"`
	Email string `json:"email"`
	Name struct { DisplayName string `json:"display_name"` } `json:"name"`
}

func (c *Client) CurrentAccount(ctx context.Context) (*Account, error) {
	var out Account
	if err := c.rpc(ctx, "https://api.dropboxapi.com/2/users/get_current_account", map[string]any{}, &out); err != nil { return nil, err }
	return &out, nil
}

func (c *Client) rpc(ctx context.Context, url string, in any, out any) error {
	b, err := json.Marshal(in); if err != nil { return err }
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b)); if err != nil { return err }
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req); if err != nil { return err }
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 { return fmt.Errorf("dropbox rpc %s: %s", url, res.Status) }
	return json.NewDecoder(res.Body).Decode(out)
}
