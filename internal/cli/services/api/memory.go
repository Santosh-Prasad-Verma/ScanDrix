// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api

import (
	"context"
	"net/http"
	"time"
)

// MEMORY & DECISION CAPTURE DTOs

// DecisionCapturePayload describes an architectural decision or agent session event.
type DecisionCapturePayload struct {
	Agent     string    `json:"agent"`
	Event     string    `json:"event"`
	Summary   string    `json:"summary,omitempty"`
	Payload   any       `json:"payload,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// MEMORY API OPERATIONS

// CaptureDecision submits an agent decision or lifecycle hook event to the server.
func (c *Client) CaptureDecision(ctx context.Context, payload DecisionCapturePayload) error {
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now().UTC()
	}
	return c.Do(ctx, http.MethodPost, "/api/v1/decisions/capture", payload, nil)
}
