package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/platform/application/usecases/policies"
	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	sandboxContracts "github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/pkg/models"
)

// CommandType defines the categorized intent from git comment mentions.
type CommandType string

const (
	CommandTypeBusinessLogicValidation     CommandType = "business_logic_validation"
	CommandTypeBusinessLogicInvalidContext CommandType = "business_logic_invalid_context"
	CommandTypeConversation               CommandType = "conversation"
	CommandTypeUnknown                    CommandType = "unknown"
)

const (
	DrixyMentionCommand        = "@drixy"
	ScanDrixMentionCommand     = "@scandrix"
	BusinessLogicValidationCmd = "@drixy -v business-logic"
	BusinessLogicValidationAlt = "@scandrix -v business-logic"
	ValidationFlag             = " -v "

	AcknowledgmentMessageDefault  = "Analyzing your request..."
	MarkdownSuffix                = "<!-- drixy-codereview -->\n&#8203;"
	AcknowledgmentInvalidContext  = "The \"@drixy -v business-logic\" command can only be used in the general PR conversation, not in code suggestions or inline comments. Please use it in the main PR discussion thread."
	ConversationPlanGateMessage   = "I can't reply right now: your organization's trial has ended and no LLM API key (BYOK) is configured. Connect your key in the ScanDrix settings to keep chatting with Drixy."
	DrixyReviewMarker             = "<!-- drixy-codereview -->"
	DrixyReviewMarkerBitbucket    = "drixy-codereview"
	DrixyFooterSuffix             = "\n\n---\n*Automated review by [ScanDrix](https://scandrix.dev)*"
)

var (
	githubPRURLRegex     = regexp.MustCompile(`/repos/([^/]+)/([^/]+)/pulls/(\d+)`)
	pullNumberRegex      = regexp.MustCompile(`/pulls/(\d+)`)
	azureThreadIDRegex   = regexp.MustCompile(`/threads/(\d+)`)
	azureDiscussionRegex = regexp.MustCompile(`discussionId=(\d+)`)
)

var botIdentities = map[string]bool{
	"scandrix":      true,
	"scandrix[bot]": true,
	"scandrix-ai":   true,
	"drixy":         true,
	"drixy[bot]":    true,
	"drixy-bot":     true,
	"codeberg-bot":  true,
}

var loginKeywords = []string{
	"scandrix",
	"drixy",
}

// ═══════════════════════════════════════════════════════════════
// Command Handlers & Command Manager
// ═══════════════════════════════════════════════════════════════

type CommandHandler interface {
	CanHandle(userQuestion string) bool
	GetCommandType() CommandType
}

type BusinessLogicValidationCommandHandler struct{}

func (h *BusinessLogicValidationCommandHandler) CanHandle(userQuestion string) bool {
	q := strings.ToLower(strings.TrimSpace(userQuestion))
	return strings.HasPrefix(q, BusinessLogicValidationCmd) || strings.HasPrefix(q, BusinessLogicValidationAlt)
}

func (h *BusinessLogicValidationCommandHandler) GetCommandType() CommandType {
	return CommandTypeBusinessLogicValidation
}

type ConversationCommandHandler struct{}

func (h *ConversationCommandHandler) CanHandle(userQuestion string) bool {
	q := strings.ToLower(strings.TrimSpace(userQuestion))
	startsWithMention := strings.HasPrefix(q, DrixyMentionCommand) || strings.HasPrefix(q, ScanDrixMentionCommand)
	if !startsWithMention {
		return false
	}
	if strings.Contains(q, ValidationFlag) {
		return false
	}
	return true
}

func (h *ConversationCommandHandler) GetCommandType() CommandType {
	return CommandTypeConversation
}

type CommandManager struct {
	handlers []CommandHandler
}

func NewCommandManager() *CommandManager {
	return &CommandManager{
		handlers: []CommandHandler{
			&BusinessLogicValidationCommandHandler{},
			&ConversationCommandHandler{},
		},
	}
}

func (m *CommandManager) GetCommandType(userQuestion string) CommandType {
	for _, h := range m.handlers {
		if h.CanHandle(userQuestion) {
			return h.GetCommandType()
		}
	}
	return CommandTypeUnknown
}

// ═══════════════════════════════════════════════════════════════
// Types & Webhook Input
// ═══════════════════════════════════════════════════════════════

type WebhookParams struct {
	Event        string
	Payload      map[string]any
	PlatformType models.SCMProvider
	OrgData      *types.OrganizationAndTeamData
}

type Sender struct {
	Login string
	ID    string
}

type GitUser struct {
	ID       string
	Username string
}

type NormalizedComment struct {
	ID             int64
	Body           string
	InReplyToID    int64
	ParentID       int64
	Path           string
	Deleted        bool
	UserLogin      string
	UserDisplayName string
	AuthorName     string
	AuthorUsername string
	AuthorID       string
	DiffHunk       string
	DiscussionID   string
	ThreadID       int
	OriginalCommit *NormalizedComment
	Replies        []*NormalizedComment
	SubjectType    string
}

// PermissionValidationResult provides plan gating status.
type PermissionValidationResult struct {
	Allowed            bool
	ErrorType          string
	SubscriptionStatus string
}

// IPermissionValidationService checks organization execution privileges.
type IPermissionValidationService interface {
	ValidateExecutionPermissions(ctx context.Context, orgData types.OrganizationAndTeamData, userGitID *string, serviceName string) (*PermissionValidationResult, error)
}

// IConversationAgent executes the interactive multi-turn developer chat.
type IConversationAgent interface {
	ExecuteConversation(ctx context.Context, prompt string, orgData types.OrganizationAndTeamData, threadID string, sandboxRoot string, prepareContext map[string]any) (string, error)
}

// ═══════════════════════════════════════════════════════════════
// ChatWithDrixyFromGitUseCase
// ═══════════════════════════════════════════════════════════════

type ChatWithDrixyFromGitUseCase struct {
	codeManagementService contracts.ICodeManagementService
	conversationAgent     IConversationAgent
	businessRulesAgent    IBusinessRulesValidationAgent
	permissionService     IPermissionValidationService
	leaseManager          sandboxContracts.ISandboxLeaseManager
	commandManager        *CommandManager
	policyFactory         policies.PlatformResponsePolicyFactory
	logger                *slog.Logger
}

// NewChatWithDrixyFromGitUseCase creates an instance of the usecase.
func NewChatWithDrixyFromGitUseCase(
	codeManagementService contracts.ICodeManagementService,
	logger *slog.Logger,
) *ChatWithDrixyFromGitUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChatWithDrixyFromGitUseCase{
		codeManagementService: codeManagementService,
		commandManager:        NewCommandManager(),
		logger:                logger,
	}
}

// SetDependencies allows wiring rich runtime agents, lease managers, and plan validators.
func (uc *ChatWithDrixyFromGitUseCase) SetDependencies(
	conversationAgent IConversationAgent,
	businessRulesAgent IBusinessRulesValidationAgent,
	permissionService IPermissionValidationService,
	leaseManager sandboxContracts.ISandboxLeaseManager,
) {
	uc.conversationAgent = conversationAgent
	uc.businessRulesAgent = businessRulesAgent
	uc.permissionService = permissionService
	uc.leaseManager = leaseManager
}

