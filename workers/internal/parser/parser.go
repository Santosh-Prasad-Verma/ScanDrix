package parser

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/codehound/codehound/shared/domain"
	"github.com/google/uuid"
)

// ParseResult encapsulates extracted symbols and call edges for a file.
type ParseResult struct {
	Symbols []domain.CodeSymbol
	Edges   []domain.CodeEdge
}

// SimpleParser provides basic symbol extraction for demonstration and testing.
type SimpleParser struct{}

func New() *SimpleParser {
	return &SimpleParser{}
}

// ComputeHash returns the SHA-256 hash of a file's content.
func ComputeHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ExtractBasicSymbols provides fallback extraction.
func (p *SimpleParser) ExtractBasicSymbols(fileID uuid.UUID, content []byte) ParseResult {
	return ParseResult{
		Symbols: []domain.CodeSymbol{
			{
				ID:         uuid.New(),
				CodeFileID: fileID,
				SymbolKey:  "main",
				Name:       "main",
				Kind:       "FUNCTION",
				StartLine:  1,
				EndLine:    10,
				Visibility: "PUBLIC",
			},
		},
		Edges: nil,
	}
}
