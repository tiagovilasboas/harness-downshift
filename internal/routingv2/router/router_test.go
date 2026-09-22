// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package router

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// mockResolver is a minimal Resolver implementation for testing.
type mockResolver struct {
	models       map[string]map[core.Tier]core.Model
	lookupByID   map[string]core.Model
	explicitOnly map[string]bool
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		models: map[string]map[core.Tier]core.Model{
			"claude-code": {
				core.TierSmall:    {ID: "haiku", Tier: core.TierSmall, InputM: 0.25, OutputM: 1.25, Harness: "claude-code"},
				core.TierMid:      {ID: "sonnet", Tier: core.TierMid, InputM: 3.0, OutputM: 15.0, Harness: "claude-code"},
				core.TierFrontier: {ID: "opus", Tier: core.TierFrontier, InputM: 15.0, OutputM: 75.0, Harness: "claude-code"},
			},
		},
		lookupByID: map[string]core.Model{
			"haiku":  {ID: "haiku", Tier: core.TierSmall, InputM: 0.25, OutputM: 1.25, Harness: "claude-code"},
			"sonnet": {ID: "sonnet", Tier: core.TierMid, InputM: 3.0, OutputM: 15.0, Harness: "claude-code"},
			"opus":   {ID: "opus", Tier: core.TierFrontier, InputM: 15.0, OutputM: 75.0, Harness: "claude-code"},
		},
		explicitOnly: map[string]bool{},
	}
}

func (m *mockResolver) ModelFor(harness string, tier core.Tier) core.Model {
	if h, ok := m.models[harness]; ok {
		if model, ok := h[tier]; ok {
			return model
		}
	}
	// Fallback
	return core.Model{ID: "fallback", Tier: tier, Harness: harness}
}

func (m *mockResolver) LookupByID(harness, modelID string) (core.Model, bool) {
	model, ok := m.lookupByID[modelID]
	return model, ok
}

func (m *mockResolver) SavingsRatio(from, to core.Model) float64 {
	if from.InputM == 0 || to.InputM >= from.InputM {
		return 0
	}
	return 1 - (to.InputM / from.InputM)
}

func (m *mockResolver) EffortFor(harness, modelID string, effort core.Effort) string {
	return effort.String()
}

func (m *mockResolver) IsExplicitOnly(harness, modelID string) bool {
	return m.explicitOnly[modelID]
}

// mockClassifier returns fixed probabilities for testing.
type mockClassifier struct {
	probs domain.TierProbabilities
	err   error
}

func (m *mockClassifier) Classify(fv domain.FeatureVector) (domain.TierProbabilities, error) {
	return m.probs, m.err
}

func TestRouter_Route_MechanicalTask(t *testing.T) {
	// Mechanical tasks (rename, format) should route to Small
	r := New()
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:         "rename the variable userName to username",
		Harness:        "claude-code",
		CurrentModelID: "opus",
		Resolver:       resolver,
	})

	// Should recommend Small tier for mechanical work
	if decision.Tier != core.TierSmall {
		t.Errorf("expected TierSmall for mechanical task, got %v", decision.Tier)
	}

	// Verdict should be downshift (from opus to haiku)
	if decision.Verdict != core.VerdictDownshift {
		t.Errorf("expected VerdictDownshift, got %v", decision.Verdict)
	}

	// Should have positive savings
	if decision.Savings <= 0 {
		t.Errorf("expected positive savings, got %v", decision.Savings)
	}

	// Model should be haiku
	if decision.Model.ID != "haiku" {
		t.Errorf("expected model haiku, got %v", decision.Model.ID)
	}
}

func TestRouter_Route_SecurityTask_SafetyFloor(t *testing.T) {
	// Security tasks should trigger safety floor to Frontier
	r := New()
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:         "implement JWT authentication with secure token rotation and RBAC",
		Harness:        "claude-code",
		CurrentModelID: "haiku",
		Resolver:       resolver,
	})

	// Safety rules should enforce Frontier for security tasks
	if decision.Tier != core.TierFrontier {
		t.Errorf("expected TierFrontier for security task, got %v", decision.Tier)
	}

	// Safety constraint should be populated
	if decision.Safety.MinTier == core.TierSmall {
		t.Errorf("expected safety constraint for security task")
	}

	// Verdict should be upshift (from haiku to opus)
	if decision.Verdict != core.VerdictUpshift {
		t.Errorf("expected VerdictUpshift, got %v", decision.Verdict)
	}
}

