// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package cursor_test

import (
	"encoding/json"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/cursor"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

var cat = catalog.Load()

func catID(tier core.Tier) string {
	return cat.ModelFor("cursor", tier).ID
}

func decodeUpdated(t *testing.T, out cursor.Output) map[string]any {
	t.Helper()
	if out.UpdatedInput == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(out.UpdatedInput, &m); err != nil {
		t.Fatalf("updated_input not valid JSON: %v", err)
	}
	return m
}

func TestHandle_DownshiftsTrivialSubagent(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  frontierID,
		ToolInput: json.RawMessage(`{
			"task": "rename the userId variable to userIdentifier",
			"model": "` + frontierID + `"
		}`),
	}
	out, note := cursor.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	if out.Permission != "allow" {
		t.Errorf("permission = %s, want allow", out.Permission)
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
	if m["task"] == nil {
		t.Error("task field must be preserved in updated_input")
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	smallID := catID(core.TierSmall)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  smallID,
		ToolInput: json.RawMessage(`{
			"task": "rearchitect the payment flow across services",
			"model": "` + smallID + `"
		}`),
	}
	out, note := cursor.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierFrontier)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_OKKeepsGear(t *testing.T) {
	midID := catID(core.TierMid)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  midID,
		ToolInput: json.RawMessage(`{
			"task": "implement the CSV export feature",
			"model": "` + midID + `"
		}`),
	}
	out, note := cursor.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no change, got %q", note)
	}
	if out.UpdatedInput != nil {
		t.Error("must not rewrite when gear is already right")
	}
}

func TestHandle_IgnoresNonTaskTools(t *testing.T) {
	ev := cursor.Event{
		ToolName:  "Shell",
		ModelID:   catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	}
	_, note := cursor.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no note for non-Task tool, got %q", note)
	}
}

func TestHandle_FallsBackToPromptField(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  frontierID,
		ToolInput: json.RawMessage(`{
			"prompt": "fix a typo in the readme",
			"model": "` + frontierID + `"
		}`),
	}
	out, note := cursor.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift using prompt field")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_FallsBackToEventModel(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  frontierID,
		ToolInput: json.RawMessage(`{"task": "rename the variable"}`),
	}
	out, note := cursor.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift using event model_id as current")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

// TestHandle_SiblingFieldsPreserved is a regression test ensuring that a Cursor
// preToolUse rewrite does not drop fields like timeout or run_in_background.
func TestHandle_SiblingFieldsPreserved(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := cursor.Event{
		ToolName: "Task",
		ModelID:  frontierID,
		ToolInput: json.RawMessage(`{
			"task":              "rename the userId variable",
			"model":             "` + frontierID + `",
			"timeout":           45,
			"run_in_background": true
		}`),
	}
	out, _ := cursor.Handle(ev, cat)
	m := decodeUpdated(t, out)
	if m == nil {
		t.Fatal("expected updated_input")
	}
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
	if m["timeout"] == nil {
		t.Error("timeout field was dropped from updated_input")
	}
	if m["run_in_background"] == nil {
		t.Error("run_in_background field was dropped from updated_input")
	}
	if m["task"] == nil {
		t.Error("task field was dropped from updated_input")
	}
}
