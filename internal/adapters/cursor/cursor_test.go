package cursor

import (
	"encoding/json"
	"testing"
)

func decodeUpdated(t *testing.T, out Output) map[string]any {
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
	ev := Event{
		HookEventName: "preToolUse",
		ToolName:      "Task",
		ModelID:       "claude-opus-4.8",
		ToolInput: json.RawMessage(`{
			"task": "rename the userId variable to userIdentifier",
			"model": "claude-opus-4.8"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected a downshift note, got none")
	}
	if out.Permission != "allow" {
		t.Errorf("permission = %s, want allow", out.Permission)
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-haiku-4" {
		t.Errorf("model = %v, want claude-haiku-4", m["model"])
	}
	if m["task"] == nil {
		t.Error("task field must be preserved in updated_input")
	}
}

func TestHandle_UpshiftsComplexSubagent(t *testing.T) {
	ev := Event{
		ToolName: "Task",
		ModelID:  "claude-haiku-4",
		ToolInput: json.RawMessage(`{
			"task": "rearchitect the payment flow across services",
			"model": "claude-haiku-4"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected an upshift note, got none")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-opus-4.8" {
		t.Errorf("model = %v, want claude-opus-4.8", m["model"])
	}
}

func TestHandle_OKKeepsGear(t *testing.T) {
	ev := Event{
		ToolName: "Task",
		ModelID:  "claude-sonnet-4.6",
		ToolInput: json.RawMessage(`{
			"task": "implement the CSV export feature",
			"model": "claude-sonnet-4.6"
		}`),
	}
	out, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no change, got %q", note)
	}
	if out.UpdatedInput != nil {
		t.Error("must not rewrite when gear is already right")
	}
}

func TestHandle_IgnoresNonTaskTools(t *testing.T) {
	ev := Event{
		ToolName:  "Shell",
		ModelID:   "claude-opus-4.8",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	}
	_, note := Handle(ev)
	if note != "" {
		t.Errorf("expected no note for non-Task tool, got %q", note)
	}
}

func TestHandle_FallsBackToPromptField(t *testing.T) {
	// Some Task inputs use "prompt" instead of "task".
	ev := Event{
		ToolName: "Task",
		ModelID:  "claude-opus-4.8",
		ToolInput: json.RawMessage(`{
			"prompt": "fix a typo in the readme",
			"model": "claude-opus-4.8"
		}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift using prompt field")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-haiku-4" {
		t.Errorf("model = %v, want claude-haiku-4", m["model"])
	}
}

func TestHandle_FallsBackToEventModel(t *testing.T) {
	// Task input has no model → use model_id from the event.
	ev := Event{
		ToolName: "Task",
		ModelID:  "claude-opus-4.8",
		ToolInput: json.RawMessage(`{"task": "rename the variable"}`),
	}
	out, note := Handle(ev)
	if note == "" {
		t.Fatal("expected downshift using event model_id as current")
	}
	m := decodeUpdated(t, out)
	if m["model"] != "claude-haiku-4" {
		t.Errorf("model = %v, want claude-haiku-4", m["model"])
	}
}
