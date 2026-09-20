// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/rulescli"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

// RULES COMMAND FLAGS

var (
	ruleTitle    string
	ruleBody     string
	ruleRepoID   string
	ruleSeverity string
	ruleScope    string
	rulePathGlob string
	ruleUUID     string
	ruleForce    bool
)

// RULES PARENT COMMAND

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Create, update, and view ScanDrix security & quality rules",
	Long: `Manage custom repository and organization security & quality rules,
including local starter configurations and centralized rules.`,
}

func printRule(rule *api.RuleModel, fallbackRepoID string) {
	if rule == nil {
		return
	}
	repoID := rule.RepoID
	if repoID == "" {
		repoID = fallbackRepoID
	}
	if repoID == "" {
		repoID = "global"
	}
	fmt.Printf("Rule UUID: %s\n", rule.UUID)
	fmt.Printf("Repository ID: %s\n", repoID)
	fmt.Printf("Rule Title: %s\n", rule.Title)
	fmt.Printf("Rule: %s\n", rule.Rule)
	if rule.Severity != "" {
		fmt.Printf("Severity: %s\n", rule.Severity)
	}
	if rule.Scope != "" {
		fmt.Printf("Scope: %s\n", rule.Scope)
	}
	if rule.Path != "" {
		fmt.Printf("Path: %s\n", rule.Path)
	}
}

func printRuleList(rules []api.RuleModel, fallbackRepoID string) {
	if len(rules) == 0 {
		fmt.Println("No rules found.")
		return
	}

	for i := range rules {
		printRule(&rules[i], fallbackRepoID)
		if i < len(rules)-1 {
			fmt.Println()
		}
	}
}

// RULES CREATE (Direct or PR Proposal)

var rulesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new custom rule on the central server or propose via PR",
	RunE: func(cmd *cobra.Command, args []string) error {
		if ruleTitle == "" || ruleBody == "" {
			return fmt.Errorf("both --title <title> and --rule <rule> are required")
		}

		client := api.NewClient("", "", "")
		resp, err := client.CreateRule(cmd.Context(), ruleTitle, ruleBody, ruleRepoID, ruleSeverity, ruleScope, rulePathGlob)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules create", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if resp.PRURL != "" {
			utils.Success("✔ Rule change proposed through centralized pull request.")
			if resp.Message != "" {
				fmt.Println(resp.Message)
			}
			fmt.Printf("PR URL: %s\n", resp.PRURL)
			if resp.PRNumber != 0 {
				fmt.Printf("PR Number: %d\n", resp.PRNumber)
			}
			return nil
		}

		utils.Success("✔ Rule created successfully.")
		if resp.Rule != nil {
			printRule(resp.Rule, ruleRepoID)
		} else {
			printRule(&api.RuleModel{
				Title:    ruleTitle,
				Rule:     ruleBody,
				RepoID:   ruleRepoID,
				Severity: ruleSeverity,
				Scope:    ruleScope,
				Path:     rulePathGlob,
			}, ruleRepoID)
		}
		return nil
	},
}

// RULES UPDATE

var rulesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing custom rule by UUID",
	RunE: func(cmd *cobra.Command, args []string) error {
		if ruleUUID == "" {
			return fmt.Errorf("required flag --uuid <uuid> not specified")
		}

		client := api.NewClient("", "", "")
		resp, err := client.UpdateRule(cmd.Context(), ruleUUID, ruleTitle, ruleBody, ruleRepoID, ruleSeverity, ruleScope, rulePathGlob)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules update", resp, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if resp.PRURL != "" {
			utils.Success("✔ Rule update proposed through centralized pull request.")
			if resp.Message != "" {
				fmt.Println(resp.Message)
			}
			fmt.Printf("PR URL: %s\n", resp.PRURL)
			if resp.PRNumber != 0 {
				fmt.Printf("PR Number: %d\n", resp.PRNumber)
			}
			return nil
		}

		utils.Success("✔ Rule updated successfully.")
		if resp.Rule != nil {
			printRule(resp.Rule, ruleRepoID)
		} else {
			fmt.Printf("Rule UUID: %s\n", ruleUUID)
		}
		return nil
	},
}

// RULES VIEW & LIST

var rulesViewCmd = &cobra.Command{
	Use:     "view",
	Aliases: []string{"list"},
	Short:   "View custom and catalog rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		rulesList, err := client.ViewRules(cmd.Context(), ruleUUID, ruleRepoID)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules view", rulesList, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		printRuleList(rulesList, ruleRepoID)
		return nil
	},
}