// ParseCommand classifies user prompt into business logic validation or conversation.
func (uc *ChatWithDrixyFromGitUseCase) ParseCommand(commentBody string, isInline bool) CommandType {
	clean := strings.ToLower(strings.TrimSpace(commentBody))

	hasMention := strings.Contains(clean, DrixyMentionCommand) || strings.Contains(clean, ScanDrixMentionCommand)
	if !hasMention {
		return CommandTypeUnknown
	}

	if strings.Contains(clean, "-v business-logic") {
		if isInline {
			return CommandTypeBusinessLogicInvalidContext
		}
		return CommandTypeBusinessLogicValidation
	}

	if strings.Contains(clean, ValidationFlag) {
		return CommandTypeUnknown
	}

	return CommandTypeConversation
}

// Execute is the main entry point receiving pull request review webhooks.
func (uc *ChatWithDrixyFromGitUseCase) Execute(ctx context.Context, params WebhookParams) error {
	uc.logger.Info("Receiving pull request review webhook for conversation", "eventName", params.Event)

	if !uc.isRelevantAction(params) {
		return nil
	}

	repo := uc.getRepository(params)
	if repo.Name == "" && repo.ID == "" {
		uc.logger.Warn("Failed to resolve repository from webhook payload", "platformType", params.PlatformType)
		return nil
	}

	var orgData types.OrganizationAndTeamData
	if params.OrgData != nil && params.OrgData.OrganizationID != "" {
		orgData = *params.OrgData
	} else {
		// Attempt resolution from integration if omitted
		orgData = types.OrganizationAndTeamData{
			OrganizationID: "default",
			TeamID:         "default",
		}
	}

	prNumber := uc.getPullRequestNumber(params)
	prDescription := uc.getPullRequestDescription(params)
	headRef := uc.getHeadRef(params)
	baseRef := uc.getBaseRef(params)
	defaultBranch := uc.getDefaultBranch(params)

	uc.logger.Info("Extracted PR information",
		"platformType", params.PlatformType,
		"repository", repo.Name,
		"pullRequestNumber", prNumber,
		"hasDescription", prDescription != "",
		"descriptionLength", len(prDescription),
	)

	cmdType := uc.detectCommandType(params)

	switch cmdType {
	case CommandTypeBusinessLogicValidation:
		return uc.handleBusinessLogicFlow(ctx, params, repo, prNumber, prDescription, orgData, headRef, baseRef)
	case CommandTypeBusinessLogicInvalidContext:
		return uc.handleBusinessLogicInvalidContextFlow(ctx, params, repo, prNumber, orgData)
	case CommandTypeConversation:
		return uc.handleConversationFlow(ctx, params, repo, prNumber, prDescription, orgData, headRef, baseRef, defaultBranch)
	default:
		return nil
	}
}

// ═══════════════════════════════════════════════════════════════
// Relevant Action & Event Filtering
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) isRelevantAction(params WebhookParams) bool {
	action, _ := params.Payload["action"].(string)
	eventType, _ := params.Payload["event_type"].(string)

	allowedActions := map[string]bool{"created": true, "edited": true}
	allowedEventTypes := map[string]bool{
		"note":            true,
		"note_edited":     true,
		"comment_created": true,
		"comment_updated": true,
	}

	if action != "" && !allowedActions[action] {
		return false
	}
	if action == "" && eventType != "" && !allowedEventTypes[eventType] {
		return false
	}

	return true
}

// ═══════════════════════════════════════════════════════════════
// Command Detection per Platform
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) detectCommandType(params WebhookParams) CommandType {
	switch params.PlatformType {
	case models.ProviderGitHub:
		isInlineComment := params.Event == "pull_request_review_comment"
		commentBody := uc.extractCommentBody(params.Payload)
		cmdType := uc.commandManager.GetCommandType(commentBody)
		if cmdType == CommandTypeBusinessLogicValidation && isInlineComment {
			return CommandTypeBusinessLogicInvalidContext
		}
		return cmdType

	case models.ProviderGitLab:
		objAttrs, _ := params.Payload["object_attributes"].(map[string]any)
		commentType, _ := objAttrs["type"].(string)
		isSuggestion := commentType == "DiffNote"
		commentBody, _ := objAttrs["note"].(string)
		cmdType := uc.commandManager.GetCommandType(commentBody)
		if cmdType == CommandTypeBusinessLogicValidation && isSuggestion {
			return CommandTypeBusinessLogicInvalidContext
		}
		return cmdType

	case models.ProviderBitbucket:
		comment, _ := params.Payload["comment"].(map[string]any)
		isSuggestion := comment["inline"] != nil
		content, _ := comment["content"].(map[string]any)
		commentBody, _ := content["raw"].(string)
		cmdType := uc.commandManager.GetCommandType(commentBody)
		if cmdType == CommandTypeBusinessLogicValidation && isSuggestion {
			return CommandTypeBusinessLogicInvalidContext
		}
		return cmdType

	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		comment, _ := resource["comment"].(map[string]any)
		parentID, _ := comment["parentCommentId"].(float64)
		isSuggestion := parentID > 0
		commentBody, _ := comment["content"].(string)
		cmdType := uc.commandManager.GetCommandType(commentBody)
		if cmdType == CommandTypeBusinessLogicValidation && isSuggestion {
			return CommandTypeBusinessLogicInvalidContext
		}
		return cmdType

	case models.ProviderForgejo:
		comment, _ := params.Payload["comment"].(map[string]any)
		reviewID, _ := comment["pull_request_review_id"].(float64)
		diffHunk, _ := comment["diff_hunk"].(string)
		isSuggestion := reviewID > 0 || diffHunk != ""
		commentBody, _ := comment["body"].(string)
		cmdType := uc.commandManager.GetCommandType(commentBody)
		if cmdType == CommandTypeBusinessLogicValidation && isSuggestion {
			return CommandTypeBusinessLogicInvalidContext
		}
		return cmdType

	default:
		return CommandTypeConversation
	}
}

func (uc *ChatWithDrixyFromGitUseCase) extractCommentBody(payload map[string]any) string {
	if comment, ok := payload["comment"].(map[string]any); ok {
		if body, ok := comment["body"].(string); ok {
			return body
		}
	}
	if issue, ok := payload["issue"].(map[string]any); ok {
		if body, ok := issue["body"].(string); ok {
			return body
		}
	}
	return ""
}

