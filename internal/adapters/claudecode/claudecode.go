// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package claudecode adapts core routing decisions to Claude Code's hook
// protocol. Claude Code spawns subagents via the Task tool; a PreToolUse hook
// can rewrite the tool input before the subagent starts — including its model.
// This is the one place a subagent's model can be set programmatically, before
// the child process loads it.
package claudecode

import (
	"encoding/json"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/hookutil"
)

const harnessID = "claude-code"

// Event is the JSON Claude Code sends on stdin for a PreToolUse hook.
// We only decode the fields we need; the rest is ignored.
type Event struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	Model         string          `json:"model"` // the session's current model
	Prompt        string          `json:"prompt"`
}

// Output is the JSON we print on stdout to steer Claude Code.
type Output struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

// HookSpecificOutput carries the PreToolUse decision, including the rewritten
// tool input that changes the subagent's model.
type HookSpecificOutput struct {
	HookEventName      string          `json:"hookEventName"`
	PermissionDecision string          `json:"permissionDecision"`       // "allow"
	UpdatedInput       json.RawMessage `json:"updatedInput,omitempty"`   // rewritten Task input
}

// Handle processes a PreToolUse event. When the tool is Task (a subagent
// spawn), it classifies the subagent's prompt and, if a cheaper/stronger model
// fits, rewrites the Task input's `model` field. Returns the Output to print
// and a human-readable note (empty when nothing changed).
func Handle(ev Event) (Output, string) {
	// Only act on subagent spawns.
	if !isTaskTool(ev.ToolName) {
		return allow(), ""
	}

	var ti map[string]any
	if err := json.Unmarshal(ev.ToolInput, &ti); err != nil {
		return allow(), ""
	}

	// The subagent's own prompt drives its complexity, not the parent session.
	subPrompt := hookutil.StringField(ti, "prompt")
	if subPrompt == "" {
		subPrompt = hookutil.StringField(ti, "description")
	}
	if subPrompt == "" {
		return allow(), ""
	}

	// The current model for this subagent: the one already on the Task input,
	// else the session model from the event.
	currentModel := hookutil.StringField(ti, "model")
	if currentModel == "" {
		currentModel = ev.Model
	}

	decision := core.Route(subPrompt, harnessID, currentModel)

	// Only rewrite when a change actually helps.
	if decision.Verdict != core.VerdictDownshift && decision.Verdict != core.VerdictUpshift {
		return allow(), ""
	}

	// Rewrite the model field and re-marshal, preserving all other fields.
	ti["model"] = decision.Model.ID
	updated, err := json.Marshal(ti)
	if err != nil {
		return allow(), ""
	}

	out := Output{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       updated,
		},
		SystemMessage: "downshift: " + decision.Summary(),
	}
	return out, decision.Summary()
}

// isTaskTool reports whether a tool name is a subagent-spawning tool.
// Claude Code uses "Task"; some variants use "Agent".
func isTaskTool(name string) bool {
	n := strings.ToLower(name)
	return n == "task" || n == "agent"
}

// allow returns a no-op PreToolUse output that lets the tool run unchanged.
func allow() Output {
	return Output{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
		},
	}
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
