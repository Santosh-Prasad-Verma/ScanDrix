package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const MaxShellOutput = 10000

// CheckTypesToolOptions configures type checker and compiler execution.
type CheckTypesToolOptions struct {
	Sandbox SandboxExecutor
}

// NewCheckTypesTool creates the multi-language compiler, linter, and type checker tool.
func NewCheckTypesTool(opts CheckTypesToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "checkTypes",
		ToolDesc: "Run a local type checker, compiler, or linter for a file or directory. Auto-detects language and scopes " +
			"checks to the nearest package/project when possible (nearest tsconfig, go.mod, Cargo.toml, csproj). " +
			"Use this to confirm concrete type errors, compile errors, wiring issues, and import problems.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "File or directory to check (default: '.'). Example: 'internal/review/orchestrator.go' or 'src/api'",
				},
			},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			target := NormalizePath(StringArg(input, "path", "file", "target"))
			if target == "" {
				target = "."
			}

			if opts.Sandbox == nil {
				return contracts.ToolResult{
					Output: "⚠️ Sandbox executor unavailable — cannot run local type checkers or compilers.",
				}, nil
			}

			results, anyExecuted, err := runMultiLangTypeChecks(ctx.Context, opts.Sandbox, target)
			if err != nil {
				return contracts.ToolResult{
					Output:  fmt.Sprintf("Error checking types for %s: %v", target, err),
					IsError: true,
				}, nil
			}

			if len(results) == 0 {
				if anyExecuted {
					return contracts.ToolResult{
						Output: "Ran a build/type check on the changed files — no type errors or linter diagnostics found.",
					}, nil
				}
				return contracts.ToolResult{
					Output: "⚠️ Could NOT run a build/type check here — no compiler/linter for these files is available in the sandbox (missing toolchain or dependencies). " +
						"This is NOT a clean result: the code was not verified. Do not assume it compiles or that signatures and types are correct. " +
						"Verify manually — read the callee/definition with readFile and confirm the call matches its signature.",
				}, nil
			}

			combined := strings.Join(results, "\n\n")
			if len(combined) > MaxShellOutput {
				combined = combined[:MaxShellOutput] + "\n... (truncated)"
			}
			return contracts.ToolResult{Output: combined}, nil
		},
	}
}

func runMultiLangTypeChecks(ctx context.Context, sandbox SandboxExecutor, target string) ([]string, bool, error) {
	safeTarget := ShellQuote(target)

	// Detect which language extensions exist in target
	scanCmd := fmt.Sprintf("[ -f %s ] && printf '%%s\\n' %s || find %s -maxdepth 3 -type f 2>/dev/null | head -n 50",
		safeTarget, safeTarget, safeTarget)
	stdout, _, _, err := sandbox.Exec(ctx, scanCmd)
	if err != nil {
		return nil, false, err
	}
	fileList := stdout

	results := make([]string, 0)
	anyCheckerExecuted := false

	pushResult := func(lang, scope, rawOutput string) {
		output := strings.TrimSpace(rawOutput)
		if output != "" && strings.Contains(output, "command not found") {
			return
		}
		anyCheckerExecuted = true
		if output == "" {
			return
		}

		filtered := FilterDiagnosticsToTarget(output, target, scope)
		if filtered != "" {
			res := fmt.Sprintf("[%s — scope: %s]\n%s", lang, scope, filtered)
			if len(res) > MaxShellOutput {
				res = res[:MaxShellOutput] + "\n... (truncated)"
			}
			results = append(results, res)
		} else {
			results = append(results, fmt.Sprintf("[%s — scope: %s]\nNo diagnostics matched %s; omitted unrelated diagnostics outside local scope.", lang, scope, target))
		}
	}

	// 1. Go checks
	if strings.Contains(fileList, ".go") {
		scopeDir := target
		if strings.HasSuffix(target, ".go") {
			scopeDir = filepath.Dir(target)
		}
		packageScope := "."
		if scopeDir != "" && scopeDir != "." {
			packageScope = "./" + scopeDir
		}

		for _, cmd := range []string{
			fmt.Sprintf("go vet %s 2>&1 | head -n 30", ShellQuote(packageScope)),
			fmt.Sprintf("go build -o /dev/null %s 2>&1 | head -n 30", ShellQuote(packageScope)),
		} {
			out, _, _, _ := sandbox.Exec(ctx, cmd)
			pushResult("Go", packageScope, out)
		}
	}

	// 2. TypeScript / JavaScript checks
	if strings.Contains(fileList, ".ts") || strings.Contains(fileList, ".tsx") {
		tsconfig := findNearestConfig(ctx, sandbox, target, "tsconfig.json")
		var cmd string
		if tsconfig != "" {
			cmd = fmt.Sprintf("npx tsc --noEmit -p %s 2>&1 | head -n 40", ShellQuote(tsconfig))
		} else {
			cmd = fmt.Sprintf("npx tsc --noEmit --pretty false %s 2>&1 | head -n 40", safeTarget)
		}
		out, _, _, _ := sandbox.Exec(ctx, cmd)
		scope := target
		if tsconfig != "" {
			scope = tsconfig
		}
		pushResult("TypeScript", scope, out)
	}

	// 3. Python checks
	if strings.Contains(fileList, ".py") {
		pyFiles := collectFilesWithExt(ctx, sandbox, target, ".py", 10)
		if len(pyFiles) > 0 {
			quoted := make([]string, len(pyFiles))
			for i, f := range pyFiles {
				quoted[i] = ShellQuote(f)
			}
			pyCmd := fmt.Sprintf("python3 -m py_compile %s 2>&1", strings.Join(quoted, " "))
			out, _, _, _ := sandbox.Exec(ctx, pyCmd)
			pushResult("Python", target, out)

			mypyCmd := fmt.Sprintf("mypy %s --no-error-summary --no-color 2>&1 | head -n 30", safeTarget)
			out, _, _, _ = sandbox.Exec(ctx, mypyCmd)
			pushResult("Python (mypy)", target, out)
		}
	}

	// 4. Rust checks
	if strings.Contains(fileList, ".rs") {
		cargoToml := findNearestConfig(ctx, sandbox, target, "Cargo.toml")
		if cargoToml != "" {
			cmd := fmt.Sprintf("cargo check --manifest-path %s 2>&1 | head -n 30", ShellQuote(cargoToml))
			out, _, _, _ := sandbox.Exec(ctx, cmd)
			pushResult("Rust", cargoToml, out)
		}
	}

	// 5. C# / .NET checks
	if strings.Contains(fileList, ".cs") {
		csproj := findNearestConfig(ctx, sandbox, target, "*.csproj")
		if csproj != "" {
			cmd := fmt.Sprintf("dotnet build %s --no-restore 2>&1 | grep -E 'error|warning' | head -n 30", ShellQuote(csproj))
			out, _, _, _ := sandbox.Exec(ctx, cmd)
			pushResult("C#", csproj, out)
		}
	}

	// 6. Java checks
	if strings.Contains(fileList, ".java") {
		javaFiles := collectFilesWithExt(ctx, sandbox, target, ".java", 5)
		if len(javaFiles) > 0 {
			quoted := make([]string, len(javaFiles))
			for i, f := range javaFiles {
				quoted[i] = ShellQuote(f)
			}
			cmd := fmt.Sprintf("javac -d /tmp/javaout %s 2>&1 | head -n 30", strings.Join(quoted, " "))
			out, _, _, _ := sandbox.Exec(ctx, cmd)
			pushResult("Java", target, out)
		}
	}

	// 7. Ruby checks
	if strings.Contains(fileList, ".rb") {
		rbFiles := collectFilesWithExt(ctx, sandbox, target, ".rb", 10)
		if len(rbFiles) > 0 {
			quoted := make([]string, len(rbFiles))
			for i, f := range rbFiles {
				quoted[i] = ShellQuote(f)
			}
			cmd := fmt.Sprintf("ruby -c %s 2>&1 | grep -v 'Syntax OK' | head -n 20", strings.Join(quoted, " "))
			out, _, _, _ := sandbox.Exec(ctx, cmd)
			pushResult("Ruby", target, out)
		}
	}

	return results, anyCheckerExecuted, nil
}

