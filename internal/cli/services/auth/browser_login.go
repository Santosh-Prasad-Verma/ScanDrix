package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// BrowserLoginService handles the OAuth browser-based login flow for the ScanDrix CLI.
type BrowserLoginService struct {
	baseURL      string
	openBrowser  func(string) error
	tokenStorage string
	mu           sync.Mutex
}

// CredentialsFile models the stored CLI auth session.
type CredentialsFile struct {
	Token     string    `json:"token"`
	Email     string    `json:"email,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// NewBrowserLoginService creates a new BrowserLoginService.
func NewBrowserLoginService(baseURL ...string) *BrowserLoginService {
	url := "https://scandrix.dev"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	}

	home, _ := os.UserHomeDir()
	credPath := filepath.Join(home, ".scandrix", "credentials.json")

	return &BrowserLoginService{
		baseURL:      url,
		openBrowser:  defaultOpenBrowser,
		tokenStorage: credPath,
	}
}

// SetTokenStorage overrides the credential storage file path (useful for testing).
func (s *BrowserLoginService) SetTokenStorage(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenStorage = path
}

// SetOpenBrowser overrides the browser launcher function (useful for testing).
func (s *BrowserLoginService) SetOpenBrowser(fn func(string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openBrowser = fn
}

// Login initiates the browser flow and blocks until the callback is received or context cancelled.
func (s *BrowserLoginService) Login(ctx context.Context) (*CredentialsFile, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generate random state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind loopback port: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	authURL := fmt.Sprintf("%s/cli/login?port=%d&state=%s", s.baseURL, port, state)

	tokenChan := make(chan *CredentialsFile, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		receivedState := query.Get("state")
		token := query.Get("token")
		email := query.Get("email")

		if receivedState != state {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			errChan <- fmt.Errorf("state mismatch in OAuth callback")
			return
		}

		if token == "" {
			http.Error(w, "Missing authentication token", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing token in callback")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:50px;">
			<h2>ScanDrix CLI Authentication Successful</h2>
			<p>You can close this tab and return to your terminal.</p>
		</body></html>`)

		creds := &CredentialsFile{
			Token:     token,
			Email:     email,
			CreatedAt: time.Now().UTC(),
		}
		tokenChan <- creds
	})

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
	defer server.Shutdown(context.Background())

	// Launch browser
	if err := s.openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("launch browser at %s: %w", authURL, err)
	}

	select {
	case creds := <-tokenChan:
		if err := s.SaveCredentials(creds); err != nil {
			return nil, fmt.Errorf("save credentials: %w", err)
		}
		return creds, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SaveCredentials writes the credentials file to disk.
func (s *BrowserLoginService) SaveCredentials(creds *CredentialsFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.tokenStorage)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	if err := os.WriteFile(s.tokenStorage, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// LoadCredentials reads stored credentials from disk.
func (s *BrowserLoginService) LoadCredentials() (*CredentialsFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.tokenStorage)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds CredentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}
	return &creds, nil
}

// Logout removes stored credentials from disk.
func (s *BrowserLoginService) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.tokenStorage); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

func defaultOpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}
