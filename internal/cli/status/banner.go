// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package status

import (
	"fmt"
	"strings"
)

// ANSI color codes
const (
	ColorReset    = "\033[0m"
	ColorBold     = "\033[1m"
	ColorDim      = "\033[2m"
	ColorCyan     = "\033[36m"
	ColorBlue     = "\033[34m"
	ColorPurple   = "\033[35m"
	ColorYellow   = "\033[33m"
	ColorGreen    = "\033[32m"
	ColorRed      = "\033[31m"
	ColorGray     = "\033[90m"
	ColorNeonBlue = "\033[38;5;75m"
	ColorViolet   = "\033[38;5;141m"
	ColorMuted    = "\033[38;5;248m"
)

const AsciiLogo = "\n" +
	"  ███████╗ ██████╗ █████╗ ███╗   ██╗██████╗ ██████╗ ██╗██╗  ██╗\n" +
	"  ██╔\u2550\u2550\u2550\u2550╝██╔\u2550\u2550\u2550\u2550╝██╔\u2550\u2550██╗████╗  ██║██╔\u2550\u2550██╗██╔\u2550\u2550██╗██║╚██╗██╔╝\n" +
	"  ███████╗██║     ███████║██╔██╗ ██║██║  ██║██████╔╝██║ ╚███╔╝ \n" +
	"  ╚\u2550\u2550\u2550\u2550██║██║     ██╔\u2550\u2550██║██║╚██╗██║██║  ██║██╔\u2550\u2550██╗██║ ██╔██╗ \n" +
	"  ███████║╚██████╗██║  ██║██║ ╚████║██████╔╝██║  ██║██║██╔╝ ██╗\n" +
	"  ╚\u2550\u2550\u2550\u2550\u2550\u2550╝ ╚\u2550\u2550\u2550\u2550\u2550╝╚═╝  ╚═╝╚═╝  ╚\u2550\u2550\u2550╝╚\u2550\u2550\u2550\u2550\u2550╝ ╚═╝  ╚═╝╚═╝╚═╝  ╚═╝"

// PrintBanner prints the rich 2-column developer cockpit overview to terminal.
func PrintBanner(workDir string) {
	st, err := GetStatus(workDir)
	if err != nil {
		st = &StatusResult{
			Version:       CLIVersion,
			ServerURL:     "https://api.scandrix.dev",
			AuthMode:      "Not Authenticated",
			TeamKeyStatus: "Not Configured",
			Repository:    "current directory",
			CurrentBranch: "main",
			PreCommitHook: "Not Installed",
			PrePushHook:   "Not Installed",
		}
	}

	fmt.Println(ColorNeonBlue + ColorBold + AsciiLogo + ColorReset)
	fmt.Printf("   %sAutonomous AI Code Review & Security Gate%s  %s(%s)%s\n\n",
		ColorViolet, ColorReset, ColorDim, st.Version, ColorReset)

	const leftColWidth = 36
	const rightColWidth = 46

	sepLine := ColorGray + "  ┌" + strings.Repeat("─", leftColWidth) + "┬" + strings.Repeat("─", rightColWidth) + "┐" + ColorReset
	midLine := ColorGray + "  ├" + strings.Repeat("─", leftColWidth) + "┼" + strings.Repeat("─", rightColWidth) + "┤" + ColorReset
	botLine := ColorGray + "  └" + strings.Repeat("─", leftColWidth) + "┴" + strings.Repeat("─", rightColWidth) + "┘" + ColorReset

	fmt.Println(sepLine)

	// Header row
	leftHdr := fmt.Sprintf("%s%sSystem & Workspace%s", ColorBold, ColorCyan, ColorReset)
	rightHdr := fmt.Sprintf("%s%sCommon Commands%s", ColorBold, ColorViolet, ColorReset)

	printRow(leftHdr, rightHdr, leftColWidth, rightColWidth)
	fmt.Println(midLine)

	// Auth mode display
	authDisplay := "\033[90mNot Authenticated\033[0m"
	if strings.Contains(st.AuthMode, "OAuth") || strings.Contains(st.AuthMode, "JWT") {
		authDisplay = "\033[32mActive (OAuth)\033[0m"
	} else if strings.Contains(st.AuthMode, "Team") {
		authDisplay = "\033[32mActive (Team Key)\033[0m"
	}

	teamKeyDisplay := "\033[33mNot Configured\033[0m"
	if strings.Contains(st.TeamKeyStatus, "Configured") && !strings.Contains(st.TeamKeyStatus, "Not") {
		teamKeyDisplay = "\033[32mConfigured\033[0m"
	}

	// Rows
	rows := []struct {
		left  string
		right string
	}{
		{
			left:  fmt.Sprintf("Auth: %s", authDisplay),
			right: fmt.Sprintf("%sscandrix review%s  %sReview local diff%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Team Key: %s", teamKeyDisplay),
			right: fmt.Sprintf("%sscandrix review --fix%s  %sAuto-apply fixes%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Repo: %s", truncate(st.Repository, 28)),
			right: fmt.Sprintf("%sscandrix tui%s  %sInteractive cockpit%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Branch: %s%s%s", ColorBold, truncate(st.CurrentBranch, 26), ColorReset),
			right: fmt.Sprintf("%sscandrix pr suggestions%s  %sTriage PRs%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Pre-commit: %s", colorHook(st.PreCommitHook)),
			right: fmt.Sprintf("%sscandrix pr business-validation%s  %sTask check%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Pre-push: %s", colorHook(st.PrePushHook)),
			right: fmt.Sprintf("%sscandrix trace <paths>%s  %sDecision memory%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Skills: %s%d bundled%s", ColorGreen, st.BundledSkills, ColorReset),
			right: fmt.Sprintf("%sscandrix skills install%s  %sSync agent skills%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
		{
			left:  fmt.Sprintf("Server: %s", truncate(st.ServerURL, 26)),
			right: fmt.Sprintf("%sscandrix rules sync%s  %sPull org rules%s", ColorCyan, ColorReset, ColorMuted, ColorReset),
		},
	}

	for _, r := range rows {
		printRow(r.left, r.right, leftColWidth, rightColWidth)
	}

	fmt.Println(botLine)
	fmt.Printf("\n  %sQuick Start:%s %sscandrix auth login%s → %sscandrix review --staged%s → %sscandrix tui%s\n\n",
		ColorBold+ColorYellow, ColorReset, ColorNeonBlue, ColorReset, ColorNeonBlue, ColorReset, ColorNeonBlue, ColorReset)
}

func printRow(left, right string, leftWidth, rightWidth int) {
	vLeft := visibleLen(left)
	padLeft := max(0, leftWidth-vLeft-2)
	lCell := fmt.Sprintf("  │ %s%s", left, strings.Repeat(" ", padLeft))

	vRight := visibleLen(right)
	padRight := max(0, rightWidth-vRight-2)
	rCell := fmt.Sprintf(" │ %s%s │", right, strings.Repeat(" ", padRight))

	fmt.Println(lCell + rCell)
}

func visibleLen(s string) int {
	inEscape := false
	length := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			inEscape = true
		} else if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
		} else {
			length++
		}
	}
	return length
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func colorHook(h string) string {
	if strings.Contains(h, "Active") {
		return ColorGreen + h + ColorReset
	}
	return ColorGray + h + ColorReset
}
