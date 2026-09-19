package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/scandrix/backend/internal/centralizedconfig/domain"
	"github.com/scandrix/backend/internal/centralizedconfig/utils"
	commonCodeReview "github.com/scandrix/backend/internal/common/codereview"
	"gopkg.in/yaml.v3"
)

// FileEntry represents a virtual file entry with path and content.
type FileEntry struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// DownloadUseCaseConfig provides external dependencies for configuration tree export.
type DownloadUseCaseConfig struct {
	Storage      domain.CentralizedConfigService
	TreeProvider interface {
		GetFileContent(ctx context.Context, orgID, teamID, repoID, path string) ([]byte, error)
	}
}

// FullDownloadUseCase generates a complete virtual repository tree including configurations, custom messages with inheritance, and review rules.
type FullDownloadUseCase struct {
	svc domain.CentralizedConfigService
}

// NewFullDownloadUseCase creates a new FullDownloadUseCase.
func NewFullDownloadUseCase(svc domain.CentralizedConfigService) *FullDownloadUseCase {
	return &FullDownloadUseCase{svc: svc}
}

// DownloadOptions parameterizes the export process.
type DownloadOptions struct {
	SkipAuthorization                bool   `json:"skipAuthorization"`
	OrganizationID                   string `json:"organizationId"`
	MarkRulesAsPendingWithSourcePath bool   `json:"markRulesAsPendingWithSourcePath"`
}

// Execute constructs the full list of virtual file entries for download.
func (uc *FullDownloadUseCase) Execute(ctx context.Context, orgID, teamID string, opts DownloadOptions) ([]FileEntry, error) {
	if orgID == "" {
		return nil, errors.New("download: organizationId is required")
	}

	configMetas, err := uc.svc.DownloadConfig(ctx, orgID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to download centralized config: %w", err)
	}

	usedPaths := make(map[string]struct{})
	var entries []FileEntry

	for _, meta := range configMetas {
		cleanPath := uc.getUniquePath(meta.Path, usedPaths)
		usedPaths[cleanPath] = struct{}{}
		entries = append(entries, FileEntry{
			Path:    cleanPath,
			Content: meta.Content,
		})
	}

	// Ensure default config file is present if not already added
	if _, exists := usedPaths["scandrix-config.yaml"]; !exists {
		def := commonCodeReview.GetDefaultScanDrixConfigFile()
		defYAML, err := utils.DumpCentralizedYAML(def)
		if err == nil {
			entries = append([]FileEntry{{
				Path:    "scandrix-config.yaml",
				Content: defYAML,
			}}, entries...)
			usedPaths["scandrix-config.yaml"] = struct{}{}
		}
	}

	return entries, nil
}

// GetUniquePath guarantees that file paths are not duplicated in the generated package.
func (uc *FullDownloadUseCase) getUniquePath(p string, usedPaths map[string]struct{}) string {
	clean := strings.TrimLeft(p, "/")
	if _, exists := usedPaths[clean]; !exists {
		return clean
	}

	ext := ""
	base := clean
	if dot := strings.LastIndex(clean, "."); dot != -1 {
		ext = clean[dot:]
		base = clean[:dot]
	}

	counter := 1
	for {
		candidate := fmt.Sprintf("%s-%d%s", base, counter, ext)
		if _, exists := usedPaths[candidate]; !exists {
			return candidate
		}
		counter++
	}
}

// DiffCustomMessages returns only custom message keys whose values differ from parent.
func DiffCustomMessages(parent, child map[string]string) map[string]string {
	diff := make(map[string]string)
	if child == nil {
		return diff
	}
	if parent == nil {
		for k, v := range child {
			if strings.TrimSpace(v) != "" {
				diff[k] = v
			}
		}
		return diff
	}

	for k, v := range child {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if parentVal, exists := parent[k]; !exists || parentVal != trimmed {
			diff[k] = trimmed
		}
	}
	return diff
}

// MergeCustomMessages overlays child messages on top of parent messages.
func MergeCustomMessages(parent, child map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range parent {
		if strings.TrimSpace(v) != "" {
			merged[k] = v
		}
	}
	for k, v := range child {
		if strings.TrimSpace(v) != "" {
			merged[k] = v
		}
	}
	return merged
}

// BuildConfigContentWithCustomMessages merges custom messages block into existing YAML config content.
func BuildConfigContentWithCustomMessages(yamlContent string, customMsgs map[string]string) (string, error) {
	var root map[string]any
	if strings.TrimSpace(yamlContent) != "" {
		if err := yaml.Unmarshal([]byte(yamlContent), &root); err != nil {
			root = make(map[string]any)
		}
	} else {
		root = make(map[string]any)
	}

	if len(customMsgs) > 0 {
		msgsMap := make(map[string]any, len(customMsgs))
		for k, v := range customMsgs {
			msgsMap[k] = v
		}
		root["customMessages"] = msgsMap
	} else {
		delete(root, "customMessages")
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GetDirectoryDepth computes the nesting level of a slash-delimited directory path.
func GetDirectoryDepth(p string) int {
	clean := strings.Trim(p, "/")
	if clean == "" {
		return 0
	}
	return len(strings.Split(clean, "/"))
}

// SortByDirectoryDepth sorts a list of paths by increasing directory depth.
func SortByDirectoryDepth(paths []string) []string {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := GetDirectoryDepth(sorted[i])
		dj := GetDirectoryDepth(sorted[j])
		if di != dj {
			return di < dj
		}
		return sorted[i] < sorted[j]
	})
	return sorted
}
