package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/contextpack"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/docdiscovery"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/knowledge"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// ITraceDecisionLoader retrieves recorded architectural decisions for diff paths.
type ITraceDecisionLoader interface {
	IsTraceContextEnabled(ctx context.Context, orgID, repoID string) (bool, error)
	LoadDecisionsForFiles(ctx context.Context, orgID, repoID string, filePaths []string) ([]pipeline.TraceDecisionInfo, error)
}

// IIssueTrackerResolver extracts issue tracking ticket references (e.g. Jira/Linear) from PR metadata.
type IIssueTrackerResolver interface {
	ResolveIssueContext(ctx context.Context, title, description string) (*pipeline.ExternalIssueContext, error)
}

// DeepLoadExternalContextStage implements Stage 6 context loading with Trace architectural records,
// issue tracker links, knowledge graph blast radius, and multi-layer context pack assembly.
type DeepLoadExternalContextStage struct {
	traceLoader      ITraceDecisionLoader
	issueResolver    IIssueTrackerResolver
	refDetector      *contextpack.ReferenceDetector
	indexer          *knowledge.SymbolIndex
	callGraph        *knowledge.CallGraph
	blastCalc        *knowledge.BlastRadiusCalculator
	contextAssembler *contextpack.ContextPackAssembler
	manifestParser   *docdiscovery.ManifestParser
	docPlanner       *docdiscovery.DocQueryPlanner
	docCache         *docdiscovery.DocSearchCache
}

// NewDeepLoadExternalContextStage constructs Stage 6 with concrete context and knowledge components.
func NewDeepLoadExternalContextStage(tl ITraceDecisionLoader, ir IIssueTrackerResolver) *DeepLoadExternalContextStage {
	indexer := knowledge.NewSymbolIndex()
	callGraph := knowledge.NewCallGraph()
	blastCalc := knowledge.NewBlastRadiusCalculator()
	assembler := contextpack.NewContextPackAssembler(nil)
	docCache := docdiscovery.NewDocSearchCache(0)
	docCache.PreloadStandardLibraryDocs()

	return &DeepLoadExternalContextStage{
		traceLoader:      tl,
		issueResolver:    ir,
		refDetector:      contextpack.NewReferenceDetector(),
		indexer:          indexer,
		callGraph:        callGraph,
		blastCalc:        blastCalc,
		contextAssembler: assembler,
		manifestParser:   docdiscovery.NewManifestParser(),
		docPlanner:       docdiscovery.NewDocQueryPlanner(),
		docCache:         docCache,
	}
}

// WithReferenceDetector overrides the reference detector.
func (s *DeepLoadExternalContextStage) WithReferenceDetector(rd *contextpack.ReferenceDetector) *DeepLoadExternalContextStage {
	s.refDetector = rd
	return s
}

// WithKnowledgeGraph overrides the symbol index and call graph.
func (s *DeepLoadExternalContextStage) WithKnowledgeGraph(indexer *knowledge.SymbolIndex, cg *knowledge.CallGraph) *DeepLoadExternalContextStage {
	s.indexer = indexer
	s.callGraph = cg
	return s
}

// WithContextPackAssembler overrides the context pack assembler.
func (s *DeepLoadExternalContextStage) WithContextPackAssembler(cpa *contextpack.ContextPackAssembler) *DeepLoadExternalContextStage {
	s.contextAssembler = cpa
	return s
}

func (s *DeepLoadExternalContextStage) Name() string {
	return "DeepLoadExternalContextStage"
}

