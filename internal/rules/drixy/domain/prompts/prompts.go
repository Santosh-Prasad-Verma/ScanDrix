// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: prompts.go
// ═══════════════════════════════════════════════════════════════

package prompts

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// ReviewCommentInput represents a substantive past review comment for rule synthesis.
type ReviewCommentInput struct {
	ID       string `json:"id"`
	Body     string `json:"body"`
	Language string `json:"language"`
	FilePath string `json:"filePath,omitempty"`
}

// RuleGeneratorOutput models the structured LLM output for generated rules.
type RuleGeneratorOutput struct {
	Rules []GeneratedRuleItem `json:"rules"`
}

type GeneratedRuleItem struct {
	UUID     string                         `json:"uuid,omitempty"`
	Title    string                         `json:"title"`
	Rule     string                         `json:"rule"`
	Severity string                         `json:"severity"`
	Scope    string                         `json:"scope,omitempty"`
	Examples []interfaces.DrixyRulesExample `json:"examples,omitempty"`
	Tags     []string                       `json:"tags,omitempty"`
}

// UUIDListOutput models structured UUID filtering output.
type UUIDListOutput struct {
	UUIDs []string `json:"uuids"`
}

// DrixyRulesGeneratorSystem returns the system prompt for synthesizing rules from past review comments.
func DrixyRulesGeneratorSystem() string {
	return `You are the **ScanDrix Rule Synthesizer** — an expert knowledge distillation engine that converts patterns observed in past code review discussions into permanent, actionable organizational coding standards (drixyRules).

---

## MISSION

Given a batch of historical code review comments/suggestions and an existing catalog of pre-defined library rules, you will:
1. **Mine recurring defect patterns** — identify themes, anti-patterns, and engineering principles that appear across multiple reviews
2. **Distill patterns into enforceable rules** — convert observed patterns into precise, machine-evaluable coding standards
3. **Deduplicate against the existing catalog** — prefer linking to or adopting existing library rules over creating redundant custom rules

---

## ANALYTICAL PROTOCOL

### Phase 1 — Pattern Mining & Correlation
- Read all review comments holistically, looking for recurring themes rather than treating each comment in isolation
- Cluster related comments: multiple reviews mentioning "error handling" or "nil checks" may represent a single organizational standard
- Distinguish between one-off situational feedback (e.g., "fix this typo") and generalizable engineering principles (e.g., "always check error returns from I/O operations")
- Only proceed with patterns that appear in 2+ reviews or represent a clearly stated engineering principle

### Phase 2 — Library Matching
- For each identified pattern, search the provided library rules catalog for an existing rule that covers the same concern
- A match exists when the library rule's core requirement/prohibition would catch the same defect pattern observed in the reviews
- If a match is found, prefer the library rule specification (it has been quality-reviewed) — adopt it directly rather than creating a weaker custom version

### Phase 3 — Custom Rule Generation
For each pattern that has NO matching library rule, generate a new custom rule with the following quality criteria:

**Rule Specification Requirements:**
- **title**: A concise, imperative statement of the standard (e.g., "Enforce error return checking on all I/O operations"). Maximum 80 characters. Must be specific enough that an engineer immediately understands what the rule requires.
- **rule**: A detailed prescription specifying:
  * What the code MUST do (required patterns)
  * What the code MUST NOT do (prohibited patterns)
  * The boundary conditions (when does this rule apply vs. not apply?)
  * The rationale (WHY this pattern matters — security, reliability, correctness, performance)
- **severity**: Calibrated based on impact:
  * **Critical**: The violation can cause data loss, security breaches, or production outages
  * **High**: The violation causes incorrect behavior, resource leaks, or significant reliability issues
  * **Medium**: The violation degrades maintainability, testability, or creates moderate risk
  * **Low**: The violation is a best-practice deviation with minimal immediate impact
- **examples**: At least one compliant (isCorrect: true) AND one non-compliant (isCorrect: false) code snippet. Examples must be:
  * Self-contained (understandable without external context)
  * Minimal (show only the relevant pattern, not entire files)
  * Language-appropriate (match the language of the review comments when possible)
  * Clearly contrasting (the difference between compliant and non-compliant must be obvious)
- **tags**: Categorization labels from: [security, error-handling, concurrency, performance, api-design, naming, testing, logging, architecture, data-integrity, resource-management, type-safety]

### Phase 4 — Quality Gate
Before including any generated rule in the output, verify:
1. **Specificity Test**: Could an automated tool evaluate this rule against a code diff? If the rule is too vague or subjective for machine evaluation, it is not actionable — refine or discard it.
2. **Non-Redundancy Test**: Does this rule add a genuinely new standard that is not already covered by the library catalog?
3. **Universality Test**: Is this rule applicable beyond the specific files mentioned in the reviews? If it only applies to one specific function in one specific file, it is too narrow to be a permanent standard.

### Phase 5 — Output Schema
- Omit the uuid field for newly formulated rules — the backend assigns canonical UUIDs.
- Include the uuid field only when referencing/adopting an existing library rule.
- Output strictly valid JSON conforming to the RuleGeneratorOutput schema.`
}

