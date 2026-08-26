package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codehound/codehound/core/pkg/config"
)

const (
	DopplerDefaultBaseURL = "https://api.doppler.com/v3"
	DefaultCacheTTL       = 5 * time.Minute
)

// SecretProvider defines the contract for secrets management engines.
type SecretProvider interface {
	GetSecret(ctx context.Context, key string) (string, error)
	GetAllSecrets(ctx context.Context) (map[string]string, error)
}

// DopplerClient provides high-performance access to Doppler Secrets with in-memory caching and env fallback.
type DopplerClient struct {
	token      string
	project    string
	configName string
	baseURL    string
	httpClient *http.Client

	mu        sync.RWMutex
	cache     map[string]string
	cacheTime time.Time
	cacheTTL  time.Duration
}

// NewDopplerClient creates a new DopplerClient instance.
func NewDopplerClient(cfg *config.AppConfig) *DopplerClient {
	return &DopplerClient{
		token:      cfg.DopplerToken,
		project:    cfg.DopplerProject,
		configName: cfg.DopplerConfig,
		baseURL:    DopplerDefaultBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]string),
		cacheTTL:   DefaultCacheTTL,
	}
}

// NewCustomDopplerClient allows custom baseURL (useful for mock testing).
func NewCustomDopplerClient(token, project, configName, baseURL string, ttl time.Duration) *DopplerClient {
	if baseURL == "" {
		baseURL = DopplerDefaultBaseURL
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &DopplerClient{
		token:      token,
		project:    project,
		configName: configName,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]string),
		cacheTTL:   ttl,
	}
}

// GetSecret fetches a single secret from Doppler or fallback environment variables.
func (d *DopplerClient) GetSecret(ctx context.Context, key string) (string, error) {
	// 1. Check in-memory cache
	d.mu.RLock()
	if val, ok := d.cache[key]; ok && time.Since(d.cacheTime) < d.cacheTTL {
		d.mu.RUnlock()
		return val, nil
	}
	d.mu.RUnlock()

	// 2. If no token, fallback to local environment variables
	if d.token == "" {
		if val := os.Getenv(key); val != "" {
			return val, nil
		}
		return "", fmt.Errorf("secret '%s' not found (no Doppler token and not in environment)", key)
	}

	// 3. Fetch from Doppler REST API
	reqURL := fmt.Sprintf("%s/configs/config/secret?project=%s&config=%s&name=%s",
		d.baseURL, d.project, d.configName, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create doppler request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		// Fallback to local environment if Doppler request fails
		if val := os.Getenv(key); val != "" {
			return val, nil
		}
		return "", fmt.Errorf("doppler request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if val := os.Getenv(key); val != "" {
			return val, nil
		}
		return "", fmt.Errorf("secret '%s' not found in doppler project '%s'", key, d.project)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("doppler api error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Name  string `json:"name"`
		Value struct {
			Raw      string `json:"raw"`
			Computed string `json:"computed"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode doppler response: %w", err)
	}

	secretVal := result.Value.Computed
	if secretVal == "" {
		secretVal = result.Value.Raw
	}

	// Cache value
	d.mu.Lock()
	d.cache[key] = secretVal
	d.cacheTime = time.Now()
	d.mu.Unlock()

	return secretVal, nil
}

// GetAllSecrets retrieves all secrets for the project & config in bulk.
func (d *DopplerClient) GetAllSecrets(ctx context.Context) (map[string]string, error) {
	d.mu.RLock()
	if len(d.cache) > 0 && time.Since(d.cacheTime) < d.cacheTTL {
		cachedCopy := make(map[string]string, len(d.cache))
		for k, v := range d.cache {
			cachedCopy[k] = v
		}
		d.mu.RUnlock()
		return cachedCopy, nil
	}
	d.mu.RUnlock()

	if d.token == "" {
		// Return environment variables
		envSecrets := make(map[string]string)
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				envSecrets[parts[0]] = parts[1]
			}
		}
		return envSecrets, nil
	}

	reqURL := fmt.Sprintf("%s/configs/config/secrets?project=%s&config=%s",
		d.baseURL, d.project, d.configName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create doppler secrets request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doppler request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("doppler api error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Secrets map[string]struct {
			Raw      string `json:"raw"`
			Computed string `json:"computed"`
		} `json:"secrets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode doppler response: %w", err)
	}

	secretsMap := make(map[string]string, len(result.Secrets))
	for k, v := range result.Secrets {
		val := v.Computed
		if val == "" {
			val = v.Raw
		}
		secretsMap[k] = val
	}

	d.mu.Lock()
	d.cache = secretsMap
	d.cacheTime = time.Now()
	d.mu.Unlock()

	return secretsMap, nil
}
