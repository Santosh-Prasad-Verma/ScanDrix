package clireview_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/clireview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSessionEventRepository struct {
	eventsByUUID     map[string]*clireview.SessionEvent
	eventsBySession  map[string][]*clireview.SessionEvent
	skippedCalls     []struct{ uuid, reason string }
	processingCalls  []string
	completedCalls   []struct {
		uuid      string
		decisions []clireview.CliSessionClassifiedDecision
		source    string
	}
	failedCalls []struct{ uuid, err string }
	failOnComplete   bool
}

func newMockSessionEventRepo() *mockSessionEventRepository {
	return &mockSessionEventRepository{
		eventsByUUID:    make(map[string]*clireview.SessionEvent),
		eventsBySession: make(map[string][]*clireview.SessionEvent),
	}
}

func (m *mockSessionEventRepository) Create(ctx context.Context, event *clireview.SessionEvent) (*clireview.SessionEvent, error) {
	m.eventsByUUID[event.UUID] = event
	m.eventsBySession[event.SessionID] = append(m.eventsBySession[event.SessionID], event)
	return event, nil
}

func (m *mockSessionEventRepository) FindByUUID(ctx context.Context, uuid string) (*clireview.SessionEvent, error) {
	return m.eventsByUUID[uuid], nil
}

func (m *mockSessionEventRepository) FindBySessionID(ctx context.Context, sessionID, orgID string) ([]*clireview.SessionEvent, error) {
	return m.eventsBySession[sessionID], nil
}

func (m *mockSessionEventRepository) MarkClassificationProcessing(ctx context.Context, uuid string) error {
	m.processingCalls = append(m.processingCalls, uuid)
	return nil
}

func (m *mockSessionEventRepository) MarkClassificationCompleted(ctx context.Context, uuid string, decisions []clireview.CliSessionClassifiedDecision, source string) error {
	if m.failOnComplete {
		return errors.New("DB write failed")
	}
	m.completedCalls = append(m.completedCalls, struct {
		uuid      string
		decisions []clireview.CliSessionClassifiedDecision
		source    string
	}{uuid: uuid, decisions: decisions, source: source})
	return nil
}

func (m *mockSessionEventRepository) MarkClassificationFailed(ctx context.Context, uuid, errorMsg string) error {
	m.failedCalls = append(m.failedCalls, struct{ uuid, err string }{uuid: uuid, err: errorMsg})
	return nil
}

func (m *mockSessionEventRepository) MarkClassificationSkipped(ctx context.Context, uuid, reason string) error {
	m.skippedCalls = append(m.skippedCalls, struct{ uuid, reason string }{uuid: uuid, reason: reason})
	return nil
}

func (m *mockSessionEventRepository) FindOrphanedSessions(ctx context.Context, inactivityMinutes int, limit int) ([]clireview.OrphanedSessionRef, error) {
	return nil, nil
}

type mockLLMDecisionClient struct {
	decisions   []clireview.CliSessionClassifiedDecision
	err         error
	calls       []string
	lastPayload map[string]any
}

func (m *mockLLMDecisionClient) ExtractDecisions(ctx context.Context, systemPrompt, userPayload string) ([]clireview.CliSessionClassifiedDecision, error) {
	m.calls = append(m.calls, userPayload)
	var p map[string]any
	_ = json.Unmarshal([]byte(userPayload), &p)
	m.lastPayload = p

	if m.err != nil {
		return nil, m.err
	}
	return m.decisions, nil
}

