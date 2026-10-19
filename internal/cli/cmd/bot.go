package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scandrix/backend/internal/integrations/slack"
	"github.com/scandrix/backend/internal/integrations/teams"
	"github.com/scandrix/backend/internal/webhooks/controllers"
	"github.com/scandrix/backend/pkg/models"
	"github.com/spf13/cobra"
)

var (
	botPlatformFlag string
	botPortFlag     int
	botWebhookURL   string
)

var botCmd = &cobra.Command{
	Use:   "bot",
	Short: "Interactive Chatbot Ecosystem — Slack Block Kit & Microsoft Teams Bot Daemon",
	Long: `Manage and run the ScanDrix interactive bot daemon for Slack and Microsoft Teams.
Receives real-time PR approval and change request button callbacks and updates PR state.`,
}

var botDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the interactive bot webhook daemon listener",
	RunE: func(cmd *cobra.Command, args []string) error {
		slackSecret := os.Getenv("SLACK_SIGNING_SECRET")
		if slackSecret == "" {
			slackSecret = os.Getenv("SLACK_WEBHOOK_SECRET")
		}
		teamsSecret := os.Getenv("TEAMS_WEBHOOK_SECRET")
		if teamsSecret == "" {
			teamsSecret = os.Getenv("TEAMS_SIGNING_SECRET")
		}

		botCtrl := controllers.NewBotActionsController(nil, slackSecret, teamsSecret)

		mux := http.NewServeMux()
		mux.HandleFunc("/slack/actions", botCtrl.HandleSlackActions)
		mux.HandleFunc("/notifications/slack/actions", botCtrl.HandleSlackActions)
		mux.HandleFunc("/api/v1/notifications/slack/actions", botCtrl.HandleSlackActions)
		mux.HandleFunc("/teams/actions", botCtrl.HandleTeamsActions)
		mux.HandleFunc("/notifications/teams/actions", botCtrl.HandleTeamsActions)
		mux.HandleFunc("/api/v1/notifications/teams/actions", botCtrl.HandleTeamsActions)

		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","daemon":"bot"}`))
		})

		addr := fmt.Sprintf(":%d", botPortFlag)
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		fmt.Printf("🤖 ScanDrix Interactive Bot Daemon listening on http://localhost%s\n", addr)
		fmt.Printf("  • Slack Block Kit Actions: POST http://localhost%s/slack/actions\n", addr)
		fmt.Printf("  • MS Teams Adaptive Cards: POST http://localhost%s/teams/actions\n\n", addr)

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("Daemon server error: %v\n", err)
			}
		}()

		<-stop
		fmt.Println("\nShutting down bot daemon gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	},
}

var botTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a sample interactive code review card to Slack or Teams",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetURL := botWebhookURL
		if targetURL == "" {
			if botPlatformFlag == "slack" {
				targetURL = os.Getenv("SLACK_WEBHOOK_URL")
			} else {
				targetURL = os.Getenv("TEAMS_WEBHOOK_URL")
			}
		}

		if targetURL == "" {
			return fmt.Errorf("no webhook URL provided; specify --webhook-url or set SLACK_WEBHOOK_URL / TEAMS_WEBHOOK_URL")
		}

		sampleFindings := []models.CodeFinding{
			{
				Title:     "Hardcoded API Token detected",
				Severity:  models.SeverityCritical,
				FilePath:  "pkg/auth/credentials.go",
				StartLine: 28,
			},
			{
				Title:     "Missing CSRF protection in form handler",
				Severity:  models.SeverityHigh,
				FilePath:  "server/handlers/form.go",
				StartLine: 65,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if botPlatformFlag == "slack" {
			notifier := slack.NewNotifier(targetURL)
			err := notifier.PostInteractiveReviewSummary(
				ctx,
				"scandrix/sample-app",
				101,
				"Add OAuth authentication and CSRF guards",
				"https://scandrix.internal/pull-requests/repo-uuid/101",
				sampleFindings,
				"ws-sample-1",
			)
			if err != nil {
				return fmt.Errorf("failed delivering Slack card: %w", err)
			}
			fmt.Println("✅ Successfully dispatched interactive Slack Block Kit card to webhook!")
			return nil
		}

		notifier := teams.NewNotifier(targetURL)
		err := notifier.PostInteractiveReviewSummary(
			ctx,
			"scandrix/sample-app",
			101,
			"Add OAuth authentication and CSRF guards",
			"https://scandrix.internal/pull-requests/repo-uuid/101",
			sampleFindings,
			"ws-sample-1",
		)
		if err != nil {
			return fmt.Errorf("failed delivering Teams card: %w", err)
		}
		fmt.Println("✅ Successfully dispatched interactive MS Teams Adaptive Card to webhook!")
		return nil
	},
}

func init() {
	botDaemonCmd.Flags().IntVarP(&botPortFlag, "port", "p", 8099, "Port for the interactive bot daemon")
	botTestCmd.Flags().StringVarP(&botPlatformFlag, "platform", "f", "slack", "Target platform (slack or teams)")
	botTestCmd.Flags().StringVarP(&botWebhookURL, "webhook-url", "u", "", "Destination incoming webhook URL")

	botCmd.AddCommand(botDaemonCmd)
	botCmd.AddCommand(botTestCmd)
}
