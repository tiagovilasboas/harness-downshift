// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package safety

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

func TestEvaluate_NoConstraint(t *testing.T) {
	ev := NewEvaluator()

	// Mechanical task with no risk signals
	fv := domain.FeatureVector{
		Mechanical: 0.8,
		Coding:     0.2,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierSmall {
		t.Errorf("expected TierSmall, got %v", c.MinTier)
	}
	if c.Trigger != "" {
		t.Errorf("expected no trigger, got %q", c.Trigger)
	}
}

func TestEvaluate_SecurityHigh(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Security: 0.8,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierFrontier {
		t.Errorf("expected TierFrontier for high security, got %v", c.MinTier)
	}
	if c.Trigger != "security_high" {
		t.Errorf("expected trigger 'security_high', got %q", c.Trigger)
	}
}

func TestEvaluate_SecurityModerate(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Security: 0.5, // between 0.4 and 0.7
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierMid {
		t.Errorf("expected TierMid for moderate security, got %v", c.MinTier)
	}
	if c.Trigger != "security_moderate" {
		t.Errorf("expected trigger 'security_moderate', got %q", c.Trigger)
	}
}

func TestEvaluate_MigrationData(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Migration: 0.7,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierFrontier {
		t.Errorf("expected TierFrontier for migration, got %v", c.MinTier)
	}
	if c.Trigger != "migration_data" {
		t.Errorf("expected trigger 'migration_data', got %q", c.Trigger)
	}
}

func TestEvaluate_ConcurrencyComplex(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Concurrency: 0.7,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierFrontier {
		t.Errorf("expected TierFrontier for concurrency, got %v", c.MinTier)
	}
	if c.Trigger != "concurrency_complex" {
		t.Errorf("expected trigger 'concurrency_complex', got %q", c.Trigger)
	}
}

func TestEvaluate_ArchitectureCrossModule(t *testing.T) {
	ev := NewEvaluator()

	// Both thresholds must be met
	fv := domain.FeatureVector{
		Architecture: 0.8,
		CrossModule:  0.6,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierFrontier {
		t.Errorf("expected TierFrontier for arch+cross_module, got %v", c.MinTier)
	}
	if c.Trigger != "architecture_cross_module" {
		t.Errorf("expected trigger 'architecture_cross_module', got %q", c.Trigger)
	}
}

func TestEvaluate_ArchitectureAlone(t *testing.T) {
	ev := NewEvaluator()

	// Architecture high but cross_module low — should not trigger
	fv := domain.FeatureVector{
		Architecture: 0.9,
		CrossModule:  0.2,
	}

	c := ev.Evaluate(fv)
	if c.MinTier == core.TierFrontier {
		t.Errorf("architecture alone should not force Frontier")
	}
}

func TestEvaluate_AmbiguityHigh(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Ambiguity: 0.9,
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierMid {
		t.Errorf("expected TierMid for high ambiguity, got %v", c.MinTier)
	}
}

func TestEvaluate_HighestWins(t *testing.T) {
	ev := NewEvaluator()

	// Both security_moderate (Mid) and concurrency_complex (Frontier) match
	fv := domain.FeatureVector{
		Security:    0.5, // triggers Mid
		Concurrency: 0.7, // triggers Frontier
	}

	c := ev.Evaluate(fv)
	if c.MinTier != core.TierFrontier {
		t.Errorf("highest constraint should win, got %v", c.MinTier)
	}
	if c.Trigger != "concurrency_complex" {
		t.Errorf("expected Frontier trigger, got %q", c.Trigger)
	}
}

func TestEvaluateAll(t *testing.T) {
	ev := NewEvaluator()

	fv := domain.FeatureVector{
		Security:    0.5,
		Concurrency: 0.7,
	}

	matched := ev.EvaluateAll(fv)
	if len(matched) < 2 {
		t.Errorf("expected at least 2 rules to match, got %d", len(matched))
	}
}

func TestCustomRules(t *testing.T) {
	custom := []Rule{
		{
			Name:    "test_rule",
			Check:   func(fv domain.FeatureVector) bool { return fv.Coding > 0.5 },
			MinTier: core.TierMid,
			Reason:  "test",
		},
	}

	ev := NewEvaluatorWithRules(custom)
	fv := domain.FeatureVector{Coding: 0.6}

	c := ev.Evaluate(fv)
	if c.Trigger != "test_rule" {
		t.Errorf("expected custom rule to trigger, got %q", c.Trigger)
	}
}
