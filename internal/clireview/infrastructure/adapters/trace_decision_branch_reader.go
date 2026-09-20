package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// TraceBranch is the git orphan branch storing trace records.
const TraceBranch = "scandrix/trace/v1"

// TraceRecordPath computes the deterministic storage path for a branch's trace records.
func TraceRecordPath(branch string) string {
	h := sha256.Sum256([]byte(branch))
	hashStr := hex.EncodeToString(h[:])
	return fmt.Sprintf("records/%s/%s.json", hashStr[:2], hashStr)
}

// IRepositoryContentReader abstracts fetching raw file content from code hosts.
type IRepositoryContentReader interface {
	GetRepositoryContentFile(ctx context.Context, orgID, teamID, repoID, filename, branch string) (string, error)
}

// TraceDecisionBranchReaderService reads branch-specific decision records.
type TraceDecisionBranchReaderService struct {
	reader IRepositoryContentReader
}

// NewTraceDecisionBranchReaderService creates a new reader service.
func NewTraceDecisionBranchReaderService(reader IRepositoryContentReader) *TraceDecisionBranchReaderService {
	return &TraceDecisionBranchReaderService{
		reader: reader,
	}
}

// Read loads and decodes the decision record for the specified repository branch.
func (s *TraceDecisionBranchReaderService) Read(ctx context.Context, input domain.ReadTraceDecisionBranchInput) (*domain.TraceDecisionBranchRecord, error) {
	branch := strings.TrimPrefix(strings.TrimSpace(input.Branch), "refs/heads/")
	if input.OrganizationID == "" || input.TeamID == "" || input.RepositoryID == "" || input.RepositoryName == "" || branch == "" {
		return nil, nil
	}

	if s.reader == nil {
		return nil, nil
	}

	filename := TraceRecordPath(branch)
	raw, err := s.reader.GetRepositoryContentFile(ctx, input.OrganizationID, input.TeamID, input.RepositoryID, filename, TraceBranch)
	if err != nil || raw == "" {
		return nil, nil
	}

	content := decodeBranchContent(raw)
	if content == "" {
		return nil, nil
	}

	var parsed domain.TraceDecisionBranchRecord
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, nil
	}

	if parsed.Version != 1 || parsed.Branch != branch {
		return nil, nil
	}

	return &parsed, nil
}

func decodeBranchContent(raw string) string {
	rawTrim := strings.TrimSpace(raw)
	if decoded, err := base64.StdEncoding.DecodeString(rawTrim); err == nil && len(decoded) > 0 {
		return string(decoded)
	}
	return rawTrim
}
