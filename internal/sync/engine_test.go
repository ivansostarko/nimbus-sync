package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/providers"
)

func TestExcluded(t *testing.T) {
	cases := []struct {
		rel      string
		patterns []string
		want     bool
	}{
		{"notes.tmp", []string{"*.tmp"}, true},
		{"docs/notes.tmp", []string{"*.tmp"}, true}, // matches basename
		{"docs/report.pdf", []string{"*.tmp"}, false},
		{"~$draft.docx", []string{"~$*"}, true},
		{".DS_Store", []string{".DS_Store"}, true},
		{"keep.txt", nil, false},
	}
	for _, c := range cases {
		if got := excluded(c.rel, c.patterns); got != c.want {
			t.Errorf("excluded(%q, %v) = %v, want %v", c.rel, c.patterns, got, c.want)
		}
	}
}

func TestNeedsDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	mod := time.Now().Add(-time.Hour).Truncate(time.Second)

	// Missing file → download.
	if !needsDownload(path, providers.RemoteFile{Size: 5, Modified: mod}) {
		t.Error("missing local file should need download")
	}

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}

	// Same size + mtime → no download.
	if needsDownload(path, providers.RemoteFile{Size: 5, Modified: mod}) {
		t.Error("identical file should not need download")
	}
	// Different size → download.
	if !needsDownload(path, providers.RemoteFile{Size: 9, Modified: mod}) {
		t.Error("size change should need download")
	}
	// Remote modified later → download.
	if !needsDownload(path, providers.RemoteFile{Size: 5, Modified: mod.Add(time.Minute)}) {
		t.Error("newer remote mtime should need download")
	}
	// Zero remote mtime with same size → no download.
	if needsDownload(path, providers.RemoteFile{Size: 5}) {
		t.Error("zero mtime with matching size should not need download")
	}
}

func TestExpandPath(t *testing.T) {
	got, err := expandPath("~/x")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if got != filepath.Join(home, "x") {
		t.Errorf("expandPath(~/x) = %q", got)
	}
}
