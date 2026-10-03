package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/scandrix/backend/internal/netguard"
)

// SystemController provides system introspection, build information, and cluster status.
// SystemStatusRepository is the subset of the repository needed to report
// real dependency health. The previous implementation had no dependencies at
// all and therefore could not check anything it claimed.
type SystemStatusRepository interface {
	Ping(ctx context.Context) error
}

type SystemController struct {
	repo     SystemStatusRepository
	bootTime time.Time
	version  string
}

// NewSystemController creates the system controller.
func NewSystemController(repo SystemStatusRepository) *SystemController {
	version := os.Getenv("RELEASE_VERSION")
	if version == "" {
		version = "dev"
	}
	return &SystemController{
		repo:     repo,
		bootTime: time.Now(),
		version:  version,
	}
}

// Routes mounts the /system endpoints.
func (c *SystemController) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/info", c.handleGetInfo)
	r.Get("/version", c.handleGetVersion)
	r.Get("/version-status", c.handleGetVersionStatus)
	r.Get("/status", c.handleGetStatus)

	return r
}

// VersionStatus matches the wire contract defined in apps/api/src/services/version-check.service.ts
type VersionStatus struct {
	Unknown         bool   `json:"unknown,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Current         string `json:"current,omitempty"`
	Latest          string `json:"latest,omitempty"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable,omitempty"`
	Severity        string `json:"severity,omitempty"`
}

type versionCacheEntry struct {
	latest     string
	releaseURL string
	cachedAt   time.Time
}

var (
	versionCacheMu    sync.RWMutex
	versionCacheStore *versionCacheEntry
	semverRegex       = regexp.MustCompile(`^(?:selfhosted-|v)?(\d+)\.(\d+)\.(\d+)$`)
	tagRegex          = regexp.MustCompile(`^selfhosted-(\d+)\.(\d+)\.(\d+)$`)
)

func (c *SystemController) handleGetVersionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Cloud mode check
	cloudMode := strings.EqualFold(os.Getenv("SCANDRIX_CLOUD_MODE"), "true") ||
		strings.EqualFold(os.Getenv("API_CLOUD_MODE"), "true")
	if cloudMode {
		_ = json.NewEncoder(w).Encode(VersionStatus{
			Unknown: true,
			Reason:  "cloud",
		})
		return
	}

	// 2. Current version from env
	current := strings.TrimSpace(os.Getenv("RELEASE_VERSION"))
	if current == "" {
		current = strings.TrimSpace(os.Getenv("SCANDRIX_VERSION"))
	}
	currentTuple := parseSemver(current)
	if currentTuple == nil {
		_ = json.NewEncoder(w).Encode(VersionStatus{
			Unknown: true,
			Reason:  "no-version",
			Current: current,
		})
		return
	}

	// 3. Fetch latest release from GitHub API (or memory cache)
	latest, releaseURL, err := fetchLatestRelease(r.Context())
	if err != nil || latest == "" {
		_ = json.NewEncoder(w).Encode(VersionStatus{
			Unknown: true,
			Reason:  "fetch-failed",
			Current: current,
		})
		return
	}

	latestTuple := parseSemver(latest)
	if latestTuple == nil {
		_ = json.NewEncoder(w).Encode(VersionStatus{
			Unknown: true,
			Reason:  "fetch-failed",
			Current: current,
		})
		return
	}

	updateAvailable := compareSemver(latestTuple, currentTuple) > 0
	severity := "info"
	if updateAvailable && latestTuple[0] > currentTuple[0] {
		severity = "major"
	}

	_ = json.NewEncoder(w).Encode(VersionStatus{
		Current:         current,
		Latest:          latest,
		ReleaseURL:      releaseURL,
		UpdateAvailable: updateAvailable,
		Severity:        severity,
	})
}