func makeSessionEvent(uuid, sessionID, eventType string, payload map[string]any) *clireview.SessionEvent {
	return &clireview.SessionEvent{
		UUID:           uuid,
		OrganizationID: "org-1",
		TeamID:         "team-1",
		SessionID:      sessionID,
		EventType:      eventType,
		Branch:         "main",
		EventTimestamp: time.Now(),
		Payload:        payload,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func TestClassifySessionUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("should skip if event not found", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		llm := &mockLLMDecisionClient{}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "missing-uuid")
		require.NoError(t, err)
		assert.Empty(t, repo.skippedCalls)
		assert.Empty(t, repo.processingCalls)
	})

	t.Run("should skip if event type is not session_end", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["evt-1"] = makeSessionEvent("evt-1", "sess-1", "turn_start", map[string]any{})
		llm := &mockLLMDecisionClient{}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "evt-1")
		require.NoError(t, err)
		require.Len(t, repo.skippedCalls, 1)
		assert.Equal(t, "evt-1", repo.skippedCalls[0].uuid)
		assert.Contains(t, repo.skippedCalls[0].reason, "Unsupported event type")
	})

	t.Run("should skip if no useful content in session", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-1"] = makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{})
		repo.eventsBySession["sess-1"] = []*clireview.SessionEvent{
			makeSessionEvent("start-1", "sess-1", "session_start", map[string]any{}),
			makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{}),
		}
		llm := &mockLLMDecisionClient{}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-1")
		require.NoError(t, err)
		require.Len(t, repo.skippedCalls, 1)
		assert.Equal(t, "end-1", repo.skippedCalls[0].uuid)
		assert.Equal(t, "No textual context for classification", repo.skippedCalls[0].reason)
	})

	t.Run("should call LLM and mark completed on success", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-1"] = makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{})
		repo.eventsBySession["sess-1"] = []*clireview.SessionEvent{
			makeSessionEvent("start-1", "sess-1", "session_start", map[string]any{"agentType": "claude-code"}),
			makeSessionEvent("t-start", "sess-1", "turn_start", map[string]any{"prompt": "Add authentication to the API"}),
			makeSessionEvent("t-end", "sess-1", "turn_end", map[string]any{
				"toolCalls":     []any{map[string]any{"tool": "Edit", "summary": "edited auth.ts"}},
				"filesModified": []any{"src/auth.ts"},
			}),
			makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{
			decisions: []clireview.CliSessionClassifiedDecision{
				{
					Type:       "implementation_detail",
					Decision:   "Use JWT for API authentication",
					Confidence: 0.85,
				},
			},
		}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-1")
		require.NoError(t, err)
		require.Contains(t, repo.processingCalls, "end-1")
		require.Len(t, repo.completedCalls, 1)
		assert.Equal(t, "end-1", repo.completedCalls[0].uuid)
		assert.Equal(t, "llm", repo.completedCalls[0].source)
		require.Len(t, repo.completedCalls[0].decisions, 1)
		assert.Equal(t, "Use JWT for API authentication", repo.completedCalls[0].decisions[0].Decision)
	})

	t.Run("should fallback to heuristics when LLM returns empty", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-1"] = makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{})
		repo.eventsBySession["sess-1"] = []*clireview.SessionEvent{
			makeSessionEvent("t-start", "sess-1", "turn_start", map[string]any{
				"prompt": "We decided to use Redis for caching instead of Memcached",
			}),
			makeSessionEvent("t-end", "sess-1", "turn_end", map[string]any{
				"filesModified": []any{"src/cache.ts"},
			}),
			makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{decisions: []clireview.CliSessionClassifiedDecision{}}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-1")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		assert.Equal(t, "end-1", repo.completedCalls[0].uuid)
		assert.Equal(t, "heuristic-fallback", repo.completedCalls[0].source)
		assert.NotEmpty(t, repo.completedCalls[0].decisions)
	})

	t.Run("should fallback to heuristics when LLM throws", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-1"] = makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{})
		repo.eventsBySession["sess-1"] = []*clireview.SessionEvent{
			makeSessionEvent("t-start", "sess-1", "turn_start", map[string]any{
				"prompt": "Adopt convention: always use snake_case for DB columns",
			}),
			makeSessionEvent("t-end", "sess-1", "turn_end", map[string]any{
				"filesModified": []any{"src/db.ts"},
			}),
			makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{err: errors.New("LLM timeout")}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-1")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		assert.Equal(t, "end-1", repo.completedCalls[0].uuid)
		assert.Equal(t, "heuristic-fallback", repo.completedCalls[0].source)
	})

	t.Run("should mark failed when both LLM and heuristics persistence throw", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.failOnComplete = true
		repo.eventsByUUID["end-1"] = makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{})
		repo.eventsBySession["sess-1"] = []*clireview.SessionEvent{
			makeSessionEvent("t-start", "sess-1", "turn_start", map[string]any{"prompt": "do something"}),
			makeSessionEvent("end-1", "sess-1", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{err: errors.New("LLM down")}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-1")
		require.Error(t, err)
		require.Len(t, repo.failedCalls, 1)
		assert.Equal(t, "end-1", repo.failedCalls[0].uuid)
		assert.Equal(t, "DB write failed", repo.failedCalls[0].err)
	})
}

