// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package training provides offline training utilities for the capability router.
// It reads datasets, computes weights, and outputs metrics for validation.
//
// The training process is completely offline: no LLM calls, no API usage.
// Features are extracted the same way as runtime to ensure consistency.
package training

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/extractor"
)

// Example represents a single training example.
type Example struct {
	Prompt   string            `json:"prompt"`
	Label    string            `json:"label"`    // SMALL, MID, FRONTIER
	Features domain.FeatureVector `json:"features"` // Optional: if absent, extracted automatically
}

// Tier returns the core.Tier for this example's label.
func (e Example) Tier() core.Tier {
	switch strings.ToUpper(e.Label) {
	case "SMALL":
		return core.TierSmall
	case "MID":
		return core.TierMid
	case "FRONTIER":
		return core.TierFrontier
	default:
		return core.TierMid // Fail-safe
	}
}

// Dataset holds a collection of training examples.
type Dataset struct {
	Examples []Example `json:"examples"`
}

// LoadDataset reads a dataset from a JSON file.
// Supports two formats:
//  1. Array of examples: [{"prompt": "...", "label": "SMALL"}, ...]
//  2. Object with examples field: {"examples": [...]}
func LoadDataset(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading dataset: %w", err)
	}

	// Try object format first
	var ds Dataset
	if err := json.Unmarshal(data, &ds); err == nil && len(ds.Examples) > 0 {
		return extractMissingFeatures(&ds), nil
	}

	// Try array format
	var examples []Example
	if err := json.Unmarshal(data, &examples); err != nil {
		return nil, fmt.Errorf("parsing dataset: %w", err)
	}

	ds = Dataset{Examples: examples}
	return extractMissingFeatures(&ds), nil
}

// extractMissingFeatures ensures all examples have features extracted.
// If features are provided in the dataset, they are used; otherwise extracted from prompt.
func extractMissingFeatures(ds *Dataset) *Dataset {
	for i := range ds.Examples {
		if isZeroFeatures(ds.Examples[i].Features) {
			ds.Examples[i].Features = extractor.Extract(ds.Examples[i].Prompt)
		}
	}
	return ds
}

// isZeroFeatures checks if a feature vector has all zeros (not extracted yet).
func isZeroFeatures(fv domain.FeatureVector) bool {
	for _, v := range fv.AsSlice() {
		if v != 0 {
			return false
		}
	}
	return true
}

// Split divides the dataset into training and validation sets.
// validationRatio should be in (0, 1).
func (ds *Dataset) Split(validationRatio float64) (train, val *Dataset) {
	if validationRatio <= 0 || validationRatio >= 1 {
		return ds, &Dataset{}
	}

	n := len(ds.Examples)
	valSize := int(float64(n) * validationRatio)
	if valSize < 1 {
		valSize = 1
	}
	if valSize >= n {
		valSize = n - 1
	}

	// Simple split (not stratified — could improve later)
	trainExamples := ds.Examples[:n-valSize]
	valExamples := ds.Examples[n-valSize:]

	return &Dataset{Examples: trainExamples}, &Dataset{Examples: valExamples}
}

// Size returns the number of examples in the dataset.
func (ds *Dataset) Size() int {
	return len(ds.Examples)
}

// FeatureMatrix returns the features as a 2D slice for training.
// Each row is an example, each column is a feature.
func (ds *Dataset) FeatureMatrix() [][]float64 {
	matrix := make([][]float64, len(ds.Examples))
	for i, ex := range ds.Examples {
		matrix[i] = ex.Features.AsSlice()
	}
	return matrix
}

// Labels returns the tier labels as integers (0=SMALL, 1=MID, 2=FRONTIER).
func (ds *Dataset) Labels() []int {
	labels := make([]int, len(ds.Examples))
	for i, ex := range ds.Examples {
		labels[i] = int(ex.Tier())
	}
	return labels
}

// LabelDistribution returns the count of examples per tier.
func (ds *Dataset) LabelDistribution() map[core.Tier]int {
	dist := make(map[core.Tier]int)
	for _, ex := range ds.Examples {
		dist[ex.Tier()]++
	}
	return dist
}
