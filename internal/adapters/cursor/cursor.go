// Package cursor adapts core routing decisions to Cursor's hook protocol.
//
// Cursor exposes a preToolUse hook whose output supports updated_input — the
// same interception point Claude Code uses. When the agent is about to spawn a
// subagent via the Task tool, we rewrite the tool input's model before the
// child starts.
//
// Note on subagentStart: Cursor also has a dedicated subagentStart hook, but
// its output only supports permission (allow/deny) — it cannot rewrite the
// model. So model control has to go through preToolUse + updated_input.
//
// Field names differ from Claude Code: Cursor uses snake_case JSON
// (hook_event_name, tool_name, tool_input, updated_input), while Claude Code
// uses a mix. This adapter speaks Cursor's dialect; the core decision is the
// same.
package cursor

import (
	"encoding/json"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

const harnessID = "cursor"

// Event is the JSON Cursor sends on stdin for a preToolUse hook.
// Only the fields we need are decoded.
type Event struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	Model         string          `json:"model"`    // legacy model slug of the composer
	ModelID       string          `json:"model_id"` // structured model id, when available
}

// Output is the JSON we print on stdout to steer Cursor's preToolUse.
type Output struct {
	Permission   string          `json:"permission"`              // "allow"
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"` // rewritten Task input
	AgentMessage string          `json:"agent_message,omitempty"` // note fed back to the agent
}

// Handle processes a preToolUse event. When the tool is Task (a subagent
// spawn), it classifies the subagent's task and, if a different model fits,
// rewrites the tool input's model via updated_input. Returns the Output to
// print and a human-readable note (empty when nothing changed).
func Handle(ev Event) (Output, string) {
	if !isTaskTool(ev.ToolName) {
		return allow(), ""
	}

	var ti map[string]any
	if err := json.Unmarshal(ev.ToolInput, &ti); err != nil {
		return allow(), ""
	}

	// The subagent's own task text drives its complexity.
	// Cursor's Task input commonly carries "task" and/or "prompt".
	subPrompt := stringField(ti, "task")
	if subPrompt == "" {
		subPrompt = stringField(ti, "prompt")
	}
	if subPrompt == "" {
		subPrompt = stringField(ti, "description")
	}
	if subPrompt == "" {
		return allow(), ""
	}

	// Current model: the one on the Task input, else the structured model_id,
	// else the legacy model slug.
	currentModel := stringField(ti, "model")
	if currentModel == "" {
		currentModel = ev.ModelID
	}
	if currentModel == "" {
		currentModel = ev.Model
	}

	decision := core.Route(subPrompt, harnessID, currentModel)

	if decision.Verdict != core.VerdictDownshift && decision.Verdict != core.VerdictUpshift {
		return allow(), ""
	}

	ti["model"] = decision.Model.ID
	updated, err := json.Marshal(ti)
	if err != nil {
		return allow(), ""
	}

	out := Output{
		Permission:   "allow",
		UpdatedInput: updated,
		AgentMessage: "downshift: " + decision.Summary(),
	}
	return out, decision.Summary()
}

// isTaskTool reports whether the tool name is a subagent-spawning tool.
// Cursor's preToolUse matcher uses "Task".
func isTaskTool(name string) bool {
	return strings.EqualFold(name, "task")
}

// allow returns a no-op preToolUse output that lets the tool run unchanged.
func allow() Output {
	return Output{Permission: "allow"}
}

// stringField reads a string field from a decoded JSON object, empty if absent.
func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
