// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillsCmd_List(t *testing.T) {
	root := &cobra.Command{Use: "scandrix"}
	root.AddCommand(skillsCmd)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"skills", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("skills list failed: %v", err)
	}

	_ = buf.String()
}

func TestSkillsCmd_SyncDryRun(t *testing.T) {
	root := &cobra.Command{Use: "scandrix"}
	root.AddCommand(skillsCmd)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"skills", "sync", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("skills sync --dry-run failed: %v", err)
	}
}

func TestSkillsCmd_InstallDryRun(t *testing.T) {
	root := &cobra.Command{Use: "scandrix"}
	root.AddCommand(skillsCmd)

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"skills", "install", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("skills install --dry-run failed: %v", err)
	}
}
