package catalog_test

import (
	"testing"

	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/enums"
)

func TestCatalogIntegrity(t *testing.T) {
	if len(catalog.EventDefaultsMap) < 20 {
		t.Fatalf("expected at least 20 events in EventDefaultsMap, got %d", len(catalog.EventDefaultsMap))
	}

	for event, def := range catalog.EventDefaultsMap {
		if string(event) == "" {
			t.Fatal("empty event identifier found")
		}
		if def.Category == "" {
			t.Fatalf("event %s missing category", event)
		}
		if def.Label == "" {
			t.Fatalf("event %s missing label", event)
		}
		if len(def.DefaultChannels) == 0 {
			t.Fatalf("event %s has no default channels", event)
		}

		// Ensure all default channels are active
		for _, ch := range def.DefaultChannels {
			if !ch.IsActive() {
				t.Fatalf("event %s references inactive channel: %s", event, ch)
			}
		}
	}

	// Verify Category labels
	for _, cat := range catalog.EventCategories {
		label, ok := catalog.CategoryLabels[cat]
		if !ok || label == "" {
			t.Fatalf("category %s missing display label", cat)
		}
	}

	// Verify Channel labels
	for ch := range enums.ActiveChannels {
		label, ok := catalog.ChannelLabels[ch]
		if !ok || label == "" {
			t.Fatalf("channel %s missing display label", ch)
		}
	}
}