func TestClassifySessionUseCase_HeuristicTypeInference(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		prompt       string
		expectedType clireview.CliSessionDecisionType
	}{
		{"We decided to use a microservice architecture", "architectural_decision"},
		{"Convention: always use snake_case for DB columns", "convention"},
		{"Used Redis instead of Memcached because of better pub/sub", "tradeoff"},
		{"Added express framework as dependency", "tooling"},
		{"Implemented JWT validation middleware", "implementation_detail"},
	}

	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			repo := newMockSessionEventRepo()
			repo.eventsByUUID["end-h"] = makeSessionEvent("end-h", "sess-h", "session_end", map[string]any{})
			repo.eventsBySession["sess-h"] = []*clireview.SessionEvent{
				makeSessionEvent("start", "sess-h", "session_start", map[string]any{}),
				makeSessionEvent("turn-s", "sess-h", "turn_start", map[string]any{"prompt": tc.prompt}),
				makeSessionEvent("turn-e", "sess-h", "turn_end", map[string]any{"filesModified": []any{"src/file.ts"}}),
				makeSessionEvent("end-h", "sess-h", "session_end", map[string]any{}),
			}

			llm := &mockLLMDecisionClient{err: errors.New("LLM unavailable")}
			uc := clireview.NewClassifySessionUseCase(repo, llm)

			err := uc.Execute(ctx, "end-h")
			require.NoError(t, err)
			require.Len(t, repo.completedCalls, 1)
			assert.Equal(t, "end-h", repo.completedCalls[0].uuid)
			assert.Equal(t, "heuristic-fallback", repo.completedCalls[0].source)
			require.NotEmpty(t, repo.completedCalls[0].decisions)
			assert.Equal(t, tc.expectedType, repo.completedCalls[0].decisions[0].Type)
		})
	}
}

