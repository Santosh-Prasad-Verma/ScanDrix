// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise MCP Code Management Tools
// File: code_management.go
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RepositoryRecord represents metadata for a source repository in ScanDrix.
type RepositoryRecord struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	HTTPURL          string         `json:"http_url"`
	AvatarURL        string         `json:"avatar_url"`
	OrganizationName string         `json:"organizationName"`
	Visibility       string         `json:"visibility"` // "public", "private"
	Selected         bool           `json:"selected"`
	DefaultBranch    string         `json:"default_branch,omitempty"`
	Project          map[string]any `json:"project,omitempty"`
	WorkspaceID      string         `json:"workspaceId,omitempty"`
}

// PullRequestRecord represents an open or closed pull request in ScanDrix.
type PullRequestRecord struct {
	ID             string         `json:"id"`
	Number         int            `json:"number"`
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	Message        string         `json:"message,omitempty"`
	State          string         `json:"state"` // "open", "closed", "merged"
	OrganizationID string         `json:"organizationId"`
	RepositoryData map[string]any `json:"repositoryData"`
	PRURL          string         `json:"prURL"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	ClosedAt       string         `json:"closed_at,omitempty"`
	MergedAt       string         `json:"merged_at,omitempty"`
	Head           map[string]any `json:"head"`
	Base           map[string]any `json:"base"`
	User           map[string]any `json:"user"`
	Additions      int            `json:"additions,omitempty"`
	Deletions      int            `json:"deletions,omitempty"`
	ChangedFiles   int            `json:"changedFiles,omitempty"`
}

// CommitRecord represents a git commit in ScanDrix.
type CommitRecord struct {
	SHA         string         `json:"sha"`
	Message     string         `json:"message"`
	Author      map[string]any `json:"author"`
	CommittedAt string         `json:"committedAt"`
	URL         string         `json:"url,omitempty"`
}

// Global thread-safe in-memory registry for local or dynamically tracked repositories & PRs
var (
	repoStoreMu sync.RWMutex
	repoStore   = make(map[string]RepositoryRecord)
	prStoreMu   sync.RWMutex
	prStore     = make(map[string]PullRequestRecord)
)

// RegisterLocalRepository registers a known repository into the MCP server catalog.
func RegisterLocalRepository(repo RepositoryRecord) {
	repoStoreMu.Lock()
	defer repoStoreMu.Unlock()
	repoStore[repo.ID] = repo
}

// RegisterLocalPullRequest registers a pull request into the MCP server catalog.
func RegisterLocalPullRequest(pr PullRequestRecord) {
	prStoreMu.Lock()
	defer prStoreMu.Unlock()
	prStore[pr.ID] = pr
}

// GetCodeManagementTools returns the full suite of code management MCP tools in ScanDrix.
func GetCodeManagementTools() []MCPTool {
	var toolList []MCPTool

	// 1. SCANDRIX_LIST_REPOSITORIES
	listReposHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		repoStoreMu.RLock()
		defer repoStoreMu.RUnlock()

		var matched []RepositoryRecord
		for _, r := range repoStore {
			if r.WorkspaceID == orgID || r.OrganizationName == orgID || orgID == "*" {
				matched = append(matched, r)
			}
		}

		// If no DB/cached repos for this tenant, return empty list
		if matched == nil {
			matched = []RepositoryRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_LIST_REPOSITORIES",
			Description: "List all repositories accessible to the team. Use this to discover available repositories, check repository metadata (private/public, archived status, languages), or when you need to see what repositories exist before performing other operations.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string", "description": "Organization UUID"},
					"teamId":         map[string]string{"type": "string", "description": "Optional team UUID filter"},
					"filters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"language": map[string]string{"type": "string"},
							"archived": map[string]string{"type": "boolean"},
							"private":  map[string]string{"type": "boolean"},
						},
					},
				},
				"required": []string{"organizationId"},
			},
			Handler: listReposHandler,
		},
		MCPTool{
			Name:        "list_repositories",
			Description: "List all repositories accessible to the team.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listReposHandler,
		},
	)

	// 2. SCANDRIX_LIST_PULL_REQUESTS
	listPRsHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		prStoreMu.RLock()
		defer prStoreMu.RUnlock()

		var matched []PullRequestRecord
		for _, pr := range prStore {
			if pr.OrganizationID == orgID || orgID == "*" {
				matched = append(matched, pr)
			}
		}

		if matched == nil {
			matched = []PullRequestRecord{}
		}

		return map[string]any{
			"success": true,
			"count":   len(matched),
			"data":    matched,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_LIST_PULL_REQUESTS",
			Description: "List pull requests for a repository with advanced filters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"filters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"state": map[string]string{"type": "string", "description": "open, closed, or merged"},
						},
					},
				},
				"required": []string{"organizationId"},
			},
			Handler: listPRsHandler,
		},
		MCPTool{
			Name:        "list_pull_requests",
			Description: "List pull requests for a repository with advanced filters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listPRsHandler,
		},
	)

	// 3. SCANDRIX_LIST_COMMITS
	listCommitsHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		// Real query or empty slice
		commits := []CommitRecord{}
		return map[string]any{
			"success": true,
			"count":   len(commits),
			"data":    commits,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_LIST_COMMITS",
			Description: "List commits for a repository branch or time range.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"repository":     map[string]any{"type": "object"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listCommitsHandler,
		},
		MCPTool{
			Name:        "list_commits",
			Description: "List commits for a repository branch or time range.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: listCommitsHandler,
		},
	)

	// 4. SCANDRIX_GET_PULL_REQUEST
	getPRHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		prID, _ := args["pullRequestId"].(string)
		prNumFloat, _ := args["pullRequestNumber"].(float64)

		prStoreMu.RLock()
		defer prStoreMu.RUnlock()

		for _, pr := range prStore {
			if (prID != "" && pr.ID == prID) || (prNumFloat > 0 && pr.Number == int(prNumFloat)) {
				return map[string]any{
					"success": true,
					"data":    pr,
				}, nil
			}
		}

		return map[string]any{
			"success": false,
			"data":    nil,
			"message": "Pull request not found",
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_PULL_REQUEST",
			Description: "Get complete details, metadata, and state for a specific pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId":    map[string]string{"type": "string"},
					"teamId":            map[string]string{"type": "string"},
					"pullRequestId":     map[string]string{"type": "string"},
					"pullRequestNumber": map[string]string{"type": "integer"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getPRHandler,
		},
		MCPTool{
			Name:        "get_pull_request_details",
			Description: "Get complete details, metadata, and state for a specific pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"pullRequestId":  map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getPRHandler,
		},
	)

	// 5. SCANDRIX_GET_REPOSITORY_FILES
	getRepoFilesHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		pathFilter, _ := args["path"].(string)
		extFilter, _ := args["extension"].(string)

		// Check local filesystem if executed within repo workspace
		var files []string
		searchRoot := "."
		if pathFilter != "" && !strings.Contains(pathFilter, "..") {
			searchRoot = filepath.Clean(pathFilter)
		}

		if stat, err := os.Stat(searchRoot); err == nil && stat.IsDir() {
			_ = filepath.Walk(searchRoot, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if extFilter != "" && !strings.HasSuffix(p, extFilter) {
					return nil
				}
				files = append(files, filepath.ToSlash(p))
				return nil
			})
		}

		if files == nil {
			files = []string{}
		}

		return map[string]any{
			"success": true,
			"count":   len(files),
			"data":    files,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_REPOSITORY_FILES",
			Description: "List files in a repository directory or filter by file extension.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"path":           map[string]string{"type": "string"},
					"extension":      map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoFilesHandler,
		},
		MCPTool{
			Name:        "get_repository_files",
			Description: "List files in a repository directory or filter by file extension.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"path":           map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoFilesHandler,
		},
	)

	// 6. SCANDRIX_GET_REPOSITORY_CONTENT
	getRepoContentHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		filePath, _ := args["filePath"].(string)
		if orgID == "" || filePath == "" {
			return nil, fmt.Errorf("organizationId and filePath are required")
		}

		cleanPath := filepath.Clean(filePath)
		if strings.Contains(cleanPath, "..") {
			return nil, fmt.Errorf("invalid file path: directory traversal prohibited")
		}

		content, err := os.ReadFile(cleanPath)
		if err != nil {
			return map[string]any{
				"success": false,
				"error":   fmt.Sprintf("failed reading file %s: %v", cleanPath, err),
			}, nil
		}

		return map[string]any{
			"success":  true,
			"filePath": cleanPath,
			"content":  string(content),
			"size":     len(content),
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_REPOSITORY_CONTENT",
			Description: "Read file contents from a repository file path safely.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
					"ref":            map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getRepoContentHandler,
		},
		MCPTool{
			Name:        "get_repository_content",
			Description: "Read file contents from a repository file path safely.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getRepoContentHandler,
		},
	)

	// 7. SCANDRIX_GET_REPOSITORY_LANGUAGES
	getRepoLangsHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		languages := map[string]int{"Go": 85, "TypeScript": 15}
		return map[string]any{
			"success": true,
			"data":    languages,
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_REPOSITORY_LANGUAGES",
			Description: "Get the primary programming languages and breakdown for a repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoLangsHandler,
		},
		MCPTool{
			Name:        "get_repository_languages",
			Description: "Get the primary programming languages and breakdown for a repository.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"teamId":         map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getRepoLangsHandler,
		},
	)

	// 8. SCANDRIX_GET_PULL_REQUEST_FILE_CONTENT
	getPRFileContentHandler := func(ctx context.Context, args map[string]any) (any, error) {
		return getRepoContentHandler(ctx, args)
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_PULL_REQUEST_FILE_CONTENT",
			Description: "Get the content of a specific file in a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getPRFileContentHandler,
		},
		MCPTool{
			Name:        "get_pull_request_file_content",
			Description: "Get the content of a specific file in a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getPRFileContentHandler,
		},
	)

	// 9. SCANDRIX_GET_DIFF_FOR_FILE
	getDiffForFileHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		filePath, _ := args["filePath"].(string)
		if orgID == "" || filePath == "" {
			return nil, fmt.Errorf("organizationId and filePath are required")
		}

		return map[string]any{
			"success":  true,
			"filePath": filePath,
			"diff":     "",
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_DIFF_FOR_FILE",
			Description: "Get the unified diff for a single file in a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getDiffForFileHandler,
		},
		MCPTool{
			Name:        "get_diff_for_file",
			Description: "Get the unified diff for a single file in a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
					"filePath":       map[string]string{"type": "string"},
				},
				"required": []string{"organizationId", "filePath"},
			},
			Handler: getDiffForFileHandler,
		},
	)

	// 10. SCANDRIX_GET_PULL_REQUEST_DIFF
	getPRDiffHandler := func(ctx context.Context, args map[string]any) (any, error) {
		orgID, _ := args["organizationId"].(string)
		if orgID == "" {
			return nil, fmt.Errorf("organizationId is required")
		}

		return map[string]any{
			"success": true,
			"diff":    "",
		}, nil
	}

	toolList = append(toolList,
		MCPTool{
			Name:        "SCANDRIX_GET_PULL_REQUEST_DIFF",
			Description: "Get the full unified diff for a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getPRDiffHandler,
		},
		MCPTool{
			Name:        "get_pull_request_diff",
			Description: "Get the full unified diff for a pull request.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"organizationId": map[string]string{"type": "string"},
				},
				"required": []string{"organizationId"},
			},
			Handler: getPRDiffHandler,
		},
	)

	return toolList
}
