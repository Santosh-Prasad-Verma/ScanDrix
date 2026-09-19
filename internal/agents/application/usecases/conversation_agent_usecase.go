// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Application Layer
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/agents/conversation"
)

// ConversationAgentUseCase coordinates execution of multi-turn conversational turns.
type ConversationAgentUseCase struct {
	provider *conversation.ConversationAgentProvider
}

// NewConversationAgentUseCase creates a new ConversationAgentUseCase.
func NewConversationAgentUseCase(
	provider *conversation.ConversationAgentProvider,
) *ConversationAgentUseCase {
	return &ConversationAgentUseCase{
		provider: provider,
	}
}

// Execute processes a single interactive conversational turn.
func (uc *ConversationAgentUseCase) Execute(
	ctx context.Context,
	req conversation.ConversationRequest,
) (*conversation.ConversationResponse, error) {
	if uc.provider == nil {
		return nil, fmt.Errorf("conversation agent provider is not initialized")
	}

	res, err := uc.provider.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to process conversation turn: %w", err)
	}

	return res, nil
}
