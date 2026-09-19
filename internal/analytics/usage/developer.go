package usage

import (
	"context"
	"fmt"
	"time"
)

const (
	DevTTLPast    = 4 * time.Hour
	DevTTLCurrent = 4 * time.Hour
)

// TokensByDeveloperUseCase maps token usages to pull request authors and aggregates by developer.
type TokensByDeveloperUseCase struct {
	tokenUsageService   ITokenUsageService
	pullRequestsService IPullRequestsService
	cache               ICacheService
}

// NewTokensByDeveloperUseCase creates a new developer usage use case.
func NewTokensByDeveloperUseCase(
	tokenUsageService ITokenUsageService,
	pullRequestsService IPullRequestsService,
	cache ICacheService,
) *TokensByDeveloperUseCase {
	return &TokensByDeveloperUseCase{
		tokenUsageService:   tokenUsageService,
		pullRequestsService: pullRequestsService,
		cache:               cache,
	}
}

// ExecuteAggregated aggregates token usages by developer and model.
func (uc *TokensByDeveloperUseCase) ExecuteAggregated(
	ctx context.Context,
	query TokenUsageQueryContract,
) ([]UsageByDeveloperResultContract, error) {
	cacheKey := uc.devCacheKey(query, false)
	if uc.cache != nil {
		var cached []UsageByDeveloperResultContract
		if ok := uc.cache.Get(ctx, cacheKey, &cached); ok && len(cached) > 0 {
			return cached, nil
		}
	}

	usages, err := uc.tokenUsageService.GetUsageByPr(ctx, query)
	if err != nil {
		return nil, err
	}

	prNumbers := make([]int, len(usages))
	for i, u := range usages {
		prNumbers[i] = u.PRNumber
	}

	prMap, err := uc.getPullRequestsMap(ctx, prNumbers, query.OrganizationID)
	if err != nil {
		return nil, err
	}

	mapped := make([]UsageByDeveloperResultContract, len(usages))
	for i, u := range usages {
		dev := "unknown"
		if pr, ok := prMap[u.PRNumber]; ok && pr.Username != "" {
			dev = pr.Username
		}
		mapped[i] = UsageByDeveloperResultContract{
			BaseUsageContract: u.BaseUsageContract,
			Developer:         dev,
		}
	}

	var filtered []UsageByDeveloperResultContract
	if query.Developer != "" {
		for _, m := range mapped {
			if m.Developer == query.Developer {
				filtered = append(filtered, m)
			}
		}
	} else {
		filtered = uc.groupByDeveloperAndModel(mapped)
	}

	if uc.cache != nil {
		ttl := DevTTLCurrent
		if query.End.Before(startOfTodayUTC()) {
			ttl = DevTTLPast
		}
		_ = uc.cache.Set(ctx, cacheKey, filtered, ttl)
	}

	return filtered, nil
}

// ExecuteDaily maps daily PR usages to developers.
func (uc *TokensByDeveloperUseCase) ExecuteDaily(
	ctx context.Context,
	query TokenUsageQueryContract,
) ([]DailyUsageByDeveloperResultContract, error) {
	cacheKey := uc.devCacheKey(query, true)
	if uc.cache != nil {
		var cached []DailyUsageByDeveloperResultContract
		if ok := uc.cache.Get(ctx, cacheKey, &cached); ok && len(cached) > 0 {
			return cached, nil
		}
	}

	usages, err := uc.tokenUsageService.GetDailyUsageByPr(ctx, query)
	if err != nil {
		return nil, err
	}

	prNumbers := make([]int, len(usages))
	for i, u := range usages {
		prNumbers[i] = u.PRNumber
	}

	prMap, err := uc.getPullRequestsMap(ctx, prNumbers, query.OrganizationID)
	if err != nil {
		return nil, err
	}

	mapped := make([]DailyUsageByDeveloperResultContract, 0, len(usages))
	for _, u := range usages {
		dev := "unknown"
		if pr, ok := prMap[u.PRNumber]; ok && pr.Username != "" {
			dev = pr.Username
		}
		if query.Developer != "" && dev != query.Developer {
			continue
		}
		mapped = append(mapped, DailyUsageByDeveloperResultContract{
			UsageByDeveloperResultContract: UsageByDeveloperResultContract{
				BaseUsageContract: u.BaseUsageContract,
				Developer:         dev,
			},
			Date: u.Date,
		})
	}

	if uc.cache != nil {
		ttl := DevTTLCurrent
		if query.End.Before(startOfTodayUTC()) {
			ttl = DevTTLPast
		}
		_ = uc.cache.Set(ctx, cacheKey, mapped, ttl)
	}

	return mapped, nil
}

func (uc *TokensByDeveloperUseCase) getPullRequestsMap(
	ctx context.Context,
	prNumbers []int,
	orgID string,
) (map[int]IPullRequestUserMapping, error) {
	if len(prNumbers) == 0 || uc.pullRequestsService == nil {
		return make(map[int]IPullRequestUserMapping), nil
	}

	seen := make(map[int]bool)
	uniqueNums := make([]int, 0, len(prNumbers))
	for _, n := range prNumbers {
		if !seen[n] {
			seen[n] = true
			uniqueNums = append(uniqueNums, n)
		}
	}

	mappings, err := uc.pullRequestsService.FindManyByNumbers(ctx, uniqueNums, orgID)
	if err != nil {
		return nil, err
	}

	res := make(map[int]IPullRequestUserMapping, len(mappings))
	for _, m := range mappings {
		res[m.Number] = m
	}
	return res, nil
}

func (uc *TokensByDeveloperUseCase) groupByDeveloperAndModel(
	usages []UsageByDeveloperResultContract,
) []UsageByDeveloperResultContract {
	grouped := make(map[string]*UsageByDeveloperResultContract)
	order := make([]string, 0)

	for _, u := range usages {
		key := fmt.Sprintf("%s-%s", u.Developer, u.Model)
		if existing, ok := grouped[key]; !ok {
			item := u
			grouped[key] = &item
			order = append(order, key)
		} else {
			existing.Input += u.Input
			existing.Output += u.Output
			existing.Total += u.Total
			existing.OutputReasoning += u.OutputReasoning
			existing.CacheRead += u.CacheRead
			existing.CacheWrite += u.CacheWrite
		}
	}

	out := make([]UsageByDeveloperResultContract, len(order))
	for i, key := range order {
		out[i] = *grouped[key]
	}
	return out
}

func (uc *TokensByDeveloperUseCase) devCacheKey(query TokenUsageQueryContract, daily bool) string {
	mode := "agg"
	if daily {
		mode = "daily"
	}
	byok := "sys"
	if query.BYOK {
		byok = "byok"
	}
	pr := 0
	if query.PRNumber != nil {
		pr = *query.PRNumber
	}
	tz := query.Timezone
	if tz == "" {
		tz = "UTC"
	}

	return fmt.Sprintf(
		"usage:by-dev:v1|%s|%s|%s|%d|%d|%s|%s|%d|%s|%s",
		mode,
		query.OrganizationID,
		byok,
		query.Start.UnixMilli(),
		query.End.UnixMilli(),
		tz,
		query.Models,
		pr,
		query.RepositoryID,
		query.Developer,
	)
}
