#!/usr/bin/env python3
"""Crop testing rig — visualize and compare cropping strategies on pose-estimated video.

Loads a video + keypoints.json, applies pluggable crop strategies, and produces
side-by-side comparison screenshots (original with crop overlay + cropped result).

Usage:
    uv run spikes/crop-rig/crop_rig.py <video> <keypoints.json> [options]

    # Run with baseline strategy, sample 5 frames
    uv run spikes/crop-rig/crop_rig.py testdata/videos/sample-lift.mp4 testdata/keypoints-sample.json

    # Run a specific strategy
    uv run spikes/crop-rig/crop_rig.py video.mp4 kp.json --strategy percentile

    # Run all strategies for comparison
    uv run spikes/crop-rig/crop_rig.py video.mp4 kp.json --strategy all

    # Sample more frames
    uv run spikes/crop-rig/crop_rig.py video.mp4 kp.json --num-frames 10
"""

import argparse
import json
import math
import os
import sys
from dataclasses import dataclass
from typing import Callable

import cv2
import numpy as np


# --- Data types ---

@dataclass
class CropRect:
    x: int
    y: int
    w: int
    h: int


@dataclass
class BBox:
    left: float
    top: float
    right: float
    bottom: float


@dataclass
class Frame:
    time_offset_ms: int
    bounding_box: BBox
    keypoints: list[dict]


@dataclass
class PoseResult:
    source_width: int
    source_height: int
    frames: list[Frame]


# --- Load data ---

def load_keypoints(path: str) -> PoseResult:
    with open(path) as f:
        data = json.load(f)
    frames = []
    for fd in data["frames"]:
        bb = fd["boundingBox"]
        frames.append(Frame(
            time_offset_ms=fd["timeOffsetMs"],
            bounding_box=BBox(bb["left"], bb["top"], bb["right"], bb["bottom"]),
            keypoints=fd.get("keypoints", []),
        ))
    return PoseResult(
        source_width=data["sourceWidth"],
        source_height=data["sourceHeight"],
        frames=frames,
    )


def sample_frame_indices(total_frames: int, num_samples: int) -> list[int]:
    """Pick evenly-spaced frame indices, always including first and last."""
    if total_frames <= num_samples:
        return list(range(total_frames))
    step = (total_frames - 1) / (num_samples - 1)
    return [int(round(i * step)) for i in range(num_samples)]


def read_video_frame(cap: cv2.VideoCapture, frame_idx: int) -> np.ndarray | None:
    cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
    ret, frame = cap.read()
    return frame if ret else None


# --- Crop strategies ---
# Each strategy: (frames, source_w, source_h) -> CropRect

StrategyFn = Callable[[list[Frame], int, int], CropRect]

STRATEGIES: dict[str, StrategyFn] = {}


def strategy(name: str):
    """Decorator to register a crop strategy."""
    def decorator(fn: StrategyFn) -> StrategyFn:
        STRATEGIES[name] = fn
        return fn
    return decorator


def _enforce_aspect_and_clamp(cx: float, cy: float, box_w: float, box_h: float,
                               sw: float, sh: float,
                               aspect_w: int = 9, aspect_h: int = 16) -> CropRect:
    """Shared logic: enforce aspect ratio, re-center, clamp, round to even."""
    target_ratio = aspect_w / aspect_h
    current_ratio = box_w / box_h if box_h > 0 else target_ratio

    if current_ratio > target_ratio:
        box_h = box_w / target_ratio
    else:
        box_w = box_h * target_ratio

    px_left = cx - box_w / 2
    px_top = cy - box_h / 2

    # Clamp to frame
    if px_left < 0:
        px_left = 0
    if px_top < 0:
        px_top = 0
    if px_left + box_w > sw:
        px_left = sw - box_w
    if px_top + box_h > sh:
        px_top = sh - box_h
    if px_left < 0:
        px_left = 0
        box_w = sw
    if px_top < 0:
        px_top = 0
        box_h = sh

    x = int(round(px_left))
    y = int(round(px_top))
    w = int(round(box_w))
    h = int(round(box_h))
    if w % 2 != 0:
        w -= 1
    if h % 2 != 0:
        h -= 1

    return CropRect(x, y, w, h)