func TestClassifySessionUseCase_AutoPromote(t *testing.T) {
	ctx := context.Background()

	t.Run("should set autoPromoteCandidate=true for high-confidence promotable types via LLM", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-ap"] = makeSessionEvent("end-ap", "sess-ap", "session_end", map[string]any{})
		repo.eventsBySession["sess-ap"] = []*clireview.SessionEvent{
			makeSessionEvent("t-s", "sess-ap", "turn_start", map[string]any{"prompt": "Set up the architecture"}),
			makeSessionEvent("t-e", "sess-ap", "turn_end", map[string]any{"filesModified": []any{"src/arch.ts"}}),
			makeSessionEvent("end-ap", "sess-ap", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{
			decisions: []clireview.CliSessionClassifiedDecision{
				{Type: "architectural_decision", Decision: "Use event-driven architecture", Confidence: 0.9},
				{Type: "convention", Decision: "Always use camelCase", Confidence: 0.75},
				{Type: "tradeoff", Decision: "Chose SQL over NoSQL", Confidence: 0.7},
			},
		}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-ap")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		decisions := repo.completedCalls[0].decisions
		require.Len(t, decisions, 3)
		assert.True(t, decisions[0].AutoPromoteCandidate)
		assert.True(t, decisions[1].AutoPromoteCandidate)
		assert.True(t, decisions[2].AutoPromoteCandidate)
	})

	t.Run("should set autoPromoteCandidate=false for low-confidence promotable types", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-ap2"] = makeSessionEvent("end-ap2", "sess-ap2", "session_end", map[string]any{})
		repo.eventsBySession["sess-ap2"] = []*clireview.SessionEvent{
			makeSessionEvent("t-s", "sess-ap2", "turn_start", map[string]any{"prompt": "Some architecture work"}),
			makeSessionEvent("t-e", "sess-ap2", "turn_end", map[string]any{"filesModified": []any{"src/x.ts"}}),
			makeSessionEvent("end-ap2", "sess-ap2", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{
			decisions: []clireview.CliSessionClassifiedDecision{
				{Type: "architectural_decision", Decision: "Maybe use microservices", Confidence: 0.5},
			},
		}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-ap2")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		decisions := repo.completedCalls[0].decisions
		require.Len(t, decisions, 1)
		assert.False(t, decisions[0].AutoPromoteCandidate)
	})

	t.Run("should set autoPromoteCandidate=false for non-promotable types even with high confidence", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-ap3"] = makeSessionEvent("end-ap3", "sess-ap3", "session_end", map[string]any{})
		repo.eventsBySession["sess-ap3"] = []*clireview.SessionEvent{
			makeSessionEvent("t-s", "sess-ap3", "turn_start", map[string]any{"prompt": "Implement feature"}),
			makeSessionEvent("t-e", "sess-ap3", "turn_end", map[string]any{"filesModified": []any{"src/y.ts"}}),
			makeSessionEvent("end-ap3", "sess-ap3", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{
			decisions: []clireview.CliSessionClassifiedDecision{
				{Type: "implementation_detail", Decision: "Use singleton pattern", Confidence: 0.95},
				{Type: "tooling", Decision: "Use webpack", Confidence: 0.8},
				{Type: "other", Decision: "Some other choice", Confidence: 0.9},
			},
		}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-ap3")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		for _, d := range repo.completedCalls[0].decisions {
			assert.False(t, d.AutoPromoteCandidate)
		}
	})

	t.Run("heuristic fallback decisions always have autoPromoteCandidate=false", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-h"] = makeSessionEvent("end-h", "sess-h", "session_end", map[string]any{})
		repo.eventsBySession["sess-h"] = []*clireview.SessionEvent{
			makeSessionEvent("t-s", "sess-h", "turn_start", map[string]any{
				"prompt": "We decided to use a microservice architecture",
			}),
			makeSessionEvent("t-e", "sess-h", "turn_end", map[string]any{"filesModified": []any{"src/file.ts"}}),
			makeSessionEvent("end-h", "sess-h", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{err: errors.New("LLM unavailable")}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-h")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		for _, d := range repo.completedCalls[0].decisions {
			assert.False(t, d.AutoPromoteCandidate)
		}
	})
}

func TestClassifySessionUseCase_AggregateEventsEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("should SKIP session with only session_start and session_end", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-empty"] = makeSessionEvent("end-empty", "sess-empty", "session_end", map[string]any{})
		repo.eventsBySession["sess-empty"] = []*clireview.SessionEvent{
			makeSessionEvent("start", "sess-empty", "session_start", map[string]any{
				"agentType": "claude-code",
				"gitRemote": "github.com/scandrix/example",
			}),
			makeSessionEvent("end-empty", "sess-empty", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-empty")
		require.NoError(t, err)
		require.Len(t, repo.skippedCalls, 1)
		assert.Equal(t, "end-empty", repo.skippedCalls[0].uuid)
		assert.Equal(t, "No textual context for classification", repo.skippedCalls[0].reason)
		assert.Empty(t, repo.processingCalls)
	})

	t.Run("should SKIP session with empty prompts and no tool calls", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-blank"] = makeSessionEvent("end-blank", "sess-blank", "session_end", map[string]any{})
		repo.eventsBySession["sess-blank"] = []*clireview.SessionEvent{
			makeSessionEvent("start", "sess-blank", "session_start", map[string]any{}),
			makeSessionEvent("t1", "sess-blank", "turn_start", map[string]any{"prompt": ""}),
			makeSessionEvent("t2", "sess-blank", "turn_start", map[string]any{"prompt": "   "}),
			makeSessionEvent("te", "sess-blank", "turn_end", map[string]any{
				"toolCalls":     []any{},
				"filesModified": []any{},
				"commands":      []any{},
			}),
			makeSessionEvent("end-blank", "sess-blank", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-blank")
		require.NoError(t, err)
		require.Len(t, repo.skippedCalls, 1)
		assert.Equal(t, "end-blank", repo.skippedCalls[0].uuid)
		assert.Equal(t, "No textual context for classification", repo.skippedCalls[0].reason)
	})

	t.Run("should include subagent info in aggregation", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-sub"] = makeSessionEvent("end-sub", "sess-sub", "session_end", map[string]any{})
		repo.eventsBySession["sess-sub"] = []*clireview.SessionEvent{
			makeSessionEvent("start", "sess-sub", "session_start", map[string]any{}),
			makeSessionEvent("sub-1", "sess-sub", "subagent_start", map[string]any{
				"subagentType":    "code-review",
				"taskDescription": "Review auth module",
			}),
			makeSessionEvent("end-sub", "sess-sub", "session_end", map[string]any{}),
		}

		llm := &mockLLMDecisionClient{decisions: []clireview.CliSessionClassifiedDecision{}}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-sub")
		require.NoError(t, err)
		assert.Empty(t, repo.skippedCalls)
		require.Contains(t, repo.processingCalls, "end-sub")

		require.NotNil(t, llm.lastPayload)
		subagents, ok := llm.lastPayload["subagents"].([]any)
		require.True(t, ok)
		require.Len(t, subagents, 1)
		subMap := subagents[0].(map[string]any)
		assert.Equal(t, "code-review", subMap["type"])
		assert.Equal(t, "Review auth module", subMap["task"])
	})
}

func TestClassifySessionUseCase_LargeSessionHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("should handle session with 100+ turn events without crashing and slice context", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		repo.eventsByUUID["end-large"] = makeSessionEvent("end-large", "sess-large", "session_end", map[string]any{})

		var events []*clireview.SessionEvent
		events = append(events, makeSessionEvent("start", "sess-large", "session_start", map[string]any{"agentType": "claude-code"}))

		for i := 0; i < 120; i++ {
			events = append(events, makeSessionEvent(
				fmt.Sprintf("ts-%d", i), "sess-large", "turn_start",
				map[string]any{
					"turnId": fmt.Sprintf("turn-%d", i),
					"prompt": fmt.Sprintf("Task %d: refactor module %d", i, i),
				},
			))
			events = append(events, makeSessionEvent(
				fmt.Sprintf("te-%d", i), "sess-large", "turn_end",
				map[string]any{
					"turnId":        fmt.Sprintf("turn-%d", i),
					"response":      fmt.Sprintf("Done with task %d", i),
					"toolCalls":     []any{map[string]any{"tool": "Edit", "summary": fmt.Sprintf("edited file%d.ts", i)}},
					"filesModified": []any{fmt.Sprintf("src/module%d.ts", i)},
					"filesRead":     []any{fmt.Sprintf("src/module%d.ts", i)},
					"commands":      []any{fmt.Sprintf("yarn test module%d", i)},
				},
			))
		}
		events = append(events, makeSessionEvent("end-large", "sess-large", "session_end", map[string]any{}))
		repo.eventsBySession["sess-large"] = events

		llm := &mockLLMDecisionClient{
			decisions: []clireview.CliSessionClassifiedDecision{
				{Type: "implementation_detail", Decision: "Refactored all modules", Confidence: 0.6},
			},
		}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err := uc.Execute(ctx, "end-large")
		require.NoError(t, err)
		require.Len(t, repo.completedCalls, 1)
		assert.Equal(t, "end-large", repo.completedCalls[0].uuid)
		assert.Equal(t, "llm", repo.completedCalls[0].source)

		// Verify slicing
		require.NotNil(t, llm.lastPayload)
		turns, ok := llm.lastPayload["turns"].([]any)
		require.True(t, ok)
		assert.LessOrEqual(t, len(turns), 20)

		for _, turnAny := range turns {
			turnMap := turnAny.(map[string]any)
			tc := turnMap["toolCalls"].([]any)
			fm := turnMap["filesModified"].([]any)
			assert.LessOrEqual(t, len(tc), 5)
			assert.LessOrEqual(t, len(fm), 5)
		}

		filesMod, ok := llm.lastPayload["filesModified"].([]any)
		require.True(t, ok)
		assert.LessOrEqual(t, len(filesMod), 30)

		filesRead, ok := llm.lastPayload["filesRead"].([]any)
		require.True(t, ok)
		assert.LessOrEqual(t, len(filesRead), 20)

		commands, ok := llm.lastPayload["commands"].([]any)
		require.True(t, ok)
		assert.LessOrEqual(t, len(commands), 20)
	})
}