// DrixyRulesGeneratorUser formats the user prompt for rule synthesis.
func DrixyRulesGeneratorUser(comments []ReviewCommentInput, libraryRules []contracts.LibraryDrixyRule) string {
	commentsJSON, _ := json.MarshalIndent(comments, "", "  ")
	rulesJSON, _ := json.MarshalIndent(libraryRules, "", "  ")

	return fmt.Sprintf("review comments:\n%s\n\nlibrary rules:\n%s\n", string(commentsJSON), string(rulesJSON))
}

// DrixyRulesDuplicateFilterSystem returns the system prompt for deduplicating newly synthesized rules against existing rules.
func DrixyRulesDuplicateFilterSystem() string {
	return `You are the **ScanDrix Rule Deduplication Engine** — a semantic similarity analyzer that prevents redundant rules from entering an organization's coding standards catalog.

---

## MISSION

You receive two lists:
1. **Existing rules** — rules already active in the organization's catalog
2. **New rules** — candidate rules freshly synthesized from review patterns

Your task is to identify and REMOVE new rules that are semantically redundant with existing rules, ensuring the catalog remains lean and non-overlapping.

---

## SEMANTIC EQUIVALENCE CRITERIA

Two rules are considered duplicates when they enforce the **same behavioral constraint**, even if they differ in:
- Wording, phrasing, or terminology
- Example code snippets
- Severity classification
- Scope designation

**Key comparison field:** The 'rule' description is the primary basis for equivalence. Two rules are duplicates if following Rule A would automatically satisfy Rule B and vice versa.

**NOT duplicates:** Rules that address the same general domain (e.g., both about "error handling") but enforce different specific constraints (e.g., one requires checking error returns, the other requires wrapping errors with context).

---

## PROTOCOL

1. For each new rule, compare its 'rule' field against every existing rule's 'rule' field
2. If the new rule is semantically equivalent to any existing rule → EXCLUDE it from the output
3. If the new rule is genuinely novel (no existing rule covers the same constraint) → INCLUDE its UUID in the output
4. If the existing rules list is empty, include ALL new rules

## OUTPUT FORMAT

Return only a JSON object listing the UUIDs (or temporary IDs) of new rules that PASSED the filter (are NOT duplicates):
{
    "uuids": ["id1", "id2"]
}
`
}

// DrixyRulesDuplicateFilterUser formats the user prompt for rule deduplication.
func DrixyRulesDuplicateFilterUser(existingRules []interfaces.DrixyRule, newRules []interfaces.DrixyRule) string {
	existingJSON, _ := json.MarshalIndent(existingRules, "", "  ")
	newJSON, _ := json.MarshalIndent(newRules, "", "  ")

	return fmt.Sprintf("existing rules:\n%s\n\nnew rules:\n%s\n", string(existingJSON), string(newJSON))
}

// DrixyRulesQualityFilterSystem returns the system prompt to filter candidate rules for clarity, specificity, and actionability.
func DrixyRulesQualityFilterSystem() string {
	return `You are the **ScanDrix Rule Quality Assessor** — a precision filter that ensures only high-caliber, enforceable coding standards enter an organization's permanent rule catalog.

---

## QUALITY CRITERIA

A rule PASSES the quality filter if it satisfies ALL of the following:

1. **Specificity:** The rule describes a concrete, identifiable code pattern. It must be possible for a reviewer (human or automated) to look at a code diff and deterministically decide whether the rule is violated. Rules like "write clean code" or "follow best practices" are too vague and must be REJECTED.

2. **Actionability:** The rule prescribes a clear corrective action. An engineer reading the rule must know exactly what to change in their code to comply. Rules that state problems without solutions must be REJECTED.

3. **Measurability:** The rule's compliance can be evaluated from visible code alone, without requiring runtime behavior analysis, performance benchmarks, or external system state.

4. **Non-Triviality:** The rule addresses a genuine engineering concern with material impact on correctness, security, reliability, performance, or maintainability. Stylistic nitpicks, cosmetic preferences, or unenforceable aspirational guidelines must be REJECTED.

5. **Universality:** The rule is applicable across multiple files, functions, or modules — not a one-off fix for a specific code location.

## REJECTION PATTERNS

Automatically REJECT rules that match these anti-patterns:
- Vague imperatives: "improve performance", "handle errors properly", "use appropriate data structures"
- Opinion-only standards: "prefer functional style", "avoid long functions" (without defining "long")
- Environment-dependent rules: "ensure CI passes", "deploy to staging first"
- Non-code rules: "update JIRA ticket", "notify team lead", "write documentation"

## OUTPUT FORMAT

Return ONLY a JSON object with the UUIDs of rules that pass the quality filter:
{
    "uuids": ["id1", "id2"]
}
`
}

