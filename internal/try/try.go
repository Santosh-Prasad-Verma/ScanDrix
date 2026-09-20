// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Public PR Review & Interactive Playground Engine
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package try

// Package try provides the core engine for public anonymous PR reviews,
// diff parsing, sliding-window rate limiting, and featured review snapshots.

// Public error codes matching PUBLIC_API.md specification
const (
	ErrCodeInvalidURL    = "invalid_url"
	ErrCodeRequiresAuth  = "requires_auth"
	ErrCodeTooLarge      = "too_large"
	ErrCodeRateLimited   = "rate_limited"
	ErrCodeUpstreamError = "upstream_error"
	ErrCodeNotFound      = "not_found"
	ErrCodeInvalidJobID  = "invalid_job_id"
)

// Default size limits for anonymous public PR reviews
const (
	DefaultMaxLines = 10000
	DefaultMaxFiles = 80
	DefaultMaxJobs  = 10000
)
