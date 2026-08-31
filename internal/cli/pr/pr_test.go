package pr_test

import (
	"testing"

	"github.com/scandrix/backend/internal/cli/pr"
)

func TestParsePRInput(t *testing.T) {
	tests := []struct {
		input         string
		wantNamespace string
		wantNumber    int
		wantErr       bool
	}{
		{
			input:         "42",
			wantNamespace: "",
			wantNumber:    42,
			wantErr:       false,
		},
		{
			input:         "https://github.com/kodustech/kodus-ai/pull/105",
			wantNamespace: "kodustech/kodus-ai",
			wantNumber:    105,
			wantErr:       false,
		},
		{
			input:         "https://gitlab.com/enterprise-org/backend-service/-/merge_requests/88",
			wantNamespace: "enterprise-org/backend-service",
			wantNumber:    88,
			wantErr:       false,
		},
		{
			input:   "invalid-url-string",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		ns, num, err := pr.ParsePRInput(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("input %s: got error %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if ns != tt.wantNamespace {
				t.Errorf("input %s: got namespace %s, want %s", tt.input, ns, tt.wantNamespace)
			}
			if num != tt.wantNumber {
				t.Errorf("input %s: got number %d, want %d", tt.input, num, tt.wantNumber)
			}
		}
	}
}
