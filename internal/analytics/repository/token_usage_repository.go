package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/usage"
	"github.com/scandrix/backend/internal/core/log"
)

// RawSpanRecord represents an ingested AI token telemetry span stored in the analytics database.
type RawSpanRecord struct {
	CorrelationID  string             `json:"correlationId"`
	OrganizationID string             `json:"organizationId"`
	PRNumber       int                `json:"prNumber"`
	Timestamp      time.Time          `json:"timestamp"`
	Duration       int64              `json:"duration"`
	TokenUsage     log.TokenUsageTu   `json:"tokenUsage"`
}

// TokenUsageRepository provides memory-backed and database-compatible aggregations for token analytics.
type TokenUsageRepository struct {
	mu       sync.RWMutex
	records  []RawSpanRecord
	resolver *pricing.PricingResolver
}

// NewTokenUsageRepository creates a new token usage repository.
func NewTokenUsageRepository(resolver *pricing.PricingResolver) *TokenUsageRepository {
	return &TokenUsageRepository{
		records:  make([]RawSpanRecord, 0),
		resolver: resolver,
	}
}

// IngestSpan records a raw token usage telemetry span.
func (r *TokenUsageRepository) IngestSpan(ctx context.Context, span RawSpanRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if span.Timestamp.IsZero() {
		span.Timestamp = time.Now().UTC()
	}
	r.records = append(r.records, span)
	return nil
}

// FilterSpans returns spans matching the query criteria.
func (r *TokenUsageRepository) filterSpans(q usage.TokenUsageQueryContract) []RawSpanRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []RawSpanRecord
	for _, rec := range r.records {
		if q.OrganizationID != "" && rec.OrganizationID != q.OrganizationID {
			continue
		}
		if !q.Start.IsZero() && rec.Timestamp.Before(q.Start) {
			continue
		}
		if !q.End.IsZero() && rec.Timestamp.After(q.End) {
			continue
		}
		if q.BYOK && !rec.TokenUsage.IsByok {
			continue
		}
		if !q.BYOK && rec.TokenUsage.IsByok {
			continue
		}
		if q.PRNumber != nil && rec.PRNumber != *q.PRNumber {
			continue
		}
		if len(q.PRNumbers) > 0 {
			found := false
			for _, p := range q.PRNumbers {
				if p == rec.PRNumber {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if q.Models != "" {
			reqModel := strings.ToLower(strings.TrimSpace(q.Models))
			recModel := strings.ToLower(strings.TrimSpace(rec.TokenUsage.Model))
			if !strings.Contains(recModel, reqModel) {
				continue
			}
		}

		matched = append(matched, rec)
	}

	return matched
}

// GetSummary returns flat totals across all matching spans.
func (r *TokenUsageRepository) GetSummary(ctx context.Context, q usage.TokenUsageQueryContract) (usage.UsageSummaryContract, error) {
	spans := r.filterSpans(q)
	var summary usage.UsageSummaryContract

	for _, s := range spans {
		summary.Input += s.TokenUsage.Input
		summary.Output += s.TokenUsage.Output
		summary.Total += s.TokenUsage.Total
		summary.OutputReasoning += s.TokenUsage.Reasoning
		summary.CacheRead += s.TokenUsage.CacheRead
		summary.CacheWrite += s.TokenUsage.CacheWrite
	}

	return summary, nil
}

// GetSummaryByModel aggregates token usage grouped by model name with optional tier breakdowns.
func (r *TokenUsageRepository) GetSummaryByModel(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.BaseUsageContract, error) {
	spans := r.filterSpans(q)

	var thresholdsMap map[string][]int64
	if r.resolver != nil {
		thresholdsMap = r.resolver.TieredInputThresholds(ctx)
	}

	grouped := make(map[string]*usage.BaseUsageContract)

	for _, s := range spans {
		model := strings.TrimSpace(s.TokenUsage.Model)
		if model == "" {
			model = pricing.UnknownModel
		}

		entry, ok := grouped[model]
		if !ok {
			entry = &usage.BaseUsageContract{
				Model: model,
			}
			grouped[model] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite

		// Determine tier bracket if thresholds exist
		if thresholds, hasTiers := thresholdsMap[model]; hasTiers && len(thresholds) > 0 {
			bracket := 0
			for idx, th := range thresholds {
				if s.TokenUsage.Input > th {
					bracket = idx + 1
				}
			}

			for len(entry.ByTier) <= bracket {
				entry.ByTier = append(entry.ByTier, usage.TierUsage{})
			}
			entry.ByTier[bracket].Input += s.TokenUsage.Input
			entry.ByTier[bracket].Output += s.TokenUsage.Output
			entry.ByTier[bracket].Total += s.TokenUsage.Total
			entry.ByTier[bracket].OutputReasoning += s.TokenUsage.Reasoning
			entry.ByTier[bracket].CacheRead += s.TokenUsage.CacheRead
			entry.ByTier[bracket].CacheWrite += s.TokenUsage.CacheWrite
		}
	}

	result := make([]usage.BaseUsageContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Model < result[j].Model
	})

	return result, nil
}

// GetDailyUsage returns usage grouped by date and model.
func (r *TokenUsageRepository) GetDailyUsage(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageResultContract, error) {
	spans := r.filterSpans(q)
	grouped := make(map[string]*usage.DailyUsageResultContract)

	for _, s := range spans {
		dateStr := s.Timestamp.Format("2006-01-02")
		model := strings.TrimSpace(s.TokenUsage.Model)
		key := fmt.Sprintf("%s|%s", dateStr, model)

		entry, ok := grouped[key]
		if !ok {
			entry = &usage.DailyUsageResultContract{
				BaseUsageContract: usage.BaseUsageContract{
					Model: model,
				},
				Date: dateStr,
			}
			grouped[key] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite
	}

	result := make([]usage.DailyUsageResultContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].Model < result[j].Model
		}
		return result[i].Date < result[j].Date
	})

	return result, nil
}

