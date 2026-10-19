package security

import "time"

// CTFCategory classifies the security exploitation domain.
type CTFCategory string

const (
	CategorySQLInjection     CTFCategory = "SQL_INJECTION"
	CategoryCommandInjection CTFCategory = "COMMAND_INJECTION"
	CategorySSRF             CTFCategory = "SSRF"
	CategoryIDOR             CTFCategory = "IDOR"
	CategoryPathTraversal    CTFCategory = "PATH_TRAVERSAL"
	CategoryAuthBypass       CTFCategory = "AUTH_BYPASS"
	CategoryXSS              CTFCategory = "XSS"
)

// CTFDifficulty defines challenge complexity.
type CTFDifficulty string

const (
	DifficultyEasy   CTFDifficulty = "EASY"
	DifficultyMedium CTFDifficulty = "MEDIUM"
	DifficultyHard   CTFDifficulty = "HARD"
	DifficultyInsane CTFDifficulty = "INSANE"
)

// CTFChallenge represents an individual web security capture-the-flag challenge.
type CTFChallenge struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	Category       CTFCategory   `json:"category"`
	Difficulty     CTFDifficulty `json:"difficulty"`
	Description    string        `json:"description"`
	VulnerableCode string        `json:"vulnerable_code"`
	CanaryExpected string        `json:"canary_expected"`
	PayloadRegex   string        `json:"payload_regex"` // Validation regex pattern for exploit proof
}

// CTFRunResult captures the evaluation of an autonomous agent attempting a challenge.
type CTFRunResult struct {
	ChallengeID      string        `json:"challenge_id"`
	Category         CTFCategory   `json:"category"`
	Difficulty       CTFDifficulty `json:"difficulty"`
	Solved           bool          `json:"solved"`
	GeneratedPayload string        `json:"generated_payload"`
	CanaryRetrieved  string        `json:"canary_retrieved,omitempty"`
	Latency          time.Duration `json:"latency"`
	ErrorMessage     string        `json:"error_message,omitempty"`
}

// CTFBenchmarkReport aggregates evaluation metrics across the CTF challenge suite.
type CTFBenchmarkReport struct {
	TotalChallenges    int                     `json:"total_challenges"`
	SolvedCount        int                     `json:"solved_count"`
	SolveRate          float64                 `json:"solve_rate"` // 0.0 - 1.0 (Percentage solved)
	MeanLatency        time.Duration           `json:"mean_latency"`
	SolvedByCategory   map[string]CategoryStats `json:"solved_by_category"`
	SolvedByDifficulty map[string]DiffStats    `json:"solved_by_difficulty"`
	Results            []CTFRunResult          `json:"results"`
}

// CategoryStats aggregates performance for a specific exploitation category.
type CategoryStats struct {
	Total     int     `json:"total"`
	Solved    int     `json:"solved"`
	SolveRate float64 `json:"solve_rate"`
}

// DiffStats aggregates performance for a difficulty tier.
type DiffStats struct {
	Total     int     `json:"total"`
	Solved    int     `json:"solved"`
	SolveRate float64 `json:"solve_rate"`
}
