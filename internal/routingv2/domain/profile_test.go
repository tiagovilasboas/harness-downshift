// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package domain

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func TestCapabilities_Satisfies(t *testing.T) {
	tests := []struct {
		name string
		cap  Capabilities
		req  CapabilityReq
		want bool
	}{
		{
			"all exceed",
			Capabilities{0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8},
			CapabilityReq{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5},
			true,
		},
		{
			"exact match",
			Capabilities{0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 0.7},
			CapabilityReq{0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 0.7},
			true,
		},
		{
			"one below",
			Capabilities{0.7, 0.7, 0.7, 0.7, 0.6, 0.7, 0.7},
			CapabilityReq{0.7, 0.7, 0.7, 0.7, 0.7, 0.7, 0.7},
			false,
		},
		{
			"no requirements",
			Capabilities{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
			CapabilityReq{0, 0, 0, 0, 0, 0, 0},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cap.Satisfies(tt.req)
			if got != tt.want {
				t.Errorf("Satisfies() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultCapabilities(t *testing.T) {
	tests := []struct {
		tier core.Tier
		min  float64
		max  float64
	}{
		{core.TierSmall, 0.35, 0.45},
		{core.TierMid, 0.65, 0.75},
		{core.TierFrontier, 0.90, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.tier.String(), func(t *testing.T) {
			cap := DefaultCapabilities(tt.tier)
			if cap.Coding < tt.min || cap.Coding > tt.max {
				t.Errorf("DefaultCapabilities(%v).Coding = %f, expected in [%f, %f]",
					tt.tier, cap.Coding, tt.min, tt.max)
			}
		})
	}
}

func TestModelProfile_TotalCost(t *testing.T) {
	p := ModelProfile{
		InputCost:  3.0,
		OutputCost: 15.0,
	}
	if p.TotalCost() != 18.0 {
		t.Errorf("TotalCost() = %f, want 18.0", p.TotalCost())
	}
}