// GetModelCredentialPairs returns distinct (model, credentialId) pairs recorded on BYOK usage.
func (r *TokenUsageRepository) GetModelCredentialPairs(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.ModelCredentialPair, error) {
	spans := r.filterSpans(q)
	seen := make(map[string]bool)
	var pairs []usage.ModelCredentialPair

	for _, s := range spans {
		model := strings.TrimSpace(s.TokenUsage.Model)
		cred := strings.TrimSpace(s.TokenUsage.CredentialID)
		if model != "" && cred != "" {
			key := fmt.Sprintf("%s|%s", model, cred)
			if !seen[key] {
				seen[key] = true
				pairs = append(pairs, usage.ModelCredentialPair{
					Model:        model,
					CredentialID: cred,
				})
			}
		}
	}

	return pairs, nil
}

// GetUsageByPr returns token usage grouped by pull request number and model.
func (r *TokenUsageRepository) GetUsageByPr(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByPrResultContract, error) {
	spans := r.filterSpans(q)
	grouped := make(map[string]*usage.UsageByPrResultContract)

	for _, s := range spans {
		model := strings.TrimSpace(s.TokenUsage.Model)
		key := fmt.Sprintf("%d|%s", s.PRNumber, model)

		entry, ok := grouped[key]
		if !ok {
			entry = &usage.UsageByPrResultContract{
				BaseUsageContract: usage.BaseUsageContract{
					Model: model,
				},
				PRNumber: s.PRNumber,
			}
			grouped[key] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite
	}

	result := make([]usage.UsageByPrResultContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].PRNumber == result[j].PRNumber {
			return result[i].Model < result[j].Model
		}
		return result[i].PRNumber < result[j].PRNumber
	})

	return result, nil
}

// GetDailyUsageByPr returns token usage grouped by date, pull request, and model.
func (r *TokenUsageRepository) GetDailyUsageByPr(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.DailyUsageByPrResultContract, error) {
	spans := r.filterSpans(q)
	grouped := make(map[string]*usage.DailyUsageByPrResultContract)

	for _, s := range spans {
		dateStr := s.Timestamp.Format("2006-01-02")
		model := strings.TrimSpace(s.TokenUsage.Model)
		key := fmt.Sprintf("%s|%d|%s", dateStr, s.PRNumber, model)

		entry, ok := grouped[key]
		if !ok {
			entry = &usage.DailyUsageByPrResultContract{
				UsageByPrResultContract: usage.UsageByPrResultContract{
					BaseUsageContract: usage.BaseUsageContract{
						Model: model,
					},
					PRNumber: s.PRNumber,
				},
				Date: dateStr,
			}
			grouped[key] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite
	}

	result := make([]usage.DailyUsageByPrResultContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Date == result[j].Date {
			return result[i].PRNumber < result[j].PRNumber
		}
		return result[i].Date < result[j].Date
	})

	return result, nil
}

