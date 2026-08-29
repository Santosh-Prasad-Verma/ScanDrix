package sync

import (
	"bufio"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// FileItem represents a single file to be analyzed in a sweep.
type FileItem struct {
	Path    string
	Content string
}

// CodebaseSweeper performs parallel security sweeps across full repository trees.
type CodebaseSweeper struct {
	concurrency int
}

// NewCodebaseSweeper creates a repository sweeper with a bounded worker pool.
func NewCodebaseSweeper(concurrency int) *CodebaseSweeper {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &CodebaseSweeper{concurrency: concurrency}
}

// ExecuteSweep scans an array of files against active workspace rules concurrently.
func (s *CodebaseSweeper) ExecuteSweep(ctx context.Context, req SweepRequest, activeRules []rules.RuleSpec, files []FileItem) (*SweepReport, error) {
	startTime := time.Now()

	// Compile regexes once before the sweep
	type compiledRule struct {
		spec  rules.RuleSpec
		regex *regexp.Regexp
	}

	compiled := make([]compiledRule, 0, len(activeRules))
	for _, r := range activeRules {
		if rx, err := regexp.Compile(r.RegexRule); err == nil {
			compiled = append(compiled, compiledRule{spec: r, regex: rx})
		}
	}

	report := &SweepReport{
		SweepID:       req.SweepID,
		WorkspaceID:   req.WorkspaceID,
		RepoNamespace: req.RepoNamespace,
		Branch:        req.Branch,
		FilesScanned:  len(files),
	}

	fileChan := make(chan FileItem, len(files))
	for _, f := range files {
		fileChan <- f
	}
	close(fileChan)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for file := range fileChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Count lines and scan
				scanner := bufio.NewScanner(strings.NewReader(file.Content))
				lineNo := 1
				var fileLines int64
				var localFindings []models.CodeFinding

				for scanner.Scan() {
					lineText := scanner.Text()
					fileLines++

					for _, cr := range compiled {
						if cr.spec.PathPattern != "" {
							if matched, err := filepath.Match(cr.spec.PathPattern, filepath.Base(file.Path)); err != nil || !matched {
								continue
							}
						}

						if cr.regex.MatchString(lineText) {
							localFindings = append(localFindings, models.CodeFinding{
								ID:          uuid.New(),
								WorkspaceID: req.WorkspaceID,
								FilePath:    file.Path,
								StartLine:   lineNo,
								EndLine:     lineNo,
								Severity:    cr.spec.Severity,
								Category:    cr.spec.Category,
								Title:       cr.spec.Name,
								Description: cr.spec.Description,
								Remediation: cr.spec.Remediation,
								CreatedAt:   time.Now().UTC(),
							})
						}
					}
					lineNo++
				}

				mu.Lock()
				report.LinesProcessed += fileLines
				report.Findings = append(report.Findings, localFindings...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	report.Duration = time.Since(startTime)
	report.CompletedAt = time.Now().UTC()
	report.FindingsCount = len(report.Findings)

	for _, f := range report.Findings {
		switch f.Severity {
		case models.SeverityCritical:
			report.CriticalCount++
		case models.SeverityHigh:
			report.HighCount++
		case models.SeverityMedium:
			report.MediumCount++
		case models.SeverityLow:
			report.LowCount++
		}
	}

	return report, nil
}
