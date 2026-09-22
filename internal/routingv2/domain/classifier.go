// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package domain

// Classifier maps a feature vector to tier probabilities.
// This interface allows swapping the classification algorithm without
// changing the rest of the routing pipeline.
//
// Today: SoftmaxClassifier (logistic linear + softmax)
// Future: XGBoost, ensemble, LLM-based — implement this interface.
type Classifier interface {
	// Classify returns the probability distribution over tiers for the given features.
	// The sum of probabilities should equal 1.0 (softmax output).
	Classify(features FeatureVector) (TierProbabilities, error)
}
