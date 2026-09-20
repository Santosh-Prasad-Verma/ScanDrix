package providers

import (
	"context"
	"sync"
	"testing"
)

type mockPipeline struct {
	name string
}

func (m *mockPipeline) Name() string { return m.name }
func (m *mockPipeline) Execute(ctx context.Context, payload interface{}) (interface{}, error) {
	return "ok", nil
}

type mockAnalyzer struct{}

func (m *mockAnalyzer) Analyze(ctx context.Context, filename string, content []byte) (interface{}, error) {
	return 42, nil
}

func TestProviderRegistry(t *testing.T) {
	reg := NewProviderRegistry()

	pipe := &mockPipeline{name: "TestPipeline"}
	reg.Register(PipelineProviderToken, pipe)

	resolved, err := reg.ResolvePipeline(PipelineProviderToken)
	if err != nil {
		t.Fatalf("unexpected error resolving pipeline: %v", err)
	}
	if resolved.Name() != "TestPipeline" {
		t.Fatalf("expected TestPipeline, got %s", resolved.Name())
	}

	reg.RegisterFactory(FileAnalyzerProviderToken, func() (interface{}, error) {
		return &mockAnalyzer{}, nil
	})

	analyzer, err := reg.ResolveFileAnalyzer(FileAnalyzerProviderToken)
	if err != nil {
		t.Fatalf("unexpected error resolving analyzer: %v", err)
	}
	res, _ := analyzer.Analyze(context.Background(), "test.go", []byte("package main"))
	if res != 42 {
		t.Fatalf("expected 42, got %v", res)
	}
}

func TestProviderRegistryConcurrency(t *testing.T) {
	reg := NewProviderRegistry()
	pipe := &mockPipeline{name: "ConcurrentPipeline"}
	reg.Register(CodeReviewPipelineToken, pipe)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := reg.ResolvePipeline(CodeReviewPipelineToken)
			if err != nil || p == nil {
				t.Errorf("failed concurrent resolve: %v", err)
			}
		}()
	}
	wg.Wait()
}