func (s *DeepLoadExternalContextStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	if pCtx.PipelineMetadata == nil {
		pCtx.PipelineMetadata = make(map[string]interface{})
	}

	orgID := pCtx.WorkspaceID.String()
	repoID := pCtx.RepositoryID.String()

	// 1. Scan PR metadata & commit messages for references (Jira, Linear, GitHub, SHAs)
	var textToScan strings.Builder
	textToScan.WriteString(pCtx.Title)
	textToScan.WriteString("\n")
	textToScan.WriteString(pCtx.Description)
	textToScan.WriteString("\n")
	for _, c := range pCtx.PrCommits {
		textToScan.WriteString(c.Message)
		textToScan.WriteString("\n")
	}

	if s.refDetector != nil {
		detected := s.refDetector.DetectReferences(textToScan.String())
		pCtx.PipelineMetadata["detected_references"] = &detected

		// If issue tracker resolver provided, use it
		if s.issueResolver != nil {
			issueCtx, err := s.issueResolver.ResolveIssueContext(ctx, pCtx.Title, pCtx.Description)
			if err == nil && issueCtx != nil {
				pCtx.ExternalContext = issueCtx
			}
		}

		// If no resolver or resolver returned nil, but tickets were detected, populate basic ExternalContext
		if pCtx.ExternalContext == nil && len(detected.TicketKeys) > 0 {
			firstKey := detected.TicketKeys[0]
			pCtx.ExternalContext = &pipeline.ExternalIssueContext{
				IssueKey:    firstKey,
				Title:       fmt.Sprintf("Referenced Ticket %s", firstKey),
				Description: fmt.Sprintf("Ticket detected from PR metadata: %s", firstKey),
				Status:      "OPEN",
			}
		}
	} else if s.issueResolver != nil {
		issueCtx, err := s.issueResolver.ResolveIssueContext(ctx, pCtx.Title, pCtx.Description)
		if err == nil && issueCtx != nil {
			pCtx.ExternalContext = issueCtx
		}
	}

	// 2. Load ScanDrix Trace Architectural Decisions for changed files
	if s.traceLoader != nil && len(pCtx.ChangedFiles) > 0 {
		enabled, err := s.traceLoader.IsTraceContextEnabled(ctx, orgID, repoID)
		if err == nil && enabled {
			var filePaths []string
			for _, f := range pCtx.ChangedFiles {
				if f.Filename != "" {
					filePaths = append(filePaths, f.Filename)
				}
			}

			if len(filePaths) > 0 {
				decisions, err := s.traceLoader.LoadDecisionsForFiles(ctx, orgID, repoID, filePaths)
				if err == nil && len(decisions) > 0 {
					pCtx.TraceDecisions = decisions
				}
			}
		}
	}

	// 3. Build/Enrich Knowledge Graph & Calculate Blast Radius
	var changedPaths []string
	changedLineRanges := make(map[string][][2]int)
	for _, f := range pCtx.ChangedFiles {
		if f.Filename != "" {
			changedPaths = append(changedPaths, f.Filename)
			if len(f.ValidDiffLines) > 0 {
				changedLineRanges[f.Filename] = f.ValidDiffLines
			}
		}
	}

	// Index files from sandbox if available, otherwise from parsed patches
	if s.indexer != nil {
		if pCtx.SandboxHandle != nil && pCtx.SandboxHandle.RemoteCommands() != nil {
			for _, path := range changedPaths {
				content, err := pCtx.SandboxHandle.RemoteCommands().Read(ctx, path, 1, 5000)
				if err == nil && content != "" {
					s.indexer.IndexFile(path, content)
				}
			}
		} else if len(pCtx.ParsedPatches) > 0 {
			for _, patch := range pCtx.ParsedPatches {
				path := patch.NewPath
				if path == "" {
					path = patch.OldPath
				}
				if path == "" {
					continue
				}
				var lines []string
				for _, h := range patch.Hunks {
					for _, l := range h.Lines {
						if l.Type != diff.LineDeletion {
							lines = append(lines, l.Content)
						}
					}
				}
				if len(lines) > 0 {
					s.indexer.IndexFile(path, strings.Join(lines, "\n"))
				}
			}
		}
	}

	var blastReport *knowledge.BlastRadiusReport
	formatter := knowledge.NewGraphFormatter()
	if s.blastCalc != nil && len(changedPaths) > 0 {
		blastReport = s.blastCalc.CalculateBlastRadius(changedPaths, changedLineRanges, s.indexer, s.callGraph)
		if blastReport != nil {
			pCtx.PipelineMetadata["blast_radius_report"] = blastReport
			pCtx.PipelineMetadata["blast_radius_summary"] = formatter.FormatBlastRadiusMarkdown(blastReport)
			pCtx.PipelineMetadata["blast_radius_prompt"] = formatter.FormatForReviewerPrompt(blastReport, 2000)
			pCtx.PipelineMetadata["blast_radius_mermaid"] = formatter.FormatMermaidDiagram(blastReport, s.callGraph)
			pCtx.AddMetric("knowledge_blast_radius_score", 0, true, nil, int(blastReport.ImpactScore*100))
		}
	}

	// 4. Assemble Multi-Layer Context Pack
	if s.contextAssembler != nil {
		var tickets []contextpack.TicketContext
		if pCtx.ExternalContext != nil && pCtx.ExternalContext.IssueKey != "" {
			tickets = append(tickets, contextpack.TicketContext{
				IssueKey:    pCtx.ExternalContext.IssueKey,
				Provider:    "issue_tracker",
				Title:       pCtx.ExternalContext.Title,
				Description: pCtx.ExternalContext.Description,
				Status:      pCtx.ExternalContext.Status,
			})
		}

		var traceDecisions []contextpack.TraceDecision
		for _, d := range pCtx.TraceDecisions {
			traceDecisions = append(traceDecisions, contextpack.TraceDecision{
				DecisionKey: d.DecisionKey,
				Title:       d.Title,
				Summary:     d.Summary,
				Rationale:   d.Rationale,
				Files:       d.Files,
			})
		}

		kgPrompt := ""
		if blastReport != nil {
			kgPrompt = formatter.FormatForReviewerPrompt(blastReport, 2000)
		}

		var rulesSummary strings.Builder
		if len(pCtx.ActiveRules) > 0 {
			rulesSummary.WriteString("Active Drixy Rules:\n")
			for _, r := range pCtx.ActiveRules {
				rulesSummary.WriteString(fmt.Sprintf("- Rule '%s' [%s]: %s\n", r.Name, r.Severity, r.Description))
			}
		}

		assemblerInput := contextpack.AssemblerInput{
			DiffContent:          pCtx.RawDiff,
			Tickets:              tickets,
			TraceDecisions:       traceDecisions,
			KnowledgeGraphPrompt: kgPrompt,
			RulesSummary:         rulesSummary.String(),
		}

		pack, err := s.contextAssembler.Assemble(ctx, assemblerInput, contextpack.DefaultContextBudgetConfig())
		if err == nil && pack != nil {
			pCtx.PipelineMetadata["context_pack"] = pack
			pCtx.PipelineMetadata["context_pack_prompt"] = pack.AssembledPrompt
			pCtx.AddMetric("context_pack_tokens", 0, true, nil, pack.TotalTokens)
		}
	}

	// 5. Discover Modified Package Dependencies and Build Documentation Pack
	if s.manifestParser != nil && len(pCtx.ParsedPatches) > 0 {
		var modifiedPackages []docdiscovery.PackageDependency
		for _, patch := range pCtx.ParsedPatches {
			targetPath := patch.NewPath
			if targetPath == "" {
				targetPath = patch.OldPath
			}
			if s.manifestParser.IsSupportedManifest(targetPath) {
				deps, err := s.manifestParser.AnalyzeManifestDiff(patch)
				if err == nil && len(deps) > 0 {
					modifiedPackages = append(modifiedPackages, deps...)
				}
			}
		}

		if len(modifiedPackages) > 0 {
			pCtx.PipelineMetadata["modified_packages"] = modifiedPackages
			pCtx.AddMetric("modified_dependencies_count", 0, true, nil, len(modifiedPackages))

			if s.docPlanner != nil {
				tasks := s.docPlanner.PlanDocumentationQueries(pCtx.ParsedPatches, modifiedPackages)
				if len(tasks) > 0 {
					pCtx.PipelineMetadata["doc_query_tasks"] = tasks
				}
			}

			if s.docCache != nil {
				docPack := s.docCache.BuildDocumentationPack(modifiedPackages, docdiscovery.DefaultDocTokenBudget)
				if docPack != nil && (len(docPack.ModifiedPackages) > 0 || len(docPack.Snippets) > 0) {
					pCtx.PipelineMetadata["documentation_pack"] = docPack
					pCtx.PipelineMetadata["documentation_prompt"] = docPack.FormatPromptSlice()
					pCtx.AddMetric("documentation_pack_tokens", 0, true, nil, docPack.TotalEstimatedTokens)
				}
			}
		}
	}

	return nil
}