func TestClassifySessionUseCase_DuplicateSessionEnd(t *testing.T) {
	ctx := context.Background()

	t.Run("should classify two session_end events for the same session independently", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		sharedEvents := []*clireview.SessionEvent{
			makeSessionEvent("start", "sess-dup", "session_start", map[string]any{}),
			makeSessionEvent("ts", "sess-dup", "turn_start", map[string]any{
				"prompt": "We decided to adopt a monorepo convention",
			}),
			makeSessionEvent("te", "sess-dup", "turn_end", map[string]any{
				"filesModified": []any{"nx.json"},
			}),
			makeSessionEvent("end-dup-1", "sess-dup", "session_end", map[string]any{}),
			makeSessionEvent("end-dup-2", "sess-dup", "session_end", map[string]any{}),
		}
		repo.eventsByUUID["end-dup-1"] = sharedEvents[3]
		repo.eventsByUUID["end-dup-2"] = sharedEvents[4]
		repo.eventsBySession["sess-dup"] = sharedEvents

		llm := &mockLLMDecisionClient{err: errors.New("LLM unavailable")}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		err1 := uc.Execute(ctx, "end-dup-1")
		require.NoError(t, err1)
		require.Len(t, repo.completedCalls, 1)
		assert.Equal(t, "end-dup-1", repo.completedCalls[0].uuid)
		assert.Equal(t, "heuristic-fallback", repo.completedCalls[0].source)

		err2 := uc.Execute(ctx, "end-dup-2")
		require.NoError(t, err2)
		require.Len(t, repo.completedCalls, 2)
		assert.Equal(t, "end-dup-2", repo.completedCalls[1].uuid)
		assert.Equal(t, "heuristic-fallback", repo.completedCalls[1].source)
	})
}

