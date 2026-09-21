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

Works with Claude Code, Cursor, and Codex today. Any harness with subagents next.

**Keywords:** Claude Code subagent cost · LLM model routing · agent harness ·
cost optimization · Claude Code hooks · Cursor subagents · Codex model selection

---

## This is harness engineering

> *"Agent = Model + Harness."* — [Martin Fowler](https://martinfowler.com/articles/harness-engineering.html)

The **harness** is everything around the model that turns it into a working
agent: the loop, the tools, context assembly, and — the part this project cares
about — **how it delegates work to subagents**. Recent research names the
harness as *"the decisive lever against token maxing"*
([arXiv:2607.06906](https://arxiv.org/abs/2607.06906)).

`harness-downshift` is a focused piece of harness engineering. It doesn't touch
the model's reasoning; it engineers the **delegation layer** — the exact point
where a subagent's model is chosen — to optimize three things at once:

- **Token efficiency** — cheap models for cheap work; frontier tokens spent only where they earn it.
- **Cost reduction** — trivial subagents drop from frontier to small tier (~95% cheaper per call).
- **Control & predictability** — deterministic, rule-based routing you can read, test, and audit. No "Auto" black box, no LLM guessing in the loop.

Agent = Model + Harness. You can't cheaply swap the model. You *can* engineer
the harness. That's the whole game here.

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

## Install (Cursor)

Cursor's hook protocol mirrors Claude Code's — same `preToolUse` interception,
`updated_input` to rewrite the subagent's model. Build the binary, then add the
hook to `.cursor/hooks.json` (project level) or `~/.cursor/hooks.json` (global):

```json
{
  "version": 1,
  "hooks": {
    "preToolUse": [
      { "command": "downshift cursor", "matcher": "Task" }
    ]
  }
}
```

The `matcher: "Task"` scopes the hook to subagent spawns only. Cursor watches
the config and reloads it on save.

## Install (Codex)

Codex spawns subagents through a reserved `spawn_agent` tool under
`multi_agent_v2`. You can't put a model on the provider-visible call, but a
`PreToolUse` hook can inject the model **and** `reasoning_effort` into the tool
input before the child starts — which is exactly where `downshift` runs.

Enable the feature flags in `config.toml`:

```toml
[features]
codex_hooks = true

[features.multi_agent_v2]
enabled = true
```

Register the hook in `.codex/hooks.json` (project) or `~/.codex/hooks.json`
(global). The matcher covers both the `Agent` and namespaced `spawn_agent`
tool names:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "(^Agent$|spawn_agent$)",
        "hooks": [
          { "type": "command", "command": "downshift codex" }
        ]
      }
    ]
  }
}
```

On Codex, downshift routes **two axes at once**: the model tier and the
reasoning effort (`low` for trivial work, up to `high` for the frontier tier).
A trivial subagent drops from `gpt-5.3-codex` at high effort to `gpt-5.6-luna`
at low effort — cheap on both counts.

> Codex's `multi_agent_v2` spawn schema is still evolving. downshift preserves
> the reserved fields and fails open, but pin the exact Codex build you deploy
> and keep an acceptance test on a real spawn.

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
| **Cursor** | ✅ Task tool | `preToolUse` hook → `updated_input.model` | ✅ shipped |
| **Codex** | ✅ `spawn_agent` (multi_agent_v2) | `PreToolUse` hook → `updatedInput.model` + `reasoning_effort` | ✅ shipped |
| Grok CLI | ✅ parallel subagents | hook format under review | 🔜 next |
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

These are harness-engineering principles first, implementation choices second:

- **Deterministic over probabilistic.** Routing is scored rules you can read
  and test — not another model guessing. A harness you can't predict is a
  harness you can't trust with your budget.
- **Zero token overhead.** Classification is local pattern-matching. The router
  adds no LLM calls to the loop — it optimizes token spend without spending
  tokens to do it.
- **Fail-open.** A parse error, an unknown model, a bad event — any failure
  lets the subagent run unchanged. A cost optimizer must never become an
  availability risk.
- **Single binary.** Go, cross-compiled for macOS / Linux / Windows. No Python,
  no Node, no runtime. The harness layer should be boring and dependable.
- **Honest about limits.** It engineers the delegation layer — subagent models
  — not your main turn. It's a heuristic classifier, not an oracle. Good
  harness engineering states its blast radius.

---

## Where this fits

Teams running agents at scale are converging on the same shape: harnesses
(Cursor, Kiro, Claude Code, Codex) on the edge, and a **central corporate layer**
underneath them — governance, AI FinOps, observability, and a
**router / orchestrator** deciding which model handles what, across providers.

`harness-downshift` is a small, open, focused piece of that picture. It is the
**router at the delegation layer**: the deterministic rule that decides which
model a subagent gets, so cost per token is controlled by design rather than
reconstructed on the invoice. One brain, many harness adapters — the same
"all harnesses point at a central decision" architecture, in the open.

---

## Contributing

Contributions are welcome — especially prompts the classifier gets wrong.

**The single most useful contribution** is a real subagent prompt that
`downshift` misroutes. Open an issue with:

- the prompt text (redact anything private),
- the complexity `downshift try "<prompt>"` returned,
- the complexity you expected, and why.

That feedback is what tunes the classifier against reality instead of against
our assumptions.

**Other ways to help:**

- **New harness adapter** — implement `internal/adapters/<harness>/` following
  the Claude Code adapter as a template. The `core` package is harness-agnostic;
  an adapter only translates a `core.Decision` into that harness's mechanism.
- **Model catalog updates** — prices and model ids change. Corrections to
  `internal/core/models.go` (with a source) are always welcome.
- **Classifier signals** — new keyword/pattern signals for a complexity class,
  with a test case that proves the improvement.

**Ground rules:**

- Keep `go test -race -cover ./...` green.
- Every classifier change ships with a table-driven test case.
- Small, focused PRs. Conventional commits in English.
- Be honest about limits in docs — no overselling what a heuristic can do.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide.

---

## References & inspiration

- **Martin Fowler — [Harness engineering for coding agent users](https://martinfowler.com/articles/harness-engineering.html).**
  The `Agent = Model + Harness` framing and the case for engineering the harness
  rather than chasing models. This article is the conceptual backbone of the
  project.
- **[The Harness Effect: How Orchestration Design Sets the Token Economics of
  Enterprise Agentic AI](https://arxiv.org/abs/2607.06906)** — names the harness
  as the decisive lever against token maxing.
- **[Triage: Routing Software Engineering Tasks to Cost-Effective LLM Tiers](https://arxiv.org/abs/2604.07494)**
  — evidence that task signals can pick a cheaper tier without losing quality.
- **[claude-model-router-hook](https://github.com/tzachbon/claude-model-router-hook)**
  by tzachbon — a Claude-Code-only router that showed the `PreToolUse`
  `updatedInput` mechanism works. `harness-downshift` generalizes the idea
  across harnesses in a single Go binary.

---

## Status

Early but real. The Claude Code, Cursor, and Codex adapters all work and are
tested end-to-end. Grok CLI is next. The classifier will keep getting tuned
against real subagent prompts — issues and PRs with prompts it gets wrong are
the most useful contribution.

## License

[MIT](LICENSE) © 2026 [Tiago Vilas Boas](https://github.com/tiagovilasboas)
