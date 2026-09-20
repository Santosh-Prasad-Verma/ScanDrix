// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

const (
	AcknowledgmentMessageDefault = "Analyzing your request..."
	MarkdownSuffix              = "<!-- drixy-codereview -->\n&#8203;"
	ReactionRocket              = "rocket"
)

// IPlatformResponsePolicy defines the contract for how ScanDrix responds to user git commands and mentions.
// Different providers support either comment reactions (GitHub, GitLab, Forgejo)
// or acknowledgment comments (Bitbucket, Azure DevOps).
type IPlatformResponsePolicy interface {
	RequiresAcknowledgment() bool
	UsesReaction() bool
	GetAcknowledgmentReaction() (string, error)
	GetAcknowledgmentBody() (string, error)
}

// GitHubResponsePolicy uses emoji reactions on the comment (e.g. rocket).
type GitHubResponsePolicy struct{}

func (p *GitHubResponsePolicy) RequiresAcknowledgment() bool {
	return false
}

func (p *GitHubResponsePolicy) UsesReaction() bool {
	return true
}

func (p *GitHubResponsePolicy) GetAcknowledgmentReaction() (string, error) {
	return ReactionRocket, nil
}

func (p *GitHubResponsePolicy) GetAcknowledgmentBody() (string, error) {
	return "", fmt.Errorf("GitHubResponsePolicy does not use acknowledgment body; use reactions instead")
}

// GitLabResponsePolicy uses emoji award reactions.
type GitLabResponsePolicy struct{}

func (p *GitLabResponsePolicy) RequiresAcknowledgment() bool {
	return false
}

func (p *GitLabResponsePolicy) UsesReaction() bool {
	return true
}

func (p *GitLabResponsePolicy) GetAcknowledgmentReaction() (string, error) {
	return ReactionRocket, nil
}

func (p *GitLabResponsePolicy) GetAcknowledgmentBody() (string, error) {
	return "", fmt.Errorf("GitLabResponsePolicy does not use acknowledgment body; use reactions instead")
}

// BitbucketResponsePolicy posts an initial acknowledgment comment.
type BitbucketResponsePolicy struct{}

func (p *BitbucketResponsePolicy) RequiresAcknowledgment() bool {
	return true
}

func (p *BitbucketResponsePolicy) UsesReaction() bool {
	return false
}

func (p *BitbucketResponsePolicy) GetAcknowledgmentReaction() (string, error) {
	return "", fmt.Errorf("BitbucketResponsePolicy does not use reactions; use acknowledgment body instead")
}

func (p *BitbucketResponsePolicy) GetAcknowledgmentBody() (string, error) {
	return strings.TrimSpace(AcknowledgmentMessageDefault), nil
}

// AzureReposResponsePolicy posts an acknowledgment comment tagged with drixy marker.
type AzureReposResponsePolicy struct{}

func (p *AzureReposResponsePolicy) RequiresAcknowledgment() bool {
	return true
}

func (p *AzureReposResponsePolicy) UsesReaction() bool {
	return false
}

func (p *AzureReposResponsePolicy) GetAcknowledgmentReaction() (string, error) {
	return "", fmt.Errorf("AzureReposResponsePolicy does not use reactions; use acknowledgment body instead")
}

func (p *AzureReposResponsePolicy) GetAcknowledgmentBody() (string, error) {
	return strings.TrimSpace(fmt.Sprintf("%s%s", AcknowledgmentMessageDefault, MarkdownSuffix)), nil
}

// ForgejoResponsePolicy uses comment reactions.
type ForgejoResponsePolicy struct{}

func (p *ForgejoResponsePolicy) RequiresAcknowledgment() bool {
	return false
}

func (p *ForgejoResponsePolicy) UsesReaction() bool {
	return true
}

func (p *ForgejoResponsePolicy) GetAcknowledgmentReaction() (string, error) {
	return ReactionRocket, nil
}

func (p *ForgejoResponsePolicy) GetAcknowledgmentBody() (string, error) {
	return "", fmt.Errorf("ForgejoResponsePolicy does not use acknowledgment body; use reactions instead")
}

// PlatformResponsePolicyFactory returns the appropriate response policy for a given provider.
type PlatformResponsePolicyFactory struct{}

// Create resolves the provider response policy.
func (f *PlatformResponsePolicyFactory) Create(provider models.SCMProvider) IPlatformResponsePolicy {
	switch provider {
	case models.ProviderGitHub:
		return &GitHubResponsePolicy{}
	case models.ProviderGitLab:
		return &GitLabResponsePolicy{}
	case models.ProviderBitbucket:
		return &BitbucketResponsePolicy{}
	case models.ProviderAzure:
		return &AzureReposResponsePolicy{}
	case models.ProviderForgejo:
		return &ForgejoResponsePolicy{}
	default:
		return &GitLabResponsePolicy{}
	}
}
