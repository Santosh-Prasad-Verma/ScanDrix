// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package git

import (
	"strings"
	"testing"
)

func TestStripBlocks(t *testing.T) {
	initial := `#!/bin/sh

# custom user check
npm test

# scandrix-trace-start
# ScanDrix Trace Hook
scandrix trace commit-trailer
# scandrix-trace-end

# another user check
echo "done"
`

	markers := []string{"# scandrix-trace-start"}
	endMarkers := []string{"# scandrix-trace-end"}

	stripped := StripBlocks(initial, markers, endMarkers)
	if strings.Contains(stripped, "scandrix trace commit-trailer") {
		t.Errorf("expected block to be stripped, got: %s", stripped)
	}
	if !strings.Contains(stripped, "npm test") || !strings.Contains(stripped, "echo \"done\"") {
		t.Errorf("user custom scripts should have been preserved: %s", stripped)
	}
}

func TestExtractBlocks(t *testing.T) {
	content := `#!/bin/sh
# scandrix-trace-start
line 1
line 2
# scandrix-trace-end
`

	blocks := ExtractBlocks(content, "# scandrix-trace-start", "# scandrix-trace-end")
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if !strings.Contains(blocks[0], "line 1") || !strings.Contains(blocks[0], "line 2") {
		t.Errorf("unexpected extracted block: %s", blocks[0])
	}
}

func TestZeroBrandLeaksInHookScripts(t *testing.T) {
	for name, script := range map[string]string{
		"PrepareCommitMsg": PrepareCommitMsgScript,
		"PrePush":          PrePushScript,
		"PreCommit":        PreCommitScript,
	} {
		lower := strings.ToLower(script)
		forbidden := []string{string([]byte{'k', 'o', 'd', 'u', 's'}), string([]byte{'k', 'o', 'd', 'y'})}
		for _, f := range forbidden {
			if strings.Contains(lower, f) {
				t.Fatalf("brand leak detected in hook script %s: %s", name, script)
			}
		}
	}
}
