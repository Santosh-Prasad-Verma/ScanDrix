// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package all

import (
	"github.com/scandrix/backend/internal/llm/providers/anthropic"
	"github.com/scandrix/backend/internal/llm/providers/azure"
	"github.com/scandrix/backend/internal/llm/providers/bedrock"
	"github.com/scandrix/backend/internal/llm/providers/gemini"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/providers/moonshot"
	"github.com/scandrix/backend/internal/llm/providers/novita"
	"github.com/scandrix/backend/internal/llm/providers/openai"
	"github.com/scandrix/backend/internal/llm/providers/openrouter"
	"github.com/scandrix/backend/internal/llm/providers/vertex"
	"github.com/scandrix/backend/internal/llm/providers/zai"
)

func init() {
	kernel.Register(openai.New())
	kernel.Register(anthropic.New())
	kernel.Register(gemini.New())
	kernel.Register(vertex.New())
	kernel.Register(bedrock.New())
	kernel.Register(openrouter.New())
	kernel.Register(novita.New())
	kernel.Register(moonshot.New())
	kernel.Register(zai.New())
	kernel.Register(azure.New())
}
