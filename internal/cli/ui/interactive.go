// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/cli/engine"
	"github.com/scandrix/backend/internal/cli/services/review"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/pkg/models"
)

// InteractiveUI coordinates terminal navigation and interactive remediation.
type InteractiveUI struct {
	reader io.Reader
	writer io.Writer
}

// NewInteractiveUI constructs an InteractiveUI instance.
func NewInteractiveUI(r io.Reader, w io.Writer) *InteractiveUI {
	if r == nil {
		r = os.Stdin
	}
	if w == nil {
		w = os.Stdout
	}
	return &InteractiveUI{reader: r, writer: w}
}

var DefaultInteractiveUI = NewInteractiveUI(os.Stdin, os.Stdout)

// Run starts the interactive finding review loop.
func (ui *InteractiveUI) Run(result *review.ReviewResult) error {
	if result == nil {
		return nil
	}

	fmt.Fprintln(ui.writer)
	fmt.Fprintln(ui.writer, colorCyan+colorBold+"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"+colorReset)
	fmt.Fprintln(ui.writer, colorCyan+colorBold+"  ScanDrix Code Review — Interactive Navigator"+colorReset)
	fmt.Fprintln(ui.writer, colorCyan+colorBold+"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"+colorReset)
	fmt.Fprintln(ui.writer)

	if result.Summary != "" {
		fmt.Fprintf(ui.writer, "%sSummary:%s %s\n", colorDim, colorReset, result.Summary)
	}
	fmt.Fprintf(ui.writer, "%sFiles analyzed:%s %d\n", colorDim, colorReset, result.FilesAnalyzed)
	fmt.Fprintf(ui.writer, "%sFindings:%s %d\n", colorDim, colorReset, len(result.Findings))
	if result.DurationMs > 0 {
		fmt.Fprintf(ui.writer, "%sDuration:%s %dms\n", colorDim, colorReset, result.DurationMs)
	}
	fmt.Fprintln(ui.writer)

	if len(result.Findings) == 0 {
		fmt.Fprintln(ui.writer, colorGreen+colorBold+"✓ No issues found! Your code looks clean."+colorReset)
		fmt.Fprintln(ui.writer)
		return nil
	}

	fixable := GetFixableFindings(result.Findings)
	if len(fixable) > 0 {
		fmt.Fprintf(ui.writer, "%s%d finding%s can be auto-fixed%s\n\n", colorGreen, len(fixable), pluralSuffix(len(fixable)), colorReset)
	}

	grouped := GroupFindingsByFile(result.Findings)
	scanner := bufio.NewScanner(ui.reader)

	for {
		files := make([]string, 0, len(grouped))
		for f := range grouped {
			files = append(files, f)
		}
		sort.Strings(files)

		fmt.Fprintln(ui.writer, colorBold+"📁 Select a file to inspect:"+colorReset)
		for i, f := range files {
			fmt.Fprintf(ui.writer, "  %s%d)%s %s\n", colorCyan, i+1, colorReset, FormatFileChoice(f, grouped[f]))
		}

		totalRemaining := 0
		for _, fl := range grouped {
			totalRemaining += len(fl)
		}

		if len(files) > 1 {
			fmt.Fprintf(ui.writer, "  %sc)%s %sCopy ALL issues to AI agent%s (%d issues across %d files)\n",
				colorYellow, colorReset, colorBold, colorReset, totalRemaining, len(files))
		}
		fmt.Fprintf(ui.writer, "  %sq)%s Exit review\n\n", colorDim, colorReset)

		fmt.Fprint(ui.writer, "Choose option: ")
		if !scanner.Scan() {
			break
		}
		choice := strings.TrimSpace(scanner.Text())

		if choice == "q" || choice == "quit" || choice == "exit" {
			break
		}

		if choice == "c" && len(files) > 1 {
			prompt := GenerateFixPromptAll(grouped)
			if utils.CopyToClipboard(prompt) {
				fmt.Fprintln(ui.writer, colorGreen+"\n✓ All issues copied to clipboard for AI agent!"+colorReset)
				fmt.Fprintln(ui.writer, colorDim+"Paste into Claude Code, Cursor, Codex, or your IDE agent.\n"+colorReset)
			} else {
				fmt.Fprintln(ui.writer, colorYellow+"\n⚠ Clipboard unavailable. Prompt preview:"+colorReset)
				fmt.Fprintln(ui.writer, prompt)
			}
			continue
		}

		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(files) {
			fmt.Fprintln(ui.writer, colorRed+"Invalid choice. Please pick an option from the menu."+colorReset)
			continue
		}

		selectedFile := files[idx-1]
		ui.reviewFile(scanner, selectedFile, grouped[selectedFile])
	}

	return nil
}

