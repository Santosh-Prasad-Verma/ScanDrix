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
		{"Free tier sonnet blocked without BYOK", license.TierCommunity, "claude-sonnet-5", false, false},
		{"Free tier opus blocked without BYOK", license.TierCommunity, "claude-opus-5", false, false},
		{"Free tier gpt terra blocked without BYOK", license.TierCommunity, "gpt-5.6-terra", false, false},
		{"Free tier sonnet ALLOWED with BYOK", license.TierCommunity, "claude-sonnet-5", true, true},
		{"Free tier opus ALLOWED with BYOK", license.TierCommunity, "claude-opus-5", true, true},

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
	if qFree.MonthlyTokens != 500_000 || qFree.BurstLimitPerMin != 50_000 {
		t.Fatalf("unexpected free quota: %+v", qFree)
	}

	qTeam := license.GetPlanQuota(license.TierTeam)
	if qTeam.MonthlyTokens != 10_000_000 || qTeam.BurstLimitPerMin != 500_000 {
		t.Fatalf("unexpected team quota: %+v", qTeam)
	}

	qEnt := license.GetPlanQuota(license.TierEnterprise)
	if qEnt.MonthlyTokens != 100_000_000 || qEnt.BurstLimitPerMin != 2_000_000 {
		t.Fatalf("unexpected enterprise quota: %+v", qEnt)
	}
}