// GetUsageByReview returns token usage grouped by correlation id (review run).
func (r *TokenUsageRepository) GetUsageByReview(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByReviewResultContract, error) {
	spans := r.filterSpans(q)
	grouped := make(map[string]*usage.UsageByReviewResultContract)

	for _, s := range spans {
		rev := strings.TrimSpace(s.CorrelationID)
		if rev == "" {
			rev = "unknown-review"
		}
		model := strings.TrimSpace(s.TokenUsage.Model)
		key := fmt.Sprintf("%s|%s", rev, model)

		entry, ok := grouped[key]
		if !ok {
			pr := s.PRNumber
			ts := s.Timestamp
			entry = &usage.UsageByReviewResultContract{
				BaseUsageContract: usage.BaseUsageContract{
					Model: model,
				},
				Review:    rev,
				PRNumber:  &pr,
				StartedAt: &ts,
			}
			grouped[key] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite
	}

	result := make([]usage.UsageByReviewResultContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}
	return result, nil
}

// GetUsageByArea returns token usage grouped by process area.
func (r *TokenUsageRepository) GetUsageByArea(ctx context.Context, q usage.TokenUsageQueryContract) ([]usage.UsageByAreaResultContract, error) {
	spans := r.filterSpans(q)
	grouped := make(map[string]*usage.UsageByAreaResultContract)

	for _, s := range spans {
		area := string(s.TokenUsage.Area)
		if area == "" {
			area = string(log.AreaOther)
		}
		model := strings.TrimSpace(s.TokenUsage.Model)
		key := fmt.Sprintf("%s|%s", area, model)

		entry, ok := grouped[key]
		if !ok {
			entry = &usage.UsageByAreaResultContract{
				BaseUsageContract: usage.BaseUsageContract{
					Model: model,
				},
				Area: area,
			}
			grouped[key] = entry
		}

		entry.Input += s.TokenUsage.Input
		entry.Output += s.TokenUsage.Output
		entry.Total += s.TokenUsage.Total
		entry.OutputReasoning += s.TokenUsage.Reasoning
		entry.CacheRead += s.TokenUsage.CacheRead
		entry.CacheWrite += s.TokenUsage.CacheWrite
	}

	result := make([]usage.UsageByAreaResultContract, 0, len(grouped))
	for _, item := range grouped {
		result = append(result, *item)
	}
	return result, nil
}

// GetUsageOverview combines summary, byModel, daily, byPr, byArea, byTaskArea, and byTaskModelSpan.
func (r *TokenUsageRepository) GetUsageOverview(ctx context.Context, q usage.TokenUsageQueryContract) (*usage.UsageOverviewReportContract, error) {
	summary, err := r.GetSummary(ctx, q)
	if err != nil {
		return nil, err
	}

	byModel, err := r.GetSummaryByModel(ctx, q)
	if err != nil {
		return nil, err
	}

	daily, err := r.GetDailyUsage(ctx, q)
	if err != nil {
		return nil, err
	}

	byPr, err := r.GetUsageByPr(ctx, q)
	if err != nil {
		return nil, err
	}

	byArea, err := r.GetUsageByArea(ctx, q)
	if err != nil {
		return nil, err
	}

	// Task x Area breakdown & TaskModelSpan
	spans := r.filterSpans(q)
	taskAreaMap := make(map[string]*usage.UsageByTaskAreaResultContract)
	taskSpanMap := make(map[string]*usage.UsageByTaskModelSpanContract)

	for _, s := range spans {
		area := string(s.TokenUsage.Area)
		if area == "" {
			area = string(log.AreaOther)
		}
		task := s.TokenUsage.Route
		if task == "" {
			task = log.RouteFromArea(s.TokenUsage.Area)
		}
		model := strings.TrimSpace(s.TokenUsage.Model)

		// Task x Area
		taKey := fmt.Sprintf("%s|%s|%s", task, area, model)
		taEntry, ok := taskAreaMap[taKey]
		if !ok {
			taEntry = &usage.UsageByTaskAreaResultContract{
				BaseUsageContract: usage.BaseUsageContract{
					Model: model,
				},
				Task: task,
				Area: area,
			}
			taskAreaMap[taKey] = taEntry
		}
		taEntry.Input += s.TokenUsage.Input
		taEntry.Output += s.TokenUsage.Output
		taEntry.Total += s.TokenUsage.Total
		taEntry.OutputReasoning += s.TokenUsage.Reasoning
		taEntry.CacheRead += s.TokenUsage.CacheRead
		taEntry.CacheWrite += s.TokenUsage.CacheWrite

		// Task x Model Span
		tsKey := fmt.Sprintf("%s|%s", task, model)
		tsEntry, ok := taskSpanMap[tsKey]
		isoTime := s.Timestamp.Format(time.RFC3339)
		if !ok {
			tsEntry = &usage.UsageByTaskModelSpanContract{
				Task:    task,
				Model:   model,
				FirstAt: isoTime,
				LastAt:  isoTime,
			}
			taskSpanMap[tsKey] = tsEntry
		} else {
			if isoTime < tsEntry.FirstAt {
				tsEntry.FirstAt = isoTime
			}
			if isoTime > tsEntry.LastAt {
				tsEntry.LastAt = isoTime
			}
		}
	}

	byTaskArea := make([]usage.UsageByTaskAreaResultContract, 0, len(taskAreaMap))
	for _, item := range taskAreaMap {
		byTaskArea = append(byTaskArea, *item)
	}

	byTaskModelSpan := make([]usage.UsageByTaskModelSpanContract, 0, len(taskSpanMap))
	for _, item := range taskSpanMap {
		byTaskModelSpan = append(byTaskModelSpan, *item)
	}

	byModelRows := make([]usage.EnrichedModelUsage, len(byModel))
	for i, m := range byModel {
		byModelRows[i] = usage.EnrichedModelUsage{
			BaseUsageContract: m,
		}
	}

	return &usage.UsageOverviewReportContract{
		Summary: usage.UsageSummaryReportContract{
			Totals:  summary,
			ByModel: byModelRows,
		},
		Daily:           daily,
		ByPr:            byPr,
		ByArea:          byArea,
		ByTaskArea:      byTaskArea,
		ByTaskModelSpan: byTaskModelSpan,
	}, nil
}
