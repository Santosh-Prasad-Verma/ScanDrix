// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallGraphHelper_ExtractDefinitions(t *testing.T) {
	helper := NewCallGraphHelper()

	content := `package billing

import "fmt"

func ProcessPayment(amount int) error {
	fmt.Println("Processing")
	return nil
}

func VerifySignature(sig string) bool {
	return len(sig) > 0
}
`

	defs := helper.ExtractDefinitions("billing.go", content)
	require.Len(t, defs, 2)

	assert.Equal(t, "ProcessPayment", defs[0].Name)
	assert.Equal(t, 5, defs[0].Line)

	assert.Equal(t, "VerifySignature", defs[1].Name)
	assert.Equal(t, 10, defs[1].Line)
}

func TestCallGraphHelper_BuildCallGraphAndPrompt(t *testing.T) {
	helper := NewCallGraphHelper()

	files := []ChangedFile{
		{
			Filename: "internal/billing/service.go",
			Content: `package billing

func ProcessPayment(amount int) error {
	VerifySignature("abc")
	return nil
}

func VerifySignature(sig string) bool {
	return true
}
`,
		},
		{
			Filename: "internal/billing/handler.go",
			Content: `package billing

func HandleWebhook() {
	ProcessPayment(100)
}
`,
		},
	}

	graph := helper.BuildCallGraph(files)
	require.NotNil(t, graph)

	// ProcessPayment should have a caller in handler.go
	pp, exists := graph["ProcessPayment"]
	require.True(t, exists)
	assert.Len(t, pp.Callers, 1)
	assert.Equal(t, "internal/billing/handler.go", pp.Callers[0].File)

	// VerifySignature should have a caller in service.go
	vs, exists := graph["VerifySignature"]
	require.True(t, exists)
	assert.Len(t, vs.Callers, 1)

	// Assemble prompt
	prompt := helper.AssembleCallGraphPrompt(graph)
	assert.Contains(t, prompt, "ProcessPayment")
	assert.Contains(t, prompt, "HandleWebhook")
	assert.Contains(t, prompt, "VerifySignature")
}
