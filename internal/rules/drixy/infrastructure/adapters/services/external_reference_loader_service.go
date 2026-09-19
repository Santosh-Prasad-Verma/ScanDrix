// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: external_reference_loader_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// ExternalReferenceLoaderService inspects and resolves external file references in rules.
type ExternalReferenceLoaderService struct{}

// NewExternalReferenceLoaderService constructs a new loader service.
func NewExternalReferenceLoaderService() *ExternalReferenceLoaderService {
	return &ExternalReferenceLoaderService{}
}

// DetectFileReferences extracts `@file:<path>` syntax markers from rule text.
func (s *ExternalReferenceLoaderService) DetectFileReferences(ruleText string) []string {
	var references []string
	tokens := strings.Fields(ruleText)
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "@file:") {
			ref := strings.TrimPrefix(tok, "@file:")
			ref = strings.Trim(ref, `"',;:()[]{}<>`)
			if ref != "" {
				references = append(references, ref)
			}
		}
	}
	return references
}

// ValidateFileReference confirms a referenced path conforms to safety and scoping limits.
func (s *ExternalReferenceLoaderService) ValidateFileReference(ref string) error {
	if strings.Contains(ref, "..") {
		return fmt.Errorf("directory traversal path prohibited in file reference: %s", ref)
	}
	if strings.HasPrefix(ref, "/") {
		return fmt.Errorf("absolute path prohibited in file reference: %s", ref)
	}
	return nil
}

// EnrichRule attaches detected reference metadata to a rule.
func (s *ExternalReferenceLoaderService) EnrichRule(ctx context.Context, rule *interfaces.DrixyRule) {
	if rule == nil {
		return
	}
	refs := s.DetectFileReferences(rule.Rule)
	if len(refs) > 0 && rule.ContextReferenceID == "" {
		rule.ContextReferenceID = refs[0]
	}
}
