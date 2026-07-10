package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Remote describes one cloud folder to mirror locally.
type Remote struct {
	Name       string `yaml:"name"`
	Provider   string `yaml:"provider"` // onedrive | gdrive | dropbox
	RemotePath string `yaml:"remote_path"`
	LocalPath  string `yaml:"local_path"`
	Enabled    bool   `yaml:"enabled"`
	// DeleteLocal removes local files that no longer exist remotely (true mirror).
	DeleteLocal bool `yaml:"delete_local"`
	// Exclude is a list of glob patterns skipped during sync.
	Exclude []string `yaml:"exclude"`
}

// OAuthApp holds client credentials for a provider.
type OAuthApp struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type Config struct {
	// Schedule is a cron expression used by `nimbus daemon` and systemd docs.
	Schedule string `yaml:"schedule"`
	// Concurrency is the number of parallel downloads per remote.
	Concurrency int  `yaml:"concurrency"`
	Verbose     bool `yaml:"verbose"`

	Providers struct {
		OneDrive OAuthApp `yaml:"onedrive"`
		GDrive   OAuthApp `yaml:"gdrive"`
		Dropbox  OAuthApp `yaml:"dropbox"`
	} `yaml:"providers"`

	Remotes []Remote `yaml:"remotes"`

	// Derived paths (not from YAML)
	Dir       string `yaml:"-"` // config directory
	TokenDir  string `yaml:"-"`
	ReportDir string `yaml:"-"`
}

// DefaultPath returns ~/.config/nimbus-sync/config.yaml
func DefaultPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "nimbus-sync", "config.yaml")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s (run the installer or copy configs/config.example.yaml): %w", path, err)
	}
	cfg := &Config{Concurrency: 4, Schedule: "0 */6 * * *"}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}
	cfg.Dir = filepath.Dir(path)
	cfg.TokenDir = filepath.Join(cfg.Dir, "tokens")
	cfg.ReportDir = filepath.Join(cfg.Dir, "reports")
	for _, d := range []string{cfg.TokenDir, cfg.ReportDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	seen := map[string]bool{}
	for i, r := range c.Remotes {
		if r.Name == "" {
			return fmt.Errorf("remotes[%d]: name is required", i)
		}
		if seen[r.Name] {
			return fmt.Errorf("duplicate remote name %q", r.Name)
		}
		seen[r.Name] = true
		switch r.Provider {
		case "onedrive", "gdrive", "dropbox":
		default:
			return fmt.Errorf("remote %q: unknown provider %q", r.Name, r.Provider)
		}
		if r.LocalPath == "" {
			return fmt.Errorf("remote %q: local_path is required", r.Name)
		}
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	return nil
}

// App returns OAuth credentials for the given provider key.
func (c *Config) App(provider string) (OAuthApp, error) {
	switch provider {
	case "onedrive":
		return c.Providers.OneDrive, nil
	case "gdrive":
		return c.Providers.GDrive, nil
	case "dropbox":
		return c.Providers.Dropbox, nil
	}
	return OAuthApp{}, fmt.Errorf("unknown provider %q", provider)
}
