package chat

import (
	"strings"
	"testing"
)

func TestPersonaRegistry(t *testing.T) {
	if len(AvailablePersonas) == 0 {
		t.Fatalf("expected at least 1 persona in registry")
	}

	defaultPersona := GetPersona("drixy")
	if defaultPersona == nil {
		t.Fatalf("expected default drixy persona to exist")
	}

	if defaultPersona.Name != "ScanDrix Review Engine" {
		t.Errorf("expected ScanDrix Review Engine persona name, got %s", defaultPersona.Name)
	}

	// Verify persona prompt includes code review instructions
	if !strings.Contains(defaultPersona.Prompt, "ScanDrix") {
		t.Errorf("expected Prompt to mention ScanDrix")
	}
}

func TestColorThemeTokens(t *testing.T) {
	if ColorLogo == "" || ColorHighlight == "" {
		t.Errorf("expected valid color tokens in chat theme")
	}
}
