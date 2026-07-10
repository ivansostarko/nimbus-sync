package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const graphBase = "https://graph.microsoft.com/v1.0"

// OneDrive talks to Microsoft Graph /me/drive.
type OneDrive struct {
	HTTP *http.Client
}

func (o *OneDrive) Name() string { return "onedrive" }

type graphItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Folder *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder"`
	File *struct {
		Hashes struct {
			QuickXorHash string `json:"quickXorHash"`
			SHA256Hash   string `json:"sha256Hash"`
		} `json:"hashes"`
	} `json:"file"`
	LastModified time.Time `json:"lastModifiedDateTime"`
}

type graphChildren struct {
	Value    []graphItem `json:"value"`
	NextLink string      `json:"@odata.nextLink"`
}

func (o *OneDrive) List(ctx context.Context, remotePath string) ([]RemoteFile, error) {
	var out []RemoteFile
	err := o.walk(ctx, remotePath, "", &out)
	return out, err
}

func (o *OneDrive) walk(ctx context.Context, root, rel string, out *[]RemoteFile) error {
	full := strings.Trim(strings.TrimSuffix(root, "/")+"/"+rel, "/")
	var endpoint string
	if full == "" {
		endpoint = graphBase + "/me/drive/root/children"
	} else {
		endpoint = graphBase + "/me/drive/root:/" + url.PathEscape(full) + ":/children"
	}

	for endpoint != "" {
		var page graphChildren
		if err := o.getJSON(ctx, endpoint, &page); err != nil {
			return fmt.Errorf("onedrive list %q: %w", full, err)
		}
		for _, it := range page.Value {
			childRel := strings.TrimPrefix(rel+"/"+it.Name, "/")
			if it.Folder != nil {
				if err := o.walk(ctx, root, childRel, out); err != nil {
					return err
				}
				continue
			}
			hash := ""
			if it.File != nil {
				hash = it.File.Hashes.QuickXorHash
			}
			*out = append(*out, RemoteFile{
				Path:     childRel,
				Size:     it.Size,
				Modified: it.LastModified,
				Hash:     hash,
				ID:       it.ID,
			})
		}
		endpoint = page.NextLink
	}
	return nil
}

func (o *OneDrive) Download(ctx context.Context, f RemoteFile) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		graphBase+"/me/drive/items/"+f.ID+"/content", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("onedrive download %s: HTTP %d", f.Path, resp.StatusCode)
	}
	return resp.Body, nil
}

func (o *OneDrive) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		time.Sleep(2 * time.Second)
		return o.getJSON(ctx, url, v)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
