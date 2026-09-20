package codereview

import (
	"testing"
)

func TestDefaultConfigAndValidation(t *testing.T) {
	cfg := GetDefaultScanDrixConfigFile()
	if cfg.Version != "1.2" {
		t.Fatalf("expected version 1.2, got %s", cfg.Version)
	}
	if len(cfg.ReviewIgnoredFiles) == 0 {
		t.Fatalf("expected non-empty ignored files")
	}

	res := ValidateScanDrixConfigFile(&cfg)
	if !res.IsValid {
		t.Fatalf("expected default config to be valid, got errors: %v", res.ErrorMessages)
	}

	// Test deprecated migration
	cfgOld := cfg
	cfgOld.Version = "1.0"
	resOld := ValidateScanDrixConfigFile(&cfgOld)
	if !resOld.IsDeprecated {
		t.Fatalf("expected version 1.0 to be flagged as deprecated")
	}

	// Test invalid review cadence
	cfgInvalid := cfg
	cfgInvalid.ReviewCadence = "invalid_cadence"
	resInvalid := ValidateScanDrixConfigFile(&cfgInvalid)
	if resInvalid.IsValid {
		t.Fatalf("expected invalid review cadence to fail validation")
	}
}

func TestCategoryAndSeverityDefaults(t *testing.T) {
	if _, ok := V2CategoryDescriptions["bug"]; !ok {
		t.Fatalf("missing bug category description")
	}
	if _, ok := V2SeverityFlags["critical"]; !ok {
		t.Fatalf("missing critical severity flag")
	}
	if _, ok := V2LevelText["issue"]; !ok {
		t.Fatalf("missing issue level text")
	}
}
