package scoring

import (
	"math"
	"time"
)

const (
	// sWeight is the share-count weight in the Pulse Score formula.
	// Exposed as a constant so Phase 3 can make it runtime-configurable via kill switch.
	sWeight  = 1.5
	maxScore = 100.0
	// stabilityFloor is the minimum age denominator (seconds) per TRD §2.1.
	stabilityFloor = 30.0
)

// PulseScore computes the Pulse Score at write time per TRD §2.1:
//
//	Score = min(100, (S_weight * log10(R_count + 1)) + P_bias / (T_now - T_pub + 30)^1.5)
//
// shareCount = R_count (raw share/reaction count)
// pBias      = P_bias  (per-source recency bias, default 1.0)
// publishedAt = T_pub
func PulseScore(shareCount int, pBias float64, publishedAt time.Time) float64 {
	age := time.Since(publishedAt).Seconds() + stabilityFloor

	shareTerm   := sWeight * math.Log10(float64(shareCount)+1)
	recencyTerm := pBias / math.Pow(age, 1.5)

	return math.Min(maxScore, shareTerm+recencyTerm)
}
