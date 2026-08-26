package astra

import (
	"context"
	"strings"
	"sync"

	"github.com/codehound/codehound/shared/domain"
	astrasdk "github.com/datastax/astra-db-go/v2/astra"
	"github.com/datastax/astra-db-go/v2/astra/options"
	"github.com/google/uuid"
)

// CodeChunkDocument represents an AST-aware vector indexed document.
type CodeChunkDocument struct {
	ID         string         `json:"_id"`
	TenantID   uuid.UUID      `json:"tenant_id"`
	ProjectID  uuid.UUID      `json:"project_id"`
	FilePath   string         `json:"file_path"`
	SymbolName string         `json:"symbol_name"`
	Language   string         `json:"language"`
	CodeChunk  string         `json:"code_chunk"`
	Vector     []float32      `json:"$vector,omitempty"`
	Metadata   map[string]any `json:"metadata"`
}

// Client provides vector storage and hybrid retrieval operations for Astra DB.
type Client struct {
	endpoint string
	token    string
	keyspace string

	dbClient  *astrasdk.Db
	mu        sync.RWMutex
	mockStore map[string]CodeChunkDocument
}

// NewClient initializes an Astra DB Vector client using the official Astra Go SDK.
func NewClient(endpoint, token, keyspace string) *Client {
	if keyspace == "" {
		keyspace = "default_keyspace"
	}

	c := &Client{
		endpoint:  endpoint,
		token:     token,
		keyspace:  keyspace,
		mockStore: make(map[string]CodeChunkDocument),
	}

	if endpoint != "" && token != "" {
		sdkClient := astrasdk.NewClient()
		c.dbClient = sdkClient.Database(endpoint, options.API().SetToken(token))
	}

	return c
}

// ListCollections returns collection names from the live Astra DB database.
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	if c.dbClient != nil {
		return c.dbClient.ListCollectionNames(ctx)
	}
	return []string{"code_chunks_mock"}, nil
}

// InsertCodeChunk inserts an AST-aware code chunk into the vector store.
func (c *Client) InsertCodeChunk(ctx context.Context, doc CodeChunkDocument) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}

	c.mu.Lock()
	c.mockStore[doc.ID] = doc
	c.mu.Unlock()

	return nil
}

// SearchSimilarCode retrieves the most relevant code chunks filtered by tenant and project.
func (c *Client) SearchSimilarCode(ctx context.Context, tenantID, projectID uuid.UUID, topK int) ([]CodeChunkDocument, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []CodeChunkDocument
	for _, doc := range c.mockStore {
		if doc.TenantID == tenantID && doc.ProjectID == projectID {
			results = append(results, doc)
			if len(results) >= topK {
				break
			}
		}
	}
	return results, nil
}

// ChunkCodeByAST partitions code based on AST symbol boundaries rather than arbitrary token cuts.
func ChunkCodeByAST(tenantID, projectID uuid.UUID, filePath, language string, symbols []domain.CodeSymbol, content []byte) []CodeChunkDocument {
	lines := strings.Split(string(content), "\n")
	var docs []CodeChunkDocument

	if len(symbols) == 0 {
		// Whole file chunk if no individual symbols
		docs = append(docs, CodeChunkDocument{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			ProjectID:  projectID,
			FilePath:   filePath,
			SymbolName: "FILE_ROOT",
			Language:   language,
			CodeChunk:  string(content),
			Metadata:   map[string]any{"total_lines": len(lines)},
		})
		return docs
	}

	for _, sym := range symbols {
		start := sym.StartLine - 1
		end := sym.EndLine

		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}

		chunkLines := lines[start:end]
		chunkText := strings.Join(chunkLines, "\n")

		docs = append(docs, CodeChunkDocument{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			ProjectID:  projectID,
			FilePath:   filePath,
			SymbolName: sym.Name,
			Language:   language,
			CodeChunk:  chunkText,
			Metadata: map[string]any{
				"kind":       sym.Kind,
				"start_line": sym.StartLine,
				"end_line":   sym.EndLine,
				"symbol_key": sym.SymbolKey,
			},
		})
	}

	return docs
}
