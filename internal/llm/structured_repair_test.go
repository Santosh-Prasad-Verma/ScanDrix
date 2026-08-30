package llm_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm"
)

type SampleOutput struct {
	Summary string   `json:"summary"`
	Issues  []string `json:"issues"`
}

func TestCleanJSONResponse(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "```json\n{\"summary\": \"ok\"}\n```",
			expected: "{\"summary\": \"ok\"}",
		},
		{
			input:    "```\n{\"summary\": \"ok\"}\n```",
			expected: "{\"summary\": \"ok\"}",
		},
		{
			input:    "  {\"summary\": \"ok\"}  ",
			expected: "{\"summary\": \"ok\"}",
		},
	}

	for _, c := range cases {
		res := llm.CleanJSONResponse(c.input)
		if res != c.expected {
			t.Errorf("expected %q, got %q", c.expected, res)
		}
	}
}

func TestValidateAndUnmarshal(t *testing.T) {
	raw := "```json\n{\"summary\": \"review passed\", \"issues\": [\"none\"]}\n```"
	var out SampleOutput
	err := llm.ValidateAndUnmarshal(raw, &out)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if out.Summary != "review passed" || len(out.Issues) != 1 {
		t.Errorf("unexpected output fields: %+v", out)
	}
}
