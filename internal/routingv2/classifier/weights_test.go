// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package classifier

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestWeightsValidateRejectsNonFiniteAndExtremeValues(t *testing.T) {
	for name, mutate := range map[string]func(*Weights){
		"nan":      func(w *Weights) { w.Small.Bias = math.NaN() },
		"infinity": func(w *Weights) { w.Mid.W[0] = math.Inf(1) },
		"extreme":  func(w *Weights) { w.Frontier.W[0] = 1e9 },
	} {
		t.Run(name, func(t *testing.T) {
			weights := DefaultWeights()
			mutate(weights)
			if err := weights.Validate(); err == nil {
				t.Fatal("Validate() succeeded for invalid weights")
			}
		})
	}
}

func TestLoadWeightsFromFileRejectsExtremeCandidate(t *testing.T) {
	weights := DefaultWeights()
	weights.Small.W[0] = 1e9
	data, err := json.Marshal(weights)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWeightsFromFile(path); err == nil {
		t.Fatal("invalid candidate weights were accepted")
	}
}

func TestSaveWeightsUsesPrivateAtomicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weights.json")
	if err := SaveWeights(DefaultWeights(), path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %04o, want 0600", info.Mode().Perm())
	}
	if _, err := LoadWeightsFromFile(path); err != nil {
		t.Fatalf("saved weights do not load: %v", err)
	}
}
