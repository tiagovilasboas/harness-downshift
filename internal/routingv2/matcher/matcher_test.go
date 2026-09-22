// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package matcher

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// mockResolver implements core.Resolver for testing
type mockResolver struct {
	models       map[core.Tier]core.Model
	explicitOnly map[string]bool
}

func (m *mockResolver) ModelFor(harness string, tier core.Tier) core.Model {
	if model, ok := m.models[tier]; ok {
		return model
	}
	return core.Model{ID: "fallback", Tier: tier, Harness: harness}
}

func (m *mockResolver) LookupByID(harness, modelID string) (core.Model, bool) {
	for _, model := range m.models {
		if model.ID == modelID {
			return model, true
		}
	}
	return core.Model{}, false
}

func (m *mockResolver) SavingsRatio(from, to core.Model) float64 {
	if from.InputM+from.OutputM == 0 {
		return 0
	}
	fromCost := from.InputM + from.OutputM
	toCost := to.InputM + to.OutputM
	if toCost >= fromCost {
		return 0
	}
	return (fromCost - toCost) / fromCost
}

func (m *mockResolver) EffortFor(harness, modelID string, effort core.Effort) string {
	return effort.String()
}

func (m *mockResolver) IsExplicitOnly(harness, modelID string) bool {
	return m.explicitOnly[modelID]
}

func TestMatch_ReturnsCorrectTier(t *testing.T) {
	resolver := &mockResolver{
		models: map[core.Tier]core.Model{
			core.TierSmall:    {ID: "small-model", Tier: core.TierSmall, InputM: 0.1, OutputM: 0.4},
			core.TierMid:      {ID: "mid-model", Tier: core.TierMid, InputM: 1.0, OutputM: 4.0},
			core.TierFrontier: {ID: "frontier-model", Tier: core.TierFrontier, InputM: 10.0, OutputM: 30.0},
		},
		explicitOnly: map[string]bool{},
	}

	tests := []struct {
		tier   core.Tier
		wantID string
	}{
		{core.TierSmall, "small-model"},
		{core.TierMid, "mid-model"},
		{core.TierFrontier, "frontier-model"},
	}

	for _, tt := range tests {
		t.Run(tt.tier.String(), func(t *testing.T) {
			model := Match("test-harness", tt.tier, resolver)
			if model.ID != tt.wantID {
				t.Errorf("Match(%v) = %q, want %q", tt.tier, model.ID, tt.wantID)
			}
		})
	}
}

func TestMatch_SkipsExplicitOnly(t *testing.T) {
	resolver := &mockResolver{
		models: map[core.Tier]core.Model{
			core.TierSmall:    {ID: "small-explicit", Tier: core.TierSmall},
			core.TierMid:      {ID: "mid-model", Tier: core.TierMid},
			core.TierFrontier: {ID: "frontier-model", Tier: core.TierFrontier},
		},
		explicitOnly: map[string]bool{"small-explicit": true},
	}

	model := Match("test", core.TierSmall, resolver)
	// Should fall back to Mid since Small is explicit_only
	if model.ID != "mid-model" {
		t.Errorf("expected fallback to mid-model, got %q", model.ID)
	}
}

func TestMatchWithProfile_SelectsCheapest(t *testing.T) {
	profiles := []domain.ModelProfile{
		{
			ID:           "expensive",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.DefaultCapabilities(core.TierMid),
			InputCost:    10.0,
			OutputCost:   40.0,
		},
		{
			ID:           "cheap",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.DefaultCapabilities(core.TierMid),
			InputCost:    1.0,
			OutputCost:   4.0,
		},
	}

	resolver := &mockResolver{
		models:       map[core.Tier]core.Model{},
		explicitOnly: map[string]bool{},
	}

	model := MatchWithProfile("test", core.TierMid, domain.CapabilityReq{}, profiles, resolver)
	if model.ID != "cheap" {
		t.Errorf("expected cheapest model 'cheap', got %q", model.ID)
	}
}

func TestMatchWithProfile_RespectsCapabilities(t *testing.T) {
	profiles := []domain.ModelProfile{
		{
			ID:           "weak-cheap",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.Capabilities{Security: 0.3},
			InputCost:    1.0,
			OutputCost:   4.0,
		},
		{
			ID:           "strong-expensive",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.Capabilities{Security: 0.9},
			InputCost:    5.0,
			OutputCost:   20.0,
		},
	}

	resolver := &mockResolver{
		models:       map[core.Tier]core.Model{},
		explicitOnly: map[string]bool{},
	}

	// Require high security capability
	req := domain.CapabilityReq{Security: 0.8}
	model := MatchWithProfile("test", core.TierMid, req, profiles, resolver)
	if model.ID != "strong-expensive" {
		t.Errorf("expected 'strong-expensive' (meets security req), got %q", model.ID)
	}
}

func TestMatchWithProfile_ExcludesExplicitOnly(t *testing.T) {
	profiles := []domain.ModelProfile{
		{
			ID:           "explicit",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.DefaultCapabilities(core.TierMid),
			InputCost:    1.0,
			OutputCost:   4.0,
			ExplicitOnly: true,
		},
		{
			ID:           "normal",
			Harness:      "test",
			Tier:         core.TierMid,
			Capabilities: domain.DefaultCapabilities(core.TierMid),
			InputCost:    2.0,
			OutputCost:   8.0,
		},
	}

	resolver := &mockResolver{
		models:       map[core.Tier]core.Model{},
		explicitOnly: map[string]bool{},
	}

	model := MatchWithProfile("test", core.TierMid, domain.CapabilityReq{}, profiles, resolver)
	if model.ID != "normal" {
		t.Errorf("expected 'normal' (explicit excluded), got %q", model.ID)
	}
}

func TestMatchWithProfile_FallsBackToResolver(t *testing.T) {
	resolver := &mockResolver{
		models: map[core.Tier]core.Model{
			core.TierMid: {ID: "resolver-mid", Tier: core.TierMid},
		},
		explicitOnly: map[string]bool{},
	}

	// Empty profiles → fall back to resolver
	model := MatchWithProfile("test", core.TierMid, domain.CapabilityReq{}, nil, resolver)
	if model.ID != "resolver-mid" {
		t.Errorf("expected resolver fallback, got %q", model.ID)
	}
}
