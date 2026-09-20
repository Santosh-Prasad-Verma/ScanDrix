package application

import (
	"net/url"
	"strings"
)

// SendReportResult reports the dispatch outcome of an executive or repository report.
type SendReportResult struct {
	OrganizationID string       `json:"organization_id"`
	Skipped        string       `json:"skipped,omitempty"` // "no-activity" | "no-recipients" | "org-not-found"
	Sent           int          `json:"sent"`
	Failed         int          `json:"failed"`
	Failures       []SendFailure `json:"failures,omitempty"`
}

// SendFailure captures an individual delivery failure.
type SendFailure struct {
	Email  string `json:"email"`
	Reason string `json:"reason,omitempty"`
}

// CockpitLinkOptions configures query parameters for cockpit deep links.
type CockpitLinkOptions struct {
	Tab         string `json:"tab,omitempty"`          // e.g. "scandrix-review"
	Start       string `json:"start,omitempty"`        // YYYY-MM-DD
	End         string `json:"end,omitempty"`          // YYYY-MM-DD
	Repository  string `json:"repository,omitempty"`   // org/repo
	RulesHealth string `json:"rules_health,omitempty"` // "noisy" | "ignored" | "healthy" | "stale" | "all"
}

// BuildCockpitLink creates a deep link to the ScanDrix cockpit with preset filters.
func BuildCockpitLink(baseURL string, opts CockpitLinkOptions) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://app.scandrix.dev"
	}

	params := url.Values{}
	if opts.Tab != "" {
		params.Set("tab", opts.Tab)
	}
	if opts.Start != "" {
		params.Set("start", opts.Start)
	}
	if opts.End != "" {
		params.Set("end", opts.End)
	}
	if opts.Repository != "" {
		params.Set("repository", opts.Repository)
	}
	if opts.RulesHealth != "" {
		params.Set("rulesHealth", opts.RulesHealth)
	}

	qs := params.Encode()
	if qs != "" {
		return base + "/cockpit?" + qs
	}
	return base + "/cockpit"
}
