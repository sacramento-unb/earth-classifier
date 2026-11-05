"""Apply only a compatible, explicitly approved classifier to an embedding COG."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import joblib
import numpy as np
import rasterio


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--embedding", type=Path, required=True)
    parser.add_argument("--model", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--expected-dim", type=int, required=True, choices=[64, 128])
    parser.add_argument("--expected-checkpoint", required=True)
    parser.add_argument("--min-confidence", type=float, default=0.60)
    args = parser.parse_args()
    metadata = json.loads((args.model.parent / "model.json").read_text())
    if metadata.get("status") != "approved":
        raise RuntimeError("classifier is not approved; operational publication is blocked")
    if metadata.get("embedding_dimensions") != args.expected_dim or metadata.get("checkpoint_revision") != args.expected_checkpoint:
        raise RuntimeError("model contract mismatch: embedding dimension/checkpoint is incompatible")
    model = joblib.load(args.model)
    with rasterio.open(args.embedding) as src:
        features = src.read().astype("float32")
        profile = src.profile.copy()
        profile.update(count=2, dtype="uint8", nodata=255, compress="deflate")
        flat = features.reshape(features.shape[0], -1).T
        probability = model.predict_proba(flat)
        best = probability.argmax(axis=1)
        confidence = probability.max(axis=1)
        classes = np.asarray(model.classes_).astype("uint8")
        labels = np.where(confidence >= args.min_confidence, classes[best], 254).astype("uint8")
        output = labels.reshape(features.shape[1:])
        quality = np.clip(confidence * 100, 0, 100).astype("uint8").reshape(features.shape[1:])
        with rasterio.open(args.out, "w", **profile) as dst:
            dst.write(output, 1)
            dst.write(quality, 2)
            dst.set_band_description(1, "class_code; 254=undetermined")
            dst.set_band_description(2, "confidence_score_percent; score, not accuracy")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
