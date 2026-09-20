package taskcontext

import (
	"encoding/json"
	"strings"
)

// BuildTaskContextArgsCandidates generates candidate argument maps for an MCP tool.
func BuildTaskContextArgsCandidates(
	params TaskContextReadParams,
	hints TaskContextHints,
	signature *TaskContextToolSignature,
) []map[string]any {
	var allParams []string
	var requiredParams []string
	if signature != nil && signature.Properties != nil {
		for k := range signature.Properties {
			allParams = append(allParams, k)
		}
		requiredParams = signature.RequiredParams
	}

	if len(allParams) == 0 {
		if signature == nil {
			return buildGenericTaskContextArgsCandidates(hints)
		}
		supportsMaxResults := false
		if signature.NormalizedProperties != nil {
			if _, ok := signature.NormalizedProperties["maxresults"]; ok {
				supportsMaxResults = true
			}
		}
		if supportsMaxResults {
			return []map[string]any{{"maxResults": 1}}
		}
		return []map[string]any{{}}
	}

	valueByParam := make(map[string][]any)
	for _, paramName := range allParams {
		candidates := getCandidateValuesForParam(
			paramName,
			params,
			hints,
			getParamSchema(signature, paramName),
			containsString(requiredParams, paramName),
		)

		if len(candidates) > 0 {
			valueByParam[paramName] = candidates
			continue
		}

		if containsString(requiredParams, paramName) {
			return []map[string]any{}
		}
	}

	var paramsWithValues []string
	for p := range valueByParam {
		paramsWithValues = append(paramsWithValues, p)
	}

	supportsMaxResults := false
	if signature.NormalizedProperties != nil {
		if _, ok := signature.NormalizedProperties["maxresults"]; ok {
			supportsMaxResults = true
		}
	}

	if len(paramsWithValues) == 0 {
		if len(requiredParams) > 0 {
			return []map[string]any{}
		}
		if supportsMaxResults {
			return []map[string]any{{"maxResults": 1}}
		}
		return []map[string]any{{}}
	}

	combinations := combineRequiredParamValues(paramsWithValues, valueByParam, 16)
	if len(combinations) == 0 {
		return []map[string]any{}
	}

	if supportsMaxResults {
		for _, comb := range combinations {
			comb["maxResults"] = 1
		}
	}

	return combinations
}

func getParamSchema(signature *TaskContextToolSignature, paramName string) map[string]any {
	if signature == nil {
		return nil
	}
	if p, ok := signature.Properties[paramName]; ok {
		return p
	}
	if p, ok := signature.NormalizedProperties[NormalizeParamName(paramName)]; ok {
		return p
	}
	return nil
}

func getCandidateValuesForParam(
	paramName string,
	params TaskContextReadParams,
	hints TaskContextHints,
	paramSchema map[string]any,
	isRequired bool,
) []any {
	normalizedName := NormalizeParamName(paramName)
	staticCandidates := resolveStaticParamCandidates(normalizedName, params, hints, paramSchema)
	if len(staticCandidates) > 0 {
		return staticCandidates
	}

	enumCandidates := resolveEnumParamCandidates(paramSchema, hints, isRequired)
	if enumCandidates != nil {
		return enumCandidates
	}

	if !supportsStringParam(paramSchema) {
		return []any{}
	}

	explicitIssueKeys := sliceStrings(UniqueNonEmpty(hints.ExplicitIssueKeys), 4)
	explicitIssueLinks := sliceStrings(UniqueNonEmpty(hints.ExplicitIssueLinks), 4)
	issueKeys := sliceStrings(UniqueNonEmpty(append(explicitIssueKeys, hints.IssueKeys...)), 4)
	issueLinks := sliceStrings(UniqueNonEmpty(append(explicitIssueLinks, hints.IssueLinks...)), 4)
	urlHosts := sliceStrings(UniqueNonEmpty(hints.URLHosts), 2)
	siteUrls := sliceStrings(UniqueNonEmpty(hints.SiteURLs), 4)
	siteIds := sliceStrings(UniqueNonEmpty(hints.SiteIDs), 4)
	resourceIds := sliceStrings(UniqueNonEmpty(hints.ResourceIDs), 4)

	var queryTokens []string
	queryTokens = append(queryTokens, explicitIssueKeys...)
	queryTokens = append(queryTokens, issueKeys...)
	if len(explicitIssueKeys) == 0 {
		queryTokens = append(queryTokens, issueLinks...)
		if hints.QueryText != "" {
			queryTokens = append(queryTokens, hints.QueryText)
		}
	}
	queryTokens = sliceStrings(UniqueNonEmpty(queryTokens), 6)

	intent := inferParamIntent(paramName, paramSchema)

	switch intent {
	case "issue":
		if len(issueKeys) > 0 {
			return toAnySlice(issueKeys)
		}
		return toAnySlice(queryTokens)
	case "query":
		if len(explicitIssueKeys) > 0 {
			return toAnySlice(explicitIssueKeys)
		}
		if len(issueKeys) > 0 {
			return toAnySlice(issueKeys)
		}
		return toAnySlice(queryTokens)
	case "context":
		var ctxTokens []string
		ctxTokens = append(ctxTokens, siteIds...)
		ctxTokens = append(ctxTokens, siteUrls...)
		ctxTokens = append(ctxTokens, urlHosts...)
		return toAnySlice(sliceStrings(UniqueNonEmpty(ctxTokens), 6))
	case "url":
		if len(issueLinks) > 0 {
			return toAnySlice(issueLinks)
		}
		return []any{}
	case "ari":
		return toAnySlice(resourceIds)
	default:
		return toAnySlice(queryTokens)
	}
}

