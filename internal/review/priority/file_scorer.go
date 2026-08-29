package priority

import (
	"math"
	"sort"
	"strings"
)

// FileChangeStatus represents the git modification status of a file.
type FileChangeStatus string

const (
	StatusAdded    FileChangeStatus = "added"
	StatusModified FileChangeStatus = "modified"
	StatusRenamed  FileChangeStatus = "renamed"
	StatusRemoved  FileChangeStatus = "removed"
)

var statusMultipliers = map[FileChangeStatus]float64{
	StatusAdded:    1.2,
	StatusModified: 1.0,
	StatusRenamed:  0.3,
	StatusRemoved:  0.1,
}

// EdgeKind defines semantic AST graph relationships.
type EdgeKind string

const (
	EdgeCalls      EdgeKind = "CALLS"
	EdgeInherits   EdgeKind = "INHERITS"
	EdgeImplements EdgeKind = "IMPLEMENTS"
	EdgeImports    EdgeKind = "IMPORTS"
)

var edgeWeights = map[EdgeKind]float64{
	EdgeCalls:      3.0,
	EdgeInherits:   2.0,
	EdgeImplements: 2.0,
	EdgeImports:    1.0,
}

// ScoredFileChange details a file's review priority and blast-radius score.
type ScoredFileChange struct {
	FilePath         string           `json:"file_path"`
	Status           FileChangeStatus `json:"status"`
	Additions        int              `json:"additions"`
	Deletions        int              `json:"deletions"`
	Score            float64          `json:"score"`
	DiffMultiplier   float64          `json:"diff_multiplier"`
	StatusMultiplier float64          `json:"status_multiplier"`
	StructuralWeight float64          `json:"structural_weight"`
	InDegreeCalls    int              `json:"in_degree_calls"`
	InDegreeImports  int              `json:"in_degree_imports"`
}

// GraphEdge represents a cross-file reference in the repository AST.
type GraphEdge struct {
	Kind       EdgeKind
	SourceFile string
	TargetFile string
}

// ScoreAndPrioritizeFiles computes mathematical review priorities across all changed files.
// Score = DiffMultiplier * StatusMultiplier * StructuralWeight
func ScoreAndPrioritizeFiles(files []ScoredFileChange, edges []GraphEdge) []ScoredFileChange {
	if len(files) == 0 {
		return files
	}

	// 1. Compute log2 size multipliers
	maxLogSize := 1.0
	for _, f := range files {
		totalChanges := f.Additions + f.Deletions
		logSize := math.Log2(float64(totalChanges) + 1.0)
		if logSize > maxLogSize {
			maxLogSize = logSize
		}
	}

	// 2. Compute in-degree weights from call graph
	inDegreeWeights := make(map[string]float64)
	callsIn := make(map[string]int)
	importsIn := make(map[string]int)

	for _, e := range edges {
		weight := edgeWeights[e.Kind]
		if weight == 0 {
			weight = 1.0
		}
		inDegreeWeights[e.TargetFile] += weight
		if e.Kind == EdgeCalls {
			callsIn[e.TargetFile]++
		} else if e.Kind == EdgeImports {
			importsIn[e.TargetFile]++
		}
	}

	// 3. Calculate final scores
	scored := make([]ScoredFileChange, len(files))
	for i, f := range files {
		totalChanges := f.Additions + f.Deletions
		logSize := math.Log2(float64(totalChanges) + 1.0)

		// diffMultiplier mapped to [0.5, 1.5]
		diffMult := 0.5 + (logSize/maxLogSize)*1.0

		// statusMultiplier lookup
		statusMult := statusMultipliers[f.Status]
		if statusMult == 0 {
			statusMult = 1.0
		}

		// structuralWeight mapped to [1.0, 2.0] based on in-degree
		inWeight := inDegreeWeights[f.FilePath]
		structuralWeight := 1.0 + math.Min(1.0, inWeight/10.0)

		finalScore := diffMult * statusMult * structuralWeight

		f.Score = finalScore
		f.DiffMultiplier = diffMult
		f.StatusMultiplier = statusMult
		f.StructuralWeight = structuralWeight
		f.InDegreeCalls = callsIn[f.FilePath]
		f.InDegreeImports = importsIn[f.FilePath]
		scored[i] = f
	}

	// 4. Sort descending by score
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return strings.Compare(scored[i].FilePath, scored[j].FilePath) < 0
		}
		return scored[i].Score > scored[j].Score
	})

	return scored
}
