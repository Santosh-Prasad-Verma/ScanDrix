package codeowners

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// StandardCODEOWNERSLocations defines the lookup order for CODEOWNERS files across GitHub & GitLab.
var StandardCODEOWNERSLocations = []string{
	".github/CODEOWNERS",
	"CODEOWNERS",
	"docs/CODEOWNERS",
	".gitlab/CODEOWNERS",
}

// SCMContentFetcher abstracts raw file content retrieval from any SCM provider.
type SCMContentFetcher interface {
	GetFileContent(ctx context.Context, repo, ref, path string) ([]byte, error)
}

var ErrCODEOWNERSNotFound = errors.New("CODEOWNERS file not found in repository")

// Fetch searches standard repository locations in order to retrieve the active CODEOWNERS file.
// Returns the file content, the path where it was found, or ErrCODEOWNERSNotFound.
func Fetch(ctx context.Context, fetcher SCMContentFetcher, repo, ref string) (content string, location string, err error) {
	if fetcher == nil {
		return "", "", errors.New("SCM content fetcher cannot be nil")
	}

	for _, loc := range StandardCODEOWNERSLocations {
		data, err := fetcher.GetFileContent(ctx, repo, ref, loc)
		if err == nil && len(data) > 0 {
			slog.Debug("Found CODEOWNERS", "repo", repo, "ref", ref, "location", loc)
			return string(data), loc, nil
		}
	}

	return "", "", fmt.Errorf("%w for %s@%s", ErrCODEOWNERSNotFound, repo, ref)
}

// AutoAssignFromSCM fetches CODEOWNERS from the SCM repository and computes required reviewers.
func AutoAssignFromSCM(ctx context.Context, fetcher SCMContentFetcher, repo, ref string, changedFiles []string, prAuthor string) (*ReviewerAssignment, error) {
	content, _, err := Fetch(ctx, fetcher, repo, ref)
	if err != nil {
		if errors.Is(err, ErrCODEOWNERSNotFound) {
			// No CODEOWNERS file present; return empty assignment without error
			return &ReviewerAssignment{
				Reviewers: []string{},
				FileMap:   make(map[string][]string),
			}, nil
		}
		return nil, err
	}

	assigner := NewReviewerAssigner(content)
	assignment := assigner.AssignReviewers(ctx, changedFiles, prAuthor)
	return assignment, nil
}
