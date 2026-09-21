// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package claudecode_test

import (
	"encoding/json"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/claudecode"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// cat is the shared test resolver — same embedded catalog the binary uses.
var cat = catalog.Load()

func catID(tier core.Tier) string {
	return cat.ModelFor("claude-code", tier).ID
}

func decodeUpdated(t *testing.T, out claudecode.Output) map[string]any {
	t.Helper()
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.UpdatedInput == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(out.HookSpecificOutput.UpdatedInput, &m); err != nil {
		t.Fatalf("updatedInput not valid JSON: %v", err)
	}
	return m
}

func TestHandle_DownshiftsTrivialSubagent(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := claudecode.Event{
		ToolName: "Task",
		Model:    frontierID,
		ToolInput: json.RawMessage(`{
			"description": "cleanup",
			"prompt": "rename the userId variable to userIdentifier",
			"model": "` + frontierID + `"
		}`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	m := decodeUpdated(t, out)
	if m == nil {
		t.Fatal("expected updatedInput, got none")
	}
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s (small tier)", m["model"], wantID)
	}
	if m["prompt"] == nil || m["description"] == nil {
		t.Error("prompt and description must be preserved in updatedInput")
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %s, want allow", out.HookSpecificOutput.PermissionDecision)
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	smallID := catID(core.TierSmall)
	ev := claudecode.Event{
		ToolName: "Task",
		Model:    smallID,
		ToolInput: json.RawMessage(`{
			"prompt": "rearchitect the payment flow across services",
			"model": "` + smallID + `"
		}`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierFrontier)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s (frontier tier)", m["model"], wantID)
	}
}

func TestHandle_NoChangeWhenAlreadyRightGear(t *testing.T) {
	midID := catID(core.TierMid)
	ev := claudecode.Event{
		ToolName: "Task",
		Model:    midID,
		ToolInput: json.RawMessage(`{
			"prompt": "implement the CSV export feature",
			"model": "` + midID + `"
		}`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no change note, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite input when gear is already right")
	}
}

func TestHandle_IgnoresNonTaskTools(t *testing.T) {
	ev := claudecode.Event{
		ToolName:  "Bash",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{"command": "rm -rf /tmp/x"}`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no note for non-Task tool, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite input for non-Task tools")
	}
}

func TestHandle_FallsBackToSessionModel(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := claudecode.Event{
		ToolName: "Task",
		Model:    frontierID, // session model, no model on tool input
		ToolInput: json.RawMessage(`{"prompt": "fix a typo in the readme"}`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift using session model as current")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_AgentToolAlias(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := claudecode.Event{
		ToolName: "Agent",
		Model:    frontierID,
		ToolInput: json.RawMessage(`{"prompt": "rename the variable", "model": "` + frontierID + `"}`),
	}
	_, note, _ := claudecode.Handle(ev, cat)
	if note == "" {
		t.Error("expected Agent tool to be treated as a subagent spawn")
	}
}

func TestHandle_MalformedInputFailsOpen(t *testing.T) {
	ev := claudecode.Event{
		ToolName:  "Task",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{not valid`),
	}
	out, note, _ := claudecode.Handle(ev, cat)
	if note != "" {
		t.Errorf("malformed input must fail-open, got note %q", note)
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("malformed input must return allow")
	}
}

// TestHandle_SiblingFieldsPreserved is a regression test for the updatedInput
// replace-vs-patch behaviour. Claude Code's hook replaces the entire tool_input
// with updatedInput — so the adapter must re-marshal the *full* decoded map
// with only the model key changed. Any field not present in updatedInput is
// silently dropped by the harness, which can break subagents that use timeout,
// run_in_background, or other non-standard fields.
func TestHandle_SiblingFieldsPreserved(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := claudecode.Event{
		ToolName: "Task",
		Model:    frontierID,
		ToolInput: json.RawMessage(`{
			"prompt":             "rename the userId variable",
			"model":              "` + frontierID + `",
			"timeout":            30,
			"description":        "a short rename task",
			"run_in_background":  false
		}`),
	}
	out, _, _ := claudecode.Handle(ev, cat)
	m := decodeUpdated(t, out)
	if m == nil {
		t.Fatal("expected updatedInput")
	}
	// Model must be rewritten.
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
	// All sibling fields must survive the rewrite.
	if m["timeout"] == nil {
		t.Error("timeout field was dropped from updatedInput")
	}
	if m["description"] == nil {
		t.Error("description field was dropped from updatedInput")
	}
	if m["run_in_background"] == nil {
		t.Error("run_in_background field was dropped from updatedInput")
	}
	if m["prompt"] == nil {
		t.Error("prompt field was dropped from updatedInput")
	}
}
