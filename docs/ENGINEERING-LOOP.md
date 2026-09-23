# Engineering loop readiness

`harness-downshift` by Tiago de Carvalho Vilas Boas
https://github.com/tiagovilasboas/harness-downshift

## Current state

Downshift can make a deterministic routing decision at the harness hook boundary
for Claude Code, Cursor, and Codex. It records local routing metadata without
storing task prompts. The adapters share the same routing policy, so safety
changes in `internal/core` apply consistently across all three harnesses.

The runtime now supports a **manual, harness-agnostic feedback loop**. Each
routed task gets an opaque feedback ID. An engineer can mark the result as
success, retry, or failed with `downshift feedback`; only explicit minimum-tier
labels enter training. This records reviewed outcomes without storing prompts.
The loop does not receive completion signals from harnesses automatically.

## Safety gates already in the runtime

- Hook payload reads are limited to 1 MiB. Invalid or oversized input follows
  each harness's fail-open response.
- Local routing event files are written with owner-only permissions.
- A low-confidence decision cannot automatically downgrade the harness's
  selected model. Upshifts and explicit-only model preservation keep their
  existing behavior.
- Training datasets reject unknown labels and empty prompts instead of
  silently treating unknown labels as `MID`.
- Feedback events are append-only in `~/.harness-downshift/loop-events.jsonl`;
  they are separate from the cost telemetry log.
- Candidate weights can be evaluated with `benchmark --compare
  --candidate-weights=...`; evaluation never activates them.
- Train/validation splitting is deterministic and stratified. Weights are
  validated for finite bounded coefficients and saved atomically with private
  file permissions.

## Promotion loop

1. Collect routing metadata locally; never collect prompts by default.
2. After reviewing a task, record `success`, `retry`, or `failed` with its
   feedback ID. Add `--required-tier=...` only when an engineer has reviewed the
   minimum required tier. A successful run alone proves sufficiency, not a
   minimal tier.
3. Train a candidate with `downshift train --from-events --output=candidate.json`.
4. Compare candidate weights with the legacy policy on a separate held-out
   dataset using `downshift benchmark holdout.json --compare
   --candidate-weights=candidate.json`. Track
   unsafe downgrades, missed upshifts, task success, rework, and cost separately.
5. Promote only after the candidate improves cost without regressing the
   agreed quality and safety gates; retain the previous router for rollback.

There are no online weight updates from a single run. A hook decision is not
evidence that the selected model succeeded, and an unreviewed label can teach
the router to repeat a bad choice. Outcome integration with harness completion
events remains future work; the current common denominator is explicit review
through the CLI.

## Current promotion decision

The small seed benchmark is not a promotion-grade held-out set. Its latest
comparison showed the candidate router v2 at 43.3% tier accuracy versus 70.0%
for the legacy router, with unsafe downgrades at 71.4% versus 42.9%. Keep the
legacy router as the default and treat v2 as experimental until a reviewed,
independent dataset and per-harness quality results show improvement.

## Remaining work for a closed-loop system

- Integrate harness completion/retry signals where each harness has a stable
  event contract, while preserving the shared outcome schema and opt-in review.
- Report per-harness outcome rates and sample counts so small feedback samples
  cannot imply more certainty than they support.
- Add per-harness contract checks for model preservation, fail-open behavior,
  and protocol output to CI.
- Establish a promotion threshold for unsafe downgrades and quality regression,
  then require explicit review of candidate weights before release.
