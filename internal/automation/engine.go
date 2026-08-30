package automation

import (
	"context"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine evaluates automation rules against incoming pull request events.
type Engine struct {
	mu    sync.RWMutex
	rules map[uuid.UUID][]AutomationRule
}

func NewEngine() *Engine {
	return &Engine{
		rules: make(map[uuid.UUID][]AutomationRule),
	}
}

// AddRule registers a new automation rule under a workspace.
func (e *Engine) AddRule(rule AutomationRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules[rule.WorkspaceID] = append(e.rules[rule.WorkspaceID], rule)
}

// Evaluate checks workspace rules against the event context and returns triggered actions.
func (e *Engine) Evaluate(ctx context.Context, triggerCtx TriggerContext) ([]ExecutedAction, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	workspaceRules := e.rules[triggerCtx.WorkspaceID]
	var executed []ExecutedAction

	for _, rule := range workspaceRules {
		if !rule.Enabled || rule.Trigger != triggerCtx.Trigger {
			continue
		}

		if !e.matchesCondition(rule.Conditions, triggerCtx) {
			continue
		}

		for _, action := range rule.Actions {
			executed = append(executed, ExecutedAction{
				RuleID:     rule.ID,
				RuleName:   rule.Name,
				ActionType: action.Type,
				Parameters: action.Parameters,
				ExecutedAt: time.Now().UTC(),
			})
		}
	}

	return executed, nil
}

func (e *Engine) matchesCondition(cond RuleCondition, triggerCtx TriggerContext) bool {
	// 1. Branch match
	if cond.BranchPattern != "" {
		if !matchPattern(cond.BranchPattern, triggerCtx.TargetBranch) {
			return false
		}
	}

	// 2. Author match
	if cond.AuthorRegex != "" {
		re, err := regexp.Compile(cond.AuthorRegex)
		if err == nil && !re.MatchString(triggerCtx.Author) {
			return false
		}
	}

	// 3. File path glob match
	if len(cond.PathGlobs) > 0 {
		matchedAny := false
		for _, pattern := range cond.PathGlobs {
			for _, file := range triggerCtx.ChangedFiles {
				if matchGlob(pattern, file) {
					matchedAny = true
					break
				}
			}
			if matchedAny {
				break
			}
		}
		if !matchedAny {
			return false
		}
	}

	return true
}

func matchPattern(pattern, value string) bool {
	if pattern == "*" || pattern == value {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(value, prefix+"/")
	}
	matched, _ := path.Match(pattern, value)
	return matched
}

func matchGlob(pattern, filePath string) bool {
	// Support recursive ** matching
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		prefix := parts[0]
		suffix := ""
		if len(parts) > 1 {
			suffix = strings.TrimPrefix(parts[1], "/")
		}

		if prefix != "" && !strings.HasPrefix(filePath, prefix) {
			return false
		}
		if suffix != "" {
			if strings.HasPrefix(suffix, "*.") {
				ext := suffix[1:]
				return strings.HasSuffix(filePath, ext)
			}
			return strings.HasSuffix(filePath, suffix)
		}
		return true
	}

	matched, _ := path.Match(pattern, filePath)
	if matched {
		return true
	}
	base := path.Base(filePath)
	matchedBase, _ := path.Match(pattern, base)
	return matchedBase
}
