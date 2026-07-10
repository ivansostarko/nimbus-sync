package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/config"
)

const redirectURL = "http://localhost:8676/callback"

// Endpoints and scopes per provider.
func oauthConfig(cfg *config.Config, provider string) (*oauth2.Config, error) {
	app, err := cfg.App(provider)
	if err != nil {
		return nil, err
	}
	if app.ClientID == "" {
		return nil, fmt.Errorf("no client_id configured for %s — see README section 'Creating API credentials'", provider)
	}

	oc := &oauth2.Config{
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
		RedirectURL:  redirectURL,
	}

	switch provider {
	case "onedrive":
		oc.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		}
		oc.Scopes = []string{"Files.Read.All", "offline_access", "User.Read"}
	case "gdrive":
		oc.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		}
		oc.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"}
	case "dropbox":
		oc.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://www.dropbox.com/oauth2/authorize",
			TokenURL: "https://api.dropboxapi.com/oauth2/token",
		}
		// Dropbox uses token_access_type=offline for refresh tokens (added below).
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
	return oc, nil
}

func tokenPath(cfg *config.Config, provider string) string {
	return filepath.Join(cfg.TokenDir, provider+".json")
}

// Authorize runs the browser-based OAuth flow and stores the token on disk.
func Authorize(cfg *config.Config, provider string) error {
	oc, err := oauthConfig(cfg, provider)
	if err != nil {
		return err
	}

	state := fmt.Sprintf("nimbus-%d", time.Now().UnixNano())
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline}
	if provider == "dropbox" {
		opts = append(opts, oauth2.SetAuthURLParam("token_access_type", "offline"))
	}
	if provider == "gdrive" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "consent"))
	}
	url := oc.AuthCodeURL(state, opts...)

	fmt.Println("Open this URL in your browser and approve access:")
	fmt.Println()
	fmt.Println("  " + url)
	fmt.Println()
	fmt.Println("Waiting for callback on " + redirectURL + " ...")

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Addr: ":8676"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth state mismatch")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			errCh <- fmt.Errorf("no authorization code in callback")
			return
		}
		fmt.Fprintln(w, "Nimbus Sync: authorization complete. You can close this tab.")
		codeCh <- code
	})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for OAuth callback")
	}
	_ = srv.Shutdown(context.Background())

	tok, err := oc.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	return saveToken(tokenPath(cfg, provider), tok)
}

// Client returns an authenticated *http.Client for the provider,
// refreshing and re-persisting tokens automatically.
func Client(ctx context.Context, cfg *config.Config, provider string) (*http.Client, error) {
	oc, err := oauthConfig(cfg, provider)
	if err != nil {
		return nil, err
	}
	tok, err := loadToken(tokenPath(cfg, provider))
	if err != nil {
		return nil, fmt.Errorf("not authorized — run: nimbus auth %s (%w)", provider, err)
	}
	src := oc.TokenSource(ctx, tok)
	// Wrap to persist refreshed tokens.
	persisting := &persistingSource{src: src, path: tokenPath(cfg, provider), last: tok}
	return oauth2.NewClient(ctx, persisting), nil
}

type persistingSource struct {
	src  oauth2.TokenSource
	path string
	last *oauth2.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.last.AccessToken {
		p.last = tok
		_ = saveToken(p.path, tok)
	}
	return tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}
