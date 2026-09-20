package providers

import (
	"context"
	"fmt"
	"sync"
)

// ProviderToken uniquely identifies an enterprise service in the provider registry.
type ProviderToken string

const (
	PipelineProviderToken           ProviderToken = "PIPELINE_PROVIDER"
	CodeReviewPipelineToken         ProviderToken = "CODE_REVIEW_PIPELINE"
	FileAnalyzerProviderToken       ProviderToken = "FILE_ANALYZER_PROVIDER"
	ContextResolutionProviderToken  ProviderToken = "CONTEXT_RESOLUTION_PROVIDER"
)

// PipelineInstance represents an executable code review pipeline stage or workflow.
type PipelineInstance interface {
	Name() string
	Execute(ctx context.Context, payload interface{}) (interface{}, error)
}

// FileAnalyzerInstance represents an AST or heuristic code analyzer.
type FileAnalyzerInstance interface {
	Analyze(ctx context.Context, filename string, content []byte) (interface{}, error)
}

// ProviderRegistry provides thread-safe dependency injection and service locator functionality.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[ProviderToken]interface{}
	factories map[ProviderToken]func() (interface{}, error)
}

var (
	defaultRegistry *ProviderRegistry
	registryOnce    sync.Once
)

// GetDefaultRegistry returns the global ProviderRegistry singleton.
func GetDefaultRegistry() *ProviderRegistry {
	registryOnce.Do(func() {
		defaultRegistry = NewProviderRegistry()
	})
	return defaultRegistry
}

// NewProviderRegistry constructs a new ProviderRegistry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[ProviderToken]interface{}),
		factories: make(map[ProviderToken]func() (interface{}, error)),
	}
}

// Register registers a concrete instance under a token.
func (r *ProviderRegistry) Register(token ProviderToken, instance interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[token] = instance
}

// RegisterFactory registers a dynamic factory function.
func (r *ProviderRegistry) RegisterFactory(token ProviderToken, factory func() (interface{}, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[token] = factory
}

// Resolve retrieves or instantiates a service by token.
func (r *ProviderRegistry) Resolve(token ProviderToken) (interface{}, error) {
	r.mu.RLock()
	if inst, ok := r.providers[token]; ok {
		r.mu.RUnlock()
		return inst, nil
	}
	factory, hasFactory := r.factories[token]
	r.mu.RUnlock()

	if hasFactory {
		return factory()
	}

	return nil, fmt.Errorf("provider not found for token: %s", token)
}

// ResolvePipeline resolves a pipeline instance.
func (r *ProviderRegistry) ResolvePipeline(token ProviderToken) (PipelineInstance, error) {
	inst, err := r.Resolve(token)
	if err != nil {
		return nil, err
	}
	pipe, ok := inst.(PipelineInstance)
	if !ok {
		return nil, fmt.Errorf("instance for token %s does not implement PipelineInstance", token)
	}
	return pipe, nil
}

// ResolveFileAnalyzer resolves a file analyzer instance.
func (r *ProviderRegistry) ResolveFileAnalyzer(token ProviderToken) (FileAnalyzerInstance, error) {
	inst, err := r.Resolve(token)
	if err != nil {
		return nil, err
	}
	analyzer, ok := inst.(FileAnalyzerInstance)
	if !ok {
		return nil, fmt.Errorf("instance for token %s does not implement FileAnalyzerInstance", token)
	}
	return analyzer, nil
}