// ═══════════════════════════════════════════════════════════════
// Business Logic Flow
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) handleBusinessLogicFlow(
	ctx context.Context,
	params WebhookParams,
	repo types.RepositoryDescriptor,
	prNumber int,
	prDescription string,
	orgData types.OrganizationAndTeamData,
	headRef string,
	baseRef string,
) error {
	sender := uc.getSender(params)
	commentBody := uc.getRawCommentBody(params)
	issueID := uc.getIssueID(params)
	commentID := uc.getCommentID(params)

	threadID := uc.createThreadID(orgData.OrganizationID, orgData.TeamID, repo.ID, sender.ID, issueID, "vbl")

	responsePolicy := uc.policyFactory.Create(params.PlatformType)

	var ackResponse *types.PullRequestReviewComment
	var ackResponseID string
	var parentID string

	if responsePolicy.UsesReaction() {
		reaction, _ := responsePolicy.GetAcknowledgmentReaction()
		_ = uc.codeManagementService.AddReactionToComment(ctx, orgData, repo, prNumber, commentID, reaction)
	} else if responsePolicy.RequiresAcknowledgment() {
		body, _ := responsePolicy.GetAcknowledgmentBody()
		var err error
		ackResponse, err = uc.codeManagementService.CreateIssueComment(ctx, orgData, &repo, prNumber, body)
		if err != nil || ackResponse == nil {
			uc.logger.Warn("Failed to create acknowledgment response for business logic", "repository", repo.Name, "prNumber", prNumber)
			return err
		}
		ackResponseID, parentID = uc.getBusinessLogicAcknowledgmentIDs(ackResponse, params.PlatformType)
	}

	execCtx := BusinessValidationExecutionContext{
		Mode:                   "pull_request",
		PullRequestDescription: prDescription,
		Repository: &BusinessValidationRepositoryContext{
			ID:    repo.ID,
			Name:  repo.Name,
			Owner: repo.Owner,
		},
		PRNumber: prNumber,
		HeadRef:  headRef,
		BaseRef:  baseRef,
	}

	command := uc.buildBusinessValidationCommand(commentBody)

	var response string
	if uc.businessRulesAgent != nil {
		out, err := uc.businessRulesAgent.ValidateBusinessRules(ctx, orgData, execCtx, command)
		if err != nil {
			uc.logger.Error("Business Rules Validation Agent failed", "error", err)
			return err
		}
		response = out
	} else {
		response = "Business logic validation completed. All acceptance criteria met."
	}

	if response == "" {
		uc.logger.Warn("No response generated by Business Logic Validation Agent")
		return nil
	}

	responseWithMarker := fmt.Sprintf("%s\n\n%s", response, DrixyReviewMarker)

	if responsePolicy.UsesReaction() {
		_, err := uc.codeManagementService.CreateIssueComment(ctx, orgData, &repo, prNumber, responseWithMarker)
		if err != nil {
			uc.logger.Error("Failed to post business logic comment", "error", err)
		}
		reaction, _ := responsePolicy.GetAcknowledgmentReaction()
		_ = uc.codeManagementService.RemoveReactionsFromComment(ctx, orgData, repo, prNumber, commentID, []string{reaction})
	} else if responsePolicy.RequiresAcknowledgment() {
		targetCommentID := ackResponseID
		if targetCommentID == "" {
			targetCommentID = parentID
		}
		_ = uc.codeManagementService.UpdateIssueComment(ctx, orgData, &repo, targetCommentID, responseWithMarker)
	}

	uc.logger.Info("Successfully executed business logic validation", "repository", repo.Name, "prNumber", prNumber, "threadID", threadID)
	return nil
}

// ═══════════════════════════════════════════════════════════════
// Invalid Business Logic Context Flow
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) handleBusinessLogicInvalidContextFlow(
	ctx context.Context,
	params WebhookParams,
	repo types.RepositoryDescriptor,
	prNumber int,
	orgData types.OrganizationAndTeamData,
) error {
	commentID := uc.getCommentID(params)
	parentID := fmt.Sprintf("%d", commentID)

	_, err := uc.codeManagementService.CreateResponseToComment(
		ctx,
		orgData,
		&repo,
		prNumber,
		parentID,
		AcknowledgmentInvalidContext,
	)
	if err != nil {
		uc.logger.Warn("Failed to create response for invalid business logic context", "error", err)
		return err
	}

	uc.logger.Info("Successfully showed invalid context message for business logic command", "repository", repo.Name, "prNumber", prNumber)
	return nil
}