func findNearestConfig(ctx context.Context, sandbox SandboxExecutor, target, configName string) string {
	normalized := NormalizePath(target)
	cmd := fmt.Sprintf(`
dir=%s
if [ -f "$dir" ]; then
  dir=$(dirname "$dir")
fi
while [ "$dir" != "." ] && [ "$dir" != "/" ]; do
  if ls "$dir"/%s 1>/dev/null 2>&1; then
    printf '%%s\n' "$dir"/%s
    exit 0
  fi
  dir=$(dirname "$dir")
done
if ls ./%s 1>/dev/null 2>&1; then
  printf '%%s\n' ./%s
  exit 0
fi
`, ShellQuote(normalized), configName, configName, configName, configName)

	stdout, _, exitCode, err := sandbox.Exec(ctx, cmd)
	if err == nil && exitCode == 0 {
		return strings.TrimSpace(stdout)
	}
	return ""
}

func collectFilesWithExt(ctx context.Context, sandbox SandboxExecutor, target, ext string, maxFiles int) []string {
	safeTarget := ShellQuote(target)
	cmd := fmt.Sprintf(`
target=%s
if [ -f "$target" ]; then
  printf '%%s\n' "$target"
else
  find "$target" -maxdepth 3 -type f -name '*%s' 2>/dev/null | head -n %d
fi
`, safeTarget, ext, maxFiles)

	stdout, _, _, err := sandbox.Exec(ctx, cmd)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	res := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// FilterDiagnosticsToTarget filters compiler and linter output lines relevant to the target and scope.
func FilterDiagnosticsToTarget(output, targetPath, scopePath string) string {
	normalizedTarget := NormalizePath(targetPath)
	targetBase := filepath.Base(normalizedTarget)
	targetDir := filepath.Dir(normalizedTarget)
	normalizedScope := NormalizePath(scopePath)

	markers := []string{normalizedTarget, targetBase}
	if targetDir != "." {
		markers = append(markers, targetDir+"/")
	}
	if normalizedScope != "" && normalizedScope != "." {
		markers = append(markers, normalizedScope)
	}

	lines := strings.Split(output, "\n")
	keep := make(map[int]bool)

	for i, line := range lines {
		hasMarker := false
		for _, m := range markers {
			if strings.Contains(line, m) {
				hasMarker = true
				break
			}
		}
		if hasMarker {
			keep[i] = true
			if i > 0 {
				keep[i-1] = true
			}
			if i+1 < len(lines) {
				keep[i+1] = true
			}
		}
	}

	if len(keep) == 0 {
		return ""
	}

	res := make([]string, 0, len(keep))
	for i := 0; i < len(lines); i++ {
		if keep[i] {
			res = append(res, lines[i])
		}
	}
	return strings.TrimSpace(strings.Join(res, "\n"))
}
