package teams

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// TeamsActionInvoke models incoming Teams Bot Framework / Adaptive Card invoke payloads.
type TeamsActionInvoke struct {
	Type           string                 `json:"type"`
	Name           string                 `json:"name"`
	From           TeamsChannelAccount    `json:"from"`
	Conversation   TeamsConversation      `json:"conversation"`
	Value          TeamsActionDataWrapper `json:"value"`
	ChannelData    map[string]any         `json:"channelData,omitempty"`
	ServiceURL     string                 `json:"serviceUrl,omitempty"`
}

type TeamsChannelAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	AadObjectID string `json:"aadObjectId,omitempty"`
}

type TeamsConversation struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type TeamsActionDataWrapper struct {
	Action      string         `json:"action"`
	ActionName  string         `json:"verb,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	PRNumber    int            `json:"pr_number,omitempty"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
}

// ParseTeamsAction parses incoming Teams Action payload from JSON body.
func ParseTeamsAction(r *http.Request, rawBody []byte) (*TeamsActionInvoke, error) {
	var invoke TeamsActionInvoke
	if err := json.Unmarshal(rawBody, &invoke); err != nil {
		return nil, fmt.Errorf("failed to unmarshal teams action: %w", err)
	}

	// Normalize action name from invoke
	if invoke.Value.Action == "" && invoke.Value.ActionName != "" {
		invoke.Value.Action = invoke.Value.ActionName
	}
	if invoke.Value.Action == "" && invoke.Value.Data != nil {
		if act, ok := invoke.Value.Data["action"].(string); ok {
			invoke.Value.Action = act
		}
		if repo, ok := invoke.Value.Data["repo"].(string); ok {
			invoke.Value.Repo = repo
		}
		if pr, ok := invoke.Value.Data["pr_number"].(float64); ok {
			invoke.Value.PRNumber = int(pr)
		}
	}

	return &invoke, nil
}
