// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/cli/configcli"
	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/services/auth"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// COMMAND FLAGS & CONFIGURATION

var (
	loginEmail       string
	loginPassword    string
	loginInteractive bool
	loginNoBrowser   bool
	teamKeyFlag      string
)

// AUTH PARENT COMMAND

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate terminal, manage team API keys, and view session status",
	Long:  "Commands to log in via browser device flow, sign in with credentials, configure workspace team keys, and manage CI tokens.",
}

// AUTH LOGIN (Browser Device Flow + Interactive Fallback)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate terminal via interactive prompt or browser device flow",
	Long:  "Logs in to ScanDrix. Defaults to RFC 8628 browser authorization. Supports username/password with --interactive.",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := auth.DefaultService()

		// 1. If explicit credentials provided or interactive flag specified, use password login
		if loginInteractive || (loginEmail != "" && loginPassword != "") {
			return runPasswordLogin(cmd.Context(), srv)
		}

		// 2. If --no-browser, directly use RFC 8628 device flow
		if loginNoBrowser {
			return runDeviceFlowLogin(cmd.Context(), srv)
		}

		// 3. Primary: RFC 8252 Loopback Browser Login
		utils.Info("Opening browser for ScanDrix authorization...")
		userProfile, err := srv.LoginViaBrowser(cmd.Context(), func(url string) {
			fmt.Printf("🌐 Browser opened automatically.\n")
			fmt.Printf("   \033[90m(If browser does not open, visit: %s)\033[0m\n\n", url)
			utils.OpenBrowser(url)
		})
		if err == nil && userProfile != nil {
			utils.Success("✨ Successfully authenticated! Logged in as: %s", userProfile.Email)
			return nil
		}

		utils.Warn("Loopback browser login failed (%v). Falling back to device code authorization.", err)
		return runDeviceFlowLogin(cmd.Context(), srv)
	},
}

func runPasswordLogin(ctx context.Context, srv *auth.Service) error {
	email := loginEmail
	password := loginPassword

	if email == "" {
		fmt.Print("Enter email: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		email = strings.TrimSpace(input)
	}

	if email == "" {
		return fmt.Errorf("email is required")
	}

	if password == "" {
		fmt.Print("Enter password: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed reading password: %w", err)
		}
		password = strings.TrimSpace(string(bytePassword))
	}

	utils.Info("Authenticating with ScanDrix...")
	user, err := srv.Login(ctx, email, password)
	if err != nil {
		return err
	}

	utils.Success("✔ Successfully logged in as %s", user.Email)
	return nil
}

func runDeviceFlowLogin(ctx context.Context, srv *auth.Service) error {
	utils.Info("Initiating ScanDrix Device Authorization flow...")

	initResult, err := srv.StartDeviceFlow(ctx)
	if err != nil {
		utils.Warn("Device authorization failed to initiate (%v). Falling back to interactive login.", err)
		return runPasswordLogin(ctx, srv)
	}

	if initResult.UserCode == "" {
		return fmt.Errorf("server did not return a valid device authorization code")
	}

	fmt.Printf("\n🔑 Confirmation Code: \033[1;33m%s\033[0m\n", initResult.UserCode)
	if !loginNoBrowser {
		fmt.Printf("🌐 Browser opened automatically.\n")
		fmt.Printf("   \033[90m(If browser does not open, visit: %s)\033[0m\n\n", initResult.VerificationURIComplete)
		utils.OpenBrowser(initResult.VerificationURIComplete)
	} else {
		fmt.Printf("🌐 Please visit: %s\n\n", initResult.VerificationURIComplete)
	}

	// Poll until completed, expired, or cancelled
	interval := initResult.Interval
	if interval <= 0 {
		interval = 2
	}
	expiresIn := initResult.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}

	fmt.Print("Waiting for authorization")
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)
		fmt.Print(".")

		pollResult, err := srv.PollDeviceFlow(ctx, initResult.DeviceCode)
		if err != nil {
			continue
		}

		if pollResult.Status == cliauth.StatusCompleted {
			email := pollResult.UserEmail
			if email == "" && pollResult.UserEmailCamel != "" {
				email = pollResult.UserEmailCamel
			}
			accessToken := pollResult.AccessToken
			if accessToken == "" && pollResult.AccessTokenCamel != "" {
				accessToken = pollResult.AccessTokenCamel
			}
			refreshToken := pollResult.RefreshToken
			if refreshToken == "" && pollResult.RefreshTokenCamel != "" {
				refreshToken = pollResult.RefreshTokenCamel
			}

			userProfile, err := srv.SaveSessionTokens(accessToken, refreshToken, email, int64(expiresIn))
			if err != nil {
				return fmt.Errorf("failed storing tokens: %w", err)
			}

			fmt.Println()
			utils.Success("✨ Successfully authenticated! Logged in as: %s", userProfile.Email)
			return nil
		}

		if pollResult.Status == cliauth.StatusDenied {
			fmt.Println()
			return fmt.Errorf("authorization was denied by user")
		}

		if pollResult.Status == cliauth.StatusExpired {
			fmt.Println()
			return fmt.Errorf("device login session expired")
		}
	}

	fmt.Println()
	return fmt.Errorf("device authorization timed out after %d seconds", expiresIn)
}

