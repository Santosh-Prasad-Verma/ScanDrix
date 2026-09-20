// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Terminal styling constants for ScanDrix banner
const (
	BannerColorPrimary   = "\033[38;2;248;183;109m" // #F8B76D
	BannerColorSecondary = "\033[38;2;201;187;242m" // #C9BBF2
	BannerColorMuted     = "\033[38;2;205;205;223m" // #CDCDDF
	BannerColorBorder    = "\033[38;2;68;68;106m"   // #44446A
	BannerColorReset     = "\033[0m"
	BannerColorBold      = "\033[1m"
)

// TruncateLine trims a string to maxWidth with an ellipsis if exceeded.
func TruncateLine(value string, maxWidth int) string {
	if len(value) <= maxWidth {
		return value
	}
	if maxWidth <= 1 {
		return value[:maxWidth]
	}
	if maxWidth <= 3 {
		return value[:maxWidth]
	}
	return value[:maxWidth-3] + "..."
}

// MakeTwoColumnRows combines two slices of strings side-by-side with a vertical divider.
func MakeTwoColumnRows(left, right []string, totalWidth int, leftRatio float64) []string {
	separator := "  |  "
	leftWidth := int(float64(totalWidth-len(separator)) * leftRatio)
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := totalWidth - len(separator) - leftWidth
	if rightWidth < 20 {
		rightWidth = 20
	}

	rowCount := len(left)
	if len(right) > rowCount {
		rowCount = len(right)
	}

	rows := make([]string, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		lText := ""
		if i < len(left) {
			lText = left[i]
		}
		rText := ""
		if i < len(right) {
			rText = right[i]
		}

		lTrunc := TruncateLine(lText, leftWidth)
		rTrunc := TruncateLine(rText, rightWidth)

		lPad := lTrunc + strings.Repeat(" ", leftWidth-len(lTrunc))
		rPad := rTrunc + strings.Repeat(" ", rightWidth-len(rTrunc))

		rows = append(rows, fmt.Sprintf("%s%s%s", lPad, separator, rPad))
	}

	return rows
}

// ScanDrixBannerASCII contains the ScanDrix logo art.
const ScanDrixBannerASCII = `  ___                  ___       _      
 / __| __ __ _ _ _    |   \ _ _ (_)_ __ 
 \__ \/ _/ _` + "`" + ` | ' \   | |) | '_|| \ \ / 
 |___/\__\__,_|_||_|  |___/|_|  |_/_\_\ `

// RenderBanner generates the full ScanDrix interactive cockpit banner.
func RenderBanner(w io.Writer, version string, authMode string, recentActivities []string) {
	if w == nil {
		w = os.Stdout
	}

	totalWidth := 80
	divider := strings.Repeat("─", totalWidth)

	fmt.Fprintln(w, BannerColorSecondary+ScanDrixBannerASCII+BannerColorReset)
	fmt.Fprintf(w, "%sScanDrix AI Code Review Cockpit v%s%s\n", BannerColorPrimary+BannerColorBold, version, BannerColorReset)
	fmt.Fprintf(w, "%sAuth Mode: %s%s%s\n\n", BannerColorMuted, BannerColorSecondary+BannerColorBold, authMode, BannerColorReset)

	quickStart := []string{
		"Quick Start",
		"Use these commands to start:",
		"1) scandrix review - Run AI review on local changes",
		"2) scandrix scan - Security and vulnerability scan",
		"3) scandrix pr <id> - Fetch and review remote PR",
		"4) scandrix rules init - Create starter rules config",
	}

	commonCommands := []string{
		"Common Commands",
		"Frequent workflows:",
		"scandrix review --staged - Review git staged files",
		"scandrix review --fix - Automatically apply fixes",
		"scandrix config show - Inspect active configuration",
		"scandrix mcp - Start MCP stdio server for IDE",
	}

	rows := MakeTwoColumnRows(quickStart, commonCommands, totalWidth, 0.48)
	fmt.Fprintln(w, BannerColorBorder+divider+BannerColorReset)
	for _, row := range rows {
		styled := strings.Replace(row, "  |  ", fmt.Sprintf("  %s|%s  ", BannerColorBorder, BannerColorReset), 1)
		fmt.Fprintln(w, styled)
	}
	fmt.Fprintln(w, BannerColorBorder+divider+BannerColorReset)

	if len(recentActivities) > 0 {
		fmt.Fprintf(w, "\n%sRecent Activity:%s\n", BannerColorSecondary+BannerColorBold, BannerColorReset)
		for _, act := range recentActivities {
			fmt.Fprintf(w, "  %s• %s%s\n", BannerColorMuted, act, BannerColorReset)
		}
	}
}
