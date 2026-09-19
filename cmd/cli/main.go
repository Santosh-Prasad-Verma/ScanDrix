// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package main

import (
	"github.com/scandrix/backend/internal/cli/cmd"
)

// main bootstraps the ScanDrix enterprise command tree.
// All subcommands (auth, review, rules, config, diff, tui, pr, etc.) are
// registered in internal/cli/cmd and executed through Cobra.

func main() {
	cmd.Execute()
}