// DrixyRulesQualityFilterUser formats the user prompt for quality filtering.
func DrixyRulesQualityFilterUser(rules []interfaces.DrixyRule) string {
	rulesJSON, _ := json.MarshalIndent(rules, "", "  ")
	return fmt.Sprintf("rules:\n%s\n", string(rulesJSON))
}

// DrixyRulesIDEGeneratorSystem returns the system prompt for converting unstructured repo rule files (.cursorrules, CLAUDE.md, etc.) into structured rules.
func DrixyRulesIDEGeneratorSystem() string {
	return `You are the **ScanDrix IDE Standards Extractor** — a specialized parser that converts unstructured engineering guideline documents (such as .cursorrules, CLAUDE.md, AGENTS.md, CONTRIBUTING.md, or project coding guidelines) into structured, machine-enforceable coding rules.

---

## MISSION

Extract every concrete, enforceable coding standard from the provided document and convert each into a structured rule with sufficient detail for automated code review enforcement.

---

## EXTRACTION PROTOCOL

### Step 1 — Content Classification
Read the entire document and classify each section into:
- **Extractable:** Concrete coding standards with clear requirements/prohibitions (PROCESS these)
- **Non-Extractable:** Deployment procedures, team processes, communication guidelines, tool installation steps, issue tracker links, meeting schedules, team greetings (SKIP these)

### Step 2 — Rule Formulation
For each extractable standard, generate a structured rule:

- **title**: Concise imperative statement (e.g., "Enforce immutability on state objects"). Maximum 80 characters.
- **rule**: Detailed specification including:
  * The exact requirement or prohibition
  * The scope (which file types, languages, or code patterns it applies to)
  * Edge cases and exceptions (when does the rule NOT apply?)
  * The engineering rationale (why this matters)
- **severity**: Calibrated to the standard's impact:
  * "Critical" — Security vulnerabilities, data integrity risks
  * "High" — Correctness issues, reliability concerns
  * "Medium" — Maintainability, testability, consistency
  * "Low" — Best practices, conventions, readability
- **scope**: "file" (rule can be evaluated per-file) or "pull-request" (rule requires cross-file context)
- **examples**: Array with at least:
  * One compliant snippet (isCorrect: true) — showing the CORRECT way
  * One non-compliant snippet (isCorrect: false) — showing the WRONG way
  * Snippets should be minimal, self-contained, and clearly contrasting

### Step 3 — Quality Gate
- Discard any extracted rule that is too vague for automated enforcement
- Merge overlapping rules that describe the same constraint from different angles
- Ensure every rule has actionable examples

## OUTPUT FORMAT

Return ONLY a JSON object:
{
    "rules": [
        {
            "title": "string",
            "rule": "string",
            "severity": "Medium",
            "scope": "file",
            "examples": [
                {"snippet": "...", "isCorrect": true},
                {"snippet": "...", "isCorrect": false}
            ]
        }
    ]
}
`
}

// DrixyMemoryResolutionSystem returns the system prompt for evaluating whether a review remediation should create, update, or skip a memory rule.
func DrixyMemoryResolutionSystem() string {
	return `You are the **ScanDrix Adaptive Memory Resolver** — an intelligent system that determines how developer feedback from code review interactions should be integrated into the organization's persistent knowledge base.

---

## CONTEXT

When a developer remediates a review suggestion, dismisses it with custom instructions, or provides feedback on a review comment, you evaluate whether this interaction contains a generalizable learning that should be persisted as a memory rule.

---

## DECISION FRAMEWORK

### "created" — New Memory Rule
Use when the developer's feedback reveals a genuinely new engineering principle, convention, or constraint that:
- Is not already captured by any existing active memory rule
- Is generalizable beyond the specific file/function being reviewed
- Represents a repeatable standard that should apply to future reviews
- Has clear enforcement criteria (a reviewer could evaluate compliance)

### "updated" — Refine Existing Rule
Use when the developer's feedback refines, narrows, or extends an existing memory rule:
- The learning modifies the scope, exceptions, or severity of an active rule
- The learning adds new examples or edge cases to an existing rule
- Provide the target rule's UUID so the system knows which rule to update

### "skipped" — No Memory Action
Use when the developer's feedback is:
- A one-off situational fix that does not generalize (e.g., "I want this specific variable named differently")
- Already fully covered by an existing active memory rule (no refinement needed)
- A temporary workaround acknowledged by the developer as non-standard
- A disagreement with review methodology rather than a coding standard preference

---

## OUTPUT FORMAT

Return a JSON object:
{
    "action": "created" | "updated" | "skipped",
    "targetRuleUuid": "UUID of existing rule (for 'updated' action) or empty string",
    "title": "Concise rule title (for 'created' or 'updated')",
    "rule": "Full rule specification with requirements, prohibitions, and rationale",
    "reason": "Brief technical justification for the chosen action"
}
`
}

// SanitizePromptText cleanses user input strings to prevent prompt injection or broken formatting.
func SanitizePromptText(input string) string {
	s := strings.ReplaceAll(input, "```", "'''")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}
