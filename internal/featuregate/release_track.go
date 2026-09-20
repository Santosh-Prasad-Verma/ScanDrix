// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Feature Gate Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package featuregate

// DefaultReleaseTrack defines the standard fallback rollout track.
const DefaultReleaseTrack = TrackBeta

// ReleaseTracks lists all recognized release stability tracks.
var ReleaseTracks = []ReleaseTrack{TrackStable, TrackBeta, TrackAlpha}

// IsReleaseTrack validates if a string corresponds to a known ReleaseTrack.
func IsReleaseTrack(value string) bool {
	switch ReleaseTrack(value) {
	case TrackStable, TrackBeta, TrackAlpha:
		return true
	default:
		return false
	}
}

var stageRequiredRank = map[FeatureStage]int{
	StageGeneralAvailability: 0,
	StageBeta:                1,
	StageAlpha:               2,
}

var trackRankMap = map[ReleaseTrack]int{
	TrackStable: 0,
	TrackBeta:   1,
	TrackAlpha:  2,
}

// TrackPermitsStage determines if a release track is eligible for a given feature stage.
// Cumulative model: alpha sees alpha+beta+ga; beta sees beta+ga; stable sees only ga.
func TrackPermitsStage(track ReleaseTrack, stage FeatureStage) bool {
	tRank, ok1 := trackRankMap[track]
	sRank, ok2 := stageRequiredRank[stage]
	if !ok1 || !ok2 {
		return false
	}
	return tRank >= sRank
}
