package prompts

import (
	"encoding/json"
	"fmt"
)

// DrixyMemoryItem represents an engineering preference or learned rule.
type DrixyMemoryItem struct {
	UUID         string `json:"uuid,omitempty"`
	Title        string `json:"title"`
	Rule         string `json:"rule"`
	RepositoryID string `json:"repositoryId,omitempty"`
	DirectoryID  string `json:"directoryId,omitempty"`
	Path         string `json:"path,omitempty"`
}

// DrixyMemoryResolution describes curation action for a newly discovered memory.
type DrixyMemoryResolution struct {
	Action           string   `json:"action"` // "create" | "skip" | "update"
	TargetMemoryUUID string   `json:"targetMemoryUuid,omitempty"`
	UpdatedTitle     string   `json:"updatedTitle,omitempty"`
	UpdatedRule      string   `json:"updatedRule,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	Confidence       *float64 `json:"confidence,omitempty"`
}

// DrixyMemoryResolutionPayload wraps incoming and existing memories for curation.
type DrixyMemoryResolutionPayload struct {
	IncomingMemory   DrixyMemoryItem   `json:"incomingMemory"`
	ExistingMemories []DrixyMemoryItem `json:"existingMemories"`
}

// PromptDrixyMemoryResolutionSystem generates the system instructions for curating team memories.
func PromptDrixyMemoryResolutionSystem() string {
	return `You are a memory curator for engineering preferences.

You receive one incoming memory and a list of existing memories.
Your task is to decide exactly one action:
- create: incoming memory is new and should be created.
- skip: incoming memory already exists (duplicate/near-duplicate), so do not create.
- update: incoming memory is not new but is a refinement of an existing memory and should update it.

Rules:
1) Prefer skip for clear duplicates.
2) Use update only when there is a strong semantic match and incoming content meaningfully improves clarity/specificity.
3) If update, provide targetMemoryUuid and optionally updatedTitle/updatedRule.
4) If skip, provide targetMemoryUuid when possible.
5) If uncertain, choose create.

Return ONLY JSON matching the schema:
{
  "action": "create" | "skip" | "update",
  "targetMemoryUuid": "optional string",
  "updatedTitle": "optional string",
  "updatedRule": "optional string",
  "reason": "optional string",
  "confidence": 0.95
}`
}

// PromptDrixyMemoryResolutionUser generates the user prompt with memory payloads.
func PromptDrixyMemoryResolutionUser(payload DrixyMemoryResolutionPayload) string {
	incBytes, _ := json.MarshalIndent(payload.IncomingMemory, "", "  ")

	limit := len(payload.ExistingMemories)
	if limit > 50 {
		limit = 50
	}
	existBytes, _ := json.MarshalIndent(payload.ExistingMemories[:limit], "", "  ")

	return fmt.Sprintf(`Incoming memory:
%s

Existing memories:
%s

Decide one action: create, skip, or update.`, string(incBytes), string(existBytes))
}
