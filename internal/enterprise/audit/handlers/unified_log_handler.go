// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/scandrix/backend/internal/enterprise/audit"
)

// UnifiedLogHandler provides enterprise search, aggregation, and export formatting.
type UnifiedLogHandler struct {
	repo audit.IAuditLogRepository
}

// NewUnifiedLogHandler constructs a unified log service.
func NewUnifiedLogHandler(repo audit.IAuditLogRepository) *UnifiedLogHandler {
	return &UnifiedLogHandler{repo: repo}
}

// TimelineEntry models an aggregated event in an enterprise compliance timeline.
type TimelineEntry struct {
	Date        string                    `json:"date"`
	Actor       string                    `json:"actor"`
	ActionCount int                       `json:"action_count"`
	Events      []audit.EnterpriseLogEvent `json:"events"`
}

// GetTimeline groups audit logs by calendar day and actor for executive timeline views.
func (h *UnifiedLogHandler) GetTimeline(ctx context.Context, filter audit.AuditLogFilter) ([]TimelineEntry, error) {
	if h.repo == nil {
		return nil, fmt.Errorf("audit repository is not configured")
	}

	logs, _, err := h.repo.QueryLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	grouped := make(map[string]*TimelineEntry)
	var order []string

	for _, ev := range logs {
		dayKey := fmt.Sprintf("%s|%s", ev.Timestamp.Format("2006-01-02"), ev.Actor.Email)
		entry, exists := grouped[dayKey]
		if !exists {
			entry = &TimelineEntry{
				Date:   ev.Timestamp.Format("2006-01-02"),
				Actor:  ev.Actor.Email,
				Events: make([]audit.EnterpriseLogEvent, 0),
			}
			grouped[dayKey] = entry
			order = append(order, dayKey)
		}
		entry.Events = append(entry.Events, ev)
		entry.ActionCount++
	}

	var timeline []TimelineEntry
	for _, k := range order {
		timeline = append(timeline, *grouped[k])
	}

	return timeline, nil
}

// ExportCSV exports audit logs into a standard RFC 4180 CSV document.
func (h *UnifiedLogHandler) ExportCSV(ctx context.Context, filter audit.AuditLogFilter) ([]byte, error) {
	if h.repo == nil {
		return nil, fmt.Errorf("audit repository is not configured")
	}

	logs, _, err := h.repo.QueryLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write CSV Header
	header := []string{
		"Event ID", "Timestamp (UTC)", "Category", "Action",
		"Actor User ID", "Actor Email", "Client IP",
		"Target Entity ID", "Target Type", "Hash",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, ev := range logs {
		row := []string{
			ev.ID.String(),
			ev.Timestamp.Format(time.RFC3339),
			string(ev.Category),
			ev.Action,
			ev.Actor.UserID,
			ev.Actor.Email,
			ev.Actor.ClientIP,
			ev.Target.TargetEntityID,
			ev.Target.TargetType,
			ev.Hash,
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return buf.Bytes(), writer.Error()
}

// ExportJSONLines exports audit logs as newline-delimited JSON (NDJSON) for SIEM ingest.
func (h *UnifiedLogHandler) ExportJSONLines(ctx context.Context, filter audit.AuditLogFilter) ([]byte, error) {
	if h.repo == nil {
		return nil, fmt.Errorf("audit repository is not configured")
	}

	logs, _, err := h.repo.QueryLogs(ctx, filter)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)

	for _, ev := range logs {
		if err := encoder.Encode(ev); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
