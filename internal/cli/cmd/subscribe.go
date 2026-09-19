// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"time"

	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// SUBSCRIBE COMMAND SPECIFICATION (Billing Portal Launch)

var subscribeCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Open ScanDrix subscription and upgrade billing page in default browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := configcli.Load(".")
		billingURL := cfg.BillingURL
		if billingURL == "" {
			billingURL = "https://scandrix.dev/pricing"
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("subscribe", map[string]string{"url": billingURL}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		utils.Info("Opening billing page: %s", billingURL)
		utils.OpenBrowser(billingURL)
		return nil
	},
}
