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
func Handle(ev Event, r ...core.Resolver) (Output, string) {
	if !isTaskTool(ev.ToolName) {
		return allow(), ""
	}

	var ti map[string]any
	if err := json.Unmarshal(ev.ToolInput, &ti); err != nil {
		return allow(), ""
	}

	subPrompt := hookutil.StringField(ti, "prompt")
	if subPrompt == "" {
		subPrompt = hookutil.StringField(ti, "description")
	}
	if subPrompt == "" {
		return allow(), ""
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

	plan := decision.Plan(core.HarnessCapabilities{CanRewriteModel: true})
	if !plan.RewriteModel {
		return allow(), ""
	}

	ti["model"] = plan.Model.ID
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
