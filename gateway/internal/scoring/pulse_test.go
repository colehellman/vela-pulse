package scoring

import (
	"math"
	"testing"
	"time"
)

func TestPulseScore_CapAt100(t *testing.T) {
	// To trigger the cap: pBias / (age+30)^1.5 > 100
	// age ≈ 0+30=30 → need pBias > 100 * 30^1.5 ≈ 16,432
	// Use pBias=20_000 with a fresh article to guarantee the cap fires.
	score := PulseScore(0, 20_000.0, time.Now())
	if score != maxScore {
		t.Errorf("expected 100.0, got %.4f", score)
	}
}

func TestPulseScore_ZeroShares(t *testing.T) {
	// log10(0+1) = 0, so share term is 0; only recency term contributes.
	pub := time.Now().Add(-30 * time.Second)
	score := PulseScore(0, 1.0, pub)
	// age ≈ 30+30=60, recencyTerm = 1.0/60^1.5 ≈ 0.00215
	if score <= 0 || score > maxScore {
		t.Errorf("unexpected score %.6f", score)
	}
}

func TestPulseScore_StabilityFloor(t *testing.T) {
	// Two articles published at the same time vs 1 second apart should produce
	// nearly equal scores (floor prevents infinite denominator).
	now := time.Now()
	s1 := PulseScore(10, 1.0, now)
	s2 := PulseScore(10, 1.0, now.Add(-1*time.Second))
	diff := math.Abs(s1 - s2)
	if diff > 0.1 {
		t.Errorf("stability floor not working: s1=%.4f s2=%.4f diff=%.4f", s1, s2, diff)
	}
}

func TestPulseScore_HigherShareCountHigherScore(t *testing.T) {
	pub := time.Now().Add(-5 * time.Minute)
	low := PulseScore(10, 1.0, pub)
	high := PulseScore(1000, 1.0, pub)
	if high <= low {
		t.Errorf("higher share count should produce higher score: low=%.4f high=%.4f", low, high)
	}
}

func TestPulseScore_OlderArticleLowerScore(t *testing.T) {
	fresh := PulseScore(100, 1.0, time.Now().Add(-1*time.Minute))
	stale := PulseScore(100, 1.0, time.Now().Add(-24*time.Hour))
	if stale >= fresh {
		t.Errorf("older article should score lower: fresh=%.4f stale=%.4f", fresh, stale)
	}
}

func TestPulseScore_PBiasScales(t *testing.T) {
	pub := time.Now().Add(-10 * time.Minute)
	base := PulseScore(0, 1.0, pub)
	boosted := PulseScore(0, 2.0, pub)
	if boosted <= base {
		t.Errorf("higher pBias should produce higher score: base=%.4f boosted=%.4f", base, boosted)
	}
}

func TestPulseScore_NeverNegative(t *testing.T) {
	// Even a very old article with zero shares should not go negative.
	score := PulseScore(0, 0.0, time.Now().Add(-365*24*time.Hour))
	if score < 0 {
		t.Errorf("score should never be negative, got %.6f", score)
	}
}
