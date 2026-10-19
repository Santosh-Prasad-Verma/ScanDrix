// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarehouseCommandRegistration(t *testing.T) {
	require.NotNil(t, warehouseCmd)
	assert.Equal(t, "warehouse", warehouseCmd.Use)
	assert.Contains(t, warehouseCmd.Aliases, "analytics-sync")
	assert.Contains(t, warehouseCmd.Aliases, "wh")

	subcommands := warehouseCmd.Commands()
	subNames := make(map[string]bool)
	for _, cmd := range subcommands {
		subNames[cmd.Name()] = true
	}

	assert.True(t, subNames["sync"], "sync subcommand missing")
	assert.True(t, subNames["test"], "test subcommand missing")
	assert.True(t, subNames["ddl"], "ddl subcommand missing")
	assert.True(t, subNames["history"], "history subcommand missing")

	// Verify persistent flags
	assert.NotNil(t, warehouseCmd.PersistentFlags().Lookup("workspace"))
	assert.NotNil(t, warehouseCmd.PersistentFlags().Lookup("warehouse"))

	// Verify sync dry-run flag
	assert.NotNil(t, warehouseSyncCmd.Flags().Lookup("dry-run"))
}
