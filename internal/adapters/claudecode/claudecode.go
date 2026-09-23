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
type Event struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	Model         string          `json:"model"`
	Prompt        string          `json:"prompt"`
}

// TaskText returns the task content used for local, prompt-free feature
// extraction by the shared engineering loop.
func (ev Event) TaskText() string {
	var ti map[string]any
	if json.Unmarshal(ev.ToolInput, &ti) != nil {
		return ""
	}
	return hookutil.TaskText(ti, "prompt", "description")
}

// Output is the JSON we print on stdout to steer Claude Code.
type Output struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

// HookSpecificOutput carries the PreToolUse decision.
type HookSpecificOutput struct {
	HookEventName      string          `json:"hookEventName"`
	PermissionDecision string          `json:"permissionDecision"`
	UpdatedInput       json.RawMessage `json:"updatedInput,omitempty"`
}

// Handle processes a PreToolUse event using the catalog.Resolver injected by
// main. Falls back to the legacy core.Catalog when r is nil (tests).
// Returns the hook output, a human-readable note, and the full routing Decision
// so callers can record telemetry without re-classifying the prompt.
func Handle(ev Event, r ...core.Resolver) (Output, string, core.Decision) {
	if !isTaskTool(ev.ToolName) {
		return allow(), "", core.Decision{}
	}

	var ti map[string]any
	if err := json.Unmarshal(ev.ToolInput, &ti); err != nil {
		return allow(), "", core.Decision{}
	}

	subPrompt := hookutil.TaskText(ti, "prompt", "description")
	if subPrompt == "" {
		return allow(), "", core.Decision{}
	}

	currentModel := hookutil.StringField(ti, "model")
	if currentModel == "" {
		currentModel = ev.Model
	}

	var res core.Resolver
	if len(r) > 0 {
		res = r[0]
	}
	decision := core.Route(subPrompt, harnessID, currentModel, res)

	plan := decision.Plan(core.ClaudeCodeCaps, res)
	if plan.PreserveExplicit || !plan.RewriteModel {
		return allow(), "", decision
	}

	ti["model"] = plan.Model.ID
	updated, err := json.Marshal(ti)
	if err != nil {
		return allow(), "", decision
	}

	out := Output{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       updated,
		},
		SystemMessage: "downshift: " + decision.Summary(),
	}
	return out, decision.Summary(), decision
}

func isTaskTool(name string) bool {
	n := strings.ToLower(name)
	return n == "task" || n == "agent"
}

func allow() Output {
	return Output{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
		},
	}
}
