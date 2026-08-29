package featuregate

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
)

// ReleaseTrack categorizes stability tiers for features.
type ReleaseTrack string

const (
	TrackAlpha  ReleaseTrack = "alpha"
	TrackBeta   ReleaseTrack = "beta"
	TrackStable ReleaseTrack = "stable"
)

var trackRanks = map[ReleaseTrack]int{
	TrackAlpha:  1,
	TrackBeta:   2,
	TrackStable: 3,
}

// AudienceType distinguishes cloud-managed vs air-gapped installations.
type AudienceType string

const (
	AudienceCloud      AudienceType = "cloud"
	AudienceSelfHosted AudienceType = "self_hosted"
)

// FeatureFlag defines rollout policies for a platform capability.
type FeatureFlag struct {
	Key             string       `json:"key"`
	MinTrack        ReleaseTrack `json:"min_track"`
	AllowedAudience AudienceType `json:"allowed_audience"`
	RolloutPercent  int          `json:"rollout_percent"` // 0 to 100
	Enabled         bool         `json:"enabled"`
}

// EvaluationContext carries tenant and user metadata for flag checks.
type EvaluationContext struct {
	WorkspaceID string
	UserID      string
	Track       ReleaseTrack
	Audience    AudienceType
}

// FeatureGateEngine evaluates feature access across workspaces and users.
type FeatureGateEngine struct {
	mu    sync.RWMutex
	flags map[string]FeatureFlag
}

func NewFeatureGateEngine() *FeatureGateEngine {
	e := &FeatureGateEngine{
		flags: make(map[string]FeatureFlag),
	}
	e.loadDefaultFlags()
	return e
}

func (e *FeatureGateEngine) loadDefaultFlags() {
	e.flags["deep_ast_reasoning"] = FeatureFlag{
		Key:             "deep_ast_reasoning",
		MinTrack:        TrackBeta,
		AllowedAudience: AudienceCloud,
		RolloutPercent:  100,
		Enabled:         true,
	}
	e.flags["dora_executive_cockpit"] = FeatureFlag{
		Key:             "dora_executive_cockpit",
		MinTrack:        TrackStable,
		AllowedAudience: AudienceCloud,
		RolloutPercent:  100,
		Enabled:         true,
	}
	e.flags["byok_custom_endpoints"] = FeatureFlag{
		Key:             "byok_custom_endpoints",
		MinTrack:        TrackAlpha,
		AllowedAudience: AudienceSelfHosted,
		RolloutPercent:  100,
		Enabled:         true,
	}
}

// SetFlag updates or registers a flag.
func (e *FeatureGateEngine) SetFlag(flag FeatureFlag) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.flags[flag.Key] = flag
}

// IsEnabled checks whether a feature is active for the given evaluation context.
func (e *FeatureGateEngine) IsEnabled(key string, ctx EvaluationContext) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	flag, exists := e.flags[key]
	if !exists || !flag.Enabled {
		return false
	}

	// 1. Audience check
	if flag.AllowedAudience != "" && flag.AllowedAudience != ctx.Audience {
		return false
	}

	// 2. Release track check (user track must be >= flag min track)
	userTrackRank := trackRanks[ctx.Track]
	flagMinRank := trackRanks[flag.MinTrack]
	if userTrackRank < flagMinRank {
		return false
	}

	// 3. Gradual rollout percentage check via deterministic hash
	if flag.RolloutPercent < 100 {
		h := sha256.Sum256([]byte(key + ":" + ctx.WorkspaceID))
		val := binary.BigEndian.Uint32(h[:4]) % 100
		if int(val) >= flag.RolloutPercent {
			return false
		}
	}

	return true
}
