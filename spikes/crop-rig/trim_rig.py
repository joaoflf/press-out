#!/usr/bin/env python3
"""Trim testing rig — visualize and compare trim strategies on pose-estimated video.

Loads a video + keypoints.json, computes displacement timelines, applies pluggable
trim strategies, and produces timeline charts + boundary frame screenshots.

Usage:
    uv run spikes/crop-rig/trim_rig.py <video> <keypoints.json> [options]

    # Run with baseline strategy
    uv run spikes/crop-rig/trim_rig.py testdata/videos/sample-lift.mp4 testdata/keypoints-sample.json

    # Run all strategies compared
    uv run spikes/crop-rig/trim_rig.py testdata/videos/sample-lift.mp4 testdata/keypoints-sample.json --strategy all

    # Specific strategy
    uv run spikes/crop-rig/trim_rig.py video.mp4 kp.json --strategy velocity-spike
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
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.patches as mpatches


# --- Data types ---

@dataclass
class TrimResult:
    start_sec: float
    end_sec: float
    confident: bool
    label: str = ""


@dataclass
class BBox:
    left: float
    top: float
    right: float
    bottom: float


@dataclass
class Keypoint:
    name: str
    x: float
    y: float
    confidence: float


@dataclass
class Frame:
    time_offset_ms: int
    bounding_box: BBox
    keypoints: list[Keypoint]


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
        kps = [Keypoint(k["name"], k["x"], k["y"], k.get("confidence", 0))
               for k in fd.get("keypoints", [])]
        frames.append(Frame(
            time_offset_ms=fd["timeOffsetMs"],
            bounding_box=BBox(bb["left"], bb["top"], bb["right"], bb["bottom"]),
            keypoints=kps,
        ))
    return PoseResult(
        source_width=data["sourceWidth"],
        source_height=data["sourceHeight"],
        frames=frames,
    )


# --- Shared displacement computation ---

def compute_displacements(frames: list[Frame], min_confidence: float = 0.5) -> list[float]:
    """Frame-to-frame average keypoint displacement (normalized coords)."""
    displacements = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        total_disp = 0.0
        count = 0
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None:
                continue
            if prev.confidence < min_confidence or kp.confidence < min_confidence:
                continue
            dx = kp.x - prev.x
            dy = kp.y - prev.y
            total_disp += math.sqrt(dx * dx + dy * dy)
            count += 1
        displacements.append(total_disp / count if count > 0 else 0.0)
    return displacements


def smooth(values: list[float], window: int = 5) -> list[float]:
    """Moving average smoothing."""
    smoothed = []
    half = window // 2
    for i in range(len(values)):
        start = max(0, i - half)
        end = min(len(values), i + half + 1)
        smoothed.append(sum(values[start:end]) / (end - start))
    return smoothed


def estimate_fps(frames: list[Frame]) -> float:
    if len(frames) < 2:
        return 30.0
    total_ms = frames[-1].time_offset_ms - frames[0].time_offset_ms
    if total_ms <= 0:
        return 30.0
    return (len(frames) - 1) / (total_ms / 1000.0)


def frame_times(frames: list[Frame]) -> list[float]:
    """Time in seconds for each frame."""
    return [f.time_offset_ms / 1000.0 for f in frames]


# --- Shared run-merging logic ---

@dataclass
class MotionRun:
    start_idx: int
    end_idx: int
    total_disp: float
    high_motion_count: int


def merge_runs(high_motion: list[bool], displacements: list[float],
               max_gap_frames: int) -> list[MotionRun]:
    runs = []
    current = None
    gap = 0

    for i, hm in enumerate(high_motion):
        if hm:
            if current is None:
                current = MotionRun(start_idx=i, end_idx=i, total_disp=0, high_motion_count=0)
            current.total_disp += displacements[i]
            current.high_motion_count += 1
            current.end_idx = i
            gap = 0
        elif current is not None:
            gap += 1
            if gap > max_gap_frames:
                runs.append(current)
                current = None
                gap = 0

    if current is not None:
        runs.append(current)

    return runs


# --- Trim strategies ---
# Each strategy: (frames, displacements, smoothed, fps) -> TrimResult

StrategyFn = Callable[[list[Frame], list[float], list[float], float], TrimResult]

STRATEGIES: dict[str, StrategyFn] = {}


def strategy(name: str):
    def decorator(fn: StrategyFn) -> StrategyFn:
        STRATEGIES[name] = fn
        return fn
    return decorator


@strategy("baseline")
def baseline_strategy(frames: list[Frame], displacements: list[float],
                      smoothed: list[float], fps: float) -> TrimResult:
    """Current Go production logic: displacement threshold + gap bridging + padding."""
    threshold = 0.01
    min_dur = 0.8
    max_dur = 15.0
    min_hm = 5
    max_gap_sec = 2.0
    padding = 1.5

    max_gap_frames = max(1, int(max_gap_sec * fps))

    high_motion = [d > threshold for d in smoothed]
    runs = merge_runs(high_motion, smoothed, max_gap_frames)

    if not runs:
        return TrimResult(0, 0, False, "no motion detected")

    best = max(runs, key=lambda r: r.total_disp)

    start_sec = frames[best.start_idx + 1].time_offset_ms / 1000.0
    end_idx = min(best.end_idx + 1, len(frames) - 1)
    end_sec = frames[end_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < min_dur:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {min_dur}s")
    if dur > max_dur:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {max_dur}s")
    if best.high_motion_count < min_hm:
        return TrimResult(0, 0, False, f"too few motion frames: {best.high_motion_count} < {min_hm}")

    # Apply padding
    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - padding)
    end_sec = min(video_dur, end_sec + padding)

    return TrimResult(start_sec, end_sec, True)


@strategy("velocity-spike")
def velocity_spike_strategy(frames: list[Frame], displacements: list[float],
                            smoothed: list[float], fps: float) -> TrimResult:
    """Find the peak velocity region and expand outward until motion drops to near-zero.
    Better for lifts with a clear explosive phase."""
    if not smoothed:
        return TrimResult(0, 0, False, "no data")

    padding = 1.0
    floor_mult = 0.15  # expand until displacement drops below 15% of peak

    # Find peak displacement
    peak_idx = int(np.argmax(smoothed))
    peak_val = smoothed[peak_idx]

    if peak_val < 0.005:
        return TrimResult(0, 0, False, "no significant motion")

    floor = peak_val * floor_mult

    # Expand left
    start_idx = peak_idx
    for i in range(peak_idx - 1, -1, -1):
        if smoothed[i] < floor:
            break
        start_idx = i

    # Expand right
    end_idx = peak_idx
    for i in range(peak_idx + 1, len(smoothed)):
        if smoothed[i] < floor:
            break
        end_idx = i

    start_sec = frames[start_idx + 1].time_offset_ms / 1000.0
    end_idx_f = min(end_idx + 1, len(frames) - 1)
    end_sec = frames[end_idx_f].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < 0.5:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s")

    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - padding)
    end_sec = min(video_dur, end_sec + padding)

    return TrimResult(start_sec, end_sec, True)


@strategy("energy-window")
def energy_window_strategy(frames: list[Frame], displacements: list[float],
                           smoothed: list[float], fps: float) -> TrimResult:
    """Sliding window that finds the time window with the highest cumulative displacement.
    Window size adapts between 3-12 seconds."""
    if not smoothed:
        return TrimResult(0, 0, False, "no data")

    padding = 1.5
    min_win_sec = 3.0
    max_win_sec = 12.0

    min_win_frames = max(1, int(min_win_sec * fps))
    max_win_frames = min(len(smoothed), int(max_win_sec * fps))

    best_energy = -1.0
    best_start = 0
    best_end = 0

    # Try multiple window sizes
    for win_size in range(min_win_frames, max_win_frames + 1, max(1, int(fps / 2))):
        for start in range(len(smoothed) - win_size + 1):
            energy = sum(smoothed[start:start + win_size])
            if energy > best_energy:
                best_energy = energy
                best_start = start
                best_end = start + win_size - 1

    if best_energy <= 0:
        return TrimResult(0, 0, False, "no motion energy")

    start_sec = frames[best_start + 1].time_offset_ms / 1000.0
    end_idx = min(best_end + 1, len(frames) - 1)
    end_sec = frames[end_idx].time_offset_ms / 1000.0

    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - padding)
    end_sec = min(video_dur, end_sec + padding)

    return TrimResult(start_sec, end_sec, True)


def _compute_y_weighted_displacement(frames: list[Frame], y_weight: float = 3.5,
                                     min_conf: float = 0.5) -> list[float]:
    """Compute frame-to-frame displacement with Y-axis weighted more heavily.

    During lifts, keypoints move vertically (bar going up). During walking,
    keypoints translate horizontally. Y-weighting amplifies lifting and
    suppresses walking.
    """
    disp = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        total = 0.0
        count = 0
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None or prev.confidence < min_conf or kp.confidence < min_conf:
                continue
            dx = kp.x - prev.x
            dy = kp.y - prev.y
            total += math.sqrt(dx * dx + (y_weight * dy) ** 2)
            count += 1
        disp.append(total / count if count > 0 else 0.0)
    return disp


def _compute_motion_diversity(frames: list[Frame], min_conf: float = 0.5) -> list[float]:
    """Compute per-frame 'articulated motion' — deviation from rigid translation.

    For each frame transition, compute the mean displacement vector (rigid body
    translation), then measure how much each keypoint deviates from it.
    High deviation = lifting (body parts moving in different directions).
    Low deviation = walking (all keypoints translate uniformly).
    """
    diversity = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        dxs, dys = [], []
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None or prev.confidence < min_conf or kp.confidence < min_conf:
                continue
            dxs.append(kp.x - prev.x)
            dys.append(kp.y - prev.y)

        if len(dxs) < 3:
            diversity.append(0.0)
            continue

        mean_dx = sum(dxs) / len(dxs)
        mean_dy = sum(dys) / len(dys)
        dev = sum(math.sqrt((dx - mean_dx) ** 2 + (dy - mean_dy) ** 2)
                  for dx, dy in zip(dxs, dys))
        diversity.append(dev / len(dxs))
    return diversity


def _get_ankle_gap(frame: Frame, min_conf: float = 0.5) -> float | None:
    """Horizontal distance between left and right ankle (normalized coords).

    Returns None if either ankle keypoint is missing or below confidence threshold.
    Used to detect jerk split stance (wide gap) vs normal/recovered stance (narrow gap).
    """
    kps = {kp.name: kp for kp in frame.keypoints}
    la = kps.get("left_ankle")
    ra = kps.get("right_ankle")
    if la is None or ra is None:
        return None
    if la.confidence < min_conf or ra.confidence < min_conf:
        return None
    return abs(la.x - ra.x)


@strategy("energy-density")
def energy_density_strategy(frames: list[Frame], displacements: list[float],
                            smoothed: list[float], fps: float) -> TrimResult:
    """Find the densest articulated-motion window, then expand to natural boundaries.

    Three phases:
    1. Compute motion diversity (deviation from rigid translation) — separates
       lifting (body parts move differently) from walking (uniform translation)
    2. Sliding window scored by density (diversity/frames) — finds the core lift
    3. Expand outward while overall displacement stays above a floor
    """
    if len(frames) < 2:
        return TrimResult(0, 0, False, "not enough frames")

    # --- Parameters ---
    SMOOTH_WINDOW = 7
    MIN_WIN_SEC = 3.0       # minimum window to search
    MAX_WIN_SEC = 12.0      # maximum window — CJ needs ~10-12s
    WIN_STEP_SEC = 0.5      # step between window sizes
    EXPAND_FLOOR = 0.40     # expand while local displacement >= 40% of peak
    EXPAND_WINDOW = 15      # frames to average when checking expansion floor
    PADDING_SEC = 1.25
    MIN_DURATION_SEC = 2.0
    MAX_DURATION_SEC = 18.0
    MIN_PEAK_DENSITY = 0.002

    # --- Phase 1: Motion diversity signal ---
    div = _compute_motion_diversity(frames)
    div_smooth = smooth(div, window=SMOOTH_WINDOW)
    n = len(div_smooth)

    # Prefix sum for fast window queries
    cumul = [0.0] * (n + 1)
    for i in range(n):
        cumul[i + 1] = cumul[i] + div_smooth[i]

    # --- Phase 2: Find densest window (highest avg diversity per frame) ---
    min_win = max(1, int(MIN_WIN_SEC * fps))
    max_win = min(n, int(MAX_WIN_SEC * fps))
    win_step = max(1, int(WIN_STEP_SEC * fps))

    best_density = -1.0
    best_start = 0
    best_end = 0

    for win_size in range(min_win, max_win + 1, win_step):
        for start in range(n - win_size + 1):
            end = start + win_size
            density = (cumul[end] - cumul[start]) / win_size
            if density > best_density:
                best_density = density
                best_start = start
                best_end = end  # exclusive

    if best_density < MIN_PEAK_DENSITY:
        return TrimResult(0, 0, False,
                          f"peak density too low: {best_density:.4f}")

    # --- Phase 3: Use window directly (no expansion) ---
    # The diversity window already captures the core lift accurately.
    # Padding handles setup/recovery margins.
    start_frame = min(best_start + 1, len(frames) - 1)
    end_frame = min(best_end, len(frames) - 1)

    start_sec = frames[start_frame].time_offset_ms / 1000.0
    end_sec = frames[end_frame].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s")

    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    return TrimResult(start_sec, end_sec, True)


@strategy("density-bridged")
def density_bridged_strategy(frames: list[Frame], displacements: list[float],
                              smoothed: list[float], fps: float) -> TrimResult:
    """Energy-density with wider minimum window to capture multi-phase lifts.

    Same as energy-density but with MIN_WIN_SEC=5 instead of 3. This forces the
    algorithm to find windows wide enough to span both phases of a CJ, where
    the 6s window (clean+walk+jerk) naturally has the highest density among
    windows >= 5s. Asymmetric padding gives extra room for recovery at the end.
    """
    if len(frames) < 2:
        return TrimResult(0, 0, False, "not enough frames")

    # --- Parameters ---
    SMOOTH_WINDOW = 7
    MIN_WIN_SEC = 5.0       # wider min to capture CJ's clean+walk+jerk
    MAX_WIN_SEC = 12.0
    WIN_STEP_SEC = 0.5
    PADDING_SEC = 1.25
    MIN_DURATION_SEC = 2.0
    MAX_DURATION_SEC = 18.0
    MIN_PEAK_DENSITY = 0.002

    # --- Phase 1: Motion diversity signal ---
    div = _compute_motion_diversity(frames)
    div_smooth = smooth(div, window=SMOOTH_WINDOW)
    n = len(div_smooth)

    cumul = [0.0] * (n + 1)
    for i in range(n):
        cumul[i + 1] = cumul[i] + div_smooth[i]

    # --- Phase 2: Find densest window ---
    min_win = max(1, int(MIN_WIN_SEC * fps))
    max_win = min(n, int(MAX_WIN_SEC * fps))
    win_step = max(1, int(WIN_STEP_SEC * fps))

    best_density = -1.0
    best_start = 0
    best_end = 0

    for win_size in range(min_win, max_win + 1, win_step):
        for start in range(n - win_size + 1):
            end = start + win_size
            density = (cumul[end] - cumul[start]) / win_size
            if density > best_density:
                best_density = density
                best_start = start
                best_end = end

    if best_density < MIN_PEAK_DENSITY:
        return TrimResult(0, 0, False,
                          f"peak density too low: {best_density:.4f}")

    # --- Phase 2.5: Ankle split recovery extension ---
    # After a jerk catch, the lifter is in a split stance (wide ankle X gap).
    # If the window ends during a split, extend until feet come together.
    SPLIT_DETECT_GAP = 0.08      # ankle X gap indicating jerk split
    SPLIT_CONVERGE_GAP = 0.05    # recovery complete when gap narrows to this
    MAX_RECOVERY_SEC = 3.0       # max forward extension from window end
    ANKLE_SMOOTH = 5             # frames for smoothing noisy ankle gap

    end_frame = min(best_end, len(frames) - 1)
    original_end_frame = end_frame

    # Check for split at/near window end (look ahead up to 0.5s)
    detect_end = min(len(frames), end_frame + int(0.5 * fps))
    detect_gaps = [_get_ankle_gap(frames[i])
                   for i in range(max(0, end_frame - 2), detect_end)]
    max_gap = max((g for g in detect_gaps if g is not None), default=0.0)

    if max_gap >= SPLIT_DETECT_GAP:
        # Split detected — scan forward for recovery (ankle convergence)
        scan_limit = min(len(frames), end_frame + int(MAX_RECOVERY_SEC * fps))
        raw_gaps = []
        for i in range(end_frame, scan_limit):
            g = _get_ankle_gap(frames[i])
            raw_gaps.append(g if g is not None else (raw_gaps[-1] if raw_gaps else 0.0))
        if raw_gaps:
            smoothed_gaps = smooth(raw_gaps, window=ANKLE_SMOOTH)
            for j, sg in enumerate(smoothed_gaps):
                if sg < SPLIT_CONVERGE_GAP:
                    end_frame = end_frame + j
                    break
            else:
                end_frame = scan_limit - 1

    recovery_extended = end_frame > original_end_frame

    # --- Phase 3: Convert to seconds ---
    start_frame = min(best_start + 1, len(frames) - 1)

    start_sec = frames[start_frame].time_offset_ms / 1000.0
    end_sec = frames[end_frame].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s")

    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    win_dur = best_end / fps - best_start / fps
    label = f"density={best_density:.4f}, win={win_dur:.1f}s"
    if recovery_extended:
        ext_sec = (frames[end_frame].time_offset_ms -
                   frames[original_end_frame].time_offset_ms) / 1000.0
        label += f", recovery=+{ext_sec:.1f}s"
    return TrimResult(start_sec, end_sec, True, label)


@strategy("adaptive-threshold")
def adaptive_threshold_strategy(frames: list[Frame], displacements: list[float],
                                 smoothed: list[float], fps: float) -> TrimResult:
    """Y-weighted adaptive threshold: amplifies vertical (lifting) motion, suppresses
    horizontal (walking) motion, then uses a two-phase approach:
      1. Find the densest vertical-motion window (the core lift)
      2. Expand outward while Y-weighted signal stays above an adaptive threshold

    The Y-weighting (3x) makes lifting motion (pull, stand-up, overhead) register
    much higher than walking (mostly horizontal translation). Combined with a
    core-window search + adaptive expansion, this correctly:
      - Includes the walk between clean and jerk (it has enough vertical bounce)
      - Excludes post-lift walking away (predominantly horizontal)
      - Finds the actual lift even when post-lift noise is high
    """
    if len(frames) < 2:
        return TrimResult(0, 0, False, "no data")

    # --- Parameters ---
    Y_WEIGHT = 3.0               # weight Y displacement 3x more than X
    MIN_CONFIDENCE = 0.5         # keypoint confidence floor
    SMOOTH_WINDOW = 7            # smoothing window for Y-weighted signal
    CORE_WIN_MIN_SEC = 2.0       # min window for core lift search
    CORE_WIN_MAX_SEC = 8.0       # max window for core lift search
    CORE_WIN_STEP_SEC = 0.5      # step between window sizes
    EXPAND_THRESHOLD_K = 0.8     # threshold = median + k * std of Y-weighted signal
    EXPAND_MIN_THRESHOLD = 0.003 # absolute floor for expansion threshold
    GAP_BRIDGE_SEC = 2.5         # max gap to bridge during expansion
    PADDING_SEC = 1.0            # padding added to final boundaries
    MIN_DURATION_SEC = 2.0       # reject trims shorter than this
    MAX_DURATION_SEC = 15.0      # reject trims longer than this
    MIN_PEAK_ENERGY = 0.005      # minimum avg Y-weighted displacement per frame in core

    # --- Phase 0: Compute Y-weighted displacement signal ---
    # For each consecutive frame pair, compute average keypoint displacement
    # but weight vertical (Y) changes 3x more than horizontal (X) changes.
    # This amplifies lifting motion (pull, stand-up, overhead) and suppresses
    # walking (mostly horizontal translation).
    yw_displacements = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        total_disp = 0.0
        count = 0
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None:
                continue
            if prev.confidence < MIN_CONFIDENCE or kp.confidence < MIN_CONFIDENCE:
                continue
            dx = kp.x - prev.x
            dy = kp.y - prev.y
            # Weight Y displacement more heavily
            total_disp += math.sqrt(dx * dx + (Y_WEIGHT * dy) ** 2)
            count += 1
        yw_displacements.append(total_disp / count if count > 0 else 0.0)

    # Smooth the Y-weighted signal
    yw_smoothed = smooth(yw_displacements, SMOOTH_WINDOW)
    n = len(yw_smoothed)

    if n < 2:
        return TrimResult(0, 0, False, "insufficient frames")

    # --- Phase 1: Find the core lift window (densest Y-weighted energy) ---
    # Slide windows of varying size and pick the one with the highest average
    # Y-weighted displacement per frame. This locates the heart of the lift.
    cumul = [0.0] * (n + 1)
    for i in range(n):
        cumul[i + 1] = cumul[i] + yw_smoothed[i]

    min_win = max(1, int(CORE_WIN_MIN_SEC * fps))
    max_win = min(n, int(CORE_WIN_MAX_SEC * fps))
    step = max(1, int(CORE_WIN_STEP_SEC * fps))

    best_avg = -1.0
    best_start = 0
    best_end = 0

    for win_size in range(min_win, max_win + 1, step):
        for start in range(n - win_size + 1):
            end = start + win_size
            total = cumul[end] - cumul[start]
            avg = total / win_size
            if avg > best_avg:
                best_avg = avg
                best_start = start
                best_end = end  # exclusive

    if best_avg < MIN_PEAK_ENERGY:
        return TrimResult(0, 0, False,
                          f"peak Y-weighted energy too low: {best_avg:.4f}")

    # --- Phase 2: Expand outward from the core using adaptive threshold ---
    # Compute threshold from the Y-weighted signal's distribution.
    # Use a lower threshold than Phase 1 to capture the full lift
    # (setup, pull initiation, recovery) while still excluding post-lift walking.
    arr = np.array(yw_smoothed)
    median_val = float(np.median(arr))
    std_val = float(np.std(arr))
    expand_threshold = max(median_val + EXPAND_THRESHOLD_K * std_val,
                           EXPAND_MIN_THRESHOLD)

    # Expand left from core start
    gap_bridge_frames = max(1, int(GAP_BRIDGE_SEC * fps))
    expanded_start = best_start
    gap = 0
    for i in range(best_start - 1, -1, -1):
        if yw_smoothed[i] >= expand_threshold:
            expanded_start = i
            gap = 0
        else:
            gap += 1
            if gap > gap_bridge_frames:
                break

    # Expand right from core end
    expanded_end = best_end - 1  # convert to inclusive
    gap = 0
    for i in range(best_end, n):
        if yw_smoothed[i] >= expand_threshold:
            expanded_end = i
            gap = 0
        else:
            gap += 1
            if gap > gap_bridge_frames:
                break

    # --- Phase 3: Convert to seconds and validate ---
    start_frame_idx = min(expanded_start + 1, len(frames) - 1)
    end_frame_idx = min(expanded_end + 1, len(frames) - 1)

    start_sec = frames[start_frame_idx].time_offset_ms / 1000.0
    end_sec = frames[end_frame_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {MIN_DURATION_SEC}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {MAX_DURATION_SEC}s")

    # Apply padding
    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    return TrimResult(start_sec, end_sec, True,
                      f"yw_threshold={expand_threshold:.4f} "
                      f"(median={median_val:.4f}, std={std_val:.4f}, "
                      f"core={best_start/fps:.1f}-{best_end/fps:.1f}s)")


@strategy("cumul-slope")
def cumul_slope_strategy(frames: list[Frame], displacements: list[float],
                         smoothed: list[float], fps: float) -> TrimResult:
    """Find the lift by locating the steepest segment of the cumulative displacement curve.

    The cumulative sum of smoothed displacements forms a monotonically increasing curve
    that is flat during stillness and steep during motion. The lift is the segment with
    the steepest sustained slope. We try multiple window sizes (3-10s) and pick the
    window position/size with the highest average slope, then refine boundaries outward.
    """
    if not smoothed:
        return TrimResult(0, 0, False, "no data")

    # --- Parameters ---
    MIN_WIN_SEC = 3.0
    MAX_WIN_SEC = 10.0
    WIN_STEP_SEC = 0.5          # step between window sizes to try
    SLOPE_DROP_RATIO = 0.15     # expand until local slope < this fraction of peak slope
    EXPAND_WINDOW_FRAMES = 5    # local slope is measured over this many frames
    PADDING_SEC = 1.25
    MIN_DURATION_SEC = 2.0
    MAX_DURATION_SEC = 15.0
    MIN_PEAK_SLOPE = 0.005      # minimum avg displacement per frame to count as motion

    n = len(smoothed)

    # Build cumulative displacement array (prefix sum)
    # cumul[i] = sum of smoothed[0..i-1], so cumul[0]=0, cumul[n]=total
    cumul = [0.0] * (n + 1)
    for i in range(n):
        cumul[i + 1] = cumul[i] + smoothed[i]

    # --- Phase 1: Sliding window to find densest motion segment ---
    # For each window size, slide across and compute average slope
    # (= total displacement in window / number of frames in window).
    # Track the best (highest avg slope) across all sizes and positions.
    best_avg_slope = -1.0
    best_win_start = 0
    best_win_end = 0

    min_win_frames = max(1, int(MIN_WIN_SEC * fps))
    max_win_frames = min(n, int(MAX_WIN_SEC * fps))
    win_step_frames = max(1, int(WIN_STEP_SEC * fps))

    for win_size in range(min_win_frames, max_win_frames + 1, win_step_frames):
        for start in range(n - win_size + 1):
            end = start + win_size
            total_disp = cumul[end] - cumul[start]
            avg_slope = total_disp / win_size
            if avg_slope > best_avg_slope:
                best_avg_slope = avg_slope
                best_win_start = start
                best_win_end = end  # exclusive

    if best_avg_slope < MIN_PEAK_SLOPE:
        return TrimResult(0, 0, False,
                          f"peak slope too low: {best_avg_slope:.4f} < {MIN_PEAK_SLOPE}")

    # --- Phase 2: Refine boundaries by expanding outward ---
    # From the best window edges, expand outward as long as the local slope
    # (displacement averaged over a small window) remains above a fraction of the peak.
    slope_floor = best_avg_slope * SLOPE_DROP_RATIO
    half_expand = EXPAND_WINDOW_FRAMES // 2

    def local_slope(idx: int) -> float:
        """Average displacement in a small window centered on idx."""
        lo = max(0, idx - half_expand)
        hi = min(n, idx + half_expand + 1)
        if hi <= lo:
            return 0.0
        return (cumul[hi] - cumul[lo]) / (hi - lo)

    # Expand left
    refined_start = best_win_start
    for i in range(best_win_start - 1, -1, -1):
        if local_slope(i) < slope_floor:
            break
        refined_start = i

    # Expand right
    refined_end = best_win_end - 1  # convert to inclusive index
    for i in range(best_win_end, n):
        if local_slope(i) < slope_floor:
            break
        refined_end = i

    # --- Phase 3: Convert to seconds and apply padding ---
    # displacement index i corresponds to the transition between frame i and frame i+1,
    # so the start time uses frame[refined_start + 1] (matching other strategies)
    start_frame_idx = min(refined_start + 1, len(frames) - 1)
    end_frame_idx = min(refined_end + 1, len(frames) - 1)

    start_sec = frames[start_frame_idx].time_offset_ms / 1000.0
    end_sec = frames[end_frame_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {MIN_DURATION_SEC}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {MAX_DURATION_SEC}s")

    # Apply padding
    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    return TrimResult(start_sec, end_sec, True)


@strategy("vertical-weighted")
def vertical_weighted_strategy(frames: list[Frame], displacements: list[float],
                                smoothed: list[float], fps: float) -> TrimResult:
    """Weight Y-axis displacement heavily over X-axis to distinguish lifting from walking.

    During a lift, keypoints move predominantly vertically (bar goes up, body extends).
    During walking, keypoints translate horizontally. By computing displacement with
    Y weighted ~3.5x over X, walking motion registers much lower than lifting motion,
    giving cleaner trim boundaries that exclude post-lift walking.

    Approach:
    1. Compute Y-weighted displacement per frame: sqrt((dx)^2 + (3.5*dy)^2)
    2. Compute the ratio: Y-weighted / standard displacement. Frames with
       predominantly vertical motion (lift) have ratio > ~1.15; frames with
       predominantly horizontal motion (walk) have ratio near 1.0.
    3. Suppress the standard smoothed signal at frames where the ratio is low
       (horizontal/walking motion). This zeroes out walking displacement.
    4. Run baseline-style threshold + merge on the suppressed signal.
    """
    if len(frames) < 2:
        return TrimResult(0, 0, False, "not enough frames")

    # --- Parameters ---
    Y_WEIGHT = 3.5          # Y-axis weight relative to X
    MIN_CONFIDENCE = 0.5    # Minimum keypoint confidence
    SMOOTH_WINDOW = 7       # Smoothing window for signals
    MOTION_THRESHOLD = 0.003  # Min standard displacement to compute ratio
                              # (below this = idle, not walk vs lift)
    RATIO_THRESHOLD = 1.15  # Y-weighted/standard ratio above this = vertical
                            # motion (lift). Below = horizontal (walk/idle).
                            # With Y_WEIGHT=3.5, pure vertical gives ratio=3.5,
                            # pure horizontal gives ratio=1.0.
    DISP_THRESHOLD = 0.01   # Displacement threshold (same as baseline)
    MAX_GAP_SEC = 2.0       # Bridge gaps (same as baseline)
    MIN_DURATION_SEC = 1.5  # Minimum lift duration
    MAX_DURATION_SEC = 15.0 # Maximum lift duration
    MIN_HIGH_MOTION = 5     # Minimum high-motion frames in run
    PADDING_SEC = 1.0       # Padding around boundaries (tighter than baseline)

    # --- Phase 1: Compute Y-weighted displacement per frame ---
    y_weighted_disp = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        total_disp = 0.0
        count = 0
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None:
                continue
            if prev.confidence < MIN_CONFIDENCE or kp.confidence < MIN_CONFIDENCE:
                continue
            dx = kp.x - prev.x
            dy = kp.y - prev.y
            total_disp += math.sqrt(dx ** 2 + (Y_WEIGHT * dy) ** 2)
            count += 1
        y_weighted_disp.append(total_disp / count if count > 0 else 0.0)

    # --- Phase 2: Compute ratio and suppress walking frames ---
    # `displacements` is the standard per-frame displacement (passed in).
    # Frames where Y-weighted/standard ratio < RATIO_THRESHOLD are
    # horizontal/walking motion -- zero them out.
    y_smooth = smooth(y_weighted_disp, window=SMOOTH_WINDOW)
    std_smooth = smooth(displacements, window=SMOOTH_WINDOW)

    suppressed = []
    for i in range(len(std_smooth)):
        std_val = std_smooth[i]
        yw_val = y_smooth[i]

        if std_val < MOTION_THRESHOLD:
            # Below motion floor = idle, keep original (don't suppress idle)
            suppressed.append(std_val)
        elif std_val > 0:
            ratio = yw_val / std_val
            if ratio >= RATIO_THRESHOLD:
                # Vertical-dominant motion (lift) -- keep full displacement
                suppressed.append(std_val)
            else:
                # Horizontal-dominant motion (walk) -- suppress
                suppressed.append(0.0)
        else:
            suppressed.append(0.0)

    if not suppressed:
        return TrimResult(0, 0, False, "no data")

    # --- Phase 3: Threshold + run merging on suppressed signal ---
    max_gap_frames = max(1, int(MAX_GAP_SEC * fps))
    high_motion = [d > DISP_THRESHOLD for d in suppressed]
    runs = merge_runs(high_motion, suppressed, max_gap_frames)

    if not runs:
        return TrimResult(0, 0, False, "no vertical-dominant motion detected")

    # --- Phase 4: Select best run by total displacement ---
    best = max(runs, key=lambda r: r.total_disp)

    start_sec = frames[best.start_idx + 1].time_offset_ms / 1000.0
    end_idx = min(best.end_idx + 1, len(frames) - 1)
    end_sec = frames[end_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {MIN_DURATION_SEC}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {MAX_DURATION_SEC}s")
    if best.high_motion_count < MIN_HIGH_MOTION:
        return TrimResult(0, 0, False,
                          f"too few motion frames: {best.high_motion_count} < {MIN_HIGH_MOTION}")

    # Apply padding
    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    return TrimResult(start_sec, end_sec, True,
                      f"Y_WEIGHT={Y_WEIGHT}, ratio_thresh={RATIO_THRESHOLD}")


@strategy("last-peak")
def last_peak_strategy(frames: list[Frame], displacements: list[float],
                       smoothed: list[float], fps: float) -> TrimResult:
    """Find ALL significant vertical-displacement peak *clusters*, trim first to last.

    Lifting creates sharp vertical (Y-axis) peaks; walking creates horizontal
    displacement that won't register as significant vertical peaks.  By anchoring
    the trim to span from the first significant peak cluster to the last, we
    naturally include both the clean and the jerk in a C&J while excluding
    post-lift walking.

    Steps:
      1. Compute per-frame average *vertical* keypoint displacement (smoothed).
      2. Detect all local-maximum peaks above a high significance threshold.
      3. Cluster nearby peaks (within ~1.5s) into peak groups.
      4. Score each cluster by its total peak energy; keep only strong clusters.
      5. Trim spans from the first strong cluster to the last strong cluster.
      6. Expand outward until displacement drops to near-zero.
      7. Apply padding and sanity checks.
    """
    if len(frames) < 3:
        return TrimResult(0, 0, False, "not enough frames")

    # --- Parameters ---
    MIN_CONFIDENCE = 0.5
    SMOOTH_WINDOW = 7            # slightly wider than the default 5 for less noise
    PEAK_SIGNIFICANCE = 0.70     # peak must be >= 70% of global max to count
    MIN_PEAK_ABS = 0.005         # absolute floor for a peak (avoids noise peaks)
    CLUSTER_GAP_SEC = 1.0        # peaks within this gap are merged into one cluster
    MAX_SPAN_GAP_SEC = 4.0       # max gap between clusters to bridge (for C&J walk)
    CLUSTER_MIN_ENERGY = 0.35    # cluster energy must be >= 35% of strongest cluster
    FLOOR_MULT = 0.15            # expand until displacement drops below 15% of peak
    PADDING_SEC = 1.25
    MIN_DURATION_SEC = 1.5
    MAX_DURATION_SEC = 20.0

    # --- Step 1: Compute vertical (Y-axis) displacement per frame ---
    y_displacements: list[float] = []
    for i in range(1, len(frames)):
        prev_kps = {kp.name: kp for kp in frames[i - 1].keypoints}
        total_dy = 0.0
        count = 0
        for kp in frames[i].keypoints:
            prev = prev_kps.get(kp.name)
            if prev is None:
                continue
            if prev.confidence < MIN_CONFIDENCE or kp.confidence < MIN_CONFIDENCE:
                continue
            total_dy += abs(kp.y - prev.y)
            count += 1
        y_displacements.append(total_dy / count if count > 0 else 0.0)

    # Smooth the vertical displacement signal
    y_smooth = smooth(y_displacements, window=SMOOTH_WINDOW)

    # --- Step 2: Find all local-maximum peaks ---
    global_max = max(y_smooth) if y_smooth else 0.0
    if global_max < MIN_PEAK_ABS:
        return TrimResult(0, 0, False,
                          f"no significant vertical motion (max={global_max:.5f})")

    significance_threshold = max(global_max * PEAK_SIGNIFICANCE, MIN_PEAK_ABS)

    # A peak is a local maximum: y_smooth[i] >= neighbors and above threshold
    peak_indices: list[int] = []
    for i in range(1, len(y_smooth) - 1):
        if (y_smooth[i] >= y_smooth[i - 1] and
                y_smooth[i] >= y_smooth[i + 1] and
                y_smooth[i] >= significance_threshold):
            peak_indices.append(i)

    if not peak_indices:
        return TrimResult(0, 0, False,
                          f"no peaks above threshold ({significance_threshold:.4f})")

    # --- Step 3: Cluster nearby peaks ---
    # Group peaks that are within CLUSTER_GAP_SEC of each other.
    cluster_gap_frames = int(CLUSTER_GAP_SEC * fps)
    clusters: list[list[int]] = []
    current_cluster: list[int] = [peak_indices[0]]

    for i in range(1, len(peak_indices)):
        if peak_indices[i] - peak_indices[i - 1] <= cluster_gap_frames:
            current_cluster.append(peak_indices[i])
        else:
            clusters.append(current_cluster)
            current_cluster = [peak_indices[i]]
    clusters.append(current_cluster)

    # --- Step 4: Score clusters and filter weak ones ---
    # Each cluster's energy = sum of peak values in the cluster
    cluster_energies = [sum(y_smooth[idx] for idx in c) for c in clusters]
    max_energy = max(cluster_energies) if cluster_energies else 0.0
    energy_threshold = max_energy * CLUSTER_MIN_ENERGY

    # Mark which clusters are "strong"
    is_strong = [e >= energy_threshold for e in cluster_energies]

    # --- Step 4: Find best contiguous span of strong clusters ---
    # Allow bridging gaps up to MAX_SPAN_GAP_SEC between clusters.
    # Find the span that maximizes total energy of strong clusters.
    max_span_gap_frames = int(MAX_SPAN_GAP_SEC * fps)

    strong_indices = [i for i, s in enumerate(is_strong) if s]

    if not strong_indices:
        return TrimResult(0, 0, False, "no strong peak clusters found")

    best_span_energy = -1.0
    best_span_start = 0
    best_span_end = 0  # inclusive index into strong_indices

    for si in range(len(strong_indices)):
        span_energy = cluster_energies[strong_indices[si]]
        span_end = si
        for sj in range(si + 1, len(strong_indices)):
            # Check gap between last peak of previous cluster and first peak of this one
            prev_cluster_last = clusters[strong_indices[sj - 1]][-1]
            curr_cluster_first = clusters[strong_indices[sj]][0]
            gap = curr_cluster_first - prev_cluster_last
            if gap > max_span_gap_frames:
                break
            span_energy += cluster_energies[strong_indices[sj]]
            span_end = sj

        if span_energy > best_span_energy:
            best_span_energy = span_energy
            best_span_start = si
            best_span_end = span_end

    first_cluster_idx = strong_indices[best_span_start]
    last_cluster_idx = strong_indices[best_span_end]
    first_peak = clusters[first_cluster_idx][0]
    last_peak = clusters[last_cluster_idx][-1]

    # --- Step 5: Expand outward from endpoints ---
    # Use the *total* smoothed displacement for expansion to capture the full
    # motion envelope (not just vertical).
    overall_peak = max(smoothed)
    floor = overall_peak * FLOOR_MULT

    # Expand left from first peak
    start_idx = first_peak
    for i in range(first_peak - 1, -1, -1):
        if smoothed[i] < floor:
            break
        start_idx = i

    # Expand right from last peak
    end_idx = last_peak
    for i in range(last_peak + 1, len(smoothed)):
        if smoothed[i] < floor:
            break
        end_idx = i

    # --- Step 6: Convert to seconds ---
    # displacement index i corresponds to transition between frame i and frame i+1
    start_frame_idx = min(start_idx + 1, len(frames) - 1)
    end_frame_idx = min(end_idx + 1, len(frames) - 1)

    start_sec = frames[start_frame_idx].time_offset_ms / 1000.0
    end_sec = frames[end_frame_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {MIN_DURATION_SEC}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {MAX_DURATION_SEC}s")

    # --- Step 7: Apply padding ---
    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    n_span_clusters = best_span_end - best_span_start + 1
    n_peaks = sum(len(clusters[strong_indices[k]])
                  for k in range(best_span_start, best_span_end + 1))
    return TrimResult(start_sec, end_sec, True,
                      f"{n_peaks} peaks in {n_span_clusters} clusters, "
                      f"threshold >= {significance_threshold:.4f}")


@strategy("wrist-rise")
def wrist_rise_strategy(frames: list[Frame], displacements: list[float],
                        smoothed: list[float], fps: float) -> TrimResult:
    """Track wrist Y-position to find the lift period.

    During a lift, wrists undergo large vertical displacement (floor to overhead
    or shoulders). During walking/standing, wrists barely move vertically.
    In image coordinates Y=0 is top, so wrists rising means Y DECREASING.

    Uses a two-phase approach on the wrist-specific velocity signal:
    1. Sliding window finds the densest wrist-motion period (highest avg velocity)
    2. Boundary refinement expands outward while local velocity stays above a floor

    This naturally handles clean & jerk (two bursts within the window range)
    and excludes post-lift walking/bar-drop because those have lower wrist
    velocity density than the actual lift.
    """
    if len(frames) < 3:
        return TrimResult(0, 0, False, "too few frames")

    # --- Parameters ---
    WRIST_CONFIDENCE_MIN = 0.5
    SMOOTH_WINDOW = 7           # frames for smoothing wrist velocity
    MIN_WIN_SEC = 2.0           # minimum sliding window size
    MAX_WIN_SEC = 12.0          # maximum sliding window size (covers full CJ)
    WIN_STEP_SEC = 0.5          # step between window sizes
    SLOPE_DROP_RATIO = 0.15     # expand until local velocity < 15% of peak avg velocity
    EXPAND_WINDOW_FRAMES = 7    # local velocity averaging window
    MIN_PEAK_VELOCITY = 0.003   # minimum avg wrist velocity per frame to count as lift
    PADDING_SEC = 1.0
    MIN_DURATION_SEC = 1.5
    MAX_DURATION_SEC = 15.0

    # --- Step 1: Extract per-frame average wrist Y position ---
    wrist_y_values: list[float | None] = []
    for frame in frames:
        kp_map = {kp.name: kp for kp in frame.keypoints}
        lw = kp_map.get("left_wrist")
        rw = kp_map.get("right_wrist")

        ys = []
        if lw and lw.confidence >= WRIST_CONFIDENCE_MIN:
            ys.append(lw.y)
        if rw and rw.confidence >= WRIST_CONFIDENCE_MIN:
            ys.append(rw.y)

        if ys:
            wrist_y_values.append(sum(ys) / len(ys))
        else:
            wrist_y_values.append(None)

    # --- Step 2: Compute frame-to-frame wrist Y velocity ---
    wrist_velocity: list[float] = []
    for i in range(len(wrist_y_values) - 1):
        cur = wrist_y_values[i]
        nxt = wrist_y_values[i + 1]
        if cur is not None and nxt is not None:
            wrist_velocity.append(nxt - cur)
        else:
            wrist_velocity.append(0.0)

    if not wrist_velocity:
        return TrimResult(0, 0, False, "no wrist data")

    # --- Step 3: Smooth absolute velocity for boundary refinement ---
    abs_velocity = [abs(v) for v in wrist_velocity]
    smoothed_vel = smooth(abs_velocity, window=SMOOTH_WINDOW)
    n = len(smoothed_vel)

    if n == 0:
        return TrimResult(0, 0, False, "no data")

    # Forward-fill None wrist Y values for range computation
    filled_y = list(wrist_y_values)
    last_valid = None
    for i in range(len(filled_y)):
        if filled_y[i] is not None:
            last_valid = filled_y[i]
        elif last_valid is not None:
            filled_y[i] = last_valid

    if all(v is None for v in filled_y):
        return TrimResult(0, 0, False, "no valid wrist data")

    # --- Step 4: Sliding window scored by wrist Y range ---
    # The lift window has the largest Y range: wrists travel from near-hips
    # to overhead (snatch) or shoulders (clean). Walking and bar drops have
    # smaller Y range because wrists don't traverse the full vertical extent.
    best_y_range = -1.0
    best_win_start = 0
    best_win_end = 0

    min_win_frames = max(1, int(MIN_WIN_SEC * fps))
    max_win_frames = min(len(filled_y), int(MAX_WIN_SEC * fps))
    win_step_frames = max(1, int(WIN_STEP_SEC * fps))

    for win_size in range(min_win_frames, max_win_frames + 1, win_step_frames):
        for start in range(len(filled_y) - win_size + 1):
            end = start + win_size
            window_ys = [y for y in filled_y[start:end] if y is not None]
            if len(window_ys) < 2:
                continue
            y_range = max(window_ys) - min(window_ys)
            if y_range > best_y_range:
                best_y_range = y_range
                best_win_start = start
                best_win_end = end  # exclusive

    MIN_Y_RANGE = 0.15  # wrists must travel at least 15% of frame height
    if best_y_range < MIN_Y_RANGE:
        return TrimResult(0, 0, False,
                          f"max wrist Y range too small: {best_y_range:.3f}")

    # --- Step 5: Refine boundaries using smoothed velocity ---
    # Build prefix sum of smoothed velocity
    cumul = [0.0] * (n + 1)
    for i in range(n):
        cumul[i + 1] = cumul[i] + smoothed_vel[i]

    # Compute peak avg velocity within the best window for floor reference
    win_start_v = max(0, best_win_start - 1)
    win_end_v = min(n, best_win_end - 1)
    if win_end_v > win_start_v:
        peak_avg_vel = (cumul[win_end_v] - cumul[win_start_v]) / (win_end_v - win_start_v)
    else:
        peak_avg_vel = 0.0

    slope_floor = max(peak_avg_vel * SLOPE_DROP_RATIO, MIN_PEAK_VELOCITY)
    half_expand = EXPAND_WINDOW_FRAMES // 2

    def local_velocity(idx: int) -> float:
        """Average wrist velocity in a small window centered on idx."""
        lo = max(0, idx - half_expand)
        hi = min(n, idx + half_expand + 1)
        if hi <= lo:
            return 0.0
        return (cumul[hi] - cumul[lo]) / (hi - lo)

    # Map frame-index window to velocity-index space
    refined_start = max(0, best_win_start - 1)
    refined_end = min(n - 1, best_win_end - 2)

    # Expand left
    for i in range(refined_start - 1, -1, -1):
        if local_velocity(i) < slope_floor:
            break
        refined_start = i

    # Expand right
    for i in range(refined_end + 1, n):
        if local_velocity(i) < slope_floor:
            break
        refined_end = i

    # --- Step 6: Convert to seconds and apply padding ---
    start_frame_idx = min(refined_start + 1, len(frames) - 1)
    end_frame_idx = min(refined_end + 1, len(frames) - 1)

    start_sec = frames[start_frame_idx].time_offset_ms / 1000.0
    end_sec = frames[end_frame_idx].time_offset_ms / 1000.0

    dur = end_sec - start_sec
    if dur < MIN_DURATION_SEC:
        return TrimResult(0, 0, False, f"too short: {dur:.1f}s < {MIN_DURATION_SEC}s")
    if dur > MAX_DURATION_SEC:
        return TrimResult(0, 0, False, f"too long: {dur:.1f}s > {MAX_DURATION_SEC}s")

    video_dur = frames[-1].time_offset_ms / 1000.0
    start_sec = max(0, start_sec - PADDING_SEC)
    end_sec = min(video_dur, end_sec + PADDING_SEC)

    return TrimResult(start_sec, end_sec, True,
                      f"y_range={best_y_range:.3f}, peak_vel={peak_avg_vel:.4f}")


# --- Visualization ---

STRATEGY_COLORS = {
    "baseline": "#e74c3c",
    "velocity-spike": "#3498db",
    "energy-window": "#2ecc71",
    "adaptive-threshold": "#e67e22",
    "cumul-slope": "#9b59b6",
    "energy-density": "#f39c12",
    "vertical-weighted": "#1abc9c",
    "density-bridged": "#8e44ad",
    "last-peak": "#f39c12",
    "wrist-rise": "#e91e63",
}


def get_color(name: str, idx: int = 0) -> str:
    if name in STRATEGY_COLORS:
        return STRATEGY_COLORS[name]
    palette = ["#e67e22", "#9b59b6", "#1abc9c", "#f39c12", "#e91e63"]
    return palette[idx % len(palette)]


def plot_timeline(times: list[float], raw_disp: list[float], smoothed: list[float],
                  trims: dict[str, TrimResult], output_path: str, title: str = ""):
    """Plot displacement timeline with trim boundaries for each strategy."""
    # displacement times are between frames, use midpoint
    disp_times = [(times[i] + times[i + 1]) / 2 for i in range(len(times) - 1)]

    fig, ax = plt.subplots(figsize=(14, 5))

    # Raw displacement (light gray)
    ax.fill_between(disp_times, raw_disp, alpha=0.15, color="gray", label="raw displacement")
    # Smoothed displacement (dark line)
    ax.plot(disp_times, smoothed, color="black", linewidth=1.2, label="smoothed")

    # Threshold line
    ax.axhline(y=0.01, color="gray", linestyle="--", linewidth=0.8, alpha=0.5, label="threshold (0.01)")

    # Trim boundaries for each strategy
    legend_patches = []
    for idx, (name, trim) in enumerate(trims.items()):
        color = get_color(name, idx)
        if trim.confident:
            ax.axvline(x=trim.start_sec, color=color, linewidth=2, linestyle="-")
            ax.axvline(x=trim.end_sec, color=color, linewidth=2, linestyle="-")
            ax.axvspan(trim.start_sec, trim.end_sec, alpha=0.08, color=color)
            dur = trim.end_sec - trim.start_sec
            label = f"{name}: {trim.start_sec:.1f}s - {trim.end_sec:.1f}s ({dur:.1f}s)"
        else:
            label = f"{name}: NOT CONFIDENT — {trim.label}"
        legend_patches.append(mpatches.Patch(color=color, label=label))

    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Avg Keypoint Displacement (normalized)")
    chart_title = f"{title} — Trim Strategy Comparison" if title else "Displacement Timeline — Trim Strategy Comparison"
    ax.set_title(chart_title)
    ax.legend(handles=legend_patches, loc="upper right", fontsize=9)
    ax.set_xlim(times[0], times[-1])
    ax.grid(True, alpha=0.3)

    plt.tight_layout()
    plt.savefig(output_path, dpi=150)
    plt.close()


def extract_boundary_frames(cap: cv2.VideoCapture, trim: TrimResult,
                            native_fps: float, total_frames: int,
                            scale: float) -> tuple[np.ndarray | None, np.ndarray | None, np.ndarray | None]:
    """Extract start, middle, and end frames of a trim region."""
    results = []
    for sec in [trim.start_sec, (trim.start_sec + trim.end_sec) / 2, trim.end_sec]:
        frame_idx = int(sec * native_fps)
        frame_idx = max(0, min(frame_idx, total_frames - 1))
        cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
        ret, frame = cap.read()
        if ret and scale != 1.0:
            frame = cv2.resize(frame, (int(frame.shape[1] * scale), int(frame.shape[0] * scale)))
        results.append(frame if ret else None)
    return tuple(results)


def make_boundary_strip(frames: list[np.ndarray | None], labels: list[str],
                        strategy_name: str) -> np.ndarray | None:
    """Create a horizontal strip of boundary frames with labels."""
    valid = [(f, l) for f, l in zip(frames, labels) if f is not None]
    if not valid:
        return None

    # Resize all to same height
    target_h = min(f.shape[0] for f, _ in valid)
    panels = []
    for frame, label in valid:
        scale = target_h / frame.shape[0]
        resized = cv2.resize(frame, (int(frame.shape[1] * scale), target_h))
        # Add label
        cv2.putText(resized, label, (10, 25), cv2.FONT_HERSHEY_SIMPLEX, 0.6, (255, 255, 255), 2)
        cv2.putText(resized, label, (10, 25), cv2.FONT_HERSHEY_SIMPLEX, 0.6, (0, 0, 0), 1)
        panels.append(resized)

    # Strategy label on left
    strip = np.hstack(panels)
    cv2.putText(strip, strategy_name, (10, strip.shape[0] - 15),
                cv2.FONT_HERSHEY_SIMPLEX, 0.7, (255, 255, 255), 2)
    cv2.putText(strip, strategy_name, (10, strip.shape[0] - 15),
                cv2.FONT_HERSHEY_SIMPLEX, 0.7, (0, 0, 200), 1)
    return strip


# --- Auto-discovery ---

def discover_test_pairs(test_dir: str) -> list[tuple[str, str, str]]:
    """Find video+keypoints pairs in a directory.

    Convention: for each <name>.mp4, looks for <name>.json in the same dir.
    Returns list of (video_path, keypoints_path, name) tuples.
    """
    pairs = []
    for entry in sorted(os.listdir(test_dir)):
        if not entry.endswith(".mp4"):
            continue
        stem = entry[:-4]
        kp_path = os.path.join(test_dir, f"{stem}.json")
        if os.path.isfile(kp_path):
            pairs.append((os.path.join(test_dir, entry), kp_path, stem))
    return pairs


# --- Main ---

def run_rig(video_path: str, keypoints_path: str, strategy_names: list[str],
            output_dir: str, scale: float, video_label: str = ""):
    pose_result = load_keypoints(keypoints_path)
    frames = pose_result.frames

    label = video_label or os.path.basename(video_path)
    print(f"\n{'='*60}", file=sys.stderr)
    print(f"Video: {label}", file=sys.stderr)
    print(f"Source: {pose_result.source_width}x{pose_result.source_height}, "
          f"{len(frames)} pose frames", file=sys.stderr)

    if len(frames) < 2:
        print("Error: need at least 2 frames", file=sys.stderr)
        return

    fps = estimate_fps(frames)
    times = frame_times(frames)
    raw_disp = compute_displacements(frames)
    smoothed = smooth(raw_disp)

    print(f"FPS: {fps:.1f}, duration: {times[-1]:.1f}s", file=sys.stderr)

    # Run all strategies
    trims: dict[str, TrimResult] = {}
    for name in strategy_names:
        fn = STRATEGIES[name]
        result = fn(frames, raw_disp, smoothed, fps)
        trims[name] = result
        if result.confident:
            dur = result.end_sec - result.start_sec
            print(f"  {name}: {result.start_sec:.2f}s - {result.end_sec:.2f}s "
                  f"({dur:.1f}s)", file=sys.stderr)
        else:
            print(f"  {name}: NOT CONFIDENT — {result.label}", file=sys.stderr)

    os.makedirs(output_dir, exist_ok=True)

    # Plot timeline
    timeline_path = os.path.join(output_dir, "timeline.png")
    plot_timeline(times, raw_disp, smoothed, trims, timeline_path, title=label)
    print(f"Saved {timeline_path}", file=sys.stderr)

    # Extract boundary frames
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        print(f"Error: cannot open video {video_path}", file=sys.stderr)
        return

    native_fps = cap.get(cv2.CAP_PROP_FPS)
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))

    strips = []
    for name, trim in trims.items():
        if not trim.confident:
            continue
        boundary_frames = extract_boundary_frames(cap, trim, native_fps, total_frames, scale)
        labels = [
            f"START {trim.start_sec:.1f}s",
            f"MID {(trim.start_sec + trim.end_sec) / 2:.1f}s",
            f"END {trim.end_sec:.1f}s",
        ]
        strip = make_boundary_strip(list(boundary_frames), labels, name)
        if strip is not None:
            strips.append((name, strip))
            strip_path = os.path.join(output_dir, f"boundaries_{name}.png")
            cv2.imwrite(strip_path, strip)

    cap.release()

    # Combined boundary comparison (all strategies stacked)
    if len(strips) > 1:
        max_w = max(s.shape[1] for _, s in strips)
        padded = []
        for _, s in strips:
            if s.shape[1] < max_w:
                pad = np.zeros((s.shape[0], max_w - s.shape[1], 3), dtype=np.uint8)
                s = np.hstack([s, pad])
            padded.append(s)
        combined = np.vstack(padded)
        combined_path = os.path.join(output_dir, "boundaries_comparison.png")
        cv2.imwrite(combined_path, combined)

    # Summary JSON
    summary = {
        "video": video_path,
        "video_label": label,
        "keypoints": keypoints_path,
        "fps": round(fps, 1),
        "duration_sec": round(times[-1], 1),
        "strategies": {},
        "output_dir": output_dir,
    }
    for name, trim in trims.items():
        entry = {"confident": trim.confident}
        if trim.confident:
            entry["start_sec"] = round(trim.start_sec, 2)
            entry["end_sec"] = round(trim.end_sec, 2)
            entry["duration_sec"] = round(trim.end_sec - trim.start_sec, 1)
        else:
            entry["reason"] = trim.label
        summary["strategies"][name] = entry

    summary_path = os.path.join(output_dir, "summary.json")
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"Summary: {summary_path}", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description="Trim testing rig")
    parser.add_argument("video_path", nargs="?", help="Path to input video")
    parser.add_argument("keypoints_path", nargs="?", help="Path to keypoints.json")
    parser.add_argument("--test-dir",
                        help="Directory with video+keypoints pairs (*.mp4 + matching *.json)")
    parser.add_argument("--strategy", default="all",
                        help=f"Strategy name or 'all' (available: {', '.join(STRATEGIES.keys())})")
    parser.add_argument("--output-dir", default=None,
                        help="Output directory (default: spikes/crop-rig/output/<video-name>)")
    parser.add_argument("--scale", type=float, default=0.5,
                        help="Scale factor for boundary frame images (default: 0.5)")
    args = parser.parse_args()

    if args.strategy == "all":
        strategy_names = list(STRATEGIES.keys())
    else:
        strategy_names = [s.strip() for s in args.strategy.split(",")]
        for name in strategy_names:
            if name not in STRATEGIES:
                print(f"Unknown strategy: {name}", file=sys.stderr)
                print(f"Available: {', '.join(STRATEGIES.keys())}", file=sys.stderr)
                sys.exit(1)

    if args.test_dir:
        pairs = discover_test_pairs(args.test_dir)
        if not pairs:
            print(f"No video+keypoints pairs found in {args.test_dir}", file=sys.stderr)
            print("Convention: <name>.mp4 + <name>.json in same directory", file=sys.stderr)
            sys.exit(1)
        print(f"Found {len(pairs)} test videos:", file=sys.stderr)
        for _, _, name in pairs:
            print(f"  {name}", file=sys.stderr)
        for video_path, keypoints_path, name in pairs:
            output_dir = os.path.join("spikes", "crop-rig", "output", name)
            run_rig(video_path, keypoints_path, strategy_names, output_dir, args.scale,
                    video_label=name)
    else:
        if not args.video_path or not args.keypoints_path:
            parser.error("Provide video_path and keypoints_path, or use --test-dir")
        output_dir = args.output_dir
        if output_dir is None:
            label = args.strategy if args.strategy != "all" else "comparison"
            output_dir = os.path.join("spikes", "crop-rig", "output", f"trim-{label}")
        run_rig(args.video_path, args.keypoints_path, strategy_names, output_dir, args.scale)


if __name__ == "__main__":
    main()