func (ui *InteractiveUI) reviewFile(scanner *bufio.Scanner, file string, findings []models.CodeFinding) {
	fmt.Fprintf(ui.writer, "\n%s━━━ Inspecting: %s (%d findings) ━━━%s\n", colorBold, file, len(findings), colorReset)
	fmt.Fprintln(ui.writer, "  1) Review findings one-by-one")
	fmt.Fprintln(ui.writer, "  2) Copy fix prompt for AI agent")
	fmt.Fprintln(ui.writer, "  3) Back to file list")
	fmt.Fprintln(ui.writer)

	fmt.Fprint(ui.writer, "Action [1/2/3]: ")
	if !scanner.Scan() {
		return
	}
	act := strings.TrimSpace(scanner.Text())

	switch act {
	case "2":
		prompt := GenerateFixPrompt(file, findings)
		if utils.CopyToClipboard(prompt) {
			fmt.Fprintf(ui.writer, "%s\n✓ Fix prompt for %s copied to clipboard!%s\n\n", colorGreen, file, colorReset)
		} else {
			fmt.Fprintln(ui.writer, colorYellow+"\nPrompt preview:"+colorReset)
			fmt.Fprintln(ui.writer, prompt)
		}
		return
	case "3", "b", "back":
		return
	}

	// Step-by-step finding review
	for i, f := range findings {
		fmt.Fprintf(ui.writer, "\n%s[%d/%d] Finding in %s%s\n", colorBold, i+1, len(findings), file, colorReset)
		RenderFindingDetails(f)

		hasFix := strings.TrimSpace(f.SuggestedDiff) != ""
		if hasFix {
			fmt.Fprintln(ui.writer, "  p) Preview suggested patch")
			fmt.Fprintln(ui.writer, "  a) Apply automated fix")
		}
		fmt.Fprintln(ui.writer, "  s) Skip to next finding")
		fmt.Fprintln(ui.writer, "  b) Back to file list")
		fmt.Fprintln(ui.writer)

		for {
			fmt.Fprint(ui.writer, "Choice: ")
			if !scanner.Scan() {
				return
			}
			fChoice := strings.TrimSpace(scanner.Text())

			if fChoice == "p" && hasFix {
				RenderDiffPreview(f.SuggestedDiff)
				continue
			}

			if fChoice == "a" && hasFix {
				msg, err := engine.ApplyFindingFix(".", f)
				if err != nil {
					fmt.Fprintf(ui.writer, "%s✗ Failed to apply fix: %v%s\n", colorRed, err, colorReset)
				} else {
					fmt.Fprintf(ui.writer, "%s✓ %s%s\n", colorGreen, msg, colorReset)
				}
				break
			}

			if fChoice == "s" || fChoice == "" {
				break
			}

			if fChoice == "b" {
				return
			}

			fmt.Fprintln(ui.writer, colorRed+"Unrecognized option"+colorReset)
		}
	}
}

// RunQuickFix automatically applies all findings that provide machine-patchable diffs.
func (ui *InteractiveUI) RunQuickFix(result *review.ReviewResult) error {
	if result == nil {
		return nil
	}

	fixable := GetFixableFindings(result.Findings)
	if len(fixable) == 0 {
		fmt.Fprintln(ui.writer, colorYellow+"No auto-fixable issues found. Try `scandrix review --interactive` to inspect issues or run `scandrix review` to see the full report."+colorReset)
		return nil
	}

	fmt.Fprintf(ui.writer, "\n%sFound %d fixable finding%s%s\n\n", colorBold, len(fixable), pluralSuffix(len(fixable)), colorReset)
	fmt.Fprintf(ui.writer, "Apply all %d fixes to disk? [y/N]: ", len(fixable))

	scanner := bufio.NewScanner(ui.reader)
	if !scanner.Scan() {
		return nil
	}
	resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if resp != "y" && resp != "yes" {
		fmt.Fprintln(ui.writer, colorYellow+"Fixes aborted."+colorReset)
		return nil
	}

	fmt.Fprintln(ui.writer, "\nApplying fixes...")
	batchRes, err := engine.ApplyBatchFixes(".", fixable)
	if err != nil {
		fmt.Fprintf(ui.writer, "%s✗ Batch fix error: %v%s\n", colorRed, err, colorReset)
		return err
	}

	fmt.Fprintf(ui.writer, "%s✓ Applied %d fix%s%s\n", colorGreen, batchRes.Applied, pluralSuffix(batchRes.Applied), colorReset)
	if batchRes.Failed > 0 {
		fmt.Fprintf(ui.writer, "%s✗ Failed to apply %d fix%s%s\n", colorRed, batchRes.Failed, pluralSuffix(batchRes.Failed), colorReset)
	}
	fmt.Fprintln(ui.writer)

	return nil
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
