// Package translations provides multi-language localization (30 languages) and formatted review summaries for ScanDrix.
package translations

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed dictionaries/*.json
var dictionaryFS embed.FS

// Category represents available translation categories in the dictionary schema.
type Category string

const (
	CategoryReviewComment                  Category = "reviewComment"
	CategoryPullRequestFinishSummaryMarkdown Category = "pullRequestFinishSummaryMarkdown"
	CategoryPullRequestSummaryMarkdown       Category = "pullRequestSummaryMarkdown"
	CategoryConfigReviewMarkdown           Category = "configReviewMarkdown"
	CategoryDiscordFormatter               Category = "discordFormatter"
	CategoryReviewCadenceInfo              Category = "reviewCadenceInfo"
)

// ReviewComment holds copy for PR review comments.
type ReviewComment struct {
	TalkToDrixy string `json:"talkToDrixy"`
	Feedback    string `json:"feedback"`
}

// PullRequestFinishSummary holds markdown copy for completed review comments.
type PullRequestFinishSummary struct {
	WithComments        string `json:"withComments"`
	WithoutComments     string `json:"withoutComments"`
	WithErrors          string `json:"withErrors,omitempty"`
	PartialErrorsNotice string `json:"partialErrorsNotice,omitempty"`
}

// PullRequestSummary holds markdown copy for the initial PR summary comment.
type PullRequestSummary struct {
	Title             string   `json:"title"`
	CodeReviewStarted string   `json:"codeReviewStarted"`
	Description       string   `json:"description"`
	ChangedFiles      string   `json:"changedFiles"`
	FilesTable        []string `json:"filesTable"`
	Summary           string   `json:"summary"`
	TotalFiles        string   `json:"totalFiles"`
	TotalAdditions    string   `json:"totalAdditions"`
	TotalDeletions    string   `json:"totalDeletions"`
	TotalChanges      string   `json:"totalChanges"`
}

// ConfigReview holds markdown copy for configuration and usage guide.
type ConfigReview struct {
	Title                 string `json:"title"`
	InteractingTitle      string `json:"interactingTitle"`
	RequestReview         string `json:"requestReview"`
	RequestReviewDesc     string `json:"requestReviewDesc"`
	ValidateBusinessLogic string `json:"validateBusinessLogic"`
	ProvideFeedback       string `json:"provideFeedback"`
	ConfigurationTitle    string `json:"configurationTitle"`
	ReviewOptionsTitle    string `json:"reviewOptionsTitle"`
	Enabled               string `json:"enabled"`
	Disabled              string `json:"disabled"`
}

// TranslationDictionary represents the root structure of a locale dictionary.
type TranslationDictionary struct {
	ReviewComment                  ReviewComment            `json:"reviewComment"`
	PullRequestFinishSummaryMarkdown PullRequestFinishSummary `json:"pullRequestFinishSummaryMarkdown"`
	PullRequestSummaryMarkdown       PullRequestSummary       `json:"pullRequestSummaryMarkdown"`
	ConfigReviewMarkdown           ConfigReview             `json:"configReviewMarkdown"`
}

var (
	cacheMu     sync.RWMutex
	localeCache = make(map[string]*TranslationDictionary)
)

// NormalizeLanguageCode ensures a standard locale format like "en-US", "pt-BR", "es-ES".
func NormalizeLanguageCode(lang string) string {
	clean := strings.TrimSpace(lang)
	if clean == "" {
		return "en-US"
	}
	parts := strings.Split(clean, "-")
	if len(parts) == 1 {
		switch strings.ToLower(parts[0]) {
		case "en":
			return "en-US"
		case "es":
			return "es-ES"
		case "fr":
			return "fr-FR"
		case "de":
			return "de-DE"
		case "ja":
			return "ja-JP"
		case "zh":
			return "zh-CN"
		case "pt":
			return "pt-BR"
		case "ru":
			return "ru-RU"
		case "hi":
			return "hi-IN"
		default:
			return "en-US"
		}
	}
	return fmt.Sprintf("%s-%s", strings.ToLower(parts[0]), strings.ToUpper(parts[1]))
}

// GetTranslations loads and caches the TranslationDictionary for a given language.
func GetTranslations(lang string) *TranslationDictionary {
	normalized := NormalizeLanguageCode(lang)

	cacheMu.RLock()
	if dict, found := localeCache[normalized]; found {
		cacheMu.RUnlock()
		return dict
	}
	cacheMu.RUnlock()

	dict, err := loadDictionary(normalized)
	if err != nil && normalized != "en-US" {
		dict, _ = loadDictionary("en-US")
	}

	if dict == nil {
		dict = &TranslationDictionary{
			ReviewComment: ReviewComment{
				TalkToDrixy: "Talk to Drixy by mentioning @drixy",
				Feedback:    "Was this suggestion helpful? React with 👍 or 👎 to help Drixy learn.",
			},
			PullRequestFinishSummaryMarkdown: PullRequestFinishSummary{
				WithComments:    "## Code Review Completed! 🔥\n\nThe review completed successfully.\n\n",
				WithoutComments: "## Code Review Completed! 🎉\nNo issues found.",
			},
		}
	}

	cacheMu.Lock()
	localeCache[normalized] = dict
	cacheMu.Unlock()

	return dict
}

func loadDictionary(locale string) (*TranslationDictionary, error) {
	filename := fmt.Sprintf("dictionaries/%s.json", locale)
	data, err := dictionaryFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var dict TranslationDictionary
	if err := json.Unmarshal(data, &dict); err != nil {
		return nil, err
	}
	return &dict, nil
}

// Interpolate replaces `{{key}}` in template with corresponding value from params.
func Interpolate(template string, params map[string]string) string {
	result := template
	for k, v := range params {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", k), v)
	}
	return result
}
