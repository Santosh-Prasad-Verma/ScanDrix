// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"fmt"
	"runtime"
	"strings"
)

// RemoteInstallInstructionSet holds primary and fallback CLI installer shell commands.
type RemoteInstallInstructionSet struct {
	Primary  string `json:"primary"`
	Fallback string `json:"fallback,omitempty"`
}

const (
	UnixInstallerURL    = "https://raw.githubusercontent.com/scandrix/scandrix/main/scripts/install.sh"
	WindowsInstallerURL = "https://raw.githubusercontent.com/scandrix/scandrix/main/scripts/install.ps1"
)

// ResolveRemoteInstallInstructions returns shell one-liners to install the ScanDrix CLI
// remotely on the target platform (defaults to runtime.GOOS if platform is empty).
func ResolveRemoteInstallInstructions(platform string) RemoteInstallInstructionSet {
	target := strings.ToLower(strings.TrimSpace(platform))
	if target == "" {
		target = runtime.GOOS
	}

	if target == "windows" || target == "win32" {
		return RemoteInstallInstructionSet{
			Primary:  fmt.Sprintf(`powershell -NoProfile -ExecutionPolicy Bypass -Command "$tmp = Join-Path $env:TEMP 'scandrix-install.ps1'; Invoke-WebRequest %s -OutFile $tmp; & $tmp"`, WindowsInstallerURL),
			Fallback: fmt.Sprintf(`powershell -NoProfile -ExecutionPolicy Bypass -Command "$scriptPath = Join-Path (Get-Location) 'install.ps1'; Invoke-WebRequest %s -OutFile $scriptPath; & $scriptPath"`, WindowsInstallerURL),
		}
	}

	return RemoteInstallInstructionSet{
		Primary:  fmt.Sprintf("curl -fsSL %s | bash", UnixInstallerURL),
		Fallback: fmt.Sprintf("curl -fsSL %s -o /tmp/scandrix-install.sh && bash /tmp/scandrix-install.sh", UnixInstallerURL),
	}
}