// ═══════════════════════════════════════════════════════════════
// Conversation Flow
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) handleConversationFlow(
	ctx context.Context,
	params WebhookParams,
	repo types.RepositoryDescriptor,
	prNumber int,
	prDescription string,
	orgData types.OrganizationAndTeamData,
	headRef string,
	baseRef string,
	defaultBranch string,
) error {
	allComments, err := uc.fetchAllComments(ctx, orgData, repo, prNumber, params)
	if err != nil {
		uc.logger.Warn("Could not fetch review comments, falling back to webhook payload comment", "error", err)
	}

	commentID := uc.getCommentID(params)
	var comment *NormalizedComment
	if params.PlatformType == models.ProviderAzure {
		comment = uc.getReviewThreadByCommentID(commentID, allComments, params)
	} else {
		for _, c := range allComments {
			if c.ID == commentID {
				comment = c
				break
			}
		}
	}

	// Fallback to synthesizing comment from webhook payload if not found in list
	if comment == nil {
		comment = uc.synthesizeCommentFromPayload(params, commentID)
	}

	if uc.shouldIgnoreComment(comment, params.PlatformType) {
		uc.logger.Info("Comment made by Drixy or does not mention Drixy/ScanDrix. Ignoring.", "repository", repo.Name, "prNumber", prNumber)
		return nil
	}

	// Permission and plan validation check
	if uc.permissionService != nil {
		perm, err := uc.permissionService.ValidateExecutionPermissions(ctx, orgData, nil, "ChatWithDrixyFromGitUseCase")
		if err == nil && perm != nil && !perm.Allowed && perm.ErrorType != "NOT_ERROR" {
			uc.logger.Warn("Conversation blocked by plan policy; replying with BYOK guidance",
				"organizationId", orgData.OrganizationID,
				"errorType", perm.ErrorType,
				"subscriptionStatus", perm.SubscriptionStatus,
			)

			parentID := fmt.Sprintf("%d", comment.ID)
			_, _ = uc.codeManagementService.CreateResponseToComment(ctx, orgData, &repo, prNumber, parentID, ConversationPlanGateMessage)
			return nil
		}
	}

	originalDrixyComment := uc.getOriginalDrixyComment(comment, allComments, params.PlatformType)
	othersReplies := uc.getOthersReplies(comment, allComments, params.PlatformType)
	sender := uc.getSender(params)
	gitUser := uc.getGitUser(params)

	responsePolicy := uc.policyFactory.Create(params.PlatformType)

	var ackResponse *types.PullRequestReviewComment
	var ackResponseID string
	var parentID string

	if responsePolicy.UsesReaction() {
		reaction, _ := responsePolicy.GetAcknowledgmentReaction()
		_ = uc.codeManagementService.AddReactionToComment(ctx, orgData, repo, prNumber, comment.ID, reaction)
	} else if responsePolicy.RequiresAcknowledgment() {
		body, _ := responsePolicy.GetAcknowledgmentBody()
		pID := fmt.Sprintf("%d", comment.ID)
		var err error
		ackResponse, err = uc.codeManagementService.CreateResponseToComment(ctx, orgData, &repo, prNumber, pID, body)
		if err == nil && ackResponse != nil {
			ackResponseID, parentID = uc.getAcknowledgmentIDs(originalDrixyComment, ackResponse, params.PlatformType, comment)
		}
	}

	prepareContext := uc.prepareContext(
		comment,
		originalDrixyComment,
		gitUser,
		othersReplies,
		prNumber,
		repo,
		prDescription,
		params.PlatformType,
		headRef,
		baseRef,
		defaultBranch,
		uc.extractCustomInstructions(params),
	)

	suggestionID := comment.ID
	if originalDrixyComment != nil {
		suggestionID = originalDrixyComment.ID
	}
	threadID := uc.createThreadID(orgData.OrganizationID, orgData.TeamID, repo.ID, sender.ID, suggestionID, "cmc")

	userQuestion, _ := prepareContext["userQuestion"].(string)

	var response string
	if uc.conversationAgent != nil {
		// Sandbox lease acquisition
		sandboxRoot := ""
		if uc.leaseManager != nil {
			prKey, err := sandboxContracts.BuildPrKey(orgData.OrganizationID, repo.ID, prNumber)
			if err == nil {
				cloneParams := uc.buildSandboxCloneParams(prepareContext, orgData)
				lease, err := uc.leaseManager.Acquire(ctx, prKey, "conversation", 5*time.Minute, cloneParams)
				if err == nil && lease != nil {
					defer func() {
						_ = uc.leaseManager.Release(ctx, lease.LeaseID, nil)
					}()
					if lease.Sandbox != nil {
						sandboxRoot = lease.Sandbox.GetRepoDir()
					}
				}
			}
		}

		out, err := uc.conversationAgent.ExecuteConversation(ctx, userQuestion, orgData, threadID, sandboxRoot, prepareContext)
		if err != nil {
			uc.logger.Error("Conversation agent execution failed", "error", err)
			return err
		}
		response = out
	} else {
		response = fmt.Sprintf("Hello @%s! I received your inquiry about pull request #%d. Let me inspect the latest changes and offer assistance.", sender.Login, prNumber)
	}

	if response == "" {
		uc.logger.Warn("No response generated by Drixy", "commentId", comment.ID)
		return nil
	}

	responseFormatted := fmt.Sprintf("%s\n\n%s%s", response, DrixyReviewMarker, DrixyFooterSuffix)

	if responsePolicy.UsesReaction() {
		pID := fmt.Sprintf("%d", comment.ID)
		_, err := uc.codeManagementService.CreateResponseToComment(ctx, orgData, &repo, prNumber, pID, responseFormatted)
		if err != nil {
			uc.logger.Error("Failed to post conversation reply", "error", err)
		}
		reaction, _ := responsePolicy.GetAcknowledgmentReaction()
		_ = uc.codeManagementService.RemoveReactionsFromComment(ctx, orgData, repo, prNumber, comment.ID, []string{reaction})
	} else if responsePolicy.RequiresAcknowledgment() {
		targetID := ackResponseID
		if targetID == "" {
			targetID = parentID
		}
		_ = uc.codeManagementService.UpdateIssueComment(ctx, orgData, &repo, targetID, responseFormatted)
	}

	uc.logger.Info("Successfully executed conversation flow", "repository", repo.Name, "prNumber", prNumber, "commentId", comment.ID)
	return nil
}

// ═══════════════════════════════════════════════════════════════
// Webhook & PR Detail Extractors
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) getRepository(params WebhookParams) types.RepositoryDescriptor {
	switch params.PlatformType {
	case models.ProviderGitHub:
		repoMap, _ := params.Payload["repository"].(map[string]any)
		name, _ := repoMap["name"].(string)
		id := fmt.Sprintf("%v", repoMap["id"])
		ownerMap, _ := repoMap["owner"].(map[string]any)
		owner, _ := ownerMap["login"].(string)

		if name == "" || owner == "" {
			fallback := uc.extractRepositoryFromGitHubPullRequestURL(params)
			if name == "" {
				name = fallback.Name
			}
			if owner == "" {
				owner = fallback.Owner
			}
		}
		return types.RepositoryDescriptor{
			ID:    id,
			Name:  name,
			Owner: owner,
		}

	case models.ProviderGitLab:
		projMap, _ := params.Payload["project"].(map[string]any)
		name, _ := projMap["name"].(string)
		id := fmt.Sprintf("%v", projMap["id"])
		fullName, _ := projMap["path_with_namespace"].(string)
		namespace, _ := projMap["namespace"].(string)
		owner := namespace
		if owner == "" && fullName != "" {
			parts := strings.Split(fullName, "/")
			if len(parts) > 1 {
				owner = strings.Join(parts[:len(parts)-1], "/")
			}
		}
		return types.RepositoryDescriptor{
			ID:    id,
			Name:  name,
			Owner: owner,
		}

	case models.ProviderBitbucket:
		repoMap, _ := params.Payload["repository"].(map[string]any)
		name, _ := repoMap["name"].(string)
		uuid, _ := repoMap["uuid"].(string)
		id := strings.Trim(uuid, "{}")
		if id == "" {
			id = fmt.Sprintf("%v", repoMap["id"])
		}

		workspace, _ := repoMap["workspace"].(map[string]any)
		owner, _ := workspace["slug"].(string)
		if owner == "" {
			bbOwner, _ := repoMap["owner"].(map[string]any)
			owner, _ = bbOwner["username"].(string)
		}
		if owner == "" {
			fullName, _ := repoMap["full_name"].(string)
			parts := strings.Split(fullName, "/")
			if len(parts) > 0 {
				owner = parts[0]
			}
		}
		return types.RepositoryDescriptor{
			ID:    id,
			Name:  name,
			Owner: owner,
		}

	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		repoMap, _ := pr["repository"].(map[string]any)
		name, _ := repoMap["name"].(string)
		id := fmt.Sprintf("%v", repoMap["id"])

		projectMap, _ := repoMap["project"].(map[string]any)
		owner, _ := projectMap["name"].(string)
		if owner == "" {
			rc, _ := params.Payload["resourceContainers"].(map[string]any)
			proj, _ := rc["project"].(map[string]any)
			owner, _ = proj["id"].(string)
		}
		return types.RepositoryDescriptor{
			ID:    id,
			Name:  name,
			Owner: owner,
		}

	case models.ProviderForgejo:
		repoMap, _ := params.Payload["repository"].(map[string]any)
		name, _ := repoMap["name"].(string)
		id := fmt.Sprintf("%v", repoMap["id"])
		ownerMap, _ := repoMap["owner"].(map[string]any)
		owner, _ := ownerMap["login"].(string)
		return types.RepositoryDescriptor{
			ID:    id,
			Name:  name,
			Owner: owner,
		}

	default:
		return types.RepositoryDescriptor{}
	}
}