@strategy("baseline")
def baseline_strategy(frames: list[Frame], sw: int, sh: int) -> CropRect:
    """Current Go production logic: enclosing bbox + 2% padding + 9:16 aspect."""
    padding = 0.02

    min_left = min(f.bounding_box.left for f in frames)
    min_top = min(f.bounding_box.top for f in frames)
    max_right = max(f.bounding_box.right for f in frames)
    max_bottom = max(f.bounding_box.bottom for f in frames)

    px_left = min_left * sw
    px_top = min_top * sh
    px_right = max_right * sw
    px_bottom = max_bottom * sh

    box_w = px_right - px_left
    box_h = px_bottom - px_top

    pad_w = box_w * padding
    pad_h = box_h * padding
    px_left -= pad_w
    px_top -= pad_h
    px_right += pad_w
    px_bottom += pad_h

    box_w = px_right - px_left
    box_h = px_bottom - px_top

    cx = (px_left + px_right) / 2
    cy = (px_top + px_bottom) / 2

    return _enforce_aspect_and_clamp(cx, cy, box_w, box_h, float(sw), float(sh))


@strategy("percentile")
def percentile_strategy(frames: list[Frame], sw: int, sh: int) -> CropRect:
    """Use 5th/95th percentile of bounding boxes instead of min/max to reject outliers."""
    padding = 0.03

    lefts = [f.bounding_box.left for f in frames]
    tops = [f.bounding_box.top for f in frames]
    rights = [f.bounding_box.right for f in frames]
    bottoms = [f.bounding_box.bottom for f in frames]

    p_left = float(np.percentile(lefts, 5))
    p_top = float(np.percentile(tops, 5))
    p_right = float(np.percentile(rights, 95))
    p_bottom = float(np.percentile(bottoms, 95))

    px_left = p_left * sw
    px_top = p_top * sh
    px_right = p_right * sw
    px_bottom = p_bottom * sh

    box_w = px_right - px_left
    box_h = px_bottom - px_top

    pad_w = box_w * padding
    pad_h = box_h * padding
    px_left -= pad_w
    px_top -= pad_h
    box_w += 2 * pad_w
    box_h += 2 * pad_h

    cx = px_left + box_w / 2
    cy = px_top + box_h / 2

    return _enforce_aspect_and_clamp(cx, cy, box_w, box_h, float(sw), float(sh))


@strategy("median-center")
def median_center_strategy(frames: list[Frame], sw: int, sh: int) -> CropRect:
    """Center on the median bbox center; size from IQR of bbox dimensions."""
    padding = 0.05

    centers_x = [(f.bounding_box.left + f.bounding_box.right) / 2 for f in frames]
    centers_y = [(f.bounding_box.top + f.bounding_box.bottom) / 2 for f in frames]
    widths = [f.bounding_box.right - f.bounding_box.left for f in frames]
    heights = [f.bounding_box.bottom - f.bounding_box.top for f in frames]

    cx = float(np.median(centers_x)) * sw
    cy = float(np.median(centers_y)) * sh

    # Use 90th percentile width/height to cover most poses
    box_w = float(np.percentile(widths, 90)) * sw
    box_h = float(np.percentile(heights, 90)) * sh

    box_w *= (1 + 2 * padding)
    box_h *= (1 + 2 * padding)

    return _enforce_aspect_and_clamp(cx, cy, box_w, box_h, float(sw), float(sh))


# --- Visualization ---

COLOR_BBOX = (0, 255, 0)       # green - per-frame bounding box
COLOR_CROP = (0, 0, 255)       # red - crop rectangle
COLOR_CROP_FILL = (0, 0, 255)  # red with alpha
COLOR_KEYPOINT = (255, 0, 255) # magenta


