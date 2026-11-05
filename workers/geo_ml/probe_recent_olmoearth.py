"""Small, reproducible v1.3 contract probe against real Planetary Computer data.

The model checkpoint is intentionally downloaded only when --run-model is supplied.
The default path queries and reads real recent assets, writes a georeferenced input
fixture, and records the exact scene/band contract without pretending to classify it.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import resource
import time
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import numpy as np
import planetary_computer
import pystac_client
import rasterio
try:
    import torch
except ModuleNotFoundError:  # The real-data preparation path does not need PyTorch.
    torch = None  # type: ignore[assignment]
from rasterio.enums import Resampling
from rasterio.transform import Affine
from rasterio.vrt import WarpedVRT
from rasterio.windows import Window, from_bounds
from rasterio.warp import transform as reproject_transform
from shapely.geometry import box, mapping


S2_BANDS = ["B02", "B03", "B04", "B08", "B05", "B06", "B07", "B8A", "B11", "B12", "B01", "B09"]
S1_BANDS = ["vv", "vh"]
AREA = box(-46.650, -23.580, -46.640, -23.570)
POINT_LON, POINT_LAT = -46.645, -23.575


def query_items(collection: str, start: datetime, end: datetime, limit: int = 20) -> list[Any]:
    catalog = pystac_client.Client.open("https://planetarycomputer.microsoft.com/api/stac/v1")
    search = catalog.search(
        collections=[collection],
        intersects=mapping(AREA),
        datetime=f"{start.isoformat().replace('+00:00', 'Z')}/{end.isoformat().replace('+00:00', 'Z')}",
        max_items=limit,
    )
    items = list(search.items())
    if collection == "sentinel-2-l2a":
        items = [i for i in items if float(i.properties.get("eo:cloud_cover", 100)) < 80]
        items.sort(key=lambda i: float(i.properties.get("eo:cloud_cover", 100)))
    else:
        items = [
            i
            for i in items
            if i.properties.get("sar:instrument_mode") == "IW"
            and set(i.properties.get("sar:polarizations", [])) >= {"VV", "VH"}
        ]
    # One scene per 30-day bucket keeps the probe small while preserving a time series.
    selected: list[Any] = []
    seen: set[int] = set()
    for item in items:
        dt = item.datetime
        bucket = (dt.date() - start.date()).days // 30
        if bucket not in seen:
            selected.append(item)
            seen.add(bucket)
    return sorted(selected, key=lambda i: i.datetime)[:3]


def signed_asset(item: Any, key: str) -> str:
    return planetary_computer.sign(item.assets[key].href)


def read_to_grid(url: str, crs: Any, transform: Affine, width: int, height: int, dtype: str = "float32", resampling: Resampling = Resampling.bilinear) -> np.ndarray:
    with rasterio.open(url) as src:
        with WarpedVRT(
            src,
            crs=crs,
            transform=transform,
            width=width,
            height=height,
            resampling=resampling,
            dtype=dtype,
        ) as vrt:
            return vrt.read(1).astype(np.float32)


def select_grid(item: Any, size: int = 64) -> tuple[Any, Affine, Window]:
    url = signed_asset(item, "B02")
    with rasterio.open(url) as src:
        xs, ys = reproject_transform("EPSG:4326", src.crs, [POINT_LON], [POINT_LAT])
        x, y = src.index(xs[0], ys[0])
        half = size // 2
        window = Window(max(0, x - half), max(0, y - half), size, size)
        transform = src.window_transform(window)
        return src.crs, transform, window


def read_s2(item: Any, crs: Any, transform: Affine, size: int) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    values = np.stack([read_to_grid(signed_asset(item, band), crs, transform, size, size, "float32") for band in S2_BANDS], axis=-1)
    scl = read_to_grid(signed_asset(item, "SCL"), crs, transform, size, size, "float32", Resampling.nearest)
    # SCL: 3 shadow, 8/9/10 cloud/cirrus, 11 snow/ice; 0 is no data.
    valid = (~np.isin(np.rint(scl).astype(np.int16), [0, 3, 8, 9, 10, 11])) & np.isfinite(values).all(axis=-1)
    return np.nan_to_num(values, nan=0.0), valid, scl


def read_s1(item: Any, crs: Any, transform: Affine, size: int) -> tuple[np.ndarray, np.ndarray]:
    values = np.stack([read_to_grid(signed_asset(item, band), crs, transform, size, size, "float32") for band in S1_BANDS], axis=-1)
    valid = np.isfinite(values).all(axis=-1) & (values > -32700).all(axis=-1)
    return np.nan_to_num(values, nan=0.0), valid


def bucket_index(item: Any, start: datetime) -> int:
    return (item.datetime.date() - start.date()).days // 30


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, default=Path("artifacts/probe-recent"))
    parser.add_argument("--start", type=date.fromisoformat, default=None)
    parser.add_argument("--end", type=date.fromisoformat, default=date(2026, 10, 23))
    parser.add_argument("--run-model", action="store_true")
    args = parser.parse_args()
    end = datetime.combine(args.end, datetime.max.time(), tzinfo=timezone.utc)
    start = datetime.combine(args.start or (args.end - timedelta(days=90)), datetime.min.time(), tzinfo=timezone.utc)
    args.out.mkdir(parents=True, exist_ok=True)
    started = time.perf_counter()

    s2_items = query_items("sentinel-2-l2a", start, end)
    s1_items = query_items("sentinel-1-rtc", start, end)
    if not s2_items:
        raise RuntimeError("No Sentinel-2 L2A scene in the requested period")
    crs, transform, _ = select_grid(s2_items[0])
    size = 64
    s2_by_bucket = {bucket_index(item, start): item for item in s2_items}
    s1_by_bucket = {bucket_index(item, start): item for item in s1_items}
    timeline = sorted(set(s2_by_bucket) | set(s1_by_bucket))
    timeline_dates = [min((item.datetime for item in [s2_by_bucket.get(bucket), s1_by_bucket.get(bucket)] if item is not None)) for bucket in timeline]
    s2_values: list[np.ndarray] = []
    s2_valid: list[np.ndarray] = []
    s2_scl: list[np.ndarray] = []
    for bucket in timeline:
        item = s2_by_bucket.get(bucket)
        if item is None:
            v, ok, scl = np.zeros((size, size, len(S2_BANDS)), dtype=np.float32), np.zeros((size, size), dtype=bool), np.zeros((size, size), dtype=np.float32)
        else:
            v, ok, scl = read_s2(item, crs, transform, size)
        s2_values.append(v)
        s2_valid.append(ok)
        s2_scl.append(scl)
    s2 = np.stack(s2_values, axis=2)[None, ...]  # B,H,W,T,C
    s2_ok = np.stack(s2_valid, axis=2)[None, ...]

    s1 = None
    s1_ok = None
    if s1_items:
        s1_values: list[np.ndarray] = []
        s1_valid: list[np.ndarray] = []
        for bucket in timeline:
            item = s1_by_bucket.get(bucket)
            if item is None:
                v, ok = np.zeros((size, size, len(S1_BANDS)), dtype=np.float32), np.zeros((size, size), dtype=bool)
            else:
                v, ok = read_s1(item, crs, transform, size)
            s1_values.append(v)
            s1_valid.append(ok)
        s1 = np.stack(s1_values, axis=2)[None, ...]
        s1_ok = np.stack(s1_valid, axis=2)[None, ...]

    timestamps = np.array([[item.day, item.month - 1, item.year] for item in timeline_dates], dtype=np.int64)[None, ...]
    np.savez_compressed(args.out / "inputs.npz", sentinel2=s2, sentinel2_valid=s2_ok, sentinel1=s1, sentinel1_valid=s1_ok, timestamps=timestamps)
    manifest = {
        "status": "prepared",
        "area": mapping(AREA),
        "requested_period": {"start": start.isoformat(), "end": end.isoformat()},
        "effective_period": {"start": min(i.datetime for i in s2_items).isoformat(), "end": max(i.datetime for i in s2_items).isoformat()},
        "provider": "Microsoft Planetary Computer STAC",
        "collections": {"sentinel2": "sentinel-2-l2a", "sentinel1": "sentinel-1-rtc"},
        "sentinel2": [{"id": i.id, "datetime": i.datetime.isoformat(), "cloud_cover": i.properties.get("eo:cloud_cover"), "bands": S2_BANDS} for i in s2_items],
        "sentinel1": [{"id": i.id, "datetime": i.datetime.isoformat(), "bands": S1_BANDS, "product": i.properties.get("sar:product_type")} for i in s1_items],
        "grid": {"crs": str(crs), "width": size, "height": size, "transform": list(transform), "input_gsd_m": 10},
        "input_shapes": {"sentinel2_bhw_tc": list(s2.shape), "sentinel1_bhw_tc": list(s1.shape) if s1 is not None else None, "timestamps_btd": list(timestamps.shape)},
        "valid_fraction": {"sentinel2": float(s2_ok.mean()), "sentinel1": float(s1_ok.mean()) if s1_ok is not None else 0.0},
        "sentinel2_quality_debug": {"scl_min": float(np.nanmin(s2_scl)), "scl_max": float(np.nanmax(s2_scl)), "scl_unique_sample": sorted({int(v) for v in np.rint(np.concatenate([x.ravel() for x in s2_scl])).astype(np.int16)})[:20], "finite_input_fraction": float(np.isfinite(s2).mean())},
        "runtime": {"python": platform.python_version(), "torch": torch.__version__ if torch is not None else None, "cuda_available": bool(torch is not None and torch.cuda.is_available()), "max_rss_mb": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024, "seconds_prepare": time.perf_counter() - started},
        "checkpoint": None,
        "embedding": None,
    }
    (args.out / "manifest.json").write_text(json.dumps(manifest, indent=2, default=str) + "\n")
    print(json.dumps(manifest, indent=2, default=str))
    if args.run_model:
        if torch is None:
            raise RuntimeError("--run-model requires PyTorch; install requirements-probe.txt")
        from olmoearth_pretrain.data.constants import Modality
        from olmoearth_pretrain.data.normalize import Normalizer, Strategy
        from olmoearth_pretrain.datatypes import MaskValue, MaskedOlmoEarthSample
        from olmoearth_pretrain.model_loader import load_model_from_path
        from huggingface_hub import snapshot_download

        model_dir = Path(snapshot_download("allenai/OlmoEarth-v1_3-Base", revision="9e9e918a369701c105dde7ef56789b0ff1d47f50"))
        model = load_model_from_path(model_dir).eval()
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        model.to(device)
        s2_t = torch.from_numpy(Normalizer(Strategy.COMPUTED).normalize(Modality.SENTINEL2_L2A, s2)).float().to(device)
        s2_m = torch.from_numpy(np.where(s2_ok[..., None], MaskValue.ONLINE_ENCODER.value, MaskValue.MISSING.value).repeat(3, axis=-1)).float().to(device)
        kwargs: dict[str, Any] = {"timestamps": torch.from_numpy(timestamps).to(device), "sentinel2_l2a": s2_t, "sentinel2_l2a_mask": s2_m}
        if s1 is not None:
            s1_t = torch.from_numpy(Normalizer(Strategy.COMPUTED).normalize(Modality.SENTINEL1, s1)).float().to(device)
            s1_m = torch.from_numpy(np.where(s1_ok[..., None], MaskValue.ONLINE_ENCODER.value, MaskValue.MISSING.value).repeat(1, axis=-1)).float().to(device)
            kwargs.update(sentinel1=s1_t, sentinel1_mask=s1_m)
        with torch.inference_mode():
            output = model.encoder(MaskedOlmoEarthSample(**kwargs), patch_size=4, fast_pass=True)
        embedding = output["student_location_tokens"]
        embedding_np = embedding.detach().cpu().numpy()[0].transpose(2, 0, 1)
        out_path = args.out / "embedding_128.tif"
        out_transform = transform * Affine.scale(4, 4)
        with rasterio.open(out_path, "w", driver="GTiff", height=embedding_np.shape[1], width=embedding_np.shape[2], count=128, dtype="float32", crs=crs, transform=out_transform, tiled=True, compress="deflate") as dst:
            dst.write(embedding_np)
        digest = hashlib.sha256(out_path.read_bytes()).hexdigest()
        manifest["status"] = "embedding_succeeded"
        manifest["checkpoint"] = {"repo": "allenai/OlmoEarth-v1_3-Base", "revision": "9e9e918a369701c105dde7ef56789b0ff1d47f50", "source": "HF config.json + weights.pth"}
        manifest["embedding"] = {"path": str(out_path), "shape_bchw": list(embedding_np.shape), "dimensions": 128, "prefix_64_available": True, "output_gsd_m": 40, "sha256": digest, "device": str(device), "seconds_total": time.perf_counter() - started}
        (args.out / "manifest.json").write_text(json.dumps(manifest, indent=2, default=str) + "\n")
        print(json.dumps(manifest["embedding"], indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