func TestRouter_Route_UnknownCurrentModel(t *testing.T) {
	r := New()
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:         "add a unit test",
		Harness:        "claude-code",
		CurrentModelID: "", // No current model
		Resolver:       resolver,
	})

	// Should still route successfully
	if decision.Model.ID == "" {
		t.Error("expected a model recommendation")
	}

	// Verdict should be unknown when no current model
	if decision.Verdict != core.VerdictUnknown {
		t.Errorf("expected VerdictUnknown, got %v", decision.Verdict)
	}
}

func TestRouter_Route_VerdictOK(t *testing.T) {
	// Use mock classifier to ensure Small tier prediction
	mock := &mockClassifier{
		probs: domain.TierProbabilities{Small: 0.9, Mid: 0.05, Frontier: 0.05},
	}
	r := NewWithClassifier(mock)
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:         "format this file",
		Harness:        "claude-code",
		CurrentModelID: "haiku", // Already on small tier
		Resolver:       resolver,
	})

	// With mock classifier returning Small as highest, tier should be Small
	if decision.Tier != core.TierSmall {
		t.Errorf("expected TierSmall, got %v", decision.Tier)
	}

	if decision.Verdict != core.VerdictOK {
		t.Errorf("expected VerdictOK, got %v", decision.Verdict)
	}

	if decision.Savings != 0 {
		t.Errorf("expected zero savings for VerdictOK, got %v", decision.Savings)
	}
}

func TestRouter_Route_EffortMatchesTier(t *testing.T) {
	r := New()
	resolver := newMockResolver()

	tests := []struct {
		prompt       string
		expectedTier core.Tier
		expectedEff  core.Effort
	}{
		{"rename x to y", core.TierSmall, core.EffortLow},
		{"implement a simple unit test", core.TierSmall, core.EffortLow}, // May vary
	}

	for _, tc := range tests {
		decision := r.Route(domain.RoutingInput{
			Prompt:   tc.prompt,
			Harness:  "claude-code",
			Resolver: resolver,
		})

		// Effort should match tier
		expected := core.EffortFor(decision.Tier)
		if decision.Effort != expected {
			t.Errorf("prompt %q: effort %v should match EffortFor(%v)=%v",
				tc.prompt, decision.Effort, decision.Tier, expected)
		}
	}
}

func TestRouter_WithMockClassifier(t *testing.T) {
	// Test that we can inject a custom classifier
	mock := &mockClassifier{
		probs: domain.TierProbabilities{Small: 0.1, Mid: 0.2, Frontier: 0.7},
	}
	r := NewWithClassifier(mock)
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:   "any prompt",
		Harness:  "claude-code",
		Resolver: resolver,
	})

	// The mock always returns Frontier as highest probability
	// (unless safety rules override)
	if decision.Probabilities.Frontier != 0.7 {
		t.Errorf("expected Frontier probability 0.7, got %v", decision.Probabilities.Frontier)
	}
}

func TestRouter_FeaturesPopulated(t *testing.T) {
	r := New()
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:   "implement authentication with JWT tokens",
		Harness:  "claude-code",
		Resolver: resolver,
	})

	// Features should be extracted and populated
	if decision.Features.Security == 0 {
		t.Error("expected Security feature to be non-zero for auth prompt")
	}

	// Coding should also be detected
	if decision.Features.Coding == 0 {
		t.Error("expected Coding feature to be non-zero for implementation prompt")
	}
}

func TestRouter_ConfidencePopulated(t *testing.T) {
	r := New()
	resolver := newMockResolver()

	decision := r.Route(domain.RoutingInput{
		Prompt:   "rename foo to bar",
		Harness:  "claude-code",
		Resolver: resolver,
	})

	// Confidence should be computed from probabilities
	expectedConfidence := decision.Probabilities.Confidence()
	if decision.Confidence != expectedConfidence {
		t.Errorf("expected confidence %v, got %v", expectedConfidence, decision.Confidence)
	}
}

func TestRouteSimple(t *testing.T) {
	resolver := newMockResolver()

	decision := RouteSimple("format the code", "claude-code", "opus", resolver)

	// Should work without creating a router manually
	if decision.Model.ID == "" {
		t.Error("expected model recommendation from RouteSimple")
	}
}

func TestRouter_Route_NilResolver(t *testing.T) {
	r := New()

	// Should not panic with nil resolver
	decision := r.Route(domain.RoutingInput{
		Prompt:  "any prompt",
		Harness: "claude-code",
		// Resolver: nil
	})

	// Should still have a tier decision
	// Model ID may be empty (matcher returns fallback)
	if decision.Tier < core.TierSmall || decision.Tier > core.TierFrontier {
		t.Errorf("unexpected tier %v", decision.Tier)
	}

	// Verdict should be unknown without resolver
	if decision.Verdict != core.VerdictUnknown {
		t.Errorf("expected VerdictUnknown without resolver, got %v", decision.Verdict)
	}
}
