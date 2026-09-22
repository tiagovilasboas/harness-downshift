// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"fmt"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// ConfusionMatrix tracks predicted vs actual tier classifications.
type ConfusionMatrix struct {
	Matrix [3][3]int // [actual][predicted]
}

// Add records a single prediction.
func (cm *ConfusionMatrix) Add(actual, predicted core.Tier) {
	cm.Matrix[actual][predicted]++
}

// Total returns the total number of predictions.
func (cm *ConfusionMatrix) Total() int {
	var total int
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			total += cm.Matrix[i][j]
		}
	}
	return total
}

// Correct returns the number of correct predictions (diagonal).
func (cm *ConfusionMatrix) Correct() int {
	return cm.Matrix[0][0] + cm.Matrix[1][1] + cm.Matrix[2][2]
}

// Accuracy returns the fraction of correct predictions.
func (cm *ConfusionMatrix) Accuracy() float64 {
	total := cm.Total()
	if total == 0 {
		return 0
	}
	return float64(cm.Correct()) / float64(total)
}

// UnsafeDowngrade returns the count and fraction of unsafe downgrades.
// Unsafe = FRONTIER predicted as SMALL or MID.
type UnsafeDowngrade struct {
	FrontierToSmall int
	FrontierToMid   int
	Total           int
}

// UnsafeDowngrades computes unsafe downgrade statistics.
func (cm *ConfusionMatrix) UnsafeDowngrades() UnsafeDowngrade {
	frontierToSmall := cm.Matrix[core.TierFrontier][core.TierSmall]
	frontierToMid := cm.Matrix[core.TierFrontier][core.TierMid]
	return UnsafeDowngrade{
		FrontierToSmall: frontierToSmall,
		FrontierToMid:   frontierToMid,
		Total:           frontierToSmall + frontierToMid,
	}
}

// UnsafeDowngradeRate returns the fraction of FRONTIER examples incorrectly downgraded.
func (cm *ConfusionMatrix) UnsafeDowngradeRate() float64 {
	frontierTotal := cm.Matrix[core.TierFrontier][0] +
		cm.Matrix[core.TierFrontier][1] +
		cm.Matrix[core.TierFrontier][2]
	if frontierTotal == 0 {
		return 0
	}
	unsafe := cm.UnsafeDowngrades()
	return float64(unsafe.Total) / float64(frontierTotal)
}

// WastefulOverrouting returns the count of unnecessary escalations.
// Over-routing = SMALL predicted as MID/FRONTIER, or MID predicted as FRONTIER.
type OverRouting struct {
	SmallToMid       int
	SmallToFrontier  int
	MidToFrontier    int
	Total            int
}

// OverRouting computes over-routing statistics.
func (cm *ConfusionMatrix) OverRouting() OverRouting {
	smallToMid := cm.Matrix[core.TierSmall][core.TierMid]
	smallToFrontier := cm.Matrix[core.TierSmall][core.TierFrontier]
	midToFrontier := cm.Matrix[core.TierMid][core.TierFrontier]
	return OverRouting{
		SmallToMid:      smallToMid,
		SmallToFrontier: smallToFrontier,
		MidToFrontier:   midToFrontier,
		Total:           smallToMid + smallToFrontier + midToFrontier,
	}
}

// OverRoutingRate returns the fraction of examples over-routed.
func (cm *ConfusionMatrix) OverRoutingRate() float64 {
	total := cm.Total()
	if total == 0 {
		return 0
	}
	over := cm.OverRouting()
	return float64(over.Total) / float64(total)
}

// String returns a formatted confusion matrix for display.
func (cm *ConfusionMatrix) String() string {
	var sb strings.Builder
	sb.WriteString("Confusion Matrix (actual \\ predicted):\n")
	sb.WriteString("             SMALL   MID  FRONTIER\n")
	tierNames := []string{"SMALL   ", "MID     ", "FRONTIER"}
	for i := 0; i < 3; i++ {
		sb.WriteString(fmt.Sprintf("%s     %3d   %3d      %3d\n",
			tierNames[i], cm.Matrix[i][0], cm.Matrix[i][1], cm.Matrix[i][2]))
	}
	return sb.String()
}

// Metrics holds all computed metrics for a model evaluation.
type Metrics struct {
	Accuracy           float64
	UnsafeDowngradeRate float64
	OverRoutingRate    float64
	RiskWeightedLoss   float64
	ConfusionMatrix    ConfusionMatrix
	Unsafe             UnsafeDowngrade
	OverRouting        OverRouting
}

// Compute evaluates predictions against actual labels and computes all metrics.
func Compute(actual, predicted []core.Tier) Metrics {
	if len(actual) != len(predicted) {
		return Metrics{}
	}

	var cm ConfusionMatrix
	var totalLoss float64

	for i := range actual {
		cm.Add(actual[i], predicted[i])
		totalLoss += riskWeight(actual[i], predicted[i])
	}

	avgLoss := 0.0
	if len(actual) > 0 {
		avgLoss = totalLoss / float64(len(actual))
	}

	return Metrics{
		Accuracy:           cm.Accuracy(),
		UnsafeDowngradeRate: cm.UnsafeDowngradeRate(),
		OverRoutingRate:    cm.OverRoutingRate(),
		RiskWeightedLoss:   avgLoss,
		ConfusionMatrix:    cm,
		Unsafe:             cm.UnsafeDowngrades(),
		OverRouting:        cm.OverRouting(),
	}
}

// riskWeight returns the cost of predicting `pred` when the true label is `actual`.
// Higher weight for unsafe downgrades (FRONTIER → SMALL is catastrophic).
func riskWeight(actual, pred core.Tier) float64 {
	if actual == pred {
		return 0 // No cost for correct
	}

	// Costs from design doc
	switch {
	case actual == core.TierFrontier && pred == core.TierSmall:
		return 10.0 // Catastrophic under-routing
	case actual == core.TierFrontier && pred == core.TierMid:
		return 5.0 // Expensive under-routing
	case actual == core.TierMid && pred == core.TierSmall:
		return 2.0 // Moderate under-routing
	case actual == core.TierSmall && pred == core.TierMid:
		return 0.5 // Cheap over-routing
	case actual == core.TierSmall && pred == core.TierFrontier:
		return 1.0 // Over-routing but safe
	case actual == core.TierMid && pred == core.TierFrontier:
		return 0.5 // Over-routing but safe
	default:
		return 1.0
	}
}

// Summary returns a formatted summary of the metrics.
func (m Metrics) Summary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tier accuracy:        %.1f%%\n", m.Accuracy*100))
	sb.WriteString(fmt.Sprintf("Unsafe downgrade:     %.1f%%\n", m.UnsafeDowngradeRate*100))
	sb.WriteString(fmt.Sprintf("  FRONTIER → SMALL:   %d\n", m.Unsafe.FrontierToSmall))
	sb.WriteString(fmt.Sprintf("  FRONTIER → MID:     %d\n", m.Unsafe.FrontierToMid))
	sb.WriteString(fmt.Sprintf("Over-routing:         %.1f%%\n", m.OverRoutingRate*100))
	sb.WriteString(fmt.Sprintf("Risk-weighted loss:   %.3f\n", m.RiskWeightedLoss))
	sb.WriteString("\n")
	sb.WriteString(m.ConfusionMatrix.String())
	return sb.String()
}
