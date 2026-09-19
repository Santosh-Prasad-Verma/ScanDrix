// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package main

import (
	"github.com/scandrix/backend/internal/cli/cmd"
)

// main is the direct entrypoint for `go install github.com/scandrix/backend/cmd/scandrix`
// and local package invocations.

func main() {
	cmd.Execute()
}
