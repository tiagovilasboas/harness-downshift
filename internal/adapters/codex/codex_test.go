package codex

import (
	"encoding/json"
	"testing"
)

func decodeUpdated(t *testing.T, out Output) map[string]any {
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
	ev := Event{
		HookEventName: "PreToolUse",
		ToolName:      "spawn_agent",
		Model:         "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{
			"task_name": "worker_agent_rename",
			"message": "rename the userId variable to userIdentifier",
			"fork_turns": "none"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	if !out.Continue {
		t.Error("continue must be true on allow")
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %s, want allow", out.HookSpecificOutput.PermissionDecision)
	}
	m := decodeUpdated(t, out)
	if m["model"] != "gpt-5.6-luna" {
		t.Errorf("model = %v, want gpt-5.6-luna", m["model"])
	}
	if m["reasoning_effort"] != "low" {
		t.Errorf("reasoning_effort = %v, want low", m["reasoning_effort"])
	}
	// Reserved schema fields must be preserved.
	if m["message"] == nil || m["task_name"] == nil || m["fork_turns"] == nil {
		t.Error("reserved spawn_agent fields must be preserved in updatedInput")
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	ev := Event{
		ToolName: "spawn_agent",
		Model:    "gpt-5.6-luna",
		ToolInput: json.RawMessage(`{
			"task_name": "worker_agent_payments",
			"message": "rearchitect the payment flow across multiple services and migrate the schema"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "gpt-5.3-codex" {
		t.Errorf("model = %v, want gpt-5.3-codex", m["model"])
	}
	if m["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, want high", m["reasoning_effort"])
	}
}

func TestHandle_OKKeepsGear(t *testing.T) {
	ev := Event{
		ToolName: "spawn_agent",
		Model:    "gpt-5.6-terra",
		ToolInput: json.RawMessage(`{
			"message": "implement the CSV export feature",
			"model": "gpt-5.6-terra"
		}`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no change, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite when gear is already right")
	}
	if !out.Continue {
		t.Error("continue must be true on a plain allow")
	}
}

func TestHandle_IgnoresNonSpawnTools(t *testing.T) {
	ev := Event{
		ToolName:  "Bash",
		Model:     "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	}
	_, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no note for non-spawn tool, got %q", note)
	}
}

func TestHandle_MatchesFlattenedNamespacedToolName(t *testing.T) {
	// Some builds flatten the namespaced tool to "collaborationspawn_agent".
	ev := Event{
		ToolName: "collaborationspawn_agent",
		Model:    "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{
			"message": "fix a typo in the readme"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift for flattened namespaced tool name")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "gpt-5.6-luna" {
		t.Errorf("model = %v, want gpt-5.6-luna", m["model"])
	}
}

func TestHandle_MatchesAgentToolName(t *testing.T) {
	ev := Event{
		ToolName: "Agent",
		Model:    "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{
			"message": "rename a private helper method"
		}`),
	}
	_, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift for Agent tool name")
	}
}

func TestHandle_TaskNameAddsSignal(t *testing.T) {
	// A terse message plus a role-ish task_name should still classify; the
	// task_name is folded into the prompt so it contributes signal.
	ev := Event{
		ToolName: "spawn_agent",
		Model:    "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{
			"task_name": "review_agent",
			"message": "typo fix"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected a routing decision")
	}
	m := decodeUpdated(t, out)
	if m["task_name"] != "review_agent" {
		t.Errorf("task_name must be preserved, got %v", m["task_name"])
	}
}

func TestHandle_FallsBackToEventModel(t *testing.T) {
	// No model on the tool input → use the session model from the event.
	ev := Event{
		ToolName: "spawn_agent",
		Model:    "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{"message": "rename the variable"}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift using event model as current")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "gpt-5.6-luna" {
		t.Errorf("model = %v, want gpt-5.6-luna", m["model"])
	}
}

func TestHandle_MalformedInputFailsOpen(t *testing.T) {
	ev := Event{
		ToolName:  "spawn_agent",
		Model:     "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{not valid json`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected fail-open (no note), got %q", note)
	}
	if !out.Continue || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("malformed input must fail open with continue:true + allow")
	}
}

func TestHandle_EmptyMessageFailsOpen(t *testing.T) {
	ev := Event{
		ToolName:  "spawn_agent",
		Model:     "gpt-5.3-codex",
		ToolInput: json.RawMessage(`{"fork_turns":"none"}`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no decision for empty message, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite when there is no task text")
	}
}
