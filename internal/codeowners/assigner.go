package codeowners

import (
	"context"
	"sort"
	"strings"
)

// TeamResolver allows expanding organization team slugs (e.g., @org/backend) into individual member usernames.
type TeamResolver interface {
	ResolveTeamMembers(ctx context.Context, teamSlug string) ([]string, error)
}

// ReviewerAssigner determines which reviewers to request for a PR based on CODEOWNERS.
type ReviewerAssigner struct {
	codeowners   *File
	teamResolver TeamResolver
}

// NewReviewerAssigner creates an assigner from parsed CODEOWNERS content.
func NewReviewerAssigner(codeownersContent string, opts ...AssignerOption) *ReviewerAssigner {
	assigner := &ReviewerAssigner{
		codeowners: Parse(codeownersContent),
	}
	for _, opt := range opts {
		opt(assigner)
	}
	return assigner
}

// AssignerOption customizes reviewer assignment behavior.
type AssignerOption func(*ReviewerAssigner)

// WithTeamResolver sets a team expansion resolver.
func WithTeamResolver(resolver TeamResolver) AssignerOption {
	return func(a *ReviewerAssigner) {
		a.teamResolver = resolver
	}
}

// ReviewerAssignment represents the computed reviewer set for a PR.
type ReviewerAssignment struct {
	Reviewers []string            `json:"reviewers"` // Deduplicated, sorted list of @handles or emails
	FileMap   map[string][]string `json:"file_map"`  // file → owners mapping for audit trail
}

// AssignReviewers computes the reviewer set for a list of changed files in a PR.
// It expands teams (if resolver provided), deduplicates owners, strips the PR author, and sorts alphabetically.
func (a *ReviewerAssigner) AssignReviewers(ctx context.Context, changedFiles []string, prAuthor string) *ReviewerAssignment {
	assignment := &ReviewerAssignment{
		FileMap: make(map[string][]string),
	}

	ownerSet := make(map[string]struct{})
	for _, fp := range changedFiles {
		owners := a.codeowners.Match(fp)
		if len(owners) > 0 {
			assignment.FileMap[fp] = owners
			for _, o := range owners {
				// If team resolver is available and handle is a team e.g. @org/team
				if a.teamResolver != nil && strings.Contains(o, "/") {
					members, err := a.teamResolver.ResolveTeamMembers(ctx, o)
					if err == nil && len(members) > 0 {
						for _, m := range members {
							ownerSet[m] = struct{}{}
						}
						continue
					}
				}
				ownerSet[o] = struct{}{}
			}
		}
	}

	// Remove PR author from reviewer list (can't review own code)
	if prAuthor != "" {
		cleanAuthor := strings.TrimPrefix(prAuthor, "@")
		delete(ownerSet, "@"+cleanAuthor)
		delete(ownerSet, cleanAuthor)
	}

	reviewers := make([]string, 0, len(ownerSet))
	for o := range ownerSet {
		reviewers = append(reviewers, o)
	}
	sort.Strings(reviewers)

	assignment.Reviewers = reviewers
	return assignment
}
