// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package router implements the capability-based routing v2 pipeline.
// It orchestrates feature extraction, safety evaluation, classification,
// policy decision, and model matching into a single Route call.
//
// The router is isolated and model-agnostic: it knows about tiers and
// capabilities, never about model names or providers. Model selection
// is delegated to the matcher via the Resolver interface.
package router

import (
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/classifier"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/extractor"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/matcher"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/policy"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/safety"
)

// Classifier is the interface for tier classification.
// This abstraction allows swapping the algorithm (softmax, decision tree, etc.)
// without changing the router.
type Classifier interface {
	Classify(fv domain.FeatureVector) (domain.TierProbabilities, error)
}

// Router is the capability-based routing v2 implementation.
// Zero value is not usable; use New or NewWithClassifier.
type Router struct {
	classifier Classifier
	safety     *safety.Evaluator
}

// New creates a Router with the default softmax classifier and safety rules.
// This is the production constructor.
func New() *Router {
	return &Router{
		classifier: classifier.NewSoftmaxClassifierDefault(),
		safety:     safety.NewEvaluator(),
	}
}

// NewWithClassifier creates a Router with a custom classifier (for testing/experiments).
func NewWithClassifier(c Classifier) *Router {
	return &Router{
		classifier: c,
		safety:     safety.NewEvaluator(),
	}
}

// Route executes the full v2 routing pipeline:
//
//  1. Extract features from prompt → FeatureVector
//  2. Evaluate safety rules → SafetyConstraint (tier floor)
//  3. Classify features → TierProbabilities
//  4. Apply policy → target Tier (considering confidence + safety)
//  5. Match model → cheapest model that satisfies tier
//  6. Compute verdict and savings vs current model
//
// The router is fail-open: errors in classification default to TierMid.
func (r *Router) Route(input domain.RoutingInput) domain.RoutingDecision {
	// Step 1: Extract features
	features := extractor.Extract(input.Prompt)

	// Step 2: Evaluate safety rules (tier floor)
	safetyConstraint := r.safety.Evaluate(features)

	// Step 3: Classify to get tier probabilities
	probs, err := r.classifier.Classify(features)
	if err != nil {
		// Fail-open: default to Mid tier with low confidence
		probs = domain.TierProbabilities{Small: 0.1, Mid: 0.8, Frontier: 0.1}
	}

	// Step 4: Apply policy (classifier + confidence + safety floor)
	tier := policy.Decide(probs, safetyConstraint)

	// Step 5: Match the cheapest model for this tier
	var model core.Model
	if input.Resolver != nil {
		model = matcher.Match(input.Harness, tier, input.Resolver)
	} else {
		// No resolver — return a placeholder model with tier info only
		model = core.Model{Tier: tier, Harness: input.Harness}
	}

	// Step 6: Determine verdict and savings
	verdict, savings, currentModel := r.compareModels(input, model)

	// Step 7: Compute effort for the tier
	effort := core.EffortFor(tier)

	return domain.RoutingDecision{
		Tier:          tier,
		Effort:        effort,
		Model:         model,
		CurrentModel:  currentModel,
		Verdict:       verdict,
		Savings:       savings,
		Confidence:    probs.Confidence(),
		Features:      features,
		Safety:        safetyConstraint,
		Probabilities: probs,
	}
}

// compareModels determines the verdict and savings by comparing
// the recommended model to the current model.
func (r *Router) compareModels(input domain.RoutingInput, recommended core.Model) (core.Verdict, float64, core.Model) {
	if input.Resolver == nil || input.CurrentModelID == "" {
		return core.VerdictUnknown, 0, core.Model{}
	}

	current, known := input.Resolver.LookupByID(input.Harness, input.CurrentModelID)
	if !known {
		return core.VerdictUnknown, 0, core.Model{}
	}

	switch {
	case current.Tier == recommended.Tier:
		return core.VerdictOK, 0, current
	case current.Tier > recommended.Tier:
		// Current is stronger than needed → downshift saves money
		savings := input.Resolver.SavingsRatio(current, recommended)
		return core.VerdictDownshift, savings, current
	default:
		// Current is weaker than needed → upshift for capability
		return core.VerdictUpshift, 0, current
	}
}

// RouteSimple is a convenience function for simple use cases.
// It creates a default router and routes the prompt.
func RouteSimple(prompt, harness, currentModelID string, resolver core.Resolver) domain.RoutingDecision {
	r := New()
	return r.Route(domain.RoutingInput{
		Prompt:         prompt,
		Harness:        harness,
		CurrentModelID: currentModelID,
		Resolver:       resolver,
	})
}
