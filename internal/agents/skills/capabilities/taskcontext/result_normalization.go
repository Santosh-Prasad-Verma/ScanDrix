package taskcontext

import (
	"encoding/json"
	"strings"
)

// ExtractTaskContextFromToolResult extracts the best TaskContextNormalized candidate from raw tool output.
func ExtractTaskContextFromToolResult(payload any) *TaskContextNormalized {
	candidates := extractContextCandidates(payload)
	var best *TaskContextNormalized
	bestScore := -1

	for _, candidate := range candidates {
		normalized := normalizeContextCandidate(candidate)
		if normalized == nil {
			continue
		}

		score := ScoreNormalizedContext(normalized)
		if score > bestScore {
			best = normalized
			bestScore = score
		}
	}

	return best
}

func extractContextCandidates(payload any) []map[string]any {
	var candidates []map[string]any
	seen := make(map[string]struct{})

	addCandidate := func(val any) {
		m, ok := val.(map[string]any)
		if !ok || len(m) == 0 {
			return
		}
		b, err := json.Marshal(m)
		if err != nil {
			return
		}
		sig := string(b)
		if _, exists := seen[sig]; !exists {
			seen[sig] = struct{}{}
			candidates = append(candidates, m)
		}
	}

	var visit func(val any, depth int)
	visit = func(val any, depth int) {
		if depth > 8 || val == nil {
			return
		}

		if s, ok := val.(string); ok {
			if parsed, ok := TryParseJSONString(s); ok {
				visit(parsed, depth+1)
			}
			return
		}

		if arr, ok := val.([]any); ok {
			maxItems := len(arr)
			if maxItems > 25 {
				maxItems = 25
			}
			for _, item := range arr[:maxItems] {
				visit(item, depth+1)
			}
			return
		}

		m, ok := val.(map[string]any)
		if !ok || len(m) == 0 {
			return
		}

		addCandidate(m)

		singletonKeys := []string{
			"result", "data", "payload", "item", "issue", "task",
			"ticket", "page", "record", "object", "fields", "properties", "attributes",
		}
		collectionKeys := []string{
			"items", "results", "records", "nodes", "content",
		}

		for _, k := range singletonKeys {
			if child, ok := m[k]; ok {
				visit(child, depth+1)
			}
		}
		for _, k := range collectionKeys {
			if child, ok := m[k]; ok {
				visit(child, depth+1)
			}
		}
		if text, ok := m["text"].(string); ok {
			visit(text, depth+1)
		}
	}

	visit(payload, 0)
	return candidates
}

func normalizeContextCandidate(candidate map[string]any) *TaskContextNormalized {
	if LooksLikeToolErrorCandidate(candidate) {
		return nil
	}

	fields, _ := candidate["fields"].(map[string]any)
	properties, _ := candidate["properties"].(map[string]any)
	attributes, _ := candidate["attributes"].(map[string]any)
	data, _ := candidate["data"].(map[string]any)

	spaces := []map[string]any{candidate, fields, properties, attributes, data}

	id := pluckFirstString(spaces, []string{
		"key", "identifier", "code", "issueKey", "ticketKey", "taskKey",
		"id", "number", "issueId", "taskId", "ticketId", "pageId", "recordId",
	})

	title := pluckFirstString(spaces, []string{"summary", "title", "name", "subject"})
	if title == "" && properties != nil {
		title = extractPropertyText(properties, []string{"Name", "Title", "Summary", "Task", "Issue"})
	}

	descriptionRaw := pluckFirstValue(spaces, []string{
		"description", "body", "content", "text", "details", "overview", "context",
	})
	description := normalizeTextValue(descriptionRaw)

	var ac []string
	acFromPluck := extractStringArray(pluckFirstValue(spaces, []string{
		"acceptanceCriteria", "acceptance_criteria", "criteria", "requirements", "acceptance",
	}))
	ac = append(ac, acFromPluck...)
	if properties != nil {
		acFromProp := extractStringArray(extractPropertyValue(properties, []string{
			"Acceptance Criteria", "Acceptance", "Criteria", "Requirements",
		}))
		ac = append(ac, acFromProp...)
	}
	acceptanceCriteria := UniqueNonEmpty(ac)

	var linkList []string
	linksFromPluck := extractStringArray(pluckFirstValue(spaces, []string{"links", "references", "urls"}))
	linkList = append(linkList, linksFromPluck...)
	urlSingle := pluckFirstString(spaces, []string{
		"url", "webUrl", "htmlUrl", "permalink", "href", "uri", "link",
	})
	if urlSingle != "" {
		linkList = append(linkList, urlSingle)
	}
	if description != "" {
		linkList = append(linkList, ExtractLinks(description)...)
	}
	links := UniqueNonEmpty(linkList)

	normalized := &TaskContextNormalized{
		ID:                 id,
		Title:              title,
		Description:        description,
		AcceptanceCriteria: acceptanceCriteria,
		Links:              links,
		RawPayload:         candidate,
	}

	if strings.TrimSpace(normalized.Title) != "" || strings.TrimSpace(normalized.Description) != "" {
		return normalized
	}
	return nil
}

