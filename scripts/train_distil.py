"""Fine-tune DistilBERT on downshift complexity labels.

Trains on benchmark/tasks.json (prompt + TRIVIAL/SIMPLE/MEDIUM/COMPLEX)
and evaluates on benchmark/holdout.json when present.

Usage:
    pip install -r scripts/requirements-distil.txt
    python scripts/train_distil.py [--model distilbert-base-uncased] [--out models/distil-complexity] [--epochs 5]

Prompts include Portuguese, so for production prefer a multilingual base:
    python scripts/train_distil.py --model distilbert-base-multilingual-cased

The saved directory is loaded by scripts/distil_serve.py and maps 1:1 to
internal/core.Complexity (TRIVIAL/SIMPLE/MEDIUM/COMPLEX).
"""

import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def load_pairs(path: Path) -> list[tuple[str, str]]:
    rows = json.loads(path.read_text())
    return [(r["prompt"], r["label"].upper()) for r in rows]


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="distilbert-base-uncased")
    ap.add_argument("--out", default="models/distil-complexity")
    ap.add_argument("--epochs", type=int, default=5)
    args = ap.parse_args()

    from datasets import Dataset
    from transformers import (
        AutoModelForSequenceClassification,
        AutoTokenizer,
        Trainer,
        TrainingArguments,
    )

    labels = ["TRIVIAL", "SIMPLE", "MEDIUM", "COMPLEX"]
    label2id = {label: i for i, label in enumerate(labels)}

    train_rows = load_pairs(ROOT / "benchmark" / "tasks.json")
    train_ds = Dataset.from_dict(
        {
            "text": [p for p, _ in train_rows],
            "label": [label2id[lbl] for _, lbl in train_rows],
        }
    )

    eval_ds = None
    holdout = ROOT / "benchmark" / "holdout.json"
    if holdout.exists():
        holdout_rows = load_pairs(holdout)
        eval_ds = Dataset.from_dict(
            {
                "text": [p for p, _ in holdout_rows],
                "label": [label2id[lbl] for _, lbl in holdout_rows],
            }
        )

    tok = AutoTokenizer.from_pretrained(args.model)

    def tokenize(batch):
        return tok(batch["text"], truncation=True, padding="max_length", max_length=128)

    train_ds = train_ds.map(tokenize, batched=True)
    if eval_ds is not None:
        eval_ds = eval_ds.map(tokenize, batched=True)

    model = AutoModelForSequenceClassification.from_pretrained(
        args.model,
        num_labels=4,
        id2label={i: lbl for i, lbl in enumerate(labels)},
        label2id=label2id,
    )

    out_dir = ROOT / args.out
    trainer = Trainer(
        model=model,
        args=TrainingArguments(
            output_dir=str(out_dir / "checkpoints"),
            num_train_epochs=args.epochs,
            per_device_train_batch_size=16,
            learning_rate=3e-5,
            eval_strategy="epoch" if eval_ds is not None else "no",
            save_strategy="epoch",
            load_best_model_at_end=eval_ds is not None,
            seed=42,
        ),
        train_dataset=train_ds,
        eval_dataset=eval_ds,
        tokenizer=tok,
    )
    trainer.train()

    if eval_ds is not None:
        print(trainer.evaluate())

    trainer.save_model(str(out_dir))
    tok.save_pretrained(str(out_dir))
    print(f"saved to {out_dir}")


if __name__ == "__main__":
    main()
