package sync

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/auth"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/config"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/providers"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/report"
)

// Engine mirrors cloud folders to local directories.
type Engine struct {
	cfg    *config.Config
	dryRun bool
}

func NewEngine(cfg *config.Config, dryRun bool) *Engine {
	return &Engine{cfg: cfg, dryRun: dryRun}
}

// Run syncs all enabled remotes (or just `only` if non-empty) and
// returns an aggregated report.
func (e *Engine) Run(only string) (*report.Report, error) {
	ctx := context.Background()
	rep := report.New(e.dryRun)

	matched := false
	for _, r := range e.cfg.Remotes {
		if only != "" && r.Name != only {
			continue
		}
		matched = true
		if !r.Enabled && only == "" {
			rep.SkipRemote(r.Name, "disabled")
			continue
		}
		rr := rep.StartRemote(r.Name, r.Provider, r.RemotePath, r.LocalPath)
		if err := e.syncRemote(ctx, r, rr); err != nil {
			rr.Fail(err)
			log.Printf("[%s] ERROR: %v", r.Name, err)
			continue
		}
		rr.Finish()
	}
	if only != "" && !matched {
		return nil, fmt.Errorf("no remote named %q in config", only)
	}
	rep.Finish()
	return rep, nil
}

func (e *Engine) provider(ctx context.Context, key string) (providers.Provider, error) {
	client, err := auth.Client(ctx, e.cfg, key)
	if err != nil {
		return nil, err
	}
	switch key {
	case "onedrive":
		return &providers.OneDrive{HTTP: client}, nil
	case "gdrive":
		return &providers.GDrive{HTTP: client}, nil
	case "dropbox":
		return &providers.Dropbox{HTTP: client}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", key)
}

func (e *Engine) syncRemote(ctx context.Context, r config.Remote, rr *report.RemoteResult) error {
	prov, err := e.provider(ctx, r.Provider)
	if err != nil {
		return err
	}

	e.logf("[%s] listing %s:%s ...", r.Name, r.Provider, r.RemotePath)
	files, err := prov.List(ctx, r.RemotePath)
	if err != nil {
		return err
	}
	e.logf("[%s] %d remote files found", r.Name, len(files))

	local, err := expandPath(r.LocalPath)
	if err != nil {
		return err
	}
	if !e.dryRun {
		if err := os.MkdirAll(local, 0o755); err != nil {
			return err
		}
	}

	// Decide what to download.
	var todo []providers.RemoteFile
	remoteSet := map[string]bool{}
	for _, f := range files {
		if excluded(f.Path, r.Exclude) {
			rr.Excluded++
			continue
		}
		remoteSet[f.Path] = true
		dst := filepath.Join(local, filepath.FromSlash(f.Path))
		if needsDownload(dst, f) {
			todo = append(todo, f)
		} else {
			rr.Unchanged++
		}
	}

	// Download with a worker pool.
	sem := make(chan struct{}, e.cfg.Concurrency)
	var wg gosync.WaitGroup
	var mu gosync.Mutex
	var firstErr error

	for _, f := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(f providers.RemoteFile) {
			defer wg.Done()
			defer func() { <-sem }()
			dst := filepath.Join(local, filepath.FromSlash(f.Path))
			if e.dryRun {
				e.logf("[%s] would download %s (%s)", r.Name, f.Path, humanSize(f.Size))
				mu.Lock()
				rr.AddDownloaded(f.Path, f.Size)
				mu.Unlock()
				return
			}
			if err := e.download(ctx, prov, f, dst); err != nil {
				mu.Lock()
				rr.AddError(f.Path, err)
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				log.Printf("[%s] failed %s: %v", r.Name, f.Path, err)
				return
			}
			mu.Lock()
			rr.AddDownloaded(f.Path, f.Size)
			mu.Unlock()
			e.logf("[%s] downloaded %s (%s)", r.Name, f.Path, humanSize(f.Size))
		}(f)
	}
	wg.Wait()

	// Optional mirror deletion of local files that vanished remotely.
	if r.DeleteLocal {
		if err := e.pruneLocal(local, remoteSet, r, rr); err != nil {
			return err
		}
	}

	// Report partial failures without aborting other remotes.
	if len(rr.Errors) > 0 {
		return fmt.Errorf("%d file(s) failed; first error: %v", len(rr.Errors), firstErr)
	}
	return nil
}

func (e *Engine) download(ctx context.Context, prov providers.Provider, f providers.RemoteFile, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := prov.Download(ctx, f)
	if err != nil {
		return err
	}
	defer rc.Close()

	tmp := dst + ".nimbus-tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	// Preserve remote modification time so future runs can compare cheaply.
	if !f.Modified.IsZero() {
		_ = os.Chtimes(dst, f.Modified, f.Modified)
	}
	return nil
}

func (e *Engine) pruneLocal(local string, remoteSet map[string]bool, r config.Remote, rr *report.RemoteResult) error {
	return filepath.Walk(local, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(local, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".nimbus-tmp") {
			return os.Remove(path)
		}
		if excluded(rel, r.Exclude) {
			return nil
		}
		if !remoteSet[rel] {
			if e.dryRun {
				e.logf("[%s] would delete local %s", r.Name, rel)
			} else if err := os.Remove(path); err != nil {
				return err
			}
			rr.Deleted = append(rr.Deleted, rel)
		}
		return nil
	})
}

// needsDownload compares size + mtime (with 2s tolerance for FAT-like
// filesystems). Content hashes are used only as metadata in reports since
// each provider uses a different algorithm.
func needsDownload(dst string, f providers.RemoteFile) bool {
	st, err := os.Stat(dst)
	if err != nil {
		return true // missing locally
	}
	if st.Size() != f.Size {
		return true
	}
	if f.Modified.IsZero() {
		return false
	}
	diff := st.ModTime().Sub(f.Modified)
	if diff < 0 {
		diff = -diff
	}
	return diff > 2*time.Second
}

func excluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(p, filepath.Base(rel)); ok {
			return true
		}
	}
	return false
}

func expandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (e *Engine) logf(format string, args ...any) {
	if e.cfg.Verbose {
		log.Printf(format, args...)
	}
}
