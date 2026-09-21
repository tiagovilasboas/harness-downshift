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

We turn accepted misroutes into test cases, then tune the signals until they
pass. Your prompt becomes a permanent regression guard.

## Project layout

```
cmd/downshift/                 # the binary: hook mode + `try` command
internal/core/                 # harness-agnostic brain
  classifier.go                #   task text → complexity (scored, deterministic)
  models.go                    #   per-harness model catalog by tier
  policy.go                    #   complexity → tier → decision (Route)
internal/adapters/<harness>/   # one adapter per harness
  claudecode/                  #   translates a core.Decision into the harness's mechanism
```

**The rule:** `core` never imports an adapter. Adapters depend on `core`, not
the other way around. All routing logic lives in `core`; adapters only translate.

## Adding a new harness adapter

Use `internal/adapters/claudecode/` as the template. An adapter:

1. Decodes the harness's subagent-spawn event.
2. Extracts the subagent's task text and current model.
3. Calls `core.Route(prompt, harnessID, currentModel)`.
4. Translates the returned `core.Decision` into the harness's own mechanism
   (a hook rewrite, a config write, a spawn flag).
5. **Fails open** — any error lets the subagent run unchanged.

Register the harness's models in `internal/core/models.go` (`Catalog`) so tier
resolution works for it.

## Development

```bash
make build     # compile the binary
make test      # go test -race -cover ./...
make vet       # go vet ./...
make build-all # cross-compile for macOS / Linux / Windows
```

Requires Go 1.27+.

## Ground rules

- **Keep tests green.** `go test -race -cover ./...` must pass.
- **Every classifier change ships a test.** Add a table-driven case that proves
  the improvement and guards against regressions.
- **Small, focused PRs.** One concern per PR.
- **Conventional commits in English.** `feat(core): ...`, `fix(claudecode): ...`,
  `docs: ...`.
- **Fail-open, always.** A cost optimizer must never become an availability
  risk. If in doubt, let the subagent run unchanged.
- **Be honest in docs.** Don't oversell what a heuristic can do. State the blast
  radius.

## Code style

- Idiomatic Go. `gofmt` before committing.
- Comments explain *why*, not *what*. The code says what.
- No new dependencies without a clear reason — the single-binary, zero-runtime
  promise is a feature.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
