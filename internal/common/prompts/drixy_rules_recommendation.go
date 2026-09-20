package prompts

// RuleRecommendationItem describes a rule recommended for adoption in the repository.
type RuleRecommendationItem struct {
	UUID           string  `json:"uuid"`
	Reason         string  `json:"reason"`
	RelevanceScore float64 `json:"relevanceScore"`
}

// DrixyRulesRecommendation represents the collection of rule recommendations.
type DrixyRulesRecommendation struct {
	Recommendations []RuleRecommendationItem `json:"recommendations"`
}

// PromptDrixyRulesRecommendationSystem generates system instructions for recommending rules.
func PromptDrixyRulesRecommendationSystem() string {
	return `You are a software quality consultant analyzing code review history.
Your mission is to recommend Drixy Rules from the library that address recurring quality, security, or performance issues observed across pull requests.

Output format: Return ONLY valid JSON matching this schema:
{
  "recommendations": [
    {
      "uuid": "rule-uuid",
      "reason": "Clear explanation citing observed code patterns and PR impact",
      "relevanceScore": 8.5
    }
  ]
}`
}