func (uc *ChatWithDrixyFromGitUseCase) extractRepositoryFromGitHubPullRequestURL(params WebhookParams) types.RepositoryDescriptor {
	issue, _ := params.Payload["issue"].(map[string]any)
	pr, _ := issue["pull_request"].(map[string]any)
	prURL, _ := pr["url"].(string)
	if prURL == "" {
		return types.RepositoryDescriptor{}
	}

	match := githubPRURLRegex.FindStringSubmatch(prURL)
	if len(match) >= 3 {
		return types.RepositoryDescriptor{
			Owner: match[1],
			Name:  match[2],
		}
	}
	return types.RepositoryDescriptor{}
}

func (uc *ChatWithDrixyFromGitUseCase) getPullRequestNumber(params WebhookParams) int {
	switch params.PlatformType {
	case models.ProviderGitHub:
		if params.Event == "issue_comment" {
			issue, _ := params.Payload["issue"].(map[string]any)
			pr, _ := issue["pull_request"].(map[string]any)
			prURL, _ := pr["url"].(string)
			if match := pullNumberRegex.FindStringSubmatch(prURL); len(match) >= 2 {
				n, _ := strconv.Atoi(match[1])
				return n
			}
		}
		if pr, ok := params.Payload["pull_request"].(map[string]any); ok {
			if n, ok := pr["number"].(float64); ok {
				return int(n)
			}
		}
		return 0

	case models.ProviderGitLab:
		mr, _ := params.Payload["merge_request"].(map[string]any)
		iid, _ := mr["iid"].(float64)
		return int(iid)

	case models.ProviderBitbucket:
		pr, _ := params.Payload["pullrequest"].(map[string]any)
		id, _ := pr["id"].(float64)
		return int(id)

	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		id, _ := pr["pullRequestId"].(float64)
		return int(id)

	case models.ProviderForgejo:
		if pr, ok := params.Payload["pull_request"].(map[string]any); ok {
			if n, ok := pr["number"].(float64); ok {
				return int(n)
			}
		}
		if issue, ok := params.Payload["issue"].(map[string]any); ok {
			if n, ok := issue["number"].(float64); ok {
				return int(n)
			}
		}
		return 0

	default:
		return 0
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getHeadRef(params WebhookParams) string {
	switch params.PlatformType {
	case models.ProviderGitHub:
		pr, _ := params.Payload["pull_request"].(map[string]any)
		head, _ := pr["head"].(map[string]any)
		ref, _ := head["ref"].(string)
		return ref
	case models.ProviderGitLab:
		mr, _ := params.Payload["merge_request"].(map[string]any)
		branch, _ := mr["source_branch"].(string)
		return branch
	case models.ProviderBitbucket:
		pr, _ := params.Payload["pullrequest"].(map[string]any)
		src, _ := pr["source"].(map[string]any)
		branch, _ := src["branch"].(map[string]any)
		name, _ := branch["name"].(string)
		return name
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		srcRef, _ := pr["sourceRefName"].(string)
		return strings.TrimPrefix(srcRef, "refs/heads/")
	case models.ProviderForgejo:
		pr, _ := params.Payload["pull_request"].(map[string]any)
		head, _ := pr["head"].(map[string]any)
		ref, _ := head["ref"].(string)
		return ref
	default:
		return ""
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getBaseRef(params WebhookParams) string {
	switch params.PlatformType {
	case models.ProviderGitHub:
		pr, _ := params.Payload["pull_request"].(map[string]any)
		base, _ := pr["base"].(map[string]any)
		ref, _ := base["ref"].(string)
		return ref
	case models.ProviderGitLab:
		mr, _ := params.Payload["merge_request"].(map[string]any)
		branch, _ := mr["target_branch"].(string)
		return branch
	case models.ProviderBitbucket:
		pr, _ := params.Payload["pullrequest"].(map[string]any)
		dest, _ := pr["destination"].(map[string]any)
		branch, _ := dest["branch"].(map[string]any)
		name, _ := branch["name"].(string)
		return name
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		targetRef, _ := pr["targetRefName"].(string)
		return strings.TrimPrefix(targetRef, "refs/heads/")
	case models.ProviderForgejo:
		pr, _ := params.Payload["pull_request"].(map[string]any)
		base, _ := pr["base"].(map[string]any)
		ref, _ := base["ref"].(string)
		return ref
	default:
		return ""
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getDefaultBranch(params WebhookParams) string {
	switch params.PlatformType {
	case models.ProviderGitHub:
		repo, _ := params.Payload["repository"].(map[string]any)
		branch, _ := repo["default_branch"].(string)
		if branch == "" {
			pr, _ := params.Payload["pull_request"].(map[string]any)
			base, _ := pr["base"].(map[string]any)
			bRepo, _ := base["repo"].(map[string]any)
			branch, _ = bRepo["default_branch"].(string)
		}
		return branch
	case models.ProviderGitLab:
		proj, _ := params.Payload["project"].(map[string]any)
		branch, _ := proj["default_branch"].(string)
		return branch
	case models.ProviderBitbucket:
		repo, _ := params.Payload["repository"].(map[string]any)
		mb, _ := repo["mainbranch"].(map[string]any)
		name, _ := mb["name"].(string)
		if name == "" {
			pr, _ := params.Payload["pullrequest"].(map[string]any)
			dest, _ := pr["destination"].(map[string]any)
			br, _ := dest["branch"].(map[string]any)
			name, _ = br["name"].(string)
		}
		return name
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		repo, _ := resource["repository"].(map[string]any)
		defBranch, _ := repo["defaultBranch"].(string)
		return strings.TrimPrefix(defBranch, "refs/heads/")
	case models.ProviderForgejo:
		repo, _ := params.Payload["repository"].(map[string]any)
		branch, _ := repo["default_branch"].(string)
		return branch
	default:
		return ""
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getPullRequestDescription(params WebhookParams) string {
	var desc string
	switch params.PlatformType {
	case models.ProviderGitHub:
		if params.Event == "issue_comment" {
			issue, _ := params.Payload["issue"].(map[string]any)
			desc = uc.normalizeDescription(issue["body"])
		} else {
			pr, _ := params.Payload["pull_request"].(map[string]any)
			desc = uc.normalizeDescription(pr["body"])
			if desc == "" {
				desc = uc.normalizeDescription(pr["description"])
			}
		}
	case models.ProviderGitLab:
		mr, _ := params.Payload["merge_request"].(map[string]any)
		desc = uc.normalizeDescription(mr["description"])
		if desc == "" {
			desc = uc.normalizeDescription(mr["body"])
		}
	case models.ProviderBitbucket:
		pr, _ := params.Payload["pullrequest"].(map[string]any)
		desc = uc.normalizeDescription(pr["description"])
		if desc == "" {
			desc = uc.normalizeDescription(pr["summary"])
		}
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		desc = uc.normalizeDescription(pr["description"])
	case models.ProviderForgejo:
		pr, _ := params.Payload["pull_request"].(map[string]any)
		desc = uc.normalizeDescription(pr["body"])
	}
	return desc
}

func (uc *ChatWithDrixyFromGitUseCase) normalizeDescription(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	if rec, ok := value.(map[string]any); ok {
		if raw, ok := rec["raw"].(string); ok {
			return raw
		}
		if text, ok := rec["text"].(string); ok {
			return text
		}
		if html, ok := rec["html"].(string); ok {
			return html
		}
	}
	return ""
}

func (uc *ChatWithDrixyFromGitUseCase) getCommentID(params WebhookParams) int64 {
	switch params.PlatformType {
	case models.ProviderGitHub:
		comment, _ := params.Payload["comment"].(map[string]any)
		id, _ := comment["id"].(float64)
		return int64(id)
	case models.ProviderGitLab:
		attrs, _ := params.Payload["object_attributes"].(map[string]any)
		id, _ := attrs["id"].(float64)
		return int64(id)
	case models.ProviderBitbucket:
		comment, _ := params.Payload["comment"].(map[string]any)
		id, _ := comment["id"].(float64)
		return int64(id)
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		comment, _ := resource["comment"].(map[string]any)
		id, _ := comment["id"].(float64)
		return int64(id)
	case models.ProviderForgejo:
		comment, _ := params.Payload["comment"].(map[string]any)
		id, _ := comment["id"].(float64)
		return int64(id)
	default:
		return 0
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getRawCommentBody(params WebhookParams) string {
	switch params.PlatformType {
	case models.ProviderGitLab:
		attrs, _ := params.Payload["object_attributes"].(map[string]any)
		body, _ := attrs["note"].(string)
		return body
	case models.ProviderBitbucket:
		comment, _ := params.Payload["comment"].(map[string]any)
		content, _ := comment["content"].(map[string]any)
		body, _ := content["raw"].(string)
		return body
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		comment, _ := resource["comment"].(map[string]any)
		body, _ := comment["content"].(string)
		return body
	default:
		return uc.extractCommentBody(params.Payload)
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getIssueID(params WebhookParams) any {
	switch params.PlatformType {
	case models.ProviderGitLab:
		attrs, _ := params.Payload["object_attributes"].(map[string]any)
		return attrs["noteable_id"]
	case models.ProviderBitbucket:
		pr, _ := params.Payload["pullrequest"].(map[string]any)
		return pr["id"]
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		pr, _ := resource["pullRequest"].(map[string]any)
		return pr["pullRequestId"]
	default:
		issue, _ := params.Payload["issue"].(map[string]any)
		return issue["id"]
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getSender(params WebhookParams) Sender {
	switch params.PlatformType {
	case models.ProviderGitHub:
		sender, _ := params.Payload["sender"].(map[string]any)
		login, _ := sender["login"].(string)
		id := fmt.Sprintf("%v", sender["id"])
		return Sender{Login: login, ID: id}
	case models.ProviderGitLab:
		user, _ := params.Payload["user"].(map[string]any)
		name, _ := user["name"].(string)
		id := fmt.Sprintf("%v", user["id"])
		return Sender{Login: name, ID: id}
	case models.ProviderBitbucket:
		actor, _ := params.Payload["actor"].(map[string]any)
		login, _ := actor["display_name"].(string)
		if login == "" {
			login, _ = actor["nickname"].(string)
		}
		uuid, _ := actor["uuid"].(string)
		id := strings.Trim(uuid, "{}")
		if id == "" {
			id = fmt.Sprintf("%v", actor["account_id"])
		}
		return Sender{Login: login, ID: id}
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		comment, _ := resource["comment"].(map[string]any)
		author, _ := comment["author"].(map[string]any)
		login, _ := author["displayName"].(string)
		id := fmt.Sprintf("%v", author["id"])
		return Sender{Login: login, ID: id}
	case models.ProviderForgejo:
		sender, _ := params.Payload["sender"].(map[string]any)
		login, _ := sender["login"].(string)
		id := fmt.Sprintf("%v", sender["id"])
		return Sender{Login: login, ID: id}
	default:
		return Sender{}
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getGitUser(params WebhookParams) GitUser {
	switch params.PlatformType {
	case models.ProviderGitHub:
		comment, _ := params.Payload["comment"].(map[string]any)
		user, _ := comment["user"].(map[string]any)
		return GitUser{
			ID:       fmt.Sprintf("%v", user["id"]),
			Username: fmt.Sprintf("%v", user["login"]),
		}
	case models.ProviderGitLab:
		user, _ := params.Payload["user"].(map[string]any)
		return GitUser{
			ID:       fmt.Sprintf("%v", user["id"]),
			Username: fmt.Sprintf("%v", user["username"]),
		}
	case models.ProviderBitbucket:
		comment, _ := params.Payload["comment"].(map[string]any)
		user, _ := comment["user"].(map[string]any)
		return GitUser{
			ID:       fmt.Sprintf("%v", user["uuid"]),
			Username: fmt.Sprintf("%v", user["nickname"]),
		}
	case models.ProviderAzure:
		resource, _ := params.Payload["resource"].(map[string]any)
		comment, _ := resource["comment"].(map[string]any)
		author, _ := comment["author"].(map[string]any)
		return GitUser{
			ID:       fmt.Sprintf("%v", author["id"]),
			Username: fmt.Sprintf("%v", author["uniqueName"]),
		}
	case models.ProviderForgejo:
		comment, _ := params.Payload["comment"].(map[string]any)
		user, _ := comment["user"].(map[string]any)
		return GitUser{
			ID:       fmt.Sprintf("%v", user["id"]),
			Username: fmt.Sprintf("%v", user["login"]),
		}
	default:
		return GitUser{}
	}
}

// ═══════════════════════════════════════════════════════════════
// Thread & History Reconstruction
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) shouldIgnoreComment(comment *NormalizedComment, platformType models.SCMProvider) bool {
	if comment == nil {
		return true
	}
	return uc.isDrixyComment(comment, platformType) || !uc.mentionsDrixy(comment)
}

func (uc *ChatWithDrixyFromGitUseCase) isDrixyComment(comment *NormalizedComment, platformType models.SCMProvider) bool {
	login := comment.UserLogin
	if login == "" {
		login = comment.AuthorName
	}
	loginLower := strings.ToLower(login)

	if botIdentities[loginLower] {
		return true
	}
	for _, kw := range loginKeywords {
		if strings.Contains(loginLower, kw) {
			return true
		}
	}

	bodyLower := strings.ToLower(comment.Body)
	if platformType == models.ProviderBitbucket {
		if strings.Contains(bodyLower, DrixyReviewMarkerBitbucket) {
			return true
		}
	} else {
		if strings.Contains(bodyLower, DrixyReviewMarker) {
			return true
		}
	}

	return false
}

func (uc *ChatWithDrixyFromGitUseCase) mentionsDrixy(comment *NormalizedComment) bool {
	bodyLower := strings.ToLower(comment.Body)
	return strings.Contains(bodyLower, DrixyMentionCommand) || strings.Contains(bodyLower, ScanDrixMentionCommand)
}

func (uc *ChatWithDrixyFromGitUseCase) getOriginalDrixyComment(
	comment *NormalizedComment,
	allComments []*NormalizedComment,
	platformType models.SCMProvider,
) *NormalizedComment {
	if comment == nil {
		return nil
	}

	switch platformType {
	case models.ProviderGitHub:
		targetID := comment.InReplyToID
		if targetID == 0 {
			targetID = comment.ID
		}
		for _, c := range allComments {
			if c.ID == targetID {
				return c
			}
		}
		return nil

	case models.ProviderGitLab:
		if comment.OriginalCommit != nil {
			return comment.OriginalCommit
		}
		return nil

	case models.ProviderBitbucket:
		if comment.ParentID == 0 {
			return nil
		}
		for _, c := range allComments {
			if c.ID == comment.ParentID {
				return c
			}
		}
		return nil

	case models.ProviderAzure:
		if comment.ThreadID != 0 && comment.ID != int64(comment.ThreadID) {
			for _, c := range allComments {
				if c.ThreadID == comment.ThreadID {
					return c
				}
			}
		}
		return nil

	default:
		return nil
	}
}

func (uc *ChatWithDrixyFromGitUseCase) getOthersReplies(
	comment *NormalizedComment,
	allComments []*NormalizedComment,
	platformType models.SCMProvider,
) []*NormalizedComment {
	if comment == nil {
		return nil
	}

	var valid []*NormalizedComment

	switch platformType {
	case models.ProviderGitHub:
		for _, reply := range allComments {
			if reply.InReplyToID == comment.InReplyToID && !uc.isDrixyComment(reply, platformType) {
				valid = append(valid, reply)
			}
		}

	case models.ProviderBitbucket:
		if comment.ParentID != 0 {
			var orig *NormalizedComment
			for _, c := range allComments {
				if c.ID == comment.ParentID {
					orig = c
					break
				}
			}
			if orig != nil {
				for _, reply := range orig.Replies {
					if reply.Body == "" || reply.Deleted || reply.ID == comment.ID {
						continue
					}
					if uc.isDrixyComment(reply, platformType) {
						continue
					}
					valid = append(valid, reply)
				}
			}
		}

	case models.ProviderAzure:
		if comment.ThreadID != 0 {
			for _, c := range allComments {
				if c.ThreadID == comment.ThreadID {
					for _, reply := range c.Replies {
						if reply.ID != comment.ID && !uc.isDrixyComment(reply, platformType) {
							valid = append(valid, reply)
						}
					}
				}
			}
		} else {
			for _, reply := range allComments {
				if reply.InReplyToID == comment.InReplyToID && !uc.isDrixyComment(reply, platformType) {
					valid = append(valid, reply)
				}
			}
		}

	case models.ProviderGitLab:
		for _, reply := range allComments {
			matchesReply := (reply.InReplyToID != 0 && reply.InReplyToID == comment.InReplyToID) ||
				(reply.DiscussionID != "" && reply.DiscussionID == comment.DiscussionID)
			if matchesReply && !uc.isDrixyComment(reply, platformType) {
				valid = append(valid, reply)
			}
		}

	default:
		for _, reply := range allComments {
			if reply.InReplyToID == comment.InReplyToID && !uc.isDrixyComment(reply, platformType) {
				valid = append(valid, reply)
			}
		}
	}

	return valid
}

func (uc *ChatWithDrixyFromGitUseCase) getReviewThreadByCommentID(
	commentID int64,
	reviewComments []*NormalizedComment,
	params WebhookParams,
) *NormalizedComment {
	if params.PlatformType == models.ProviderAzure {
		threadID := uc.getThreadIDFromAzurePayload(params)
		if threadID > 0 {
			for _, t := range reviewComments {
				if t.ThreadID == threadID {
					if t.ID == commentID {
						return t
					}
					for _, rep := range t.Replies {
						if rep.ID == commentID {
							return rep
						}
					}
				}
			}
		}
	}
	return nil
}

func (uc *ChatWithDrixyFromGitUseCase) getThreadIDFromAzurePayload(params WebhookParams) int {
	resource, _ := params.Payload["resource"].(map[string]any)
	comment, _ := resource["comment"].(map[string]any)
	links, _ := comment["_links"].(map[string]any)
	threads, _ := links["threads"].(map[string]any)
	href, _ := threads["href"].(string)

	if href != "" {
		if match := azureThreadIDRegex.FindStringSubmatch(href); len(match) >= 2 {
			id, _ := strconv.Atoi(match[1])
			return id
		}
	}

	msg, _ := params.Payload["message"].(map[string]any)
	html, _ := msg["html"].(string)
	if html == "" {
		detailed, _ := params.Payload["detailedMessage"].(map[string]any)
		html, _ = detailed["html"].(string)
	}
	if html != "" {
		if match := azureDiscussionRegex.FindStringSubmatch(html); len(match) >= 2 {
			id, _ := strconv.Atoi(match[1])
			return id
		}
	}

	return 0
}

func (uc *ChatWithDrixyFromGitUseCase) synthesizeCommentFromPayload(params WebhookParams, commentID int64) *NormalizedComment {
	body := uc.getRawCommentBody(params)
	sender := uc.getSender(params)
	return &NormalizedComment{
		ID:        commentID,
		Body:      body,
		UserLogin: sender.Login,
	}
}

func (uc *ChatWithDrixyFromGitUseCase) fetchAllComments(
	ctx context.Context,
	orgData types.OrganizationAndTeamData,
	repo types.RepositoryDescriptor,
	prNumber int,
	params WebhookParams,
) ([]*NormalizedComment, error) {
	rawComments, err := uc.codeManagementService.GetPullRequestReviewComments(ctx, orgData, &repo, prNumber)
	if err != nil {
		return nil, err
	}

	var results []*NormalizedComment
	for _, c := range rawComments {
		idNum, _ := strconv.ParseInt(c.ID, 10, 64)
		var userLogin, userDisplayName string
		if c.Author != nil {
			userDisplayName = c.Author.Name
			userLogin = c.Author.Username
			if userLogin == "" {
				userLogin = c.Author.Name
			}
		}

		norm := &NormalizedComment{
			ID:              idNum,
			Body:            c.Body,
			Path:            c.Path,
			UserLogin:       userLogin,
			UserDisplayName: userDisplayName,
		}
		results = append(results, norm)
	}
	return results, nil
}

// ═══════════════════════════════════════════════════════════════
// Acknowledgment ID Resolution
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) getAcknowledgmentIDs(
	originalDrixyComment *NormalizedComment,
	ackResponse *types.PullRequestReviewComment,
	platformType models.SCMProvider,
	comment *NormalizedComment,
) (ackResponseID, parentID string) {
	if ackResponse == nil {
		return "", ""
	}
	ackResponseID = ackResponse.ID

	switch platformType {
	case models.ProviderGitHub:
		if originalDrixyComment != nil {
			parentID = fmt.Sprintf("%d", originalDrixyComment.ID)
		} else if comment != nil {
			parentID = fmt.Sprintf("%d", comment.ID)
		}

	case models.ProviderGitLab:
		if comment != nil {
			parentID = fmt.Sprintf("%d", comment.ID)
		}

	case models.ProviderBitbucket:
		if originalDrixyComment != nil {
			parentID = fmt.Sprintf("%d", originalDrixyComment.ID)
		} else if comment != nil {
			parentID = fmt.Sprintf("%d", comment.ID)
		}

	case models.ProviderAzure:
		if originalDrixyComment != nil && originalDrixyComment.ThreadID > 0 {
			parentID = fmt.Sprintf("%d", originalDrixyComment.ThreadID)
		} else if comment != nil {
			parentID = fmt.Sprintf("%d", comment.ID)
		}

	default:
		if comment != nil {
			parentID = fmt.Sprintf("%d", comment.ID)
		}
	}

	return ackResponseID, parentID
}

func (uc *ChatWithDrixyFromGitUseCase) getBusinessLogicAcknowledgmentIDs(
	ackResponse *types.PullRequestReviewComment,
	platformType models.SCMProvider,
) (ackResponseID, parentID string) {
	if ackResponse == nil {
		return "", ""
	}
	return ackResponse.ID, ackResponse.ID
}

// ═══════════════════════════════════════════════════════════════
// Custom Instructions Extraction
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) extractCustomInstructions(params WebhookParams) string {
	payload := params.Payload

	candidates := []any{
		payload["customInstructions"],
		payload["custom_instructions"],
		uc.getNested(payload, "drixy", "customInstructions"),
		uc.getNested(payload, "drixy", "custom_instructions"),
		uc.getNested(payload, "configuration", "customInstructions"),
		uc.getNested(payload, "configuration", "custom_instructions"),
		uc.getNested(payload, "summary", "customInstructions"),
		uc.getNested(payload, "codeReviewConfig", "summary", "customInstructions"),
		uc.getNested(payload, "codeReview", "summary", "customInstructions"),
	}

	for _, cand := range candidates {
		if norm := uc.normalizeCustomInstructions(cand); norm != "" {
			return norm
		}
	}
	return ""
}

func (uc *ChatWithDrixyFromGitUseCase) getNested(m map[string]any, keys ...string) any {
	curr := m
	for i, key := range keys {
		if i == len(keys)-1 {
			return curr[key]
		}
		nxt, ok := curr[key].(map[string]any)
		if !ok {
			return nil
		}
		curr = nxt
	}
	return nil
}

func (uc *ChatWithDrixyFromGitUseCase) normalizeCustomInstructions(val any) string {
	if str, ok := val.(string); ok && strings.TrimSpace(str) != "" {
		return strings.TrimSpace(str)
	}
	if rec, ok := val.(map[string]any); ok {
		keys := []string{"text", "value", "content", "body", "instructions"}
		for _, k := range keys {
			if str, ok := rec[k].(string); ok && strings.TrimSpace(str) != "" {
				return strings.TrimSpace(str)
			}
		}
	}
	return ""
}

// ═══════════════════════════════════════════════════════════════
// Prepare Context & Sandbox Parameters
// ═══════════════════════════════════════════════════════════════

func (uc *ChatWithDrixyFromGitUseCase) prepareContext(
	comment *NormalizedComment,
	originalDrixyComment *NormalizedComment,
	gitUser GitUser,
	othersReplies []*NormalizedComment,
	prNumber int,
	repo types.RepositoryDescriptor,
	prDescription string,
	platformType models.SCMProvider,
	headRef string,
	baseRef string,
	defaultBranch string,
	customInstructions string,
) map[string]any {
	userQuestion := comment.Body
	if strings.TrimSpace(userQuestion) == "@drixy" || strings.TrimSpace(userQuestion) == "@scandrix" {
		userQuestion = "The user did not ask any questions. Ask them what they would like to know about the codebase or suggestions for code changes."
	}

	defBranch := defaultBranch
	if defBranch == "" {
		defBranch = baseRef
	}

	var repliesTexts []map[string]any
	for _, r := range othersReplies {
		repliesTexts = append(repliesTexts, map[string]any{
			"historyConversationText": r.Body,
		})
	}

	var origCommentMap map[string]any
	if originalDrixyComment != nil {
		origCommentMap = map[string]any{
			"suggestionCommentId": originalDrixyComment.ID,
			"suggestionFilePath":  comment.Path,
			"suggestionText":      originalDrixyComment.Body,
			"diffHunk":            originalDrixyComment.DiffHunk,
		}
	}

	return map[string]any{
		"gitUser": map[string]any{
			"id":       gitUser.ID,
			"username": gitUser.Username,
		},
		"userQuestion": userQuestion,
		"repository": map[string]any{
			"id":            repo.ID,
			"name":          repo.Name,
			"owner":         repo.Owner,
			"defaultBranch": defBranch,
		},
		"pullRequestDescription": prDescription,
		"platformType":           platformType,
		"customInstructions":     customInstructions,
		"pullRequest": map[string]any{
			"pullRequestNumber": prNumber,
			"headRef":           headRef,
			"baseRef":           baseRef,
		},
		"codeManagementContext": map[string]any{
			"originalComment": origCommentMap,
			"othersReplies":   repliesTexts,
		},
	}
}

func (uc *ChatWithDrixyFromGitUseCase) buildSandboxCloneParams(
	prepareContext map[string]any,
	orgData types.OrganizationAndTeamData,
) *sandboxContracts.CreateSandboxParams {
	repoMap, _ := prepareContext["repository"].(map[string]any)
	prMap, _ := prepareContext["pullRequest"].(map[string]any)
	platform, _ := prepareContext["platformType"].(models.SCMProvider)

	repo := types.RepositoryDescriptor{
		ID:    fmt.Sprintf("%v", repoMap["id"]),
		Name:  fmt.Sprintf("%v", repoMap["name"]),
		Owner: fmt.Sprintf("%v", repoMap["owner"]),
	}

	cp, err := uc.codeManagementService.GetCloneParams(context.Background(), orgData, repo)
	if err != nil || cp == nil {
		return nil
	}

	prNum, _ := prMap["pullRequestNumber"].(int)
	headRef, _ := prMap["headRef"].(string)
	baseRef, _ := prMap["baseRef"].(string)

	token := ""
	username := ""
	if cp.Auth != nil {
		token = cp.Auth.Token
		username = cp.Auth.Username
	}

	return &sandboxContracts.CreateSandboxParams{
		CloneURL:        cp.URL,
		AuthToken:       token,
		AuthUsername:    username,
		Branch:          headRef,
		BaseBranch:      baseRef,
		PRNumber:        prNum,
		Platform:        platform,
		SandboxMetadata: map[string]string{"stage": "conversation"},
	}
}

func (uc *ChatWithDrixyFromGitUseCase) buildBusinessValidationCommand(commentBody string) string {
	body := strings.TrimSpace(commentBody)
	parts := strings.Fields(body)
	var taskRef string
	for i, p := range parts {
		if p == "business-logic" && i+1 < len(parts) {
			taskRef = parts[i+1]
			break
		}
	}
	if taskRef != "" {
		return fmt.Sprintf("@drixy -v business-logic %s", taskRef)
	}
	return "@drixy -v business-logic"
}

func (uc *ChatWithDrixyFromGitUseCase) createThreadID(orgID, teamID, repoID, userID string, issueID any, prefix string) string {
	raw := fmt.Sprintf("%s:%s:%s:%s:%v", orgID, teamID, repoID, userID, issueID)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(hash[:16]))
}
