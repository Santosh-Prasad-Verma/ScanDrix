package review

import "strings"

// ReviewError represents a classified CLI review failure with metadata.
type ReviewError struct {
	Code       string
	Message    string
	StatusCode int
}

// BuildReviewErrorHints generates actionable suggestions for common review errors.
func BuildReviewErrorHints(err ReviewError) []string {
	switch err.Code {
	case "AUTH_REQUIRED":
		return []string{
			"Run `scandrix auth login` to use your account or `scandrix auth team-key --key <your-key>` to use a team key.",
		}
	case "API_REQUEST_FAILED":
		if err.StatusCode == 403 {
			return []string{
				"The API denied the request (403). On large reviews this is usually a request-size limit enforced in front of the API — narrow the scope or use `--branch`, `--commit`, or `--fast` (which avoid inlining file contents). Run with `-v` for more detail.",
			}
		}
		if err.StatusCode == 413 {
			return []string{
				"The request exceeded the API payload size limit. Narrow the review scope or use `--branch`, `--commit`, or `--fast`.",
			}
		}
		if strings.Contains(err.Message, "Could not reach the ScanDrix API") {
			return []string{
				"Check `SCANDRIX_API_URL` and make sure the ScanDrix API is running if you are testing locally.",
			}
		}
		return nil
	case "REVIEW_TOO_LARGE":
		return []string{
			"The review is larger than the API accepts. Narrow the scope (e.g. pass specific files) or use `--branch`, `--commit`, or `--fast`, which avoid inlining file contents.",
		}
	case "NOT_IN_GIT_REPO":
		return []string{
			"Run `scandrix review` inside a Git repository, or pass explicit file paths to review.",
		}
	case "INVALID_INPUT":
		return []string{
			"Run `scandrix review --help` to see supported options, examples, and valid flag combinations.",
		}
	default:
		return nil
	}
}
