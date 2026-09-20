package ast_test

import (
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/codeanalysis/ast"
	"github.com/scandrix/backend/pkg/models"
)

func TestASTComplexityAndDeadCode(t *testing.T) {
	analyzer := ast.NewASTComplexityAnalyzer()

	// 1. Simple Function: Complexity = 1
	simpleSrc := `package test
func Add(a, b int) int {
	return a + b
}
`
	rep1, err := analyzer.Analyze("add.go", simpleSrc)
	if err != nil {
		t.Fatalf("simple analyze failed: %v", err)
	}
	if len(rep1.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(rep1.Functions))
	}
	fn1 := rep1.Functions[0]
	if fn1.CyclomaticComplexity != 1 || fn1.CognitiveComplexity != 0 || fn1.ParamCount != 2 {
		t.Fatalf("unexpected simple function metrics: %+v", fn1)
	}
	if len(rep1.Findings) != 0 {
		t.Fatalf("expected 0 findings on simple function, got %d", len(rep1.Findings))
	}

	// 2. High Cyclomatic Complexity (> 15)
	complexSrc := `package test
func ComplexBranching(x int) int {
	if x > 1 {
		if x > 2 {
			if x > 3 {
				if x > 4 {
					if x > 5 {
						if x > 6 {
							if x > 7 {
								if x > 8 {
									if x > 9 {
										if x > 10 {
											if x > 11 {
												if x > 12 {
													if x > 13 {
														if x > 14 {
															if x > 15 {
																return x * 2
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return x
}
`
	rep2, err := analyzer.Analyze("complex.go", complexSrc)
	if err != nil {
		t.Fatalf("complex analyze failed: %v", err)
	}
	fn2 := rep2.Functions[0]
	if fn2.CyclomaticComplexity <= 15 {
		t.Fatalf("expected cyclomatic > 15, got %d", fn2.CyclomaticComplexity)
	}
	if len(rep2.Findings) == 0 || rep2.Findings[0].Category != "CODE_COMPLEXITY" {
		t.Fatalf("expected code complexity finding, got %+v", rep2.Findings)
	}

	// 3. Excessive Parameters (> 5)
	excessiveParamsSrc := `package test
func ProcessOrder(id string, amount float64, currency string, customer string, priority int, dryRun bool) error {
	return nil
}
`
	rep3, err := analyzer.Analyze("order.go", excessiveParamsSrc)
	if err != nil {
		t.Fatalf("excessive params analyze failed: %v", err)
	}
	fn3 := rep3.Functions[0]
	if fn3.ParamCount != 6 {
		t.Fatalf("expected 6 parameters, got %d", fn3.ParamCount)
	}
	foundParamSmell := false
	for _, f := range rep3.Findings {
		if f.Category == "CLEAN_CODE" && f.Severity == models.SeverityLow {
			foundParamSmell = true
		}
	}
	if !foundParamSmell {
		t.Fatalf("expected excessive parameters code smell finding")
	}

	// 4. Dead Code After Return
	deadCodeSrc := `package test
func TerminateEarly() int {
	return 42
	println("this will never run")
}
`
	rep4, err := analyzer.Analyze("dead.go", deadCodeSrc)
	if err != nil {
		t.Fatalf("dead code analyze failed: %v", err)
	}
	foundDeadCode := false
	for _, f := range rep4.Findings {
		if f.Category == "DEAD_CODE" {
			foundDeadCode = true
		}
	}
	if !foundDeadCode {
		t.Fatalf("expected dead code finding")
	}

	// 5. Halstead Metrics
	halstead := rep1.Halstead
	if halstead.Volume <= 0 || halstead.ProgramVocabulary <= 0 || halstead.ProgramLength <= 0 {
		t.Fatalf("expected positive Halstead metrics, got: %+v", halstead)
	}

	// 6. MaxSourceFileSize DoS Protection (files > 2MB rejected)
	oversizedSrc := strings.Repeat("x := 1\n", (ast.MaxSourceFileSize/7)+100)
	_, errOversized := analyzer.Analyze("huge.go", oversizedSrc)
	if errOversized == nil {
		t.Fatalf("expected oversized source file (>2MB) to be rejected, got nil error")
	}
}