// RULES INIT (Starter Configuration)

var rulesInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize starter .drixy/rules.yaml rule configuration in local repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := rulescli.InitWithForce(".", ruleForce)
		if err != nil {
			return err
		}
		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules init", map[string]interface{}{
				"path":    path,
				"created": true,
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}
		utils.Success("✔ Created starter rules file at %s", path)
		return nil
	},
}

// RULES VALIDATE (Local Syntax & AST Tests)

var rulesValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate local .drixy/rules.yaml syntax and test patterns",
	RunE: func(cmd *cobra.Command, args []string) error {
		count, err := rulescli.Validate(".")
		if err != nil {
			return err
		}
		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules validate", map[string]interface{}{
				"valid":       true,
				"rules_count": count,
				"path":        rulescli.ActiveRulesPath("."),
			}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}
		utils.Success("✔ %d rules validated cleanly in %s", count, rulescli.ActiveRulesPath("."))
		return nil
	},
}

// RULES GENERATE (AI Synthesis)

var rulesGenerateCmd = &cobra.Command{
	Use:   "generate <prompt>",
	Short: "Synthesize an AST/regex rule using Drixy AI",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt := strings.Join(args, " ")
		client := api.NewClient("", "", "")
		genRule, err := client.GenerateRule(cmd.Context(), prompt)
		if err != nil {
			return err
		}

		if agentFlag || formatFlag == "json" {
			env := utils.BuildAgentSuccessEnvelope("rules generate", genRule, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Println("\n⚡ Drixy Generated Rule:")
		fmt.Printf("  Title:       %s\n", genRule.Title)
		fmt.Printf("  Severity:    %s\n", genRule.Severity)
		fmt.Printf("  Scope:       %s\n", genRule.Scope)
		fmt.Printf("  Path:        %s\n", genRule.Path)
		fmt.Printf("  Rule Pattern: %s\n", genRule.Rule)
		return nil
	},
}

// COMMAND REGISTRATION & FLAG INITIALIZATION

func init() {
	rulesCreateCmd.Flags().StringVar(&ruleTitle, "title", "", "Rule title")
	rulesCreateCmd.Flags().StringVar(&ruleBody, "rule", "", "Rule regex or AST match pattern")
	rulesCreateCmd.Flags().StringVar(&ruleRepoID, "repo-id", "global", "Repository ID (default: global)")
	rulesCreateCmd.Flags().StringVar(&ruleSeverity, "severity", "medium", "Rule severity: low, medium, high, critical")
	rulesCreateCmd.Flags().StringVar(&ruleScope, "scope", "file", "Rule scope: file or pull request")
	rulesCreateCmd.Flags().StringVar(&rulePathGlob, "path", "**/*", "Glob pattern for target files")

	rulesUpdateCmd.Flags().StringVar(&ruleUUID, "uuid", "", "Rule UUID to update")
	rulesUpdateCmd.Flags().StringVar(&ruleTitle, "title", "", "Updated rule title")
	rulesUpdateCmd.Flags().StringVar(&ruleBody, "rule", "", "Updated rule match pattern")
	rulesUpdateCmd.Flags().StringVar(&ruleRepoID, "repo-id", "", "Updated repository ID")
	rulesUpdateCmd.Flags().StringVar(&ruleSeverity, "severity", "", "Updated rule severity")
	rulesUpdateCmd.Flags().StringVar(&ruleScope, "scope", "", "Updated rule scope")
	rulesUpdateCmd.Flags().StringVar(&rulePathGlob, "path", "", "Updated glob pattern")

	rulesViewCmd.Flags().StringVar(&ruleUUID, "uuid", "", "Filter by specific rule UUID")
	rulesViewCmd.Flags().StringVar(&ruleRepoID, "repo-id", "", "Filter by repository ID")

	rulesInitCmd.Flags().BoolVarP(&ruleForce, "force", "f", false, "Overwrite existing rules file")

	_ = rulesCreateCmd.MarkFlagRequired("title")
	_ = rulesCreateCmd.MarkFlagRequired("rule")
	_ = rulesUpdateCmd.MarkFlagRequired("uuid")

	rulesCmd.AddCommand(rulesCreateCmd)
	rulesCmd.AddCommand(rulesUpdateCmd)
	rulesCmd.AddCommand(rulesViewCmd)
	rulesCmd.AddCommand(rulesInitCmd)
	rulesCmd.AddCommand(rulesValidateCmd)
	rulesCmd.AddCommand(rulesGenerateCmd)
}
