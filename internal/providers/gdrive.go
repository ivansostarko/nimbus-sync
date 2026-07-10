package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const driveBase = "https://www.googleapis.com/drive/v3"

// GDrive talks to the Google Drive v3 REST API directly (no SDK needed).
type GDrive struct {
	HTTP *http.Client
}

func (g *GDrive) Name() string { return "gdrive" }

type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	Size         string `json:"size"`
	MD5          string `json:"md5Checksum"`
	ModifiedTime string `json:"modifiedTime"`
}

type driveList struct {
	Files         []driveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken"`
}

const folderMime = "application/vnd.google-apps.folder"

// List resolves remotePath (e.g. "Projects/Reports") starting at the Drive
// root, then walks it recursively.
func (g *GDrive) List(ctx context.Context, remotePath string) ([]RemoteFile, error) {
	folderID := "root"
	if p := strings.Trim(remotePath, "/"); p != "" {
		for _, seg := range strings.Split(p, "/") {
			id, err := g.findFolder(ctx, folderID, seg)
			if err != nil {
				return nil, err
			}
			folderID = id
		}
	}
	var out []RemoteFile
	err := g.walk(ctx, folderID, "", &out)
	return out, err
}

func (g *GDrive) findFolder(ctx context.Context, parent, name string) (string, error) {
	q := fmt.Sprintf("'%s' in parents and name = '%s' and mimeType = '%s' and trashed = false",
		parent, strings.ReplaceAll(name, "'", "\\'"), folderMime)
	var res driveList
	u := driveBase + "/files?q=" + url.QueryEscape(q) + "&fields=files(id,name)"
	if err := g.getJSON(ctx, u, &res); err != nil {
		return "", err
	}
	if len(res.Files) == 0 {
		return "", fmt.Errorf("gdrive: folder %q not found under parent %s", name, parent)
	}
	return res.Files[0].ID, nil
}

func (g *GDrive) walk(ctx context.Context, folderID, rel string, out *[]RemoteFile) error {
	pageToken := ""
	for {
		q := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
		u := driveBase + "/files?q=" + url.QueryEscape(q) +
			"&fields=" + url.QueryEscape("nextPageToken,files(id,name,mimeType,size,md5Checksum,modifiedTime)") +
			"&pageSize=1000"
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var page driveList
		if err := g.getJSON(ctx, u, &page); err != nil {
			return err
		}
		for _, f := range page.Files {
			childRel := strings.TrimPrefix(rel+"/"+f.Name, "/")
			if f.MimeType == folderMime {
				if err := g.walk(ctx, f.ID, childRel, out); err != nil {
					return err
				}
				continue
			}
			// Skip Google-native docs (Docs/Sheets/Slides) — they have no
			// binary content; exporting them is a planned feature.
			if strings.HasPrefix(f.MimeType, "application/vnd.google-apps") {
				continue
			}
			size, _ := strconv.ParseInt(f.Size, 10, 64)
			mod, _ := time.Parse(time.RFC3339, f.ModifiedTime)
			*out = append(*out, RemoteFile{
				Path:     childRel,
				Size:     size,
				Modified: mod,
				Hash:     f.MD5,
				ID:       f.ID,
			})
		}
		if page.NextPageToken == "" {
			return nil
		}
		pageToken = page.NextPageToken
	}
}

func (g *GDrive) Download(ctx context.Context, f RemoteFile) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		driveBase+"/files/"+f.ID+"?alt=media", nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("gdrive download %s: HTTP %d", f.Path, resp.StatusCode)
	}
	return resp.Body, nil
}

func (g *GDrive) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		// Drive returns 403 for rate limits too; back off once.
		time.Sleep(2 * time.Second)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
