package tools

import (
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

// ReviewToolOptions bundles configuration for assembling the 18 review tools.
type ReviewToolOptions struct {
	Sandbox              SandboxExecutor
	FS                   RepositoryFS
	Git                  GitClient
	ReferenceFetcher     SCMReferenceFetcher
	DocumentationAdapter DocumentationSearchAdapter
	CallGraphData        string
	EnableCaching        bool
	EnableOutlineFirst   bool
	OutlineThreshold     int
	MaxFileReadBytes     int
	MaxDocsLength        int
	DocsMap              map[string]string
}

// MeasuredTool wraps an AgentTool to collect execution metrics.
type MeasuredTool struct {
	inner   contracts.AgentTool
	metrics *ExecutionMetrics
}

func (m *MeasuredTool) Name() string                        { return m.inner.Name() }
func (m *MeasuredTool) Description() string                 { return m.inner.Description() }
func (m *MeasuredTool) InputSchema() contracts.JSONSchema   { return m.inner.InputSchema() }
func (m *MeasuredTool) Strict() bool                        { return m.inner.Strict() }
func (m *MeasuredTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	start := time.Now()
	res, err := m.inner.Execute(ctx, input)
	duration := time.Since(start)
	if m.metrics != nil {
		m.metrics.Record(m.inner.Name(), duration, res.IsError || err != nil)
	}
	return res, err
}

// ReadOnlyNavigationTools defines tools whose responses are pure functions of repo state and can be safely cached.
var ReadOnlyNavigationTools = map[string]bool{
	"grep":            true,
	"readFile":        true,
	"listDir":         true,
	"findFile":        true,
	"getCallers":      true,
	"readReference":   true,
	"gitBlame":        true,
	"gitLog":          true,
	"gitDiffRange":    true,
	"symbolOutline":   true,
	"astQuery":        true,
	"importGraph":     true,
	"securityScanner": true,
}

// BuildCompleteToolRegistry creates the enterprise 18-tool suite with caching and metrics.
func BuildCompleteToolRegistry(opts ReviewToolOptions) (contracts.ToolRegistry, *tools.ToolCallCache, *ExecutionMetrics) {
	cache := tools.NewToolCallCache()
	metrics := NewExecutionMetrics()

	rawTools := []contracts.AgentTool{
		// 1. Grep
		NewGrepTool(GrepToolOptions{
			Sandbox: opts.Sandbox,
			FS:      opts.FS,
		}),
		// 2. ReadFile
		NewReadFileTool(ReadFileToolOptions{
			Sandbox:          opts.Sandbox,
			FS:               opts.FS,
			MaxReadLength:    opts.MaxFileReadBytes,
			OutlineFirst:     opts.EnableOutlineFirst,
			OutlineThreshold: opts.OutlineThreshold,
		}),
		// 3. ListDir
		NewListDirTool(ListDirToolOptions{
			Sandbox: opts.Sandbox,
			FS:      opts.FS,
		}),
		// 4. FindFile
		NewFindFileTool(FindFileToolOptions{
			Sandbox: opts.Sandbox,
			FS:      opts.FS,
		}),
		// 5. CheckTypes
		NewCheckTypesTool(CheckTypesToolOptions{
			Sandbox: opts.Sandbox,
		}),
		// 6. GetCallers
		NewGetCallersTool(GetCallersToolOptions{
			CallGraphData: opts.CallGraphData,
			FS:            opts.FS,
			Sandbox:       opts.Sandbox,
		}),
		// 7. ReadReference
		NewReadReferenceTool(ReadReferenceToolOptions{
			Fetcher: opts.ReferenceFetcher,
			MaxLen:  opts.MaxFileReadBytes,
		}),
		// 8. SearchDocs
		NewSearchDocsTool(SearchDocsToolOptions{
			Adapter: opts.DocumentationAdapter,
			Docs:    opts.DocsMap,
			MaxLen:  opts.MaxDocsLength,
		}),
		// 9. SubmitResult (Done tool for Finder)
		NewSubmitResultTool(),
		// 10. SubmitVerdict (Done tool for Verifier)
		NewSubmitVerdictTool(),
		// 11. GitBlame
		NewGitBlameTool(GitBlameToolOptions{
			Git:     opts.Git,
			Sandbox: opts.Sandbox,
		}),
		// 12. GitLog
		NewGitLogTool(GitLogToolOptions{
			Git:     opts.Git,
			Sandbox: opts.Sandbox,
		}),
		// 13. GitDiffRange
		NewGitDiffRangeTool(GitDiffRangeToolOptions{
			Git:     opts.Git,
			Sandbox: opts.Sandbox,
		}),
		// 14. RunTests
		NewRunTestsTool(RunTestsToolOptions{
			Sandbox: opts.Sandbox,
		}),
		// 15. SymbolOutline
		NewSymbolOutlineTool(SymbolOutlineToolOptions{
			FS:      opts.FS,
			Sandbox: opts.Sandbox,
		}),
		// 16. AstQuery
		NewAstQueryTool(AstQueryToolOptions{
			FS:      opts.FS,
			Sandbox: opts.Sandbox,
		}),
		// 17. ImportGraph
		NewImportGraphTool(ImportGraphToolOptions{
			FS:      opts.FS,
			Sandbox: opts.Sandbox,
		}),
		// 18. SecurityScanner
		NewSecurityScannerTool(SecurityScannerToolOptions{
			FS:      opts.FS,
			Sandbox: opts.Sandbox,
		}),
	}

	finalTools := make([]contracts.AgentTool, 0, len(rawTools))
	for _, tool := range rawTools {
		wrapped := contracts.AgentTool(tool)
		if opts.EnableCaching && ReadOnlyNavigationTools[tool.Name()] {
			wrapped = tools.NewCachingTool(wrapped, cache)
		}
		measured := &MeasuredTool{
			inner:   wrapped,
			metrics: metrics,
		}
		finalTools = append(finalTools, measured)
	}

	registry := tools.NewInMemoryToolRegistry(finalTools...)
	return registry, cache, metrics
}