// AUTH LOGOUT

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials and sign out of terminal session",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := auth.DefaultService()
		if err := srv.Logout(cmd.Context()); err != nil {
			return err
		}
		utils.Success("✔ Logged out successfully. Stored credentials removed.")
		return nil
	},
}

// AUTH STATUS & WHOAMI

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status, active identity, and token expiry",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := auth.DefaultService()
		creds, _ := srv.GetCredentials()
		cfg := configcli.Load(".")

		client := api.NewClient("", "", "")
		if creds != nil && creds.AccessToken != "" {
			client.SetAuthToken(creds.AccessToken)
		} else if cfg.APIKey != "" {
			client.SetTeamKey(cfg.APIKey)
		}

		var remoteProfile *api.WhoamiResponse
		if srv.IsAuthenticated() {
			profile, err := client.Whoami(cmd.Context())
			if err == nil {
				remoteProfile = profile
			}
		}

		if agentFlag || formatFlag == "json" {
			var trialStatus *utils.TrialStatus
			if !srv.IsAuthenticated() {
				trialStatus, _ = client.CheckTrialStatus(cmd.Context())
			}
			data := map[string]any{
				"authenticated": srv.IsAuthenticated(),
				"user":          creds,
				"profile":       remoteProfile,
				"team_key":      cfg.APIKey != "",
				"trial_status":  trialStatus,
			}
			env := utils.BuildAgentSuccessEnvelope("auth status", data, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		if srv.IsAuthenticated() {
			hasTeamKey := cfg.APIKey != ""
			hasUserEmail := (creds != nil && creds.User != nil && creds.User.Email != "") || (remoteProfile != nil && remoteProfile.Email != "")

			if !hasUserEmail && hasTeamKey {
				fmt.Printf("\nAuthentication Status\n\n")
				fmt.Printf("\033[90mMode:\033[0m         \033[32mTeam Key\033[0m\n")
				org := cfg.OrganizationName
				if org == "" && remoteProfile != nil && remoteProfile.Workspace != "" {
					org = remoteProfile.Workspace
				}
				if org == "" {
					org = "(unknown)"
				}
				team := cfg.TeamName
				if team == "" {
					team = "(unknown)"
				}
				fmt.Printf("\033[90mOrganization:\033[0m %s\n", org)
				fmt.Printf("\033[90mTeam:\033[0m         %s\n", team)
				fmt.Printf("\033[90mToken:\033[0m        \033[32mConfigured\033[0m\n")
				return nil
			}

			if creds == nil {
				fmt.Println("\n\033[33mNo credentials found.\033[0m")
				return nil
			}

			fmt.Printf("\nAuthentication Status\n\n")
			fmt.Printf("\033[90mMode:\033[0m  \033[32mLogged In\033[0m\n")
			email := "(unknown)"
			if remoteProfile != nil && remoteProfile.Email != "" {
				email = remoteProfile.Email
			} else if creds.User != nil && creds.User.Email != "" {
				email = creds.User.Email
			}
			fmt.Printf("\033[90mEmail:\033[0m %s\n", email)

			if creds.ExpiresAt > 0 {
				expiresAt := time.UnixMilli(creds.ExpiresAt)
				timeUntilExpiry := time.Until(expiresAt)
				hoursUntilExpiry := int(timeUntilExpiry.Hours())

				if timeUntilExpiry > 0 {
					if hoursUntilExpiry < 1 {
						fmt.Printf("\033[90mToken:\033[0m  \033[33mExpires in < 1 hour\033[0m\n")
					} else if hoursUntilExpiry < 24 {
						fmt.Printf("\033[90mToken:\033[0m  \033[33mExpires in %d hours\033[0m\n", hoursUntilExpiry)
					} else {
						fmt.Printf("\033[90mToken:\033[0m  \033[32mValid\033[0m\n")
					}
				} else {
					fmt.Printf("\033[90mToken:\033[0m  \033[31mExpired\033[0m\n")
					fmt.Println("\n\033[33mYour session has expired. Run `scandrix auth login` to refresh.\033[0m")
					return nil
				}
			}

			if creds.User != nil && len(creds.User.Organizations) > 0 {
				fmt.Println("\033[90mOrganizations:\033[0m")
				for _, org := range creds.User.Organizations {
					fmt.Printf("  \033[90m•\033[0m %s\n", org)
				}
			} else if remoteProfile != nil && remoteProfile.Workspace != "" {
				fmt.Println("\033[90mOrganizations:\033[0m")
				fmt.Printf("  \033[90m•\033[0m %s\n", remoteProfile.Workspace)
			}
		} else {
			trialStatus, _ := client.CheckTrialStatus(cmd.Context())
			if trialStatus == nil {
				trialStatus = &utils.TrialStatus{ReviewsLimit: 5, FilesLimit: 10}
			}

			fmt.Printf("\nAuthentication Status\n\n")
			fmt.Printf("\033[90mMode:\033[0m           \033[33mTrial\033[0m\n")
			fmt.Printf("\033[90mReviews today:\033[0m %d/%d\n", trialStatus.ReviewsUsed, trialStatus.ReviewsLimit)
			filesLimit := trialStatus.FilesLimit
			if filesLimit <= 0 {
				filesLimit = 10
			}
			fmt.Printf("\033[90mFiles limit:\033[0m   %d per review\n", filesLimit)
			if trialStatus.ResetsAt != "" {
				fmt.Printf("\033[90mResets at:\033[0m     %s\n", trialStatus.ResetsAt)
			}

			if trialStatus.IsLimited {
				fmt.Println("\n\033[33m⚡ Daily limit reached!\033[0m")
			}

			fmt.Println("\033[90m\nSign up to remove limits:\033[0m \033[36mscandrix auth login\033[0m")
		}

		return nil
	},
}

