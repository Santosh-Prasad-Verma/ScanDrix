package dora

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DORAStore manages production deployment logs and incident metrics.
type DORAStore struct {
	mu      sync.RWMutex
	records []DeploymentRecord
}

// NewDORAStore initializes the deployment log store.
func NewDORAStore() *DORAStore {
	return &DORAStore{
		records: make([]DeploymentRecord, 0),
	}
}

// RecordDeployment appends a new production release record.
func (s *DORAStore) RecordDeployment(ctx context.Context, rec DeploymentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.ID == uuid.Nil {
		rec.ID = uuid.New()
	}
	if rec.DeployedAt.IsZero() {
		rec.DeployedAt = time.Now().UTC()
	}

	s.records = append(s.records, rec)
	return nil
}

// MarkFailed marks a deployment as having caused a production incident.
func (s *DORAStore) MarkFailed(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].IsFailed = true
			return nil
		}
	}
	return errors.New("deployment record not found")
}

// ResolveIncident marks the incident resolution time for MTTR tracking.
func (s *DORAStore) ResolveIncident(ctx context.Context, id uuid.UUID, resolvedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].IncidentResolvedAt = &resolvedAt
			return nil
		}
	}
	return errors.New("deployment record not found")
}

// GenerateReport calculates the DORA executive report for a given workspace and window.
func (s *DORAStore) GenerateReport(
	ctx context.Context,
	wsID uuid.UUID,
	start, end time.Time,
	totalPRs, defectsCaught int,
) (*DORAReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matching []DeploymentRecord
	for _, r := range s.records {
		if r.WorkspaceID == wsID && !r.DeployedAt.Before(start) && !r.DeployedAt.After(end) {
			matching = append(matching, r)
		}
	}

	return CalculateDORA(wsID, start, end, matching, totalPRs, defectsCaught), nil
}
