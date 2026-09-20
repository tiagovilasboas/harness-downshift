package claudecode

import (
	"encoding/json"
	"testing"
)

// decodeUpdated pulls the updatedInput back into a map for assertions.
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
		ToolName:      "Task",
		Model:         "claude-opus-4-8",
		ToolInput: json.RawMessage(`{
			"description": "cleanup",
			"prompt": "rename the userId variable to userIdentifier",
			"model": "claude-opus-4-8"
		}`),
	}

	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	m := decodeUpdated(t, out)
	if m == nil {
		t.Fatal("expected updatedInput, got none")
	}
	if m["model"] != "claude-haiku-4" {
		t.Errorf("model = %v, want claude-haiku-4", m["model"])
	}
	// Other fields preserved.
	if m["prompt"] == nil || m["description"] == nil {
		t.Error("expected prompt and description preserved in updatedInput")
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %s, want allow", out.HookSpecificOutput.PermissionDecision)
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	ev := Event{
		ToolName: "Task",
		Model:    "claude-haiku-4",
		ToolInput: json.RawMessage(`{
			"prompt": "rearchitect the payment flow across services",
			"model": "claude-haiku-4"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-opus-4-8" {
		t.Errorf("model = %v, want claude-opus-4-8", m["model"])
	}
}

func TestHandle_NoChangeWhenAlreadyRightGear(t *testing.T) {
	ev := Event{
		ToolName: "Task",
		Model:    "claude-sonnet-4-6",
		ToolInput: json.RawMessage(`{
			"prompt": "implement the CSV export feature",
			"model": "claude-sonnet-4-6"
		}`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no change note, got %q", note)
	}
	// Should be a plain allow with no updatedInput.
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("expected no updatedInput when gear is already right")
	}
}

func TestHandle_IgnoresNonTaskTools(t *testing.T) {
	ev := Event{
		ToolName:  "Bash",
		Model:     "claude-opus-4-8",
		ToolInput: json.RawMessage(`{"command": "rm -rf /tmp/x"}`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no note for non-Task tool, got %q", note)
	}
	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Error("must not rewrite input for non-Task tools")
	}
}

func TestHandle_FallsBackToSessionModel(t *testing.T) {
	// Task input has no model → use the session model from the event.
	ev := Event{
		ToolName: "Task",
		Model:    "claude-opus-4-8",
		ToolInput: json.RawMessage(`{
			"prompt": "fix a typo in the readme"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift using session model as current")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-haiku-4" {
		t.Errorf("model = %v, want claude-haiku-4", m["model"])
	}
}

func TestHandle_AgentToolAlias(t *testing.T) {
	ev := Event{
		ToolName: "Agent", // some variants use "Agent" instead of "Task"
		Model:    "claude-opus-4-8",
		ToolInput: json.RawMessage(`{"prompt": "rename the variable", "model": "claude-opus-4-8"}`),
	}
	_, note := Handle(ev)
	if note == "" {
		t.Error("expected Agent tool to be treated as a subagent spawn")
	}
}