// DeepInitialCommentStage implements Stage 7 initial sticky progress comment posting with template resolution.
type DeepInitialCommentStage struct {
	commentManager domain.ICommentManagerService
	templates      domain.IMessageTemplateProcessor
}

// NewDeepInitialCommentStage constructs Stage 7.
func NewDeepInitialCommentStage(cm domain.ICommentManagerService, tpl domain.IMessageTemplateProcessor) *DeepInitialCommentStage {
	return &DeepInitialCommentStage{
		commentManager: cm,
		templates:      tpl,
	}
}

func (s *DeepInitialCommentStage) Name() string {
	return "DeepInitialCommentStage"
}

func (s *DeepInitialCommentStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview || s.commentManager == nil {
		return nil
	}

	// Check start review message configuration
	startMsgConfig := pCtx.PullRequestMessages
	isSubsequentPush := pCtx.LastExecution != nil && pCtx.LastExecution.LastAnalyzedCommit != ""

	if startMsgConfig != nil && startMsgConfig.StartReviewMessage != nil {
		status := startMsgConfig.StartReviewMessage.Status
		if status == domain.MessageStatusOff || status == domain.MessageStatusInactive {
			return nil
		}
		if status == domain.MessageStatusOnlyWhenOpened && isSubsequentPush {
			return nil
		}
	}

	// Format greeting template
	templateText := domain.DefaultStartReviewTemplate().Content
	if startMsgConfig != nil && startMsgConfig.StartReviewMessage != nil && strings.TrimSpace(startMsgConfig.StartReviewMessage.Content) != "" {
		templateText = startMsgConfig.StartReviewMessage.Content
	}

	vars := domain.TemplateVariables{
		Author:        pCtx.Author,
		PRNumber:      pCtx.PullNumber,
		RepoName:      pCtx.RepoNamespace,
		RulesChecked:  len(pCtx.ActiveRules),
		FindingsCount: len(pCtx.ChangedFiles),
	}

	body := templateText
	if s.templates != nil {
		body = s.templates.Process(templateText, vars)
	}

	// Append ScanDrix progress indicator badge and branding
	var sb strings.Builder
	sb.WriteString(body)
	sb.WriteString("\n\n")
	sb.WriteString("![ScanDrix Status: Analyzing](https://img.shields.io/badge/ScanDrix-Reviewing-blue?style=flat-square)\n\n")
	sb.WriteString(fmt.Sprintf("> 🔍 Analyzing %d changed files across %d commits.\n\n", len(pCtx.ChangedFiles), len(pCtx.PrCommits)))
	sb.WriteString("<!-- scandrix-review-progress -->")

	repo := models.TrackedRepository{
		ID:            pCtx.RepositoryID,
		WorkspaceID:   pCtx.WorkspaceID,
		Provider:      pCtx.Provider,
		NamespacePath: pCtx.RepoNamespace,
	}

	commentID, err := s.commentManager.CreateInitialComment(ctx, pCtx.WorkspaceID.String(), repo, pCtx.PullNumber, sb.String())
	if err == nil {
		pCtx.InitialCommentID = commentID
	}

	return nil
}
