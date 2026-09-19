// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CROSS-PLATFORM BROWSER LAUNCHER WITH WINDOW RAISING

// OpenBrowser attempts to open the specified URL in the user's default browser.
// On Linux/X11/Wayland, it also attempts to bring the browser window to focus.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// macOS: "open" resolves user default browser
		cmd = exec.Command("open", url)
	case "windows":
		// Windows: "start" resolves user default browser
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		// Linux/FreeBSD standard default browser handlers
		if path, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command(path, url)
		} else if path, err := exec.LookPath("gio"); err == nil {
			cmd = exec.Command(path, "open", url)
		}

		// Raise default browser window if wmctrl is available
		if wmctrlPath, err := exec.LookPath("wmctrl"); err == nil {
			go func() {
				time.Sleep(250 * time.Millisecond)

				var defaultBrowser string
				if out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output(); err == nil {
					defaultBrowser = strings.TrimSpace(string(out))
				} else if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/http").Output(); err == nil {
					defaultBrowser = strings.TrimSpace(string(out))
				}

				if defaultBrowser != "" {
					cleanName := strings.TrimSuffix(defaultBrowser, ".desktop")
					cleanName = strings.TrimPrefix(cleanName, "org.mozilla.")
					_ = exec.Command(wmctrlPath, "-x", "-a", cleanName).Run()
					_ = exec.Command(wmctrlPath, "-a", cleanName).Run()
				}

				knownBrowsers := []string{
					"Firefox", "firefox", "Mozilla Firefox",
					"Brave", "brave-browser", "Brave-browser",
					"Chrome", "Google-chrome", "google-chrome", "google-chrome-stable",
					"Chromium", "chromium", "chromium-browser",
					"Opera", "opera", "opera-browser",
					"Microsoft Edge", "msedge", "microsoft-edge", "Edge",
					"Vivaldi", "vivaldi", "vivaldi-stable",
					"Zen", "zen", "zen-browser",
					"Arc", "arc",
					"LibreWolf", "librewolf",
				}

				for _, b := range knownBrowsers {
					_ = exec.Command(wmctrlPath, "-x", "-a", b).Run()
					_ = exec.Command(wmctrlPath, "-a", b).Run()
				}
			}()
		}
	}

	if cmd != nil {
		_ = cmd.Start()
	}
}
