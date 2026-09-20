# harness-downshift

> **Cut Claude Code subagent costs** by routing every subagent to the
> right-sized model for its task. A deterministic model router that runs as a
> hook — no extra tokens, no LLM in the loop, single Go binary.

![harness-downshift](docs/img/hero.svg)

**Your subagents are running Opus to rename a variable. You're paying frontier
prices for work a cheap model does just as well.**

`harness-downshift` puts every subagent in the right gear. Trivial work goes to
the cheap model. Hard work gets the frontier model's torque. You stop burning
budget on the straights and keep the power for the curves.

Works with Claude Code today. Cursor, Codex, and any harness with subagents next.

**Keywords:** Claude Code subagent cost · LLM model routing · agent harness ·
cost optimization · Claude Code hooks · Cursor subagents · Codex model selection

---

## The pain it's born from

Agentic coding got expensive fast, and the biggest line item is invisible:
**subagents**. When your main agent spawns a subagent to explore a folder, run
tests, or read files, that subagent inherits the session's model. So a $15/1M
frontier model ends up grepping a directory — work a $0.80/1M model finishes
identically.

Studies put subagent spend at up to **85% of a heavy session**. The fix is
known — route each subagent to the cheapest model that can do its job — but
nobody wants to babysit model selection on every spawn.

That babysitting is the whole job of `harness-downshift`. It reads each
subagent's task, classifies its complexity, and rewrites the model **before the
subagent starts** — automatically, deterministically, with no LLM call in the
loop.

---

## The gearbox model

A good driver doesn't stay in high gear through a hairpin, and doesn't crawl in
first gear on the highway. They match the gear to the road.

| Road | Task | Gear | Model |
|---|---|---|---|
| Flat straight | rename, format, git commit, fix typo | high, cheap | **haiku-class** |
| Rolling hills | add a field, fix a bug, one function | mid | **sonnet-class** |
| Winding road | refactor a module, feature across files | mid | **sonnet-class** |
| Sharp curve | rearchitect, migrate, race condition | low, torque | **opus-class** |

`downshift` reads the task and picks the gear. On the straights it **downshifts**
to save fuel. On the curves it **upshifts** for control. The "Auto" your harness
ships with does neither — it leaves you in one gear the whole drive.

---

## What it actually does

It runs as a **hook**. When your harness is about to spawn a subagent, it pipes
the spawn details to `downshift`, which:

1. **Classifies** the subagent's task: `TRIVIAL / SIMPLE / MEDIUM / COMPLEX`
   — deterministic scoring, no network, no extra tokens.
2. **Maps** the complexity to the minimum capable tier: `small / mid / frontier`.
3. **Rewrites** the subagent's model to the right one for that tier, before the
   subagent process starts.

The main session keeps the model you chose. Only the subagents get right-sized.

```
$ downshift try "rename the userId variable across auth.ts"
Task:       rename the userId variable across auth.ts
Complexity: TRIVIAL
Needs tier: small
Recommend:  claude-haiku-4
Verdict:    DOWNSHIFT
→ TRIVIAL task → downshift to claude-haiku-4 (~95% cheaper)
```

---

## Install (Claude Code)

Build the single binary (no runtime, no dependencies):

```bash
go install github.com/tiagovilasboas/harness-downshift/cmd/downshift@latest
```

Add the hook to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Task",
        "hooks": [
          { "type": "command", "command": "downshift claude-code" }
        ]
      }
    ]
  }
}
```

That's it. Every subagent your session spawns now runs in the right gear.

---

## Why a hook, and why only subagents

The model of your **current turn** is loaded when the session starts — no
harness lets you swap it mid-turn from the outside (we tried; it doesn't work).
But a **subagent is a fresh process**. Its model is chosen at spawn time, in the
`Task` tool input, and a `PreToolUse` hook can rewrite that input before the
child starts.

That's the one place model selection is genuinely controllable from the outside
— and it happens to be where most of the cost hides. So that's where
`downshift` works.

---

## Harness support

| Harness | Subagents | Control mechanism | Status |
|---|---|---|---|
| **Claude Code** | ✅ Task tool | `PreToolUse` hook → `updatedInput.model` | ✅ shipped |
| **Cursor** | ✅ Task tool | subagent frontmatter `model:` / Task param | 🔜 next |
| **Codex** | ✅ | spawn-time `--model` | 🔜 next |
| Kiro (single-thread) | ❌ no subagents | — | not applicable |
| Claude.ai / ChatGPT web | ❌ closed | — | not possible |

The classifier and policy are harness-agnostic — one brain. Each adapter
translates the decision into that harness's own mechanism.

---

## How classification works

Deterministic, scored, no LLM:

- Each complexity class has weighted signals (keywords, patterns, structure).
- The task is scored against all four classes; the top score wins.
- Ties break **toward higher complexity** — never underpower a task to save a
  few cents.
- No signal at all → `MEDIUM` (your harness's usual model), so it fails safe.

It's a heuristic, not an oracle. It's tuned to be conservative: it downshifts
only when the task is clearly mechanical, and upshifts the moment a task looks
hard.

---

## Design principles

- **Fail-open.** A parse error, an unknown model, a bad event — any failure
  lets the subagent run unchanged. `downshift` never blocks your work.
- **No tokens spent.** Classification is local pattern-matching. The router adds
  zero LLM calls to your loop.
- **Single binary.** Go, cross-compiled for macOS / Linux / Windows. No Python,
  no Node, no runtime to install.
- **Honest about limits.** It controls subagent models, not your main turn.
  It's a heuristic classifier, not a perfect judge. The README says so on
  purpose.

---

## Status

Early. The Claude Code adapter works and is tested end-to-end. Cursor and Codex
adapters are next. The classifier will keep getting tuned against real subagent
prompts — issues and PRs with prompts it gets wrong are the most useful
contribution.

## License

[MIT](LICENSE) © 2026 [Tiago Vilas Boas](https://github.com/tiagovilasboas)
