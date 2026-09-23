// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func TestExample_Tier(t *testing.T) {
	tests := []struct {
		label string
		want  core.Tier
	}{
		{"SMALL", core.TierSmall},
		{"small", core.TierSmall},
		{"MID", core.TierMid},
		{"Mid", core.TierMid},
		{"FRONTIER", core.TierFrontier},
		{"frontier", core.TierFrontier},
		{"unknown", core.TierMid}, // Tier helper remains fail-safe; loaders validate labels.
	}

	for _, tc := range tests {
		ex := Example{Label: tc.label}
		if got := ex.Tier(); got != tc.want {
			t.Errorf("Example{Label: %q}.Tier() = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func TestLoadDataset_RejectsUnknownLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.json")
	if err := os.WriteFile(path, []byte(`[{"prompt":"task","label":"UNKNOWN"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDataset(path); err == nil || !strings.Contains(err.Error(), "invalid label") {
		t.Fatalf("LoadDataset error = %v, want invalid label error", err)
	}
}

func TestLoadDataset_ArrayFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.json")

	// Array format: [{"prompt": "...", "label": "..."}, ...]
	content := `[
		{"prompt": "rename foo to bar", "label": "SMALL"},
		{"prompt": "implement authentication", "label": "FRONTIER"}
	]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}

	if ds.Size() != 2 {
		t.Errorf("Size = %d, want 2", ds.Size())
	}

	if ds.Examples[0].Label != "SMALL" {
		t.Errorf("Examples[0].Label = %q, want SMALL", ds.Examples[0].Label)
	}
}

func TestLoadDataset_ObjectFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.json")

	// Object format: {"examples": [...]}
	content := `{
		"examples": [
			{"prompt": "fix the typo", "label": "SMALL"},
			{"prompt": "refactor the module", "label": "MID"}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}

	if ds.Size() != 2 {
		t.Errorf("Size = %d, want 2", ds.Size())
	}
}

func TestLoadDataset_ExtractsFeatures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.json")

	// No features provided — should be extracted from prompt
	content := `[{"prompt": "implement JWT authentication", "label": "FRONTIER"}]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("LoadDataset: %v", err)
	}

	// Features should have been extracted
	if isZeroFeatures(ds.Examples[0].Features) {
		t.Error("Features should be extracted from prompt")
	}

	// Security signal should be detected for auth prompt
	if ds.Examples[0].Features.Security == 0 {
		t.Error("Security feature should be non-zero for auth prompt")
	}
}

func TestDataset_Split(t *testing.T) {
	ds := &Dataset{
		Examples: make([]Example, 100),
	}

	train, val := ds.Split(0.2)

	if train.Size() != 80 {
		t.Errorf("Train size = %d, want 80", train.Size())
	}
	if val.Size() != 20 {
		t.Errorf("Val size = %d, want 20", val.Size())
	}
}

func TestDataset_SplitIsStratifiedAndDeterministic(t *testing.T) {
	var examples []Example
	for _, label := range []string{"SMALL", "MID", "FRONTIER"} {
		for i := 0; i < 10; i++ {
			examples = append(examples, Example{Prompt: label + string(rune('A'+i)), Label: label})
		}
	}
	ds := &Dataset{Examples: examples}
	train, val := ds.Split(0.2)
	train2, val2 := ds.Split(0.2)
	if train.Size() != 24 || val.Size() != 6 {
		t.Fatalf("split sizes = %d/%d, want 24/6", train.Size(), val.Size())
	}
	for _, label := range []string{"SMALL", "MID", "FRONTIER"} {
		trainCount, valCount := 0, 0
		for _, ex := range train.Examples {
			if ex.Label == label {
				trainCount++
			}
		}
		for _, ex := range val.Examples {
			if ex.Label == label {
				valCount++
			}
		}
		if trainCount != 8 || valCount != 2 {
			t.Errorf("%s distribution = %d/%d, want 8/2", label, trainCount, valCount)
		}
	}
	for i := range val.Examples {
		if val.Examples[i].Prompt != val2.Examples[i].Prompt {
			t.Fatal("validation split is not deterministic")
		}
	}
	_ = train2
}

func TestDataset_Split_EdgeCases(t *testing.T) {
	ds := &Dataset{
		Examples: make([]Example, 10),
	}

	// Zero ratio — all train
	train, val := ds.Split(0)
	if train.Size() != 10 || val.Size() != 0 {
		t.Errorf("Split(0): train=%d, val=%d; want train=10, val=0", train.Size(), val.Size())
	}

	// Full ratio — all val (except 1 for train)
	train, val = ds.Split(1.0)
	if train.Size() != 10 || val.Size() != 0 {
		t.Errorf("Split(1.0): train=%d, val=%d; want train=10, val=0", train.Size(), val.Size())
	}
}

func TestDataset_LabelDistribution(t *testing.T) {
	ds := &Dataset{
		Examples: []Example{
			{Label: "SMALL"},
			{Label: "SMALL"},
			{Label: "MID"},
			{Label: "FRONTIER"},
			{Label: "FRONTIER"},
			{Label: "FRONTIER"},
		},
	}

	dist := ds.LabelDistribution()

	if dist[core.TierSmall] != 2 {
		t.Errorf("TierSmall count = %d, want 2", dist[core.TierSmall])
	}
	if dist[core.TierMid] != 1 {
		t.Errorf("TierMid count = %d, want 1", dist[core.TierMid])
	}
	if dist[core.TierFrontier] != 3 {
		t.Errorf("TierFrontier count = %d, want 3", dist[core.TierFrontier])
	}
}

func TestDataset_FeatureMatrix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.json")

	content := `[
		{"prompt": "rename x", "label": "SMALL"},
		{"prompt": "fix bug", "label": "MID"}
	]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ds, _ := LoadDataset(path)
	matrix := ds.FeatureMatrix()

	if len(matrix) != 2 {
		t.Errorf("Matrix rows = %d, want 2", len(matrix))
	}

	// Each row should have 13 features
	for i, row := range matrix {
		if len(row) != 13 {
			t.Errorf("Matrix[%d] has %d columns, want 13", i, len(row))
		}
	}
}

func TestDataset_Labels(t *testing.T) {
	ds := &Dataset{
		Examples: []Example{
			{Label: "SMALL"},
			{Label: "MID"},
			{Label: "FRONTIER"},
		},
	}

	labels := ds.Labels()

	expected := []int{0, 1, 2} // TierSmall=0, TierMid=1, TierFrontier=2
	for i, want := range expected {
		if labels[i] != want {
			t.Errorf("Labels[%d] = %d, want %d", i, labels[i], want)
		}
	}
}
