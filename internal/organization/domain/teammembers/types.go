// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package memberdomain

import (
	"github.com/google/uuid"
)

// TeamMemberRole represents permission level inside a team.
type TeamMemberRole string

const (
	RoleAdmin  TeamMemberRole = "admin"
	RoleLead   TeamMemberRole = "lead"
	RoleMember TeamMemberRole = "member"
)

// CodeManagementMemberConfig binds Git hosting platform identities.
type CodeManagementMemberConfig struct {
	Name              string `json:"name,omitempty"`
	ID                string `json:"id,omitempty"`
	GitHubUsername    string `json:"github_username,omitempty"`
	GitLabUsername    string `json:"gitlab_username,omitempty"`
	BitbucketUsername string `json:"bitbucket_username,omitempty"`
	AzureDevOpsUser   string `json:"azure_devops_user,omitempty"`
}

// CommunicationMemberConfig binds chat platform identities for review pings.
type CommunicationMemberConfig struct {
	Name          string `json:"name,omitempty"`
	ID            string `json:"id,omitempty"`
	ChatID        string `json:"chatId,omitempty"`
	SlackUserID   string `json:"slack_user_id,omitempty"`
	DiscordUserID string `json:"discord_user_id,omitempty"`
	TeamsUserID   string `json:"teams_user_id,omitempty"`
}

// ProjectManagementMemberConfig binds ticketing and issue tracker identities.
type ProjectManagementMemberConfig struct {
	Name          string `json:"name,omitempty"`
	ID            string `json:"id,omitempty"`
	JiraAccountID string `json:"jira_account_id,omitempty"`
	LinearUserID  string `json:"linear_user_id,omitempty"`
	AzureBoardsID string `json:"azure_boards_id,omitempty"`
}

// MemberItem represents a member for bulk updates and formatted listings.
type MemberItem struct {
	UUID              *uuid.UUID                      `json:"uuid,omitempty"`
	Active            bool                            `json:"active"`
	CommunicationID   string                          `json:"communicationId,omitempty"`
	TeamRole          TeamMemberRole                  `json:"teamRole"`
	Role              string                          `json:"role,omitempty"` // identity role: contributor, owner, admin
	Avatar            string                          `json:"avatar,omitempty"`
	Name              string                          `json:"name,omitempty"`
	Communication     *CommunicationMemberConfig      `json:"communication,omitempty"`
	CodeManagement    *CodeManagementMemberConfig     `json:"codeManagement,omitempty"`
	ProjectManagement *ProjectManagementMemberConfig  `json:"projectManagement,omitempty"`
	Email             string                          `json:"email"`
	UserID            *uuid.UUID                      `json:"userId,omitempty"`
	UserExists        bool                            `json:"userExists,omitempty"`
	UserStatus        string                          `json:"userStatus,omitempty"`
}

// MemberInvitation represents a pending invitation.
type MemberInvitation struct {
	Email string         `json:"email"`
	Role  TeamMemberRole `json:"role"`
}

// Standard invite result status codes.
const (
	StatusInviteSent                        = "invite_sent"
	StatusUserAlreadyRegisteredInOtherOrg   = "user_already_registered_in_other_organization"
	StatusAlreadyMember                     = "already_member"
	StatusError                             = "error"
)

// InviteResult records the outcome of an onboarding invitation or upsert.
type InviteResult struct {
	Email   string     `json:"email"`
	Status  string     `json:"status"` // invite_sent, user_already_registered_in_other_organization, already_member, error
	UUID    *uuid.UUID `json:"uuid,omitempty"`
	Message string     `json:"message,omitempty"`
}

// UpdateOrCreateMembersResponse represents the response for member upserts.
type UpdateOrCreateMembersResponse struct {
	Success bool           `json:"success"`
	Results []InviteResult `json:"results"`
}
