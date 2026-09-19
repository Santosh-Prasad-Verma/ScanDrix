package clireview

import (
	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// TraceBranch is the git orphan branch storing trace records.
const TraceBranch = adapters.TraceBranch

// TraceRecordPath computes the deterministic storage path for a branch's trace records.
var TraceRecordPath = adapters.TraceRecordPath

// IRepositoryContentReader abstracts fetching raw file content from code hosts.
type IRepositoryContentReader = adapters.IRepositoryContentReader

// TraceDecisionBranchReaderService reads branch-specific decision records.
type TraceDecisionBranchReaderService = adapters.TraceDecisionBranchReaderService

// NewTraceDecisionBranchReaderService creates a new reader service.
func NewTraceDecisionBranchReaderService(reader IRepositoryContentReader) *TraceDecisionBranchReaderService {
	return adapters.NewTraceDecisionBranchReaderService(reader)
}
