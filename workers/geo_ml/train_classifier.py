"""Train and evaluate a downstream classifier on recent OlmoEarth embeddings.

Input NPZ must contain X [N,D], y [N], groups [N] and periods [N]. Groups are
required so pixels from the same farm/block cannot leak between partitions.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

import joblib
import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.metrics import confusion_matrix, f1_score, precision_score, recall_score
from sklearn.model_selection import GroupShuffleSplit


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--embedding-dim", type=int, required=True, choices=[64, 128])
    parser.add_argument("--checkpoint-revision", required=True)
    parser.add_argument("--taxonomy", required=True)
    parser.add_argument("--sensors", nargs="+", required=True)
    args = parser.parse_args()

    data = np.load(args.dataset, allow_pickle=False)
    required = {"X", "y", "groups", "periods"}
    if not required.issubset(data.files):
        raise ValueError(f"dataset must contain {sorted(required)}")
    X, y, groups, periods = data["X"], data["y"], data["groups"], data["periods"]
    if X.ndim != 2 or X.shape[1] != args.embedding_dim:
        raise ValueError(f"expected X [N,{args.embedding_dim}], got {X.shape}")
    if len({len(y), len(groups), len(periods), X.shape[0]}) != 1:
        raise ValueError("X, y, groups and periods must have the same length")
    if len(np.unique(y)) < 2:
        raise ValueError("at least two labeled classes are required")

    outer = GroupShuffleSplit(n_splits=1, test_size=0.2, random_state=42)
    train_idx, test_idx = next(outer.split(X, y, groups=groups))
    inner = GroupShuffleSplit(n_splits=1, test_size=0.25, random_state=43)
    train_rel, valid_rel = next(inner.split(X[train_idx], y[train_idx], groups=groups[train_idx]))
    train, valid = train_idx[train_rel], train_idx[valid_rel]
    model = RandomForestClassifier(n_estimators=300, class_weight="balanced_subsample", random_state=42, n_jobs=-1)
    model.fit(X[train], y[train])
    metrics = evaluate(model, X, y, {"train": train, "validation": valid, "test": test_idx})
    args.out.mkdir(parents=True, exist_ok=True)
    model_path = args.out / "classifier.joblib"
    joblib.dump(model, model_path)
    metadata = {
        "status": "draft",
        "classifier": "RandomForestClassifier",
        "checkpoint_revision": args.checkpoint_revision,
        "embedding_dimensions": args.embedding_dim,
        "sensors": args.sensors,
        "taxonomy": args.taxonomy,
        "temporal_policy": {"requires_recent_embeddings": True, "period_field": "periods", "annual_embeddings_not_accepted": True},
        "split_policy": {"method": "GroupShuffleSplit", "group_field": "groups", "random_state": 42, "test_fraction": 0.2, "validation_fraction_of_train": 0.25},
        "classes": [str(v) for v in model.classes_],
        "metrics": metrics,
        "artifact": {"path": str(model_path), "sha256": hashlib.sha256(model_path.read_bytes()).hexdigest()},
    }
    (args.out / "model.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print(json.dumps(metadata, indent=2))
    return 0


def evaluate(model: RandomForestClassifier, X: np.ndarray, y: np.ndarray, splits: dict[str, np.ndarray]) -> dict:
    output: dict[str, dict] = {}
    for name, idx in splits.items():
        predicted = model.predict(X[idx])
        labels = [str(v) for v in model.classes_]
        matrix = confusion_matrix(y[idx], predicted, labels=model.classes_).tolist()
        output[name] = {
            "count": int(len(idx)),
            "macro_f1": float(f1_score(y[idx], predicted, average="macro", zero_division=0)),
            "macro_precision": float(precision_score(y[idx], predicted, average="macro", zero_division=0)),
            "macro_recall": float(recall_score(y[idx], predicted, average="macro", zero_division=0)),
            "classes": labels,
            "confusion_matrix": matrix,
        }
    return output


if __name__ == "__main__":
    raise SystemExit(main())
