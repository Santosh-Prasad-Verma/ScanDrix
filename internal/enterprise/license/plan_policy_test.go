package license_test

import (
	"testing"

	"github.com/scandrix/backend/internal/enterprise/license"
)

func TestCanAccessModelPolicy(t *testing.T) {
	tests := []struct {
		name      string
		tier      license.LicenseTier
		modelID   string
		hasBYOK   bool
		wantAllow bool
	}{
		// Community (Free) Tests
		{"Free tier trial model allowed", license.TierCommunity, "gemini-2.5-flash-lite", false, true},
		{"Free tier trial model 2 allowed", license.TierCommunity, "minimax/minimax-m3:free", false, true},
		{"Free tier trial model glm-5.3-flash allowed", license.TierCommunity, "glm-5.3-flash", false, true},
		{"Free tier trial model zai/glm-5.3-flash allowed", license.TierCommunity, "zai/glm-5.3-flash", false, true},
		{"Free tier sonnet blocked without BYOK", license.TierCommunity, "claude-sonnet-5", false, false},
		{"Free tier opus blocked without BYOK", license.TierCommunity, "claude-opus-5", false, false},
		{"Free tier gpt terra blocked without BYOK", license.TierCommunity, "gpt-5.6-terra", false, false},
		{"Free tier sonnet ALLOWED with BYOK", license.TierCommunity, "claude-sonnet-5", true, true},
		{"Free tier opus ALLOWED with BYOK", license.TierCommunity, "claude-opus-5", true, true},

		// Developer Tier Tests
		{"Developer tier gemini-3.7-flash allowed", license.TierDeveloper, "gemini-3.7-flash", false, true},
		{"Developer tier deepseek-chat allowed", license.TierDeveloper, "deepseek-chat", false, true},
		{"Developer tier gpt-5.6-luna allowed", license.TierDeveloper, "gpt-5.6-luna", false, true},
		{"Developer tier qwen3.8-max allowed", license.TierDeveloper, "qwen3.8-max", false, true},
		{"Developer tier sonnet blocked without BYOK", license.TierDeveloper, "claude-sonnet-5", false, false},
		{"Developer tier sonnet ALLOWED with BYOK", license.TierDeveloper, "claude-sonnet-5", true, true},
		{"Developer tier opus blocked without BYOK", license.TierDeveloper, "claude-opus-5", false, false},
		{"Developer tier opus ALLOWED with BYOK", license.TierDeveloper, "claude-opus-5", true, true},

		// Pro / Team Tier Tests
		{"Pro tier sonnet 5 allowed", license.TierTeam, "claude-sonnet-5", false, true},
		{"Pro tier gpt-5.6-terra allowed", license.TierTeam, "gpt-5.6-terra", false, true},
		{"Pro tier gemini-3.7-flash allowed", license.TierTeam, "gemini-3.7-flash", false, true},
		{"Pro tier qwen3.8-max allowed", license.TierTeam, "qwen3.8-max", false, true},
		{"Pro tier claude-opus-5 blocked without BYOK", license.TierTeam, "claude-opus-5", false, false},
		{"Pro tier claude-opus-5 ALLOWED with BYOK", license.TierTeam, "claude-opus-5", true, true},

		// Enterprise Tier Tests
		{"Enterprise tier claude-opus-5 allowed", license.TierEnterprise, "claude-opus-5", false, true},
		{"Enterprise tier gpt-5.6-sol allowed", license.TierEnterprise, "gpt-5.6-sol", false, true},
		{"Enterprise tier sonnet 5 allowed", license.TierEnterprise, "claude-sonnet-5", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := license.CanAccessModel(tt.tier, tt.modelID, tt.hasBYOK)
			if allowed != tt.wantAllow {
				t.Fatalf("CanAccessModel(%s, %s, %v) = %v; want %v (reason: %s)",
					tt.tier, tt.modelID, tt.hasBYOK, allowed, tt.wantAllow, reason)
			}
		})
	}
}

func TestPlanQuotas(t *testing.T) {
	qFree := license.GetPlanQuota(license.TierCommunity)
	if qFree.MonthlyTokens != 1_000_000 || qFree.BurstLimitPerMin != 100_000 {
		t.Fatalf("unexpected free quota: %+v", qFree)
	}

	qDev := license.GetPlanQuota(license.TierDeveloper)
	if qDev.MonthlyTokens != 6_000_000 || qDev.BurstLimitPerMin != 300_000 || qDev.MaxSeats != 10 {
		t.Fatalf("unexpected developer quota: %+v", qDev)
	}

	qTeam := license.GetPlanQuota(license.TierTeam)
	if qTeam.MonthlyTokens != 14_000_000 || qTeam.BurstLimitPerMin != 700_000 || qTeam.MaxSeats != 25 {
		t.Fatalf("unexpected team quota: %+v", qTeam)
	}

	qEnt := license.GetPlanQuota(license.TierEnterprise)
	if qEnt.MonthlyTokens != 100_000_000 || qEnt.BurstLimitPerMin != 2_000_000 {
		t.Fatalf("unexpected enterprise quota: %+v", qEnt)
	}
}

func TestGetAllocatedModelsList(t *testing.T) {
	freeModels := license.GetAllocatedModelsList(license.TierCommunity)
	hasGLM := false
	hasOxAlpha := false
	for _, m := range freeModels {
		if m == "glm-5.3-flash" {
			hasGLM = true
		}
		if m == "stealth/ox-alpha" {
			hasOxAlpha = true
		}
	}
	if !hasGLM {
		t.Fatalf("expected glm-5.3-flash in free tier allocated models, got: %v", freeModels)
	}
	if hasOxAlpha {
		t.Fatalf("stealth/ox-alpha should not be in free tier allocated models")
	}

	devModels := license.GetAllocatedModelsList(license.TierDeveloper)
	if len(devModels) == 0 {
		t.Fatalf("expected non-empty developer models list")
	}

	teamModels := license.GetAllocatedModelsList(license.TierTeam)
	if len(teamModels) == 0 {
		t.Fatalf("expected non-empty team models list")
	}

	entModels := license.GetAllocatedModelsList(license.TierEnterprise)
	if len(entModels) == 0 {
		t.Fatalf("expected non-empty enterprise models list")
	}
}
