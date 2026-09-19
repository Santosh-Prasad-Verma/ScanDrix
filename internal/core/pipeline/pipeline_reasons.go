package pipeline

// PipelineReason encapsulates user-facing messages, descriptions, and recommended actions.
type PipelineReason struct {
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action,omitempty"`
}

// PipelineReasons defines all standardized skip, bypass, and failure explanations.
var PipelineReasons = struct {
	CONFIG struct {
		DISABLED        PipelineReason
		IGNORED_TITLE   PipelineReason
		DRAFT           PipelineReason
		BRANCH_MISMATCH PipelineReason
	}
	FILES struct {
		NO_CHANGES  PipelineReason
		ALL_IGNORED PipelineReason
		TOO_MANY    PipelineReason
	}
	COMMITS struct {
		NO_NEW                PipelineReason
		ONLY_MERGE            PipelineReason
		PROVIDER_RATE_LIMITED PipelineReason
	}
	PREREQUISITES struct {
		CLOSED                  PipelineReason
		LOCKED                  PipelineReason
		MISSING_DATA            PipelineReason
		NO_LICENSE              PipelineReason
		BYOK_MISSING            PipelineReason
		PLAN_LIMIT              PipelineReason
		TRIAL_CREDITS_EXHAUSTED PipelineReason
		LICENSE_UNAVAILABLE     PipelineReason
		USER_NO_LICENSE         PipelineReason
	}
	SUGGESTIONS struct {
		VALIDATION_FAILED PipelineReason
		NO_RESULTS        PipelineReason
	}
	FINE_TUNING struct {
		DISABLED   PipelineReason
		NO_MATCHES PipelineReason
	}
}{
	CONFIG: struct {
		DISABLED        PipelineReason
		IGNORED_TITLE   PipelineReason
		DRAFT           PipelineReason
		BRANCH_MISMATCH PipelineReason
	}{
		DISABLED: PipelineReason{
			Message: "Automated Review is disabled",
			Action:  "Enable 'Automated Code Review' in General Settings",
		},
		IGNORED_TITLE: PipelineReason{
			Message: "Title Ignored",
			Action:  "Remove keywords defined in 'Ignore title keywords' setting",
		},
		DRAFT: PipelineReason{
			Message: "Draft PR Skipped",
			Action:  "Enable 'Running on Draft Pull Requests' in settings or mark as Ready",
		},
		BRANCH_MISMATCH: PipelineReason{
			Message: "Branch Mismatch",
			Action:  "Review only runs on specific target branches",
		},
	},
	FILES: struct {
		NO_CHANGES  PipelineReason
		ALL_IGNORED PipelineReason
		TOO_MANY    PipelineReason
	}{
		NO_CHANGES: PipelineReason{
			Message: "No Files Changed",
		},
		ALL_IGNORED: PipelineReason{
			Message: "All Files Ignored",
			Action:  "Check your 'Ignored files' patterns in settings",
		},
		TOO_MANY: PipelineReason{
			Message: "Too Many Files",
			Action:  "Reduce PR size for better review quality",
		},
	},
	COMMITS: struct {
		NO_NEW                PipelineReason
		ONLY_MERGE            PipelineReason
		PROVIDER_RATE_LIMITED PipelineReason
	}{
		NO_NEW: PipelineReason{
			Message:     "No New Commits",
			Description: "We already reviewed the latest changes",
		},
		ONLY_MERGE: PipelineReason{
			Message:     "Only Merge Commits",
			Description: "Merge commits are skipped to avoid noise",
		},
		PROVIDER_RATE_LIMITED: PipelineReason{
			Message:     "Provider Rate Limited",
			Description: "The Git provider throttled the commit fetch (HTTP 429); review was postponed",
			Action:      "Re-run the review once the provider rate-limit window refills",
		},
	},
	PREREQUISITES: struct {
		CLOSED                  PipelineReason
		LOCKED                  PipelineReason
		MISSING_DATA            PipelineReason
		NO_LICENSE              PipelineReason
		BYOK_MISSING            PipelineReason
		PLAN_LIMIT              PipelineReason
		TRIAL_CREDITS_EXHAUSTED PipelineReason
		LICENSE_UNAVAILABLE     PipelineReason
		USER_NO_LICENSE         PipelineReason
	}{
		CLOSED: PipelineReason{
			Message: "PR is Closed",
		},
		LOCKED: PipelineReason{
			Message: "PR is Locked",
		},
		MISSING_DATA: PipelineReason{
			Message:     "Invalid Context Data",
			Description: "Required PR/Repo metadata missing",
		},
		NO_LICENSE: PipelineReason{
			Message: "No Active Subscription",
			Action:  "Check subscription settings",
		},
		BYOK_MISSING: PipelineReason{
			Message: "BYOK Configuration Required",
			Action:  "Configure API Keys in Settings",
		},
		PLAN_LIMIT: PipelineReason{
			Message: "Plan Limit Exceeded",
			Action:  "Upgrade plan to continue",
		},
		TRIAL_CREDITS_EXHAUSTED: PipelineReason{
			Message: "Trial Reviews Used Up",
			Action:  "Connect your own AI key to keep reviewing (unlimited, any plan)",
		},
		LICENSE_UNAVAILABLE: PipelineReason{
			Message: "Subscription Check Unavailable",
			Action:  "The license service is temporarily unreachable — re-run the review in a few minutes",
		},
		USER_NO_LICENSE: PipelineReason{
			Message: "User Not Licensed",
			Action:  "Assign seat to user",
		},
	},
	SUGGESTIONS: struct {
		VALIDATION_FAILED PipelineReason
		NO_RESULTS        PipelineReason
	}{
		VALIDATION_FAILED: PipelineReason{
			Message: "Suggestion Validation Failed",
			Action:  "Contact support if this persists",
		},
		NO_RESULTS: PipelineReason{
			Message:     "No Validated Suggestions",
			Description: "Validation filtered out all suggestions",
		},
	},
	FINE_TUNING: struct {
		DISABLED   PipelineReason
		NO_MATCHES PipelineReason
	}{
		DISABLED: PipelineReason{
			Message:     "Fine-Tuning Disabled",
			Description: "Context skipped as per configuration",
		},
		NO_MATCHES: PipelineReason{
			Message:     "No Matching Examples",
			Description: "No relevant fine-tuning examples found",
		},
	},
}
