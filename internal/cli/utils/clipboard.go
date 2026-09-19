// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"time"
)

// CopyToClipboard copies a string into the user's system clipboard using available OS utilities.
// Returns true if successfully copied, false otherwise.
func CopyToClipboard(text string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.CommandContext(ctx, "pbcopy")
		cmd.Stdin = bytes.NewReader([]byte(text))
		return cmd.Run() == nil

	case "windows":
		cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Set-Clipboard", "-Value", "$input")
		cmd.Stdin = bytes.NewReader([]byte(text))
		if cmd.Run() == nil {
			return true
		}
		clip := exec.CommandContext(ctx, "clip")
		clip.Stdin = bytes.NewReader([]byte(text))
		return clip.Run() == nil

	default: // Linux / BSD
		// 1. Try wl-copy (Wayland)
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.CommandContext(ctx, "wl-copy")
			cmd.Stdin = bytes.NewReader([]byte(text))
			if cmd.Run() == nil {
				return true
			}
		}

		// 2. Try xclip (X11)
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
			cmd.Stdin = bytes.NewReader([]byte(text))
			if cmd.Run() == nil {
				return true
			}
		}

		// 3. Try xsel (X11 fallback)
		if _, err := exec.LookPath("xsel"); err == nil {
			cmd := exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
			cmd.Stdin = bytes.NewReader([]byte(text))
			if cmd.Run() == nil {
				return true
			}
		}

		return false
	}
}
