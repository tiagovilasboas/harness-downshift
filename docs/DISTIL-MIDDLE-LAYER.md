# Distil middle layer

The classification cascade, cheap to expensive:

```text
regex/softmax (µs) → DistilBERT sidecar (tens of ms) → Laya/Jev (ambiguous) → LLM (never, for routing)
```

The regex classifier (`Classify` in `internal/core/classifier.go`) decides
instantly when signals are clear. The DistilBERT sidecar exists for the
middle: prompts where the regex is **not confident**. The LLM stays out of
the routing loop entirely.

## When the sidecar runs

In `classifyTask` (`internal/core/policy.go`):

1. Regex classifies. Confident → answer stands, sidecar never called.
2. Not confident **and** `DOWNSHIFT_DISTIL_URL` is set → `POST` the prompt
   to `<url>/classify`, accept the label when confidence ≥ 0.6.
3. Any failure (unset env, timeout, non-200, unknown label, low confidence)
   → keep the v1 regex result. **Fail-open, never block a spawn.**

Without the env var the binary behaves exactly like v1: zero behavior
change, zero new Go dependencies (stdlib HTTP client only).

## Run it

```bash
pip install -r scripts/requirements-distil.txt
python scripts/train_distil.py            # trains on benchmark/tasks.json, evals on benchmark/holdout.json
python scripts/distil_serve.py --port 8001

DOWNSHIFT_DISTIL_URL=http://localhost:8001 downshift try "help me with this"
```

Prompts include Portuguese: for production prefer
`--model distilbert-base-multilingual-cased` at train time.

## Contract

- Request: `POST /classify` with `{"text": "..."}`
- Response: `{"label": "TRIVIAL|SIMPLE|MEDIUM|COMPLEX", "confidence": 0.0-1.0}`
- Labels map 1:1 to `core.Complexity` (`parseDistilLabel`).
- Timeout: 300ms (`distilTimeout`). Threshold: 0.6 (`distilMinConfidence`).
- `GET /healthz` for probes.

## Why DistilBERT in the middle

Small fine-tuned models beat large generatives on fixed-label intent:
DistilBERT 0.889 accuracy at 5.3ms on CLINC-150 (151 intents), F1 ~0.92 vs
~0.88 (Phi-2) and ~0.83 (Llama-3-8B) on Banking-77/CLINC-150. The middle
layer buys that accuracy for tens of milliseconds on CPU, no GPU, no tokens.

## Next: Laya as the ambiguous tier

DistilBERT resolves the regex ties. What remains ambiguous (low sidecar
confidence) is the slot for Laya/Jev: a System One decision call with
`choice`/`noul` questions instead of an LLM judge. Same fail-open shape,
same `/classify`-style gate, different backend.