def draw_overlay(frame: np.ndarray, pose_frame: Frame, crop: CropRect,
                 sw: int, sh: int, strategy_name: str) -> np.ndarray:
    """Draw bounding box, keypoints, and crop rectangle on a frame."""
    vis = frame.copy()
    h_frame, w_frame = vis.shape[:2]

    # Scale factors if frame doesn't match source dims (shouldn't happen, but safe)
    sx = w_frame / sw
    sy = h_frame / sh

    # Per-frame bounding box (green)
    bb = pose_frame.bounding_box
    bb_x1 = int(bb.left * sw * sx)
    bb_y1 = int(bb.top * sh * sy)
    bb_x2 = int(bb.right * sw * sx)
    bb_y2 = int(bb.bottom * sh * sy)
    cv2.rectangle(vis, (bb_x1, bb_y1), (bb_x2, bb_y2), COLOR_BBOX, 2)

    # Keypoints (magenta dots)
    for kp in pose_frame.keypoints:
        if kp.get("confidence", 0) > 0.3:
            kx = int(kp["x"] * sw * sx)
            ky = int(kp["y"] * sh * sy)
            cv2.circle(vis, (kx, ky), 4, COLOR_KEYPOINT, -1)

    # Crop rectangle (red, thicker)
    cx1 = int(crop.x * sx)
    cy1 = int(crop.y * sy)
    cx2 = int((crop.x + crop.w) * sx)
    cy2 = int((crop.y + crop.h) * sy)
    cv2.rectangle(vis, (cx1, cy1), (cx2, cy2), COLOR_CROP, 3)

    # Dim area outside crop
    overlay = vis.copy()
    mask = np.ones_like(vis, dtype=np.uint8) * 60
    mask[cy1:cy2, cx1:cx2] = 0
    vis = cv2.subtract(vis, mask)

    # Label
    label = f"{strategy_name} | crop: {crop.w}x{crop.h}+{crop.x}+{crop.y}"
    cv2.putText(vis, label, (10, 30), cv2.FONT_HERSHEY_SIMPLEX, 0.7, (255, 255, 255), 2)

    return vis


def extract_crop(frame: np.ndarray, crop: CropRect, sw: int, sh: int) -> np.ndarray:
    """Extract the cropped region from a frame."""
    h_frame, w_frame = frame.shape[:2]
    sx = w_frame / sw
    sy = h_frame / sh

    x1 = int(crop.x * sx)
    y1 = int(crop.y * sy)
    x2 = int((crop.x + crop.w) * sx)
    y2 = int((crop.y + crop.h) * sy)

    x1 = max(0, min(x1, w_frame))
    y1 = max(0, min(y1, h_frame))
    x2 = max(0, min(x2, w_frame))
    y2 = max(0, min(y2, h_frame))

    return frame[y1:y2, x1:x2]


def make_comparison(frame: np.ndarray, pose_frame: Frame, crop: CropRect,
                    sw: int, sh: int, strategy_name: str) -> np.ndarray:
    """Create side-by-side: overlay on left, cropped on right."""
    overlay = draw_overlay(frame, pose_frame, crop, sw, sh, strategy_name)

    cropped = extract_crop(frame, crop, sw, sh)
    if cropped.size == 0:
        return overlay

    # Scale cropped to same height as overlay for side-by-side
    target_h = overlay.shape[0]
    scale = target_h / cropped.shape[0]
    target_w = int(cropped.shape[1] * scale)
    cropped_resized = cv2.resize(cropped, (target_w, target_h))

    return np.hstack([overlay, cropped_resized])


def make_strategy_grid(frame: np.ndarray, pose_frame: Frame,
                       strategies: dict[str, CropRect],
                       sw: int, sh: int) -> np.ndarray:
    """Create a grid comparing all strategies on the same frame."""
    panels = []
    for name, crop in strategies.items():
        panel = make_comparison(frame, pose_frame, crop, sw, sh, name)
        panels.append(panel)

    if not panels:
        return frame

    # Stack vertically, padding to same width
    max_w = max(p.shape[1] for p in panels)
    padded = []
    for p in panels:
        if p.shape[1] < max_w:
            pad = np.zeros((p.shape[0], max_w - p.shape[1], 3), dtype=np.uint8)
            p = np.hstack([p, pad])
        padded.append(p)

    return np.vstack(padded)


# --- Main ---