// AUTH TOKEN (CI/CD Pipeline Export)

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Generate or display authentication token for CI/CD pipelines",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := auth.DefaultService()
		token, err := srv.GetValidToken(cmd.Context())
		if err != nil {
			return err
		}

		if agentFlag {
			env := utils.BuildAgentSuccessEnvelope("auth token", map[string]string{"token": token}, time.Now())
			return utils.EmitAgentEnvelope(env, outputFlag)
		}

		fmt.Println(token)
		return nil
	},
}

// TEAM API KEY MANAGEMENT

var authTeamKeyCmd = &cobra.Command{
	Use:   "team-key",
	Short: "Configure workspace team API key for organization rules and repo tracking",
	RunE: func(cmd *cobra.Command, args []string) error {
		if teamKeyFlag == "" {
			return fmt.Errorf("required flag --key <key> not specified")
		}

		srv := auth.DefaultService()
		org, err := srv.SetTeamKey(cmd.Context(), teamKeyFlag)
		if err != nil {
			return err
		}

		utils.Success("✔ Team API key saved successfully for organization: %s", org)
		return nil
	},
}

var authTeamStatusCmd = &cobra.Command{
	Use:   "team-status",
	Short: "Check validation status of configured team API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := configcli.Load(".")
		if cfg.APIKey == "" {
			fmt.Println("No team API key configured in ~/.scandrix/config.json.")
			return nil
		}

		client := api.NewClient("", "", cfg.APIKey)
		valid, org, err := client.VerifyTeamKey(cmd.Context(), cfg.APIKey)
		if err != nil || !valid {
			fmt.Printf("Team API Key: \033[31mInvalid or revoked\033[0m\n")
			return nil
		}

		fmt.Printf("Team API Key: \033[32mActive & Valid\033[0m (Organization: %s)\n", org)
		return nil
	},
}

// CONVENIENCE ALIASES (login, logout, whoami)

var loginAliasCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to ScanDrix (alias for 'auth login')",
	RunE:  authLoginCmd.RunE,
}

var logoutAliasCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of ScanDrix (alias for 'auth logout')",
	RunE:  authLogoutCmd.RunE,
}

var whoamiAliasCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show authenticated user details (alias for 'auth status')",
	RunE:  authStatusCmd.RunE,
}

func init() {
	authLoginCmd.Flags().StringVarP(&loginEmail, "email", "e", "", "User account email")
	authLoginCmd.Flags().StringVarP(&loginPassword, "password", "p", "", "User account password")
	authLoginCmd.Flags().BoolVarP(&loginInteractive, "interactive", "i", false, "Use interactive email/password prompts instead of browser flow")
	authLoginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Print verification URL without launching browser")

	authTeamKeyCmd.Flags().StringVar(&teamKeyFlag, "key", "", "Team API key from ScanDrix dashboard (scandrix_*)")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authTokenCmd)
	authCmd.AddCommand(authTeamKeyCmd)
	authCmd.AddCommand(authTeamStatusCmd)
}