func TestClassifySessionUseCase_ExtractWithLLMParity(t *testing.T) {
	ctx := context.Background()

	modelDecisions := []clireview.CliSessionClassifiedDecision{
		{
			Type:       "architectural_decision",
			Origin:     "human",
			Decision:   "Use event sourcing for the audit log",
			Rationale:  "Full auditability of every state change",
			Confidence: 0.9,
			Evidence:   []string{"src/audit/store.ts", "src/audit/replay.ts"},
		},
		{
			Type:       "tooling",
			Decision:   "Adopt pnpm as the package manager",
			Confidence: 0.4,
		},
	}

	aggregated := &clireview.AggregatedSession{
		AgentType: "claude-code",
		GitRemote: "git@github.com:scandrix/example.git",
		Turns: []clireview.SessionTurnPair{
			{
				Prompt:        "Design the audit log",
				Response:      "I chose event sourcing.",
				ToolCalls:     []string{"Edit"},
				FilesModified: []string{"src/audit/store.ts"},
			},
		},
		Prompts:       []string{"Design the audit log"},
		Responses:     []string{"I chose event sourcing."},
		ToolCalls:     []string{"Edit"},
		FilesModified: []string{"src/audit/store.ts", "src/audit/replay.ts"},
	}

	t.Run("maps model decisions byte-for-byte to CliSessionClassifiedDecision", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		llm := &mockLLMDecisionClient{decisions: modelDecisions}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		decisions, err := uc.ExtractWithLLM(ctx, aggregated, "org-123")
		require.NoError(t, err)
		require.Len(t, decisions, 2)

		d0 := decisions[0]
		assert.Equal(t, clireview.CliSessionDecisionType("architectural_decision"), d0.Type)
		assert.Equal(t, clireview.CliSessionDecisionOrigin("human"), d0.Origin)
		assert.Equal(t, "Use event sourcing for the audit log", d0.Decision)
		assert.Equal(t, "Full auditability of every state change", d0.Rationale)
		assert.Equal(t, 0.9, d0.Confidence)
		assert.Equal(t, []string{"src/audit/store.ts", "src/audit/replay.ts"}, d0.Evidence)
		// Falls back to filesModified when no scope is provided
		assert.Equal(t, []string{"src/audit/store.ts", "src/audit/replay.ts"}, d0.Scope)
		assert.True(t, d0.AutoPromoteCandidate)

		d1 := decisions[1]
		assert.Equal(t, clireview.CliSessionDecisionType("tooling"), d1.Type)
		assert.Equal(t, "Adopt pnpm as the package manager", d1.Decision)
		assert.Equal(t, 0.4, d1.Confidence)
		assert.Empty(t, d1.Evidence)
		assert.Equal(t, []string{"src/audit/store.ts", "src/audit/replay.ts"}, d1.Scope)
		assert.False(t, d1.AutoPromoteCandidate)
	})

	t.Run("empty decisions returns empty without error", func(t *testing.T) {
		repo := newMockSessionEventRepo()
		llm := &mockLLMDecisionClient{decisions: []clireview.CliSessionClassifiedDecision{}}
		uc := clireview.NewClassifySessionUseCase(repo, llm)

		decisions, err := uc.ExtractWithLLM(ctx, aggregated, "org-123")
		require.NoError(t, err)
		assert.Empty(t, decisions)
	})
}
