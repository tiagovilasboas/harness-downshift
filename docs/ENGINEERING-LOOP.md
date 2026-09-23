# Engineering loop readiness

`harness-downshift` by Tiago de Carvalho Vilas Boas
https://github.com/tiagovilasboas/harness-downshift

## Current state

Downshift can make a deterministic routing decision at the harness hook boundary
for Claude Code, Cursor, and Codex. It records local routing metadata without
storing task prompts. The adapters share the same routing policy, so safety
changes in `internal/core` apply consistently across all three harnesses.

This is a **routing loop**, not yet a complete **engineering feedback loop**.
The current event log says which model was selected; it does not say whether the
result was correct, complete, or required rework. Training on routing events
alone therefore cannot establish engineering quality.

## Safety gates already in the runtime

- Hook payload reads are limited to 1 MiB. Invalid or oversized input follows
  each harness's fail-open response.
- Local routing event files are written with owner-only permissions.
- A low-confidence decision cannot automatically downgrade the harness's
  selected model. Upshifts and explicit-only model preservation keep their
  existing behavior.
- Training datasets reject unknown labels and empty prompts instead of
  silently treating unknown labels as `MID`.

## Promotion loop

1. Collect routing metadata locally; never collect prompts by default.
2. Let an engineer label outcome quality and rework separately from predicted
   complexity. Keep this feedback opt-in and local unless a future sharing
   mechanism is explicitly designed.
3. Build a versioned, stratified dataset with reviewed labels and a held-out
   evaluation set that is not used for training or tuning.
4. Compare a candidate router against the current policy per harness. Track
   unsafe downgrades, missed upshifts, task success, rework, and cost separately.
5. Promote only after the candidate improves cost without regressing the
   agreed quality and safety gates; retain the previous router for rollback.

There must be no online weight updates from a single run. A hook decision is
not evidence that the selected model succeeded, and an unreviewed label can
otherwise teach the router to repeat a bad choice.

## Current promotion decision

The small seed benchmark is not a promotion-grade held-out set. Its latest
comparison showed the candidate router v2 at 43.3% tier accuracy versus 70.0%
for the legacy router, with unsafe downgrades at 71.4% versus 42.9%. Keep the
legacy router as the default and treat v2 as experimental until a reviewed,
independent dataset and per-harness quality results show improvement.

## Remaining work before calling this an engineering loop

- Define a minimal outcome-feedback format that records correctness and rework
  without copying task prompts.
- Make train/validation splitting stratified and reproducible, and report class
  counts so small datasets cannot produce misleading metrics.
- Add per-harness contract checks for model preservation, fail-open behavior,
  and protocol output to CI.
- Establish a promotion threshold for unsafe downgrades and quality regression,
  then require explicit review of candidate weights before release.