func inferParamIntent(paramName string, paramSchema map[string]any) string {
	normalizedName := NormalizeParamName(paramName)
	var parts []string
	parts = append(parts, paramName)
	if paramSchema != nil {
		if t, ok := paramSchema["title"].(string); ok {
			parts = append(parts, t)
		}
		if d, ok := paramSchema["description"].(string); ok {
			parts = append(parts, d)
		}
	}
	descriptor := strings.ToLower(strings.Join(parts, " "))

	if strings.Contains(descriptor, "resource identifier") ||
		strings.Contains(descriptor, "ari") ||
		normalizedName == "ari" ||
		strings.Contains(normalizedName, "resourceidentifier") {
		return "ari"
	}

	if strings.Contains(normalizedName, "cloud") ||
		strings.Contains(normalizedName, "host") ||
		strings.Contains(normalizedName, "domain") ||
		strings.Contains(normalizedName, "site") ||
		strings.Contains(normalizedName, "workspace") {
		return "context"
	}

	if strings.Contains(descriptor, "issue") ||
		strings.Contains(descriptor, "ticket") ||
		strings.Contains(descriptor, "task") ||
		strings.Contains(normalizedName, "issue") ||
		strings.Contains(normalizedName, "ticket") ||
		strings.Contains(normalizedName, "task") ||
		strings.Contains(normalizedName, "key") ||
		strings.HasSuffix(normalizedName, "id") {
		return "issue"
	}

	if strings.Contains(descriptor, "query") ||
		strings.Contains(descriptor, "search") ||
		strings.Contains(normalizedName, "query") ||
		strings.Contains(normalizedName, "search") ||
		normalizedName == "text" ||
		normalizedName == "input" {
		return "query"
	}

	if strings.Contains(descriptor, "url") ||
		strings.Contains(descriptor, "link") ||
		strings.Contains(descriptor, "resource") ||
		strings.Contains(normalizedName, "url") ||
		strings.Contains(normalizedName, "link") {
		return "url"
	}

	return "generic"
}

func resolveEnumParamCandidates(
	paramSchema map[string]any,
	hints TaskContextHints,
	isRequired bool,
) []any {
	if paramSchema == nil {
		return nil
	}
	rawEnum, ok := paramSchema["enum"].([]any)
	if !ok || len(rawEnum) == 0 {
		return nil
	}

	var enumValues []string
	for _, e := range rawEnum {
		if s, ok := e.(string); ok {
			enumValues = append(enumValues, s)
		}
	}
	if len(enumValues) == 0 {
		return nil
	}

	var allTokens []string
	allTokens = append(allTokens, hints.ExplicitIssueKeys...)
	allTokens = append(allTokens, hints.IssueKeys...)
	allTokens = append(allTokens, hints.ExplicitIssueLinks...)
	allTokens = append(allTokens, hints.IssueLinks...)
	if hints.QueryText != "" {
		allTokens = append(allTokens, hints.QueryText)
	}

	var matching []any
	for _, ev := range enumValues {
		for _, token := range allTokens {
			if strings.EqualFold(ev, token) {
				matching = append(matching, ev)
				break
			}
		}
	}

	if len(matching) > 0 {
		return matching
	}

	if isRequired && len(enumValues) > 0 {
		return []any{enumValues[0]}
	}

	return []any{}
}

