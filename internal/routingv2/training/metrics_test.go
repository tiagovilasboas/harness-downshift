// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"math"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func TestConfusionMatrix_Accuracy(t *testing.T) {
	var cm ConfusionMatrix

	// Add 10 predictions: 8 correct, 2 wrong
	for i := 0; i < 3; i++ {
		cm.Add(core.TierSmall, core.TierSmall)
	}
	for i := 0; i < 3; i++ {
		cm.Add(core.TierMid, core.TierMid)
	}
	for i := 0; i < 2; i++ {
		cm.Add(core.TierFrontier, core.TierFrontier)
	}
	// 2 errors
	cm.Add(core.TierSmall, core.TierMid)
	cm.Add(core.TierFrontier, core.TierMid)

	if cm.Total() != 10 {
		t.Errorf("Total = %d, want 10", cm.Total())
	}

	if cm.Correct() != 8 {
		t.Errorf("Correct = %d, want 8", cm.Correct())
	}

	acc := cm.Accuracy()
	if math.Abs(acc-0.8) > 0.001 {
		t.Errorf("Accuracy = %.3f, want 0.8", acc)
	}
}

func TestConfusionMatrix_UnsafeDowngrades(t *testing.T) {
	var cm ConfusionMatrix

	// FRONTIER → SMALL (2 times)
	cm.Add(core.TierFrontier, core.TierSmall)
	cm.Add(core.TierFrontier, core.TierSmall)

	// FRONTIER → MID (1 time)
	cm.Add(core.TierFrontier, core.TierMid)

	// FRONTIER → FRONTIER (correct)
	cm.Add(core.TierFrontier, core.TierFrontier)

	unsafe := cm.UnsafeDowngrades()

	if unsafe.FrontierToSmall != 2 {
		t.Errorf("FrontierToSmall = %d, want 2", unsafe.FrontierToSmall)
	}

	if unsafe.FrontierToMid != 1 {
		t.Errorf("FrontierToMid = %d, want 1", unsafe.FrontierToMid)
	}

	if unsafe.Total != 3 {
		t.Errorf("Total unsafe = %d, want 3", unsafe.Total)
	}

	rate := cm.UnsafeDowngradeRate()
	// 3 unsafe out of 4 FRONTIER examples = 75%
	if math.Abs(rate-0.75) > 0.001 {
		t.Errorf("UnsafeDowngradeRate = %.3f, want 0.75", rate)
	}
}

func TestConfusionMatrix_OverRouting(t *testing.T) {
	var cm ConfusionMatrix

	// SMALL → MID
	cm.Add(core.TierSmall, core.TierMid)
	cm.Add(core.TierSmall, core.TierMid)

	// SMALL → FRONTIER
	cm.Add(core.TierSmall, core.TierFrontier)

	// MID → FRONTIER
	cm.Add(core.TierMid, core.TierFrontier)

	over := cm.OverRouting()

	if over.SmallToMid != 2 {
		t.Errorf("SmallToMid = %d, want 2", over.SmallToMid)
	}

	if over.SmallToFrontier != 1 {
		t.Errorf("SmallToFrontier = %d, want 1", over.SmallToFrontier)
	}

	if over.MidToFrontier != 1 {
		t.Errorf("MidToFrontier = %d, want 1", over.MidToFrontier)
	}

	if over.Total != 4 {
		t.Errorf("Total over-routing = %d, want 4", over.Total)
	}
}

func TestCompute_Metrics(t *testing.T) {
	actual := []core.Tier{
		core.TierSmall, core.TierSmall,
		core.TierMid, core.TierMid,
		core.TierFrontier, core.TierFrontier,
	}
	predicted := []core.Tier{
		core.TierSmall, core.TierMid, // 1 correct, 1 over-route
		core.TierMid, core.TierSmall, // 1 correct, 1 under-route
		core.TierFrontier, core.TierSmall, // 1 correct, 1 catastrophic
	}

	metrics := Compute(actual, predicted)

	// 3 correct out of 6 = 50%
	if math.Abs(metrics.Accuracy-0.5) > 0.001 {
		t.Errorf("Accuracy = %.3f, want 0.5", metrics.Accuracy)
	}

	// 1 unsafe downgrade (FRONTIER → SMALL) out of 2 FRONTIER = 50%
	if math.Abs(metrics.UnsafeDowngradeRate-0.5) > 0.001 {
		t.Errorf("UnsafeDowngradeRate = %.3f, want 0.5", metrics.UnsafeDowngradeRate)
	}

	// Risk loss should be > 0 (we have a catastrophic error)
	if metrics.RiskWeightedLoss <= 0 {
		t.Error("RiskWeightedLoss should be > 0")
	}
}

func TestRiskWeight(t *testing.T) {
	tests := []struct {
		actual, pred core.Tier
		want         float64
	}{
		{core.TierSmall, core.TierSmall, 0},       // Correct
		{core.TierMid, core.TierMid, 0},           // Correct
		{core.TierFrontier, core.TierFrontier, 0}, // Correct

		{core.TierFrontier, core.TierSmall, 10.0}, // Catastrophic
		{core.TierFrontier, core.TierMid, 5.0},    // Expensive under-route
		{core.TierMid, core.TierSmall, 2.0},       // Moderate under-route

		{core.TierSmall, core.TierMid, 0.5},       // Cheap over-route
		{core.TierSmall, core.TierFrontier, 1.0},  // Over-route but safe
		{core.TierMid, core.TierFrontier, 0.5},    // Over-route but safe
	}

	for _, tc := range tests {
		got := riskWeight(tc.actual, tc.pred)
		if math.Abs(got-tc.want) > 0.001 {
			t.Errorf("riskWeight(%v, %v) = %.1f, want %.1f",
				tc.actual, tc.pred, got, tc.want)
		}
	}
}

func TestConfusionMatrix_String(t *testing.T) {
	var cm ConfusionMatrix
	cm.Add(core.TierSmall, core.TierSmall)
	cm.Add(core.TierMid, core.TierMid)
	cm.Add(core.TierFrontier, core.TierFrontier)

	s := cm.String()

	// Should contain the header
	if len(s) == 0 {
		t.Error("String() returned empty")
	}

	// Should contain "SMALL" and "FRONTIER"
	if !contains(s, "SMALL") || !contains(s, "FRONTIER") {
		t.Error("String() should contain tier names")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestMetrics_Summary(t *testing.T) {
	actual := []core.Tier{core.TierSmall, core.TierMid, core.TierFrontier}
	predicted := []core.Tier{core.TierSmall, core.TierMid, core.TierFrontier}

	metrics := Compute(actual, predicted)
	summary := metrics.Summary()

	if len(summary) == 0 {
		t.Error("Summary() returned empty")
	}

	// Should contain accuracy
	if !containsSubstring(summary, "accuracy") {
		t.Error("Summary should contain 'accuracy'")
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
