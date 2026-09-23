// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package classifier

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

//go:embed weights/default.json
var embeddedWeights embed.FS

// Weights holds the trained parameters for the softmax classifier.
// Structure: weights[tier][feature] + bias[tier]
type Weights struct {
	Version  string      `json:"version"`
	Small    TierWeights `json:"small"`
	Mid      TierWeights `json:"mid"`
	Frontier TierWeights `json:"frontier"`
}

// TierWeights holds weights and bias for one tier.
type TierWeights struct {
	// Weights for each of the 13 features in canonical order
	W    [domain.NumFeatures]float64 `json:"w"`
	Bias float64                     `json:"bias"`
}

// LoadWeights loads weights with the following precedence:
// 1. User override at ~/.harness-downshift/weights.json
// 2. Embedded default weights
func LoadWeights() (*Weights, error) {
	// Try user override first
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".harness-downshift", "weights.json")
		if data, err := os.ReadFile(userPath); err == nil {
			var w Weights
			if err := json.Unmarshal(data, &w); err == nil && w.Validate() == nil {
				return &w, nil
			}
		}
	}

	// Fall back to embedded
	return LoadEmbeddedWeights()
}

// LoadEmbeddedWeights loads only the embedded default weights.
func LoadEmbeddedWeights() (*Weights, error) {
	data, err := embeddedWeights.ReadFile("weights/default.json")
	if err != nil {
		return nil, err
	}

	var w Weights
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}

// LoadWeightsFromFile loads weights from a specific path.
func LoadWeightsFromFile(path string) (*Weights, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var w Weights
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}

// Validate rejects non-finite or extreme coefficients before they can turn
// classifier scores into NaN/Inf and silently corrupt routing decisions.
func (w *Weights) Validate() error {
	if w == nil {
		return fmt.Errorf("weights are nil")
	}
	for tierName, tier := range map[string]TierWeights{"small": w.Small, "mid": w.Mid, "frontier": w.Frontier} {
		if math.IsNaN(tier.Bias) || math.IsInf(tier.Bias, 0) || math.Abs(tier.Bias) > 1e6 {
			return fmt.Errorf("invalid %s bias", tierName)
		}
		for i, value := range tier.W {
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 1e6 {
				return fmt.Errorf("invalid %s weight at feature %d", tierName, i)
			}
		}
	}
	return nil
}

// SaveWeights saves weights to a file with current timestamp as version.
func SaveWeights(w *Weights, path string) error {
	if err := w.Validate(); err != nil {
		return err
	}
	w.Version = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".weights-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// DefaultWeights returns reasonable initial weights for cold start.
// These are tuned to produce sensible behavior before any training.
func DefaultWeights() *Weights {
	return &Weights{
		Version: "default-v1",
		Small: TierWeights{
			W: [domain.NumFeatures]float64{
				1.5,  // mechanical → small
				-0.2, // coding
				-0.5, // debugging
				-0.3, // refactoring
				-1.0, // architecture
				-1.5, // migration
				-1.5, // security
				-1.5, // concurrency
				-0.5, // planning
				0.2,  // tool_use
				-0.3, // ambiguity
				-0.8, // cross_module
				-0.5, // context_size
			},
			Bias: 0.5,
		},
		Mid: TierWeights{
			W: [domain.NumFeatures]float64{
				-0.5, // mechanical
				0.5,  // coding → mid
				0.4,  // debugging
				0.5,  // refactoring → mid
				0.3,  // architecture
				0.2,  // migration
				0.3,  // security
				0.2,  // concurrency
				0.4,  // planning
				0.3,  // tool_use
				0.2,  // ambiguity
				0.3,  // cross_module
				0.3,  // context_size
			},
			Bias: 0.0,
		},
		Frontier: TierWeights{
			W: [domain.NumFeatures]float64{
				-1.0, // mechanical
				0.2,  // coding
				0.5,  // debugging
				0.3,  // refactoring
				1.0,  // architecture → frontier
				1.2,  // migration → frontier
				1.2,  // security → frontier
				1.2,  // concurrency → frontier
				0.6,  // planning
				0.3,  // tool_use
				0.5,  // ambiguity
				0.8,  // cross_module → frontier
				0.6,  // context_size
			},
			Bias: -0.5,
		},
	}
}
