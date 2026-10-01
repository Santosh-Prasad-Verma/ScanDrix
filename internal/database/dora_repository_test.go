package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDORAReport_AbsenceContract(t *testing.T) {
	// Master Rule 2.7 Verification:
	// When repository is nil or database has no metrics, GetDORAMetrics must return
	// nil pointers and explicit unavailable reasons rather than fake zeros or fabricated 88.0 scores.
	repo := (*Repository)(nil)
	wsID := uuid.New()
	since := time.Now().AddDate(0, 0, -30)

	report, err := repo.GetDORAMetrics(context.Background(), wsID, since)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, wsID, report.WorkspaceID)

	// Metrics must be nil when absent
	assert.Nil(t, report.DeploymentFrequency)
	assert.Nil(t, report.LeadTimeSeconds)
	assert.Nil(t, report.LeadTimeP90Seconds)
	assert.Nil(t, report.ChangeFailureRate)
	assert.Nil(t, report.MTTRSeconds)

	// Machine-readable absence indicator
	assert.Contains(t, report.Unavailable, "no_data_source")
}

func TestDORAReport_StructureAndPointers(t *testing.T) {
	wsID := uuid.New()
	deployVal := 4.5
	leadVal := 3600.0
	p90Val := 7200.0
	cfrVal := 0.05
	mttrVal := 1800.0

	report := &DORAReport{
		WorkspaceID:         wsID,
		PeriodStart:         time.Now().Add(-24 * time.Hour),
		PeriodEnd:           time.Now(),
		DeploymentFrequency: &deployVal,
		LeadTimeSeconds:     &leadVal,
		LeadTimeP90Seconds:  &p90Val,
		ChangeFailureRate:   &cfrVal,
		MTTRSeconds:         &mttrVal,
		TotalReviews:        12,
		TotalFindings:       3,
		Unavailable:         []string{},
	}

	assert.Equal(t, 4.5, *report.DeploymentFrequency)
	assert.Equal(t, 3600.0, *report.LeadTimeSeconds)
	assert.Equal(t, 7200.0, *report.LeadTimeP90Seconds)
	assert.Equal(t, 0.05, *report.ChangeFailureRate)
	assert.Equal(t, 1800.0, *report.MTTRSeconds)
	assert.Empty(t, report.Unavailable)
}
