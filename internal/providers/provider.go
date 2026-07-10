package providers

import (
	"context"
	"io"
	"time"
)

// RemoteFile is a normalized view of a file on any cloud provider.
type RemoteFile struct {
	// Path is the file path relative to the synced remote folder, using '/' separators.
	Path     string
	Size     int64
	Modified time.Time
	// Hash is a provider-native content hash when available (may be empty).
	Hash string
	// ID is the provider-native identifier used for downloading.
	ID string
	IsDir bool
}

// Provider lists and downloads files from one cloud service.
type Provider interface {
	// Name returns the provider key (onedrive, gdrive, dropbox).
	Name() string
	// List recursively enumerates all files under remotePath.
	List(ctx context.Context, remotePath string) ([]RemoteFile, error)
	// Download opens a reader for the given file.
	Download(ctx context.Context, f RemoteFile) (io.ReadCloser, error)
}
