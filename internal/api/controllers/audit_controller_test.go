// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
)

type mockAuditRepo struct {
	logs           []models.AuditLogRecord
	pentestReports map[uuid.UUID]*models.PentestAuditReportRecord
}

func newMockAuditRepo() *mockAuditRepo {
	return &mockAuditRepo{
		logs:           make([]models.AuditLogRecord, 0),
		pentestReports: make(map[uuid.UUID]*models.PentestAuditReportRecord),
	}
}

func (m *mockAuditRepo) ListAuditLogs(ctx context.Context, wsID uuid.UUID, limit int) ([]models.AuditLogRecord, error) {
	var res []models.AuditLogRecord
	for _, l := range m.logs {
		if l.WorkspaceID == wsID {
			res = append(res, l)
		}
	}
	return res, nil
}

func (m *mockAuditRepo) CreatePentestAuditReport(ctx context.Context, report *models.PentestAuditReportRecord) error {
	if report.ID == uuid.Nil {
		report.ID = uuid.New()
	}
	m.pentestReports[report.ID] = report
	return nil
}

func (m *mockAuditRepo) GetPentestAuditReport(ctx context.Context, wsID, reportID uuid.UUID) (*models.PentestAuditReportRecord, error) {
	rep, ok := m.pentestReports[reportID]
	if !ok || rep.WorkspaceID != wsID {
		return nil, http.ErrNoLocation
	}
	return rep, nil
}

func (m *mockAuditRepo) ListPentestAuditReports(ctx context.Context, wsID uuid.UUID, limit, offset int) ([]models.PentestAuditReportRecord, error) {
	var list []models.PentestAuditReportRecord
	for _, r := range m.pentestReports {
		if r.WorkspaceID == wsID {
			list = append(list, *r)
		}
	}
	return list, nil
}

func TestAuditController_PentestEndpoints(t *testing.T) {
	repo := newMockAuditRepo()
	ctrl := NewAuditController(repo)

	wsID := uuid.New()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := auth.WithWorkspaceContext(req.Context(), wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/workspaces/{workspaceId}/audit-logs", ctrl.Routes())

	// 1. Test POST /pentest-reports
	createPayload := models.PentestAuditReportRecord{
		TargetURI:      "https://api.example.com",
		TargetType:     "http_endpoint",
		Mode:           "autonomous",
		PassAssessment: true,
		FindingsCount:  2,
		CriticalCount:  0,
		HighCount:      1,
		Summary:        "Autonomous scan clean",
		ReportMarkdown: "# Pentest Report\nPassed.",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	body, _ := json.Marshal(createPayload)

	req := httptest.NewRequest(http.MethodPost, "/workspaces/"+wsID.String()+"/audit-logs/pentest-reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created models.PentestAuditReportRecord
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed decoding created report: %v", err)
	}
	if created.WorkspaceID != wsID {
		t.Errorf("expected WorkspaceID %s, got %s", wsID, created.WorkspaceID)
	}

	// 2. Test GET /pentest-reports (List)
	listReq := httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/pentest-reports", nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", listW.Code, listW.Body.String())
	}

	var reports []models.PentestAuditReportRecord
	if err := json.Unmarshal(listW.Body.Bytes(), &reports); err != nil {
		t.Fatalf("failed decoding list of reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report in list, got %d", len(reports))
	}

	// 3. Test GET /pentest-reports/{reportId} (Get by ID)
	getReq := httptest.NewRequest(http.MethodGet, "/workspaces/"+wsID.String()+"/audit-logs/pentest-reports/"+created.ID.String(), nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for report retrieval, got %d: %s", getW.Code, getW.Body.String())
	}

	var fetched models.PentestAuditReportRecord
	if err := json.Unmarshal(getW.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("failed decoding fetched report: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("expected report ID %s, got %s", created.ID, fetched.ID)
	}
}
