// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package classifier provides statistical classifiers for tier prediction.
// The default implementation is a multinomial logistic regression with softmax.
package classifier

import (
	"math"

	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// SoftmaxClassifier implements domain.Classifier using multinomial logistic regression.
// It computes: scores[tier] = dot(weights[tier], features) + bias[tier]
// Then applies softmax to get probabilities.
type SoftmaxClassifier struct {
	weights *Weights
}

// NewSoftmaxClassifier creates a classifier with the given weights.
func NewSoftmaxClassifier(w *Weights) *SoftmaxClassifier {
	return &SoftmaxClassifier{weights: w}
}

// NewSoftmaxClassifierDefault creates a classifier with default embedded weights.
// Falls back to DefaultWeights() if embedded loading fails.
func NewSoftmaxClassifierDefault() *SoftmaxClassifier {
	w, err := LoadWeights()
	if err != nil {
		w = DefaultWeights()
	}
	return &SoftmaxClassifier{weights: w}
}

// Classify returns the probability distribution over tiers.
func (c *SoftmaxClassifier) Classify(fv domain.FeatureVector) (domain.TierProbabilities, error) {
	features := fv.AsSlice()

	// Compute raw scores
	scoreSmall := c.score(c.weights.Small, features)
	scoreMid := c.score(c.weights.Mid, features)
	scoreFrontier := c.score(c.weights.Frontier, features)

	// Apply softmax
	probs := softmax(scoreSmall, scoreMid, scoreFrontier)

	return domain.TierProbabilities{
		Small:    probs[0],
		Mid:      probs[1],
		Frontier: probs[2],
	}, nil
}

// score computes dot(w, features) + bias
func (c *SoftmaxClassifier) score(tw TierWeights, features []float64) float64 {
	sum := tw.Bias
	for i := 0; i < len(features) && i < len(tw.W); i++ {
		sum += tw.W[i] * features[i]
	}
	return sum
}

// softmax converts raw scores to probabilities that sum to 1.
// Uses the log-sum-exp trick for numerical stability.
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

// Weights returns the current weights (for inspection/serialization).
func (c *SoftmaxClassifier) Weights() *Weights {
	return c.weights
}