func pluckFirstString(spaces []map[string]any, keys []string) string {
	for _, space := range spaces {
		if space == nil {
			continue
		}
		for _, key := range keys {
			if val, ok := space[key]; ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func pluckFirstValue(spaces []map[string]any, keys []string) any {
	for _, space := range spaces {
		if space == nil {
			continue
		}
		for _, key := range keys {
			if val, ok := space[key]; ok && val != nil {
				return val
			}
		}
	}
	return nil
}

func extractPropertyValue(properties map[string]any, names []string) any {
	if properties == nil {
		return nil
	}
	for _, name := range names {
		if val, ok := properties[name]; ok && val != nil {
			return val
		}
	}
	return nil
}

func extractPropertyText(properties map[string]any, names []string) string {
	val := extractPropertyValue(properties, names)
	return normalizeTextValue(val)
}

func normalizeTextValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}

	if adf := extractAdfText(value); adf != "" {
		return adf
	}

	if rich := extractRichText(value); rich != "" {
		return rich
	}

	b, err := json.Marshal(value)
	if err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" && s != "null" && s != "{}" && s != "[]" {
			return s
		}
	}
	return ""
}

func extractAdfText(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	t, _ := m["type"].(string)
	if t != "doc" {
		return ""
	}
	contentArr, ok := m["content"].([]any)
	if !ok {
		return ""
	}

	var lines []string
	var visitNode func(node any)
	visitNode = func(node any) {
		nm, ok := node.(map[string]any)
		if !ok {
			return
		}
		nodeType, _ := nm["type"].(string)
		if nodeType == "text" {
			if text, ok := nm["text"].(string); ok {
				lines = append(lines, text)
			}
			return
		}
		if childContent, ok := nm["content"].([]any); ok {
			for _, child := range childContent {
				visitNode(child)
			}
		}
	}

	for _, block := range contentArr {
		visitNode(block)
		lines = append(lines, "\n")
	}

	res := strings.Join(lines, "")
	return strings.TrimSpace(res)
}

func extractRichText(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	if arr, ok := value.([]any); ok {
		var parts []string
		for _, item := range arr {
			if part := extractRichText(item); part != "" {
				parts = append(parts, part)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	m, ok := value.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
	}

	if direct := FirstNonEmptyString(m["plain_text"], m["text"], m["content"], m["value"], m["name"], m["title"]); direct != "" {
		return direct
	}

	return FirstNonEmptyString(
		extractRichText(m["rich_text"]),
		extractRichText(m["title"]),
		extractRichText(m["description"]),
		extractRichText(m["content"]),
		extractRichText(m["text"]),
	)
}

func extractStringArray(value any) []string {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return []string{strings.TrimSpace(s)}
	}
	if arr, ok := value.([]any); ok {
		var res []string
		for _, item := range arr {
			if text := extractRichText(item); text != "" {
				res = append(res, text)
			}
		}
		return res
	}
	if text := extractRichText(value); text != "" {
		return []string{text}
	}
	return nil
}
