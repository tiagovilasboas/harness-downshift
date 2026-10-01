"""Serve the fine-tuned DistilBERT as the downshift middle layer.

Protocol (consumed by internal/core/distil.go, stdlib HTTP, no new Go deps):

    POST /classify  {"text": "..."}  ->  {"label": "SIMPLE", "confidence": 0.82}
    GET  /healthz                     ->  {"ok": true, "model": "<dir>"}

Usage:
    pip install -r scripts/requirements-distil.txt
    python scripts/train_distil.py            # once, produces models/distil-complexity
    python scripts/distil_serve.py [--model models/distil-complexity] [--port 8001]

Fail-open contract: the Go hook treats every non-200, timeout, or malformed
response as "sidecar unavailable" and keeps the v1 regex result.
"""

import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

LABELS = ["TRIVIAL", "SIMPLE", "MEDIUM", "COMPLEX"]


class Handler(BaseHTTPRequestHandler):
    model = None
    tok = None
    model_name = ""

    def log_message(self, *args):
        pass  # keep the sidecar quiet; hook telemetry lives in Go

    def _send(self, code: int, payload: dict) -> None:
        raw = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path == "/healthz":
            self._send(200, {"ok": True, "model": self.model_name})
        else:
            self._send(404, {"ok": False})

    def do_POST(self):
        if self.path != "/classify":
            self._send(404, {"ok": False})
            return
        try:
            length = int(self.headers.get("Content-Length", 0))
        except ValueError:
            length = 0
        if length <= 0 or length > 1 << 20:
            self._send(400, {"error": "bad body"})
            return
        try:
            text = json.loads(self.rfile.read(length).decode())["text"]
        except (KeyError, ValueError, UnicodeDecodeError):
            self._send(400, {"error": "want {\"text\": string}"})
            return

        import torch

        inputs = self.tok(text, return_tensors="pt", truncation=True, max_length=128)
        with torch.no_grad():
            logits = self.model(**inputs).logits[0]
        probs = torch.softmax(logits, dim=-1)
        idx = int(probs.argmax())
        self._send(
            200, {"label": LABELS[idx], "confidence": round(float(probs[idx]), 4)}
        )


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="models/distil-complexity")
    ap.add_argument("--port", type=int, default=8001)
    args = ap.parse_args()

    from transformers import AutoModelForSequenceClassification, AutoTokenizer

    model_dir = Path(args.model)
    if not model_dir.exists():
        raise SystemExit(
            f"model dir {model_dir} not found — run scripts/train_distil.py first"
        )
    Handler.model = AutoModelForSequenceClassification.from_pretrained(model_dir)
    Handler.model.eval()
    Handler.tok = AutoTokenizer.from_pretrained(model_dir)
    Handler.model_name = str(model_dir)

    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"distil sidecar: {model_dir} on 127.0.0.1:{args.port}")
    server.serve_forever()


if __name__ == "__main__":
    main()
