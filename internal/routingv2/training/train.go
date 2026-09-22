// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"math"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/classifier"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// TrainConfig holds hyperparameters for training.
type TrainConfig struct {
	Epochs       int     // Number of training iterations
	LearningRate float64 // Step size for gradient descent
	RiskWeighted bool    // Use risk-weighted loss (asymmetric)
}

// DefaultTrainConfig returns sensible defaults for training.
func DefaultTrainConfig() TrainConfig {
	return TrainConfig{
		Epochs:       100,
		LearningRate: 0.1,
		RiskWeighted: true,
	}
}

// TrainResult holds the output of training.
type TrainResult struct {
	Weights      *classifier.Weights
	TrainMetrics Metrics
	ValMetrics   Metrics
	FinalLoss    float64
}

// Train optimizes classifier weights using gradient descent on the dataset.
// Uses risk-weighted loss to penalize unsafe downgrades heavily.
func Train(train, val *Dataset, config TrainConfig) TrainResult {
	// Start from default weights
	weights := classifier.DefaultWeights()

	// Training loop
	for epoch := 0; epoch < config.Epochs; epoch++ {
		// Compute gradients and update weights
		weights = sgdStep(weights, train, config.LearningRate, config.RiskWeighted)
	}

	// Evaluate on both sets
	clf := classifier.NewSoftmaxClassifier(weights)
	trainMetrics := evaluate(clf, train)
	valMetrics := evaluate(clf, val)

	return TrainResult{
		Weights:      weights,
		TrainMetrics: trainMetrics,
		ValMetrics:   valMetrics,
		FinalLoss:    trainMetrics.RiskWeightedLoss,
	}
}

// sgdStep performs one epoch of stochastic gradient descent.
func sgdStep(w *classifier.Weights, ds *Dataset, lr float64, riskWeighted bool) *classifier.Weights {
	// Initialize gradient accumulators
	gradSmall := make([]float64, domain.NumFeatures+1) // +1 for bias
	gradMid := make([]float64, domain.NumFeatures+1)
	gradFrontier := make([]float64, domain.NumFeatures+1)

	// Process each example
	for _, ex := range ds.Examples {
		features := ex.Features.AsSlice()
		target := int(ex.Tier())

		// Forward pass: compute probabilities
		probs := softmax(
			dot(w.Small.W[:], features)+w.Small.Bias,
			dot(w.Mid.W[:], features)+w.Mid.Bias,
			dot(w.Frontier.W[:], features)+w.Frontier.Bias,
		)

		// Risk weight for this example
		rw := 1.0
		if riskWeighted {
			rw = riskWeightForGradient(ex.Tier(), maxTier(probs))
		}

		// Compute gradients: dL/dw = rw * (p - y) * x
		for k := 0; k < 3; k++ {
			targetK := 0.0
			if k == target {
				targetK = 1.0
			}
			error := rw * (probs[k] - targetK)

			grad := gradientFor(k, gradSmall, gradMid, gradFrontier)
			for i, f := range features {
				grad[i] += error * f
			}
			grad[domain.NumFeatures] += error // bias
		}
	}

	// Average gradients
	n := float64(len(ds.Examples))
	if n == 0 {
		return w
	}
	for i := range gradSmall {
		gradSmall[i] /= n
		gradMid[i] /= n
		gradFrontier[i] /= n
	}

	// Update weights
	newWeights := &classifier.Weights{
		Version: w.Version,
	}

	// Small
	for i := 0; i < domain.NumFeatures; i++ {
		newWeights.Small.W[i] = w.Small.W[i] - lr*gradSmall[i]
	}
	newWeights.Small.Bias = w.Small.Bias - lr*gradSmall[domain.NumFeatures]

	// Mid
	for i := 0; i < domain.NumFeatures; i++ {
		newWeights.Mid.W[i] = w.Mid.W[i] - lr*gradMid[i]
	}
	newWeights.Mid.Bias = w.Mid.Bias - lr*gradMid[domain.NumFeatures]

	// Frontier
	for i := 0; i < domain.NumFeatures; i++ {
		newWeights.Frontier.W[i] = w.Frontier.W[i] - lr*gradFrontier[i]
	}
	newWeights.Frontier.Bias = w.Frontier.Bias - lr*gradFrontier[domain.NumFeatures]

	return newWeights
}

// gradientFor returns the gradient slice for the given tier index.
func gradientFor(tier int, small, mid, frontier []float64) []float64 {
	switch core.Tier(tier) {
	case core.TierSmall:
		return small
	case core.TierMid:
		return mid
	default:
		return frontier
	}
}

// riskWeightForGradient returns gradient scaling based on the type of error.
// Higher weight = more gradient = model learns harder from that mistake.
func riskWeightForGradient(actual, predicted core.Tier) float64 {
	if actual == predicted {
		return 1.0
	}

	switch {
	case actual == core.TierFrontier && predicted == core.TierSmall:
		return 10.0 // Catastrophic
	case actual == core.TierFrontier && predicted == core.TierMid:
		return 5.0 // Expensive
	case actual == core.TierMid && predicted == core.TierSmall:
		return 2.0 // Moderate
	default:
		return 1.0 // Over-routing or correct
	}
}

// evaluate runs the classifier on a dataset and computes metrics.
func evaluate(clf *classifier.SoftmaxClassifier, ds *Dataset) Metrics {
	actual := make([]core.Tier, len(ds.Examples))
	predicted := make([]core.Tier, len(ds.Examples))

	for i, ex := range ds.Examples {
		actual[i] = ex.Tier()

		probs, _ := clf.Classify(ex.Features)
		predicted[i] = probs.MaxTier()
	}

	return Compute(actual, predicted)
}

// Helper functions

func dot(a, b []float64) float64 {
	sum := 0.0
	for i := 0; i < len(a) && i < len(b); i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func softmax(scores ...float64) []float64 {
	if len(scores) == 0 {
		return nil
	}

	// Find max for numerical stability
	maxScore := scores[0]
	for _, s := range scores[1:] {
		if s > maxScore {
			maxScore = s
		}
	}

	// Compute exp(score - max)
	exps := make([]float64, len(scores))
	sum := 0.0
	for i, s := range scores {
		exps[i] = math.Exp(s - maxScore)
		sum += exps[i]
	}

	// Normalize
	probs := make([]float64, len(scores))
	for i := range exps {
		probs[i] = exps[i] / sum
	}

	return probs
}

func maxTier(probs []float64) core.Tier {
	if len(probs) < 3 {
		return core.TierMid
	}
	if probs[2] >= probs[1] && probs[2] >= probs[0] {
		return core.TierFrontier
	}
	if probs[1] >= probs[0] {
		return core.TierMid
	}
	return core.TierSmall
}
