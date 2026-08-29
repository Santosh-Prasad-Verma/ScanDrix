package ast

import (
	"go/scanner"
	"go/token"
	"math"
)

// CalculateHalstead computes software science volume and effort metrics for Go source code.
func CalculateHalstead(src string) HalsteadMetrics {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	s.Init(file, []byte(src), nil, scanner.ScanComments)

	operators := make(map[string]int)
	operands := make(map[string]int)

	totalOperators := 0
	totalOperands := 0

	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		if tok.IsKeyword() || tok.IsOperator() {
			opStr := tok.String()
			operators[opStr]++
			totalOperators++
		} else if tok.IsLiteral() || tok == token.IDENT {
			opVal := lit
			if opVal == "" {
				opVal = tok.String()
			}
			operands[opVal]++
			totalOperands++
		}
	}

	distinctOperators := len(operators)
	distinctOperands := len(operands)

	vocabulary := distinctOperators + distinctOperands
	length := totalOperators + totalOperands

	volume := 0.0
	if vocabulary > 0 {
		volume = float64(length) * math.Log2(float64(vocabulary))
	}

	difficulty := 0.0
	if distinctOperands > 0 {
		difficulty = (float64(distinctOperators) / 2.0) * (float64(totalOperands) / float64(distinctOperands))
	}

	effort := difficulty * volume

	return HalsteadMetrics{
		DistinctOperators: distinctOperators,
		DistinctOperands:  distinctOperands,
		TotalOperators:    totalOperators,
		TotalOperands:     totalOperands,
		ProgramVocabulary: vocabulary,
		ProgramLength:     length,
		Volume:            math.Round(volume*100) / 100,
		Difficulty:        math.Round(difficulty*100) / 100,
		Effort:            math.Round(effort*100) / 100,
	}
}
