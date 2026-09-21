# Contributing to harness-downshift

Thanks for helping make agent harnesses cheaper and more predictable. This
project is small and focused on purpose — contributions that keep it that way
are the most valuable.

## The most useful contribution: a misrouted prompt

`harness-downshift` classifies a task's complexity with deterministic rules.
Rules are only as good as the prompts they've seen. If you find a prompt it gets
wrong, that's gold.

Open an issue titled `misroute: <short description>` with:

1. **The prompt** (redact anything private).
2. **What `downshift` returned:**
   ```bash
   downshift try "<the prompt>"
   ```
3. **What you expected** — the complexity class and why.

Accepted misroutes become test cases in `classifier_edge_test.go`, then the
signals are tuned until they pass. Your prompt becomes a permanent regression
guard.

## Project layout

```
cmd/downshift/                 # binary: hook mode + try + models subcommands
internal/core/                 # harness-agnostic brain
  classifier.go                #   task text → complexity (scored, deterministic)
  signals.go                   #   RawSignals table (exported, tunable)
  escalation.go                #   EscalationIntent (Trivial/Normal/Review/Preserved)
  models.go                    #   Tier, Effort types (no model ID strings)
  policy.go                    #   complexity → tier → Decision (Route)
  capabilities.go              #   HarnessCapabilities per harness; Plan()
  resolver.go                  #   Resolver interface (catalog implements this)
internal/catalog/              # model data (IDs, costs, effort maps, routing flags)
  catalog.go                   #   Load(), LookupByID (exact→alias→family), EffortFor
  policy.go                    #   mergeEntries + validateEntries
  catalog.json                 #   embedded default (committed + go:embed'd)
internal/adapters/<harness>/   # one adapter per harness
  claudecode/                  #   PreToolUse + updatedInput for Claude Code
  cursor/                      #   preToolUse + updated_input for Cursor
  codex/                       #   PreToolUse + updatedInput + reasoning_effort for Codex
internal/hookutil/             # shared utilities (StringField)
internal/models/               # models subcommands (list, check, pull)
```

**Dependency rule:** `core` never imports `catalog` or any adapter.
Direction: `catalog → core`, `adapters → core + hookutil`, `models → catalog + core`.

## Adding a new harness adapter

Use `internal/adapters/claudecode/` as the template. An adapter:

1. Decodes the harness's subagent-spawn event.
2. Extracts the subagent's task text and current model ID.
3. Calls `core.Route(prompt, harnessID, currentModelID, resolver)`.
4. Translates the returned `core.Decision` into the harness's mechanism.
5. **Fails open** — any error returns an allow decision and exits 0.

Then:
- Add catalog entries for the new harness in `internal/catalog/catalog.json`
  (no Go changes needed — it's just JSON data with `id`, `family`, `tier`,
  `effort_map`, and cost fields).
- Wire a new subcommand in `cmd/downshift/main.go`.
- Add table-driven tests that inject `catalog.Load()` as the resolver.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full step-by-step guide.

## Updating model data

Model IDs, costs, and effort mappings live in `internal/catalog/catalog.json`.
To add or update a model:

1. Edit `catalog.json` directly — no Go changes needed.
2. Add a `family` field (the stable prefix for version-agnostic matching).
3. Add `"routing": "explicit_only"` if the model should never be an automatic
   routing target (e.g. reserved top-tier models like `gpt-6-astra`).
4. Use `{ "_section": "your label" }` objects as human-readable section
   headers — the parser silently skips any entry where `id` or `harness` is
   empty, so these are safe to include.
5. Open a PR with a source link (provider docs or pricing page).

Or run `downshift models pull` with the provider's API key to discover new
models automatically (they arrive with `tier: "unknown"` for you to assign).

## Development

```bash
go build ./...                  # compile
go test -race -cover ./...      # run tests
go vet ./...                    # vet
goreleaser check                # validate release config (needs goreleaser installed)
```

Requires Go 1.27+.

## Ground rules

- **Keep tests green.** `go test -race -cover ./...` must pass before every commit.
- **Every classifier change ships a test.** Add a table-driven case in
  `classifier_edge_test.go` that proves the improvement.
- **Small, focused PRs.** One concern per PR. No "also fixed X" bundling.
- **Conventional commits in English.** `feat(core): ...`, `fix(claudecode): ...`, `docs: ...`
- **Fail-open, always.** A cost optimizer must never block a subagent spawn.
- **Honest in docs.** Don't oversell what a heuristic can do. State the blast radius.

## Code style

- Idiomatic Go. `gofmt` before committing.
- Comments explain *why*, not *what*. The code says what.
- No new runtime dependencies — the single-binary, zero-runtime promise is a feature.

## Contributor licence

By submitting a contribution to this repository, you irrevocably assign to
Tiago de Carvalho Vilas Boas all copyright in that contribution and grant the
Author the right to use, modify, and relicence it under any terms, including
proprietary terms. See Section 8 of [LICENSE](LICENSE) for the full terms.

The Author will credit significant contributors in project documentation.

## Licence

`harness-downshift` is published under the
[Business Source License 1.1](LICENSE) (BUSL-1.1), not MIT.
Non-commercial use is free. Commercial use requires a written licence from
the Author. See [LICENSE](LICENSE).
