// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package codex_test

import (
	"encoding/json"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/codex"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

var cat = catalog.Load()

func catID(tier core.Tier) string {
	return cat.ModelFor("codex", tier).ID
}

func decodeUpdated(t *testing.T, out codex.Output) map[string]any {
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
	ev := codex.Event{
		ToolName: "spawn_agent",
		Model:    frontierID,
		ToolInput: json.RawMessage(`{
			"task_name": "worker_agent_rename",
			"message": "rename the userId variable to userIdentifier",
			"fork_turns": "none"
		}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %s, want allow", out.HookSpecificOutput.PermissionDecision)
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
	// reasoning_effort must be set (not empty).
	if m["reasoning_effort"] == "" || m["reasoning_effort"] == nil {
		t.Error("reasoning_effort must be set on downshift")
	}
	// Reserved schema fields must be preserved.
	if m["message"] == nil || m["task_name"] == nil || m["fork_turns"] == nil {
		t.Error("reserved spawn_agent fields must be preserved in updatedInput")
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	smallID := catID(core.TierSmall)
	ev := codex.Event{
		ToolName: "spawn_agent",
		Model:    smallID,
		ToolInput: json.RawMessage(`{
			"message": "rearchitect the payment flow across multiple services and migrate the schema"
		}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierFrontier)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_OKRewritesReasoningEffort(t *testing.T) {
	midID := catID(core.TierMid)
	ev := codex.Event{
		ToolName: "spawn_agent",
		Model:    midID,
		ToolInput: json.RawMessage(`{
			"message": "implement the CSV export feature",
			"model": "` + midID + `"
		}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Error("expected an effort-routing note")
	}
	m := decodeUpdated(t, out)
	if m["model"] != midID {
		t.Errorf("model = %v, want %s", m["model"], midID)
	}
	if m["reasoning_effort"] != "medium" {
		t.Errorf("reasoning_effort = %v, want medium", m["reasoning_effort"])
	}
}

func TestHandle_IgnoresNonSpawnTools(t *testing.T) {
	ev := codex.Event{
		ToolName:  "Bash",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	}
	_, note := codex.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no note for non-spawn tool, got %q", note)
	}
}

func TestHandle_MatchesFlattenedNamespacedToolName(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := codex.Event{
		ToolName:  "collaborationspawn_agent",
		Model:     frontierID,
		ToolInput: json.RawMessage(`{"message": "fix a typo in the readme"}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift for flattened namespaced tool name")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_MatchesAgentToolName(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := codex.Event{
		ToolName:  "Agent",
		Model:     frontierID,
		ToolInput: json.RawMessage(`{"message": "rename a private helper method"}`),
	}
	_, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift for Agent tool name")
	}
}

func TestHandle_TaskNameAddsSignal(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := codex.Event{
		ToolName: "spawn_agent",
		Model:    frontierID,
		ToolInput: json.RawMessage(`{
			"task_name": "review_agent",
			"message": "typo fix"
		}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected a routing decision")
	}
	m := decodeUpdated(t, out)
	if m["task_name"] != "review_agent" {
		t.Errorf("task_name must be preserved, got %v", m["task_name"])
	}
}

func TestHandle_FallsBackToEventModel(t *testing.T) {
	frontierID := catID(core.TierFrontier)
	ev := codex.Event{
		ToolName:  "spawn_agent",
		Model:     frontierID,
		ToolInput: json.RawMessage(`{"message": "rename the variable"}`),
	}
	out, note := codex.Handle(ev, cat)
	if note == "" {
		t.Fatal("expected downshift using event model as current")
	}
	m := decodeUpdated(t, out)
	wantID := catID(core.TierSmall)
	if m["model"] != wantID {
		t.Errorf("model = %v, want %s", m["model"], wantID)
	}
}

func TestHandle_MalformedInputFailsOpen(t *testing.T) {
	ev := codex.Event{
		ToolName:  "spawn_agent",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{not valid json`),
	}
	out, note := codex.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected fail-open (no note), got %q", note)
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("malformed input must fail open with permissionDecision: allow")
	}
}

func TestHandle_UsesSupportedCodexPreToolUseEnvelope(t *testing.T) {
	ev := codex.Event{
		ToolName:  "spawn_agent",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{"message": "rename the variable"}`),
	}
	out, _ := codex.Handle(ev, cat)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if _, found := decoded["continue"]; found {
		t.Error("PreToolUse output must not contain unsupported continue")
	}
}

func TestHandle_EmptyMessageFailsOpen(t *testing.T) {
	ev := codex.Event{
		ToolName:  "spawn_agent",
		Model:     catID(core.TierFrontier),
		ToolInput: json.RawMessage(`{"fork_turns":"none"}`),
	}
	out, note := codex.Handle(ev, cat)
	if note != "" {
		t.Errorf("expected no decision for empty message, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite when there is no task text")
	}
}