func supportsStringParam(paramSchema map[string]any) bool {
	if paramSchema == nil || len(paramSchema) == 0 {
		return true
	}
	t, ok := paramSchema["type"].(string)
	if ok && t != "" {
		return strings.EqualFold(t, "string")
	}
	return true
}

func resolveStaticParamCandidates(
	normalizedName string,
	params TaskContextReadParams,
	hints TaskContextHints,
	paramSchema map[string]any,
) []any {
	switch normalizedName {
	case "organizationid":
		if params.OrganizationID != "" {
			return []any{params.OrganizationID}
		}
	case "teamid":
		if params.TeamID != "" {
			return []any{params.TeamID}
		}
	case "issuenumber", "issueid":
		if len(hints.IssueNumbers) > 0 {
			var nums []any
			for _, n := range hints.IssueNumbers {
				nums = append(nums, n)
			}
			return nums
		}
	case "pullrequestnumber":
		if params.PullRequestNumber > 0 {
			return []any{params.PullRequestNumber}
		}
	case "repository":
		if params.RepositoryOwner != "" && params.RepositoryName != "" {
			return []any{map[string]any{
				"owner": params.RepositoryOwner,
				"name":  params.RepositoryName,
			}}
		}
	case "owner", "repositoryowner":
		if params.RepositoryOwner != "" {
			return []any{params.RepositoryOwner}
		}
	case "repo", "repositoryname":
		if params.RepositoryName != "" {
			return []any{params.RepositoryName}
		}
	}
	return nil
}

func combineRequiredParamValues(
	requiredParams []string,
	valueByParam map[string][]any,
	limit int,
) []map[string]any {
	var results []map[string]any

	var walk func(index int, current map[string]any)
	walk = func(index int, current map[string]any) {
		if len(results) >= limit {
			return
		}
		if index >= len(requiredParams) {
			clone := make(map[string]any, len(current))
			for k, v := range current {
				clone[k] = v
			}
			results = append(results, clone)
			return
		}

		param := requiredParams[index]
		values := valueByParam[param]
		for _, val := range values {
			current[param] = val
			walk(index+1, current)
			if len(results) >= limit {
				return
			}
		}
	}

	walk(0, make(map[string]any))
	return results
}

func buildGenericTaskContextArgsCandidates(hints TaskContextHints) []map[string]any {
	var tokens []string
	tokens = append(tokens, hints.ExplicitIssueKeys...)
	tokens = append(tokens, hints.ExplicitIssueLinks...)
	if len(hints.ExplicitIssueKeys) == 0 {
		tokens = append(tokens, hints.IssueLinks...)
	}
	tokens = append(tokens, hints.IssueKeys...)
	if len(hints.ExplicitIssueKeys) == 0 && hints.QueryText != "" {
		tokens = append(tokens, hints.QueryText)
	}
	tokens = sliceStrings(UniqueNonEmpty(tokens), 4)

	var args []map[string]any
	for _, token := range tokens {
		if IsLikelyURL(token) {
			args = append(args,
				map[string]any{"url": token},
				map[string]any{"resource": token},
				map[string]any{"link": token},
				map[string]any{"query": token},
				map[string]any{"input": token},
			)
		} else if IsLikelyIssueKey(token) {
			args = append(args,
				map[string]any{"id": token},
				map[string]any{"key": token},
				map[string]any{"issueKey": token},
				map[string]any{"ticketId": token},
				map[string]any{"taskId": token},
				map[string]any{"query": token},
				map[string]any{"input": token},
			)
		} else {
			args = append(args,
				map[string]any{"query": token},
				map[string]any{"text": token},
				map[string]any{"search": token},
				map[string]any{"input": token},
				map[string]any{"task": token},
				map[string]any{"issue": token},
			)
		}
	}

	seen := make(map[string]struct{})
	var deduped []map[string]any
	for _, arg := range args {
		b, _ := json.Marshal(arg)
		s := string(b)
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
			deduped = append(deduped, arg)
			if len(deduped) >= 16 {
				break
			}
		}
	}

	return deduped
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func sliceStrings(slice []string, maxLen int) []string {
	if len(slice) > maxLen {
		return slice[:maxLen]
	}
	return slice
}

func toAnySlice(slice []string) []any {
	res := make([]any, len(slice))
	for i, s := range slice {
		res[i] = s
	}
	return res
}
