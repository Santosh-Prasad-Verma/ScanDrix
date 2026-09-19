package repositories

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
)

// CliSessionCaptureRepository provides thread-safe storage for CLI session captures.
type CliSessionCaptureRepository struct {
	mu          sync.RWMutex
	byCaptureID map[string]*domain.CliSessionCapture
	byDedupKey  map[string]*domain.CliSessionCapture
}

// NewCliSessionCaptureRepository initializes the repository.
func NewCliSessionCaptureRepository() *CliSessionCaptureRepository {
	return &CliSessionCaptureRepository{
		byCaptureID: make(map[string]*domain.CliSessionCapture),
		byDedupKey:  make(map[string]*domain.CliSessionCapture),
	}
}

// Create inserts or records a new session capture.
func (r *CliSessionCaptureRepository) Create(ctx context.Context, capture *domain.CliSessionCapture) (*domain.CliSessionCapture, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if capture.CaptureID == "" {
		capture.CaptureID = uuid.New().String()
	}
	if capture.CreatedAt.IsZero() {
		capture.CreatedAt = time.Now().UTC()
	}
	capture.UpdatedAt = time.Now().UTC()
	if capture.Status == "" {
		capture.Status = "pending"
	}

	dedupKey := fmt.Sprintf("%s:%s:%s", capture.OrganizationID, capture.TeamID, capture.CaptureID)
	r.byCaptureID[capture.CaptureID] = capture
	r.byDedupKey[dedupKey] = capture

	return capture, nil
}

// FindByDedupKey locates a capture using an idempotency deduplication key.
func (r *CliSessionCaptureRepository) FindByDedupKey(ctx context.Context, dedupKey string) (*domain.CliSessionCapture, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.byDedupKey[dedupKey]
	if !ok {
		return nil, nil
	}
	return rec, nil
}

// FindByCaptureID finds a capture record by its unique captureId.
func (r *CliSessionCaptureRepository) FindByCaptureID(ctx context.Context, captureID string) (*domain.CliSessionCapture, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.byCaptureID[captureID]
	if !ok {
		return nil, nil
	}
	return rec, nil
}

// MarkProcessing updates the capture status to processing.
func (r *CliSessionCaptureRepository) MarkProcessing(ctx context.Context, captureID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.byCaptureID[captureID]
	if !ok {
		return fmt.Errorf("capture %s not found", captureID)
	}

	rec.Status = "processing"
	rec.ErrorMessage = ""
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkCompleted records extracted decisions and sets status to completed.
func (r *CliSessionCaptureRepository) MarkCompleted(
	ctx context.Context,
	captureID string,
	decisions []domain.CliSessionClassifiedDecision,
	source string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.byCaptureID[captureID]
	if !ok {
		return fmt.Errorf("capture %s not found", captureID)
	}

	rec.Status = "completed"
	rec.ClassifiedDecisions = decisions
	rec.ClassificationSource = source
	rec.ErrorMessage = ""
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkSkipped sets classification status to skipped with rationale.
func (r *CliSessionCaptureRepository) MarkSkipped(ctx context.Context, captureID string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.byCaptureID[captureID]
	if !ok {
		return fmt.Errorf("capture %s not found", captureID)
	}

	rec.Status = "skipped"
	rec.ErrorMessage = reason
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkFailed records classification error.
func (r *CliSessionCaptureRepository) MarkFailed(ctx context.Context, captureID string, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.byCaptureID[captureID]
	if !ok {
		return fmt.Errorf("capture %s not found", captureID)
	}

	rec.Status = "failed"
	rec.ErrorMessage = errorMsg
	rec.UpdatedAt = time.Now().UTC()
	return nil
}
