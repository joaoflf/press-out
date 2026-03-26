# Trim Strategy Exploration

## Goal

Trim weightlifting videos to include only the lift: from just before the pull to after the recovery stand-up, before the bar is dropped or the lifter walks away.

## Test Bed

| Video | Type | Challenge |
|---|---|---|
| sample-snatch-1 | Snatch | Standard lift |
| sample-snatch-2 | Snatch | Standard lift, late in video |
| sample-cj-walk-away | Clean & Jerk | Walk between clean and jerk — must be included |
| sample-clean-walk-away | Clean | Walk with bar on shoulders after lift — must be excluded |

Key constraints:
- Trim end = before bar is dropped (consistent across all lift types)
- Walking = horizontal translation, Lifting = articulated vertical movement
- CJ walk-between must be included; clean walk-after must be excluded

## Current Best: density-bridged (with ankle recovery)

Extends energy-density with two additions: wider minimum window (5s instead of 3s) to span multi-phase lifts, and ankle split recovery extension for jerk catches.

### Algorithm
1. Compute per-frame motion diversity (deviation from rigid translation)
2. Smooth with 7-frame moving average
3. Sliding window (**5–12s**, scored by avg diversity/frame) finds the densest articulated-motion segment
4. **Ankle split recovery**: if ankle X gap >= 0.08 at window end, extend until gap < 0.05 (capped at 3s max, 5-frame smoothed)
5. Apply 1.25s padding to final boundaries

### Latest Results

| Video | density-bridged | Assessment |
|---|---|---|
| **cj-walk-away** | **4.25–15.05s (10.8s)** | Full CJ: clean setup through jerk recovery. Feet together, bar overhead at end. Recovery +2.3s. |
| **clean-walk-away** | 4.62–12.08s (7.5s) | Lifter bent over bar at end, no walking. Ankle scan not triggered. |
| **snatch-1** | 4.93–12.03s (7.1s) | Setup through standing with bar overhead. Ankle scan not triggered. |
| **snatch-2** | 14.28–20.50s (6.2s) | Setup through overhead. Ankle scan not triggered. |

### Key Design Decisions

1. **MIN_WIN_SEC = 5.0**: Forces windows wide enough to span CJ's clean+walk+jerk. The 6s window has density only 7% lower than the 3s jerk-only window.
2. **Ankle split recovery**: After jerk catch, lifter has feet in wide split (ankle X gap ~0.24). Forward scan detects when ankles converge (gap < 0.05), extending end by ~2.3s for CJ. Does not fire for cleans (no split) or snatches (consistent foot width ~0.01-0.03).
3. **Thresholds**: SPLIT_DETECT_GAP=0.08 (between normal ~0.03 and split ~0.24), SPLIT_CONVERGE_GAP=0.05, ANKLE_SMOOTH=5 frames to ignore single-frame noise.
4. COCO-17 ankle keypoints: `left_ankle` (index 15), `right_ankle` (index 16).

### Approaches Tried and Rejected for CJ Fix

1. **Two-pass density scoring** (prefer highest total among windows above density threshold): threshold too fragile — either too permissive (selects full video) or too strict (no effect)
2. **Diversity threshold + run merging**: diversity signal too noisy — walking and lifting have overlapping values, so thresholding bleeds everywhere
3. **Secondary peak bridging**: searches for nearby dense regions to merge. Diversity too uniform to distinguish genuine secondary lifts from background noise
4. **Wrist-based end extension**: extends while wrists are high (bar held). Works for clean walk-away (wrists drop when bar released) but CJ bar stays overhead too long (until ~15s), so extension overshoots
5. **Asymmetric end padding** (2.0s): helps CJ recovery but pushes clean-walk-away into walking territory

## Round 1 Results (deprecated — kept for reference)

| Strategy | cj-walk-away | clean-walk-away | snatch-1 | snatch-2 |
|---|---|---|---|---|
| **baseline** | 3.13–15.70s (12.6s) | 4.33–13.37s (9.0s) | 0.00–5.09s (5.1s) | 14.83–20.50s (5.7s) |
| **vertical-weighted** | 3.63–15.70s (12.1s) | 4.80–13.37s (8.6s) | 0.00–4.56s (4.6s) | 15.33–20.50s (5.2s) |
| **adaptive-threshold** | 3.57–15.70s (12.1s) | 5.33–13.37s (8.0s) | 0.03–12.03s (12.0s) | 9.03–20.50s (11.5s) |
| **last-peak** | 3.22–11.58s (8.4s) | FAILED (too short) | 4.70–9.43s (4.7s) | 14.85–20.50s (5.6s) |
| **wrist-rise** | 0.00–15.37s (15.4s) | 1.60–13.37s (11.8s) | 1.86–12.03s (10.2s) | 11.67–20.50s (8.8s) |
| velocity-spike | 12.06–15.13s (3.1s) | 11.03–13.37s (2.3s) | 8.84–12.03s (3.2s) | 15.10–20.50s (5.4s) |
| energy-window | 0.00–14.16s (14.2s) | 0.00–13.37s (13.4s) | 0.00–12.03s (12.0s) | 7.03–20.50s (13.5s) |
| cumul-slope | FAILED | 0.00–13.37s (13.4s) | 0.00–12.03s (12.0s) | 7.72–20.50s (12.8s) |

### Round 1 Learnings

1. **Y-weighting helps marginally** but doesn't solve walk-away. Walking with bar on shoulders still has vertical bounce. For cleans (bar only to shoulders), the vertical range during the lift is small (0.18 of frame height), comparable to walking bounce.
2. **Motion diversity is the breakthrough signal** — it cleanly separates articulated motion (lifting) from rigid translation (walking), regardless of direction.
3. **Expansion is dangerous** — all expansion approaches (Y-weighted, standard displacement, diversity) tend to bleed into walking periods because the floor threshold is hard to tune.
4. **No expansion + padding** gives the most reliable boundaries for single lifts.
5. **CJ remains the hardest case** — needs a mechanism to bridge the walk gap between clean and jerk while keeping the window tight for single lifts.

## Implementation Notes

- Helper functions: `_compute_motion_diversity()` and `_compute_y_weighted_displacement()` in trim_rig.py
- Test rig supports `--test-dir` for multi-video batch runs and comma-separated `--strategy` names
- Validation report (`validate.py --open`) shows cross-video comparison table + per-video timelines and boundary frames
- Keypoints: COCO-17 format, coords normalized 0-1, Y=0 is top of frame