def run_rig(video_path: str, keypoints_path: str, strategy_names: list[str],
            num_frames: int, output_dir: str, scale: float):
    pose_result = load_keypoints(keypoints_path)
    sw = pose_result.source_width
    sh = pose_result.source_height

    print(f"Video source: {sw}x{sh}, {len(pose_result.frames)} pose frames", file=sys.stderr)

    # Compute crops for requested strategies
    crops: dict[str, CropRect] = {}
    for name in strategy_names:
        fn = STRATEGIES[name]
        crop = fn(pose_result.frames, sw, sh)
        crops[name] = crop
        print(f"Strategy '{name}': crop {crop.w}x{crop.h}+{crop.x}+{crop.y}", file=sys.stderr)

    # Open video
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        print(f"Error: cannot open video {video_path}", file=sys.stderr)
        sys.exit(1)

    total_video_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    native_fps = cap.get(cv2.CAP_PROP_FPS)

    # Map pose frames to video frames
    pose_frame_indices = sample_frame_indices(len(pose_result.frames), num_frames)

    os.makedirs(output_dir, exist_ok=True)

    for i, pf_idx in enumerate(pose_frame_indices):
        pose_frame = pose_result.frames[pf_idx]

        # Map pose frame time to video frame index
        video_frame_idx = int(pose_frame.time_offset_ms / 1000.0 * native_fps)
        video_frame_idx = min(video_frame_idx, total_video_frames - 1)

        frame = read_video_frame(cap, video_frame_idx)
        if frame is None:
            print(f"Warning: could not read video frame {video_frame_idx}", file=sys.stderr)
            continue

        # Apply scale for faster processing / smaller output
        if scale != 1.0:
            new_w = int(frame.shape[1] * scale)
            new_h = int(frame.shape[0] * scale)
            frame = cv2.resize(frame, (new_w, new_h))
            # Scale source dims accordingly for overlay drawing
            eff_sw = int(sw * scale)
            eff_sh = int(sh * scale)
            eff_crops = {
                name: CropRect(
                    int(c.x * scale), int(c.y * scale),
                    int(c.w * scale), int(c.h * scale)
                )
                for name, c in crops.items()
            }
        else:
            eff_sw, eff_sh = sw, sh
            eff_crops = crops

        if len(strategy_names) == 1:
            name = strategy_names[0]
            result = make_comparison(frame, pose_frame, eff_crops[name], eff_sw, eff_sh, name)
        else:
            result = make_strategy_grid(frame, pose_frame, eff_crops, eff_sw, eff_sh)

        time_sec = pose_frame.time_offset_ms / 1000.0
        filename = f"frame_{i:03d}_t{time_sec:.1f}s.png"
        out_path = os.path.join(output_dir, filename)
        cv2.imwrite(out_path, result)
        print(f"Saved {out_path}", file=sys.stderr)

    cap.release()

    # Write a summary JSON for programmatic comparison
    summary = {
        "video": video_path,
        "keypoints": keypoints_path,
        "source_dims": [sw, sh],
        "strategies": {name: {"x": c.x, "y": c.y, "w": c.w, "h": c.h} for name, c in crops.items()},
        "num_screenshots": len(pose_frame_indices),
        "output_dir": output_dir,
    }
    summary_path = os.path.join(output_dir, "summary.json")
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"\nSummary: {summary_path}", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description="Crop testing rig")
    parser.add_argument("video_path", help="Path to input video")
    parser.add_argument("keypoints_path", help="Path to keypoints.json")
    parser.add_argument("--strategy", default="all",
                        help=f"Strategy name or 'all' (available: {', '.join(STRATEGIES.keys())})")
    parser.add_argument("--num-frames", type=int, default=5,
                        help="Number of frames to sample (default: 5)")
    parser.add_argument("--output-dir", default=None,
                        help="Output directory (default: spikes/crop-rig/output/<strategy>)")
    parser.add_argument("--scale", type=float, default=0.5,
                        help="Scale factor for output images (default: 0.5)")
    args = parser.parse_args()

    if args.strategy == "all":
        strategy_names = list(STRATEGIES.keys())
    else:
        strategy_names = [args.strategy]
        for name in strategy_names:
            if name not in STRATEGIES:
                print(f"Unknown strategy: {name}", file=sys.stderr)
                print(f"Available: {', '.join(STRATEGIES.keys())}", file=sys.stderr)
                sys.exit(1)

    output_dir = args.output_dir
    if output_dir is None:
        label = args.strategy if args.strategy != "all" else "comparison"
        output_dir = os.path.join("spikes", "crop-rig", "output", label)

    run_rig(args.video_path, args.keypoints_path, strategy_names,
            args.num_frames, output_dir, args.scale)


if __name__ == "__main__":
    main()
