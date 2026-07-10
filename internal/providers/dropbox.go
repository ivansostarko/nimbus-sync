package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Dropbox talks to the Dropbox API v2.
type Dropbox struct {
	HTTP *http.Client
}

func (d *Dropbox) Name() string { return "dropbox" }

type dbxEntry struct {
	Tag            string    `json:".tag"`
	Name           string    `json:"name"`
	PathDisplay    string    `json:"path_display"`
	ID             string    `json:"id"`
	Size           int64     `json:"size"`
	ContentHash    string    `json:"content_hash"`
	ServerModified time.Time `json:"server_modified"`
}

type dbxListResult struct {
	Entries []dbxEntry `json:"entries"`
	Cursor  string     `json:"cursor"`
	HasMore bool       `json:"has_more"`
}

func (d *Dropbox) List(ctx context.Context, remotePath string) ([]RemoteFile, error) {
	root := strings.TrimSuffix(remotePath, "/")
	if root != "" && !strings.HasPrefix(root, "/") {
		root = "/" + root
	}

	body := map[string]any{"path": root, "recursive": true, "limit": 2000}
	var res dbxListResult
	if err := d.rpc(ctx, "https://api.dropboxapi.com/2/files/list_folder", body, &res); err != nil {
		return nil, fmt.Errorf("dropbox list %q: %w", root, err)
	}

	var out []RemoteFile
	collect := func(entries []dbxEntry) {
		for _, e := range entries {
			if e.Tag != "file" {
				continue
			}
			rel := strings.TrimPrefix(e.PathDisplay, root)
			rel = strings.TrimPrefix(rel, "/")
			out = append(out, RemoteFile{
				Path:     rel,
				Size:     e.Size,
				Modified: e.ServerModified,
				Hash:     e.ContentHash,
				ID:       e.PathDisplay, // download uses the path
			})
		}
	}
	collect(res.Entries)

	for res.HasMore {
		next := map[string]any{"cursor": res.Cursor}
		res = dbxListResult{}
		if err := d.rpc(ctx, "https://api.dropboxapi.com/2/files/list_folder/continue", next, &res); err != nil {
			return nil, err
		}
		collect(res.Entries)
	}
	return out, nil
}

func (d *Dropbox) Download(ctx context.Context, f RemoteFile) (io.ReadCloser, error) {
	arg, _ := json.Marshal(map[string]string{"path": f.ID})
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://content.dropboxapi.com/2/files/download", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Dropbox-API-Arg", string(arg))
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("dropbox download %s: HTTP %d: %s", f.Path, resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (d *Dropbox) rpc(ctx context.Context, url string, body any, v any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		time.Sleep(2 * time.Second)
		return d.rpc(ctx, url, body, v)
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