func fetchLatestRelease(ctx context.Context) (latest, releaseURL string, err error) {
	now := time.Now()

	versionCacheMu.RLock()
	if versionCacheStore != nil && now.Sub(versionCacheStore.cachedAt) < 24*time.Hour {
		latest = versionCacheStore.latest
		releaseURL = versionCacheStore.releaseURL
		versionCacheMu.RUnlock()
		return latest, releaseURL, nil
	}
	versionCacheMu.RUnlock()

	repo := os.Getenv("SCANDRIX_VERSION_REPO")
	if repo == "" {
		repo = os.Getenv("VERSION_CHECK_REPO")
	}
	if repo == "" {
		repo = "scandrix/scandrix"
	}

	// The host is fixed, but `repo` is interpolated into the path from the
	// environment. Validating the whole URL means the request cannot be aimed
	// elsewhere even if the format changes later, and it rejects a repo value
	// carrying path traversal or query injection.
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", repo)
	parsed, err := netguard.Validate(apiURL, netguard.Options{AllowedHosts: []string{"api.github.com"}})
	if err != nil {
		return "", "", fmt.Errorf("refusing update check URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil) // #nosec G704 -- apiURL is validated by netguard.Validate with an api.github.com host allowlist
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "scandrix-self-hosted-update-check")

	client := netguard.NewClient(5, netguard.Options{AllowedHosts: []string{"api.github.com"}})
	resp, err := client.Do(req) // #nosec G704 -- client is netguard.NewClient, which revalidates every redirect hop
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var releases []struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", "", err
	}

	type candidate struct {
		tuple [3]int
		tag   string
		url   string
	}
	var candidates []candidate
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		if tagRegex.MatchString(r.TagName) || semverRegex.MatchString(r.TagName) {
			t := parseSemver(r.TagName)
			if t != nil {
				candidates = append(candidates, candidate{
					tuple: *t,
					tag:   r.TagName,
					url:   r.HTMLURL,
				})
			}
		}
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no matching releases found")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return compareSemver(&candidates[i].tuple, &candidates[j].tuple) > 0
	})

	top := candidates[0]
	cleanLatest := strings.TrimPrefix(strings.TrimPrefix(top.tag, "selfhosted-"), "v")

	versionCacheMu.Lock()
	versionCacheStore = &versionCacheEntry{
		latest:     cleanLatest,
		releaseURL: top.url,
		cachedAt:   now,
	}
	versionCacheMu.Unlock()

	return cleanLatest, top.url, nil
}

func parseSemver(input string) *[3]int {
	m := semverRegex.FindStringSubmatch(strings.TrimSpace(input))
	if len(m) < 4 {
		return nil
	}
	var tuple [3]int
	for i := 0; i < 3; i++ {
		val, err := strconv.Atoi(m[i+1])
		if err != nil {
			return nil
		}
		tuple[i] = val
	}
	return &tuple
}

func compareSemver(a, b *[3]int) int {
	if a[0] != b[0] {
		return a[0] - b[0]
	}
	if a[1] != b[1] {
		return a[1] - b[1]
	}
	return a[2] - b[2]
}

// resolveEnvironment reports the deployment environment, or "unknown" when it
// is not configured. It previously defaulted to "production", so any dev,
// staging or QA instance without SCANDRIX_ENV publicly announced itself as
// production, which misdirects support triage and incident response
// (AUDIT_REMEDIATION.md F-42).
func resolveEnvironment() string {
	for _, key := range []string{"SCANDRIX_ENV", "ENVIRONMENT", "APP_ENV"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "unknown"
}

// resolveBuildSHA reports the build commit, or "unknown" when it is not set.
func resolveBuildSHA() string {
	if v := strings.TrimSpace(os.Getenv("SCANDRIX_BUILD_SHA")); v != "" {
		return v
	}
	return "unknown"
}

func (c *SystemController) handleGetInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"service":     "scandrix-api",
		"version":     c.version,
		"commit":      resolveBuildSHA(),
		"environment": resolveEnvironment(),
		"goVersion":   runtime.Version(),
		"numCPU":      runtime.NumCPU(),
		"arch":        runtime.GOARCH,
		"os":          runtime.GOOS,
		"serverTime":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (c *SystemController) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"version": c.version,
		"commit":  resolveBuildSHA(),
	})
}

// handleGetStatus reports observed dependency health. Every field is measured;
// nothing here is a constant. A failed probe returns 503 so load balancers and
// uptime monitors see the outage instead of a green dashboard.
func (c *SystemController) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	database := "not_configured"
	code := http.StatusOK
	if c.repo != nil {
		if err := c.repo.Ping(ctx); err != nil {
			database = "down"
			code = http.StatusServiceUnavailable
		} else {
			database = "up"
		}
	}

	overall := "healthy"
	if code != http.StatusOK {
		overall = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":        overall,
		"service":       "scandrix-api",
		"version":       c.version,
		"database":      database,
		"uptimeSeconds": int(time.Since(c.bootTime).Seconds()),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}
