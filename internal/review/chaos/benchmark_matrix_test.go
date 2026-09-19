package chaos_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/review/chaos"
)

func BenchmarkCommentPayloadChunker_LargeMarkdown(b *testing.B) {
	chunker := chaos.NewCommentPayloadChunker()

	var sb strings.Builder
	sb.WriteString("# Code Review Findings Summary\n\n```typescript\n")
	for i := 0; i < 2000; i++ {
		sb.WriteString("export async function handleRequest(ctx: Context, req: Request): Promise<Response> { return new Response('ok'); }\n")
	}
	sb.WriteString("```\n")
	rawContent := sb.String()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		chunks := chunker.SplitComment(rawContent, chaos.PlatformGitHub)
		if len(chunks) == 0 {
			b.Fatal("expected chunks")
		}
	}
}

func BenchmarkValidateDiffHunkPosition(b *testing.B) {
	modifiedRanges := [][2]int{
		{10, 25},
		{50, 75},
		{100, 150},
		{200, 280},
		{500, 620},
		{1000, 1150},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		line := (i % 1200) + 1
		_ = chaos.ValidateDiffHunkPosition(line, line+2, modifiedRanges)
	}
}

func BenchmarkSCMChaosInjector_InterceptUnderLoad(b *testing.B) {
	injector := chaos.NewSCMChaosInjector()
	for i := 0; i < 100; i++ {
		op := fmt.Sprintf("op_%d", i)
		injector.InjectFault(op, chaos.ChaosFaultRule{
			FaultType:    chaos.FaultNone,
			TriggerCount: 0,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			op := fmt.Sprintf("op_%d", i%100)
			_ = injector.Intercept(nil, chaos.PlatformGitHub, op)
			i++
		}
	})
}
