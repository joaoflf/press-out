# Story 2.9: Density-Bridged Trim Algorithm

Status: ready-for-dev

## Story

As a lifter,
I want the system to accurately trim my video to just the lift using motion diversity analysis,
so that clean & jerk lifts include both phases while excluding post-lift walking, and snatches/cleans are tightly framed.

## Problem

The current trim algorithm (baseline displacement threshold + gap bridging) cannot distinguish walking from lifting — both produce similar displacement magnitudes. This causes:
- Clean & Jerk: walk-away included (displacement stays high during walking)
- Clean: walk-away sometimes included for the same reason
- Snatches: boundaries are loose because the algorithm lacks a strong lift-vs-idle signal

The spike-validated "density-bridged" strategy solves this using **motion diversity** — the deviation of per-keypoint displacements from the mean displacement vector. During lifting, body parts move in different directions (high diversity). During walking, all keypoints translate uniformly (low diversity). This signal cleanly separates lift from walk.

## Acceptance Criteria

1. **Given** keypoints.json exists with a snatch lift, **When** the trim stage runs, **Then** the trimmed video starts before the pull and ends after recovery, with boundaries within 2s of the spike-validated results for `sample-snatch-1` and `sample-snatch-2`

2. **Given** keypoints.json exists with a clean & jerk, **When** the trim stage runs, **Then** the trimmed video includes both the clean and the jerk (including the walk between them), **And** excludes post-jerk walking away, with boundaries within 2s of the spike-validated results for `sample-cj-walk-away`

3. **Given** keypoints.json exists with a clean (no jerk), **When** the trim stage runs, **Then** the trimmed video excludes post-lift walking, with boundaries within 2s of the spike-validated results for `sample-clean-walk-away`

4. **Given** keypoints.json does not exist, **When** the trim stage runs, **Then** the original video is preserved (graceful degradation, unchanged from current behavior)

5. **Given** keypoints.json has low/no motion, **When** the trim stage runs, **Then** the original video is preserved (density below threshold, unchanged from current behavior)

6. **Given** the trim stage encounters an FFmpeg error, **When** the subprocess fails, **Then** the stage returns an error to the orchestrator (unchanged from current behavior)

### Spike-Validated Reference Boundaries

These are the density-bridged outputs from `uv run spikes/crop-rig/trim_rig.py --test-dir testdata/videos --strategy density-bridged`:

| Video | Trim Start | Trim End | Notes |
|---|---|---|---|
| sample-cj-walk-away | ~4.25s | ~15.05s | Includes clean + walk + jerk. Feet together, bar overhead at end |
| sample-clean-walk-away | ~4.62s | ~12.08s | Clean only, excludes post-lift walk |
| sample-snatch-1 | ~4.93s | ~12.03s | Snatch with tight boundaries |
| sample-snatch-2 | ~14.28s | ~20.50s | Snatch later in video |

## Tasks / Subtasks

- [ ] Replace `detectLiftFromKeypoints` in `internal/pipeline/stages/trim.go` with density-bridged algorithm (AC: 1-3)
  - [ ] Add `computeMotionDiversity(frames []pose.Frame) []float64` — per-frame deviation from rigid translation
  - [ ] Add `getAnkleGap(frame pose.Frame) (float64, bool)` — horizontal distance between ankles
  - [ ] Replace constants: remove baseline constants, add density-bridged constants (see Dev Notes)
  - [ ] Implement density window search: prefix sum on smoothed diversity, sliding window scored by density (diversity/frames)
  - [ ] Implement ankle split recovery extension: detect jerk split stance, extend end until feet converge
  - [ ] Keep `smoothDisplacements` (rename to `smoothValues` since it's now used for diversity too) and `estimateFPS` — they're shared
  - [ ] Remove `computeDisplacements`, `mergeRuns`, `motionRun` — no longer needed
  - [ ] Keep `detectLiftDensityBridged` callable from crop stage (same `stages` package, unexported is fine) — story 2-10 depends on it

- [ ] Update test fixtures in `internal/pipeline/stages/trim_test.go` (AC: 1-6)
  - [ ] Replace `sampleKeypointsPath` to use `testdata/videos/sample-snatch-1.json` (old `testdata/keypoints-sample.json` was deleted)
  - [ ] Replace `sampleLiftVideo` to use `testdata/videos/sample-snatch-1.mp4` (old `testdata/videos/sample-lift.mp4` was deleted)
  - [ ] Update `TestDetectLiftFromKeypoints` expected boundaries for new algorithm
  - [ ] Add `TestDetectLiftDensityBridged_AllVideos` — table-driven test loading all 4 keypoints JSONs, verifying each produces confident=true with start and end within 2s tolerance of reference:
    - `sample-snatch-1.json`: start ~4.93s, end ~12.03s
    - `sample-snatch-2.json`: start ~14.28s, end ~20.50s
    - `sample-cj-walk-away.json`: start ~4.25s, end ~15.05s
    - `sample-clean-walk-away.json`: start ~4.62s, end ~12.08s
  - [ ] Keep existing tests: `TestTrimStage_Name`, `TestTrimStage_NoKeypoints`, `TestTrimStage_LowConfidence`, `TestTrimStage_FFmpegFailure`, `TestTrimStage_ContextCancellation`

## Dev Notes

### Algorithm: Density-Bridged (source: `spikes/crop-rig/trim_rig.py`, lines 482-593)

**Phase 1 — Motion diversity signal:**
For each consecutive frame pair, compute the mean displacement vector (rigid body translation), then measure how much each keypoint deviates from it. High deviation = lifting (body parts moving in different directions). Low deviation = walking (all keypoints translate uniformly).

```
For frames[i-1] -> frames[i]:
  1. Collect per-keypoint displacement vectors (dx, dy) for keypoints with confidence >= 0.5
  2. Compute mean vector: mean_dx = avg(dxs), mean_dy = avg(dys)
  3. Compute per-keypoint deviation: sqrt((dx - mean_dx)^2 + (dy - mean_dy)^2)
  4. diversity[i] = average of all deviations
  Result: one value per frame transition (len = frames-1)
```

**Phase 2 — Densest window search:**
Smooth the diversity signal, then slide windows of varying size across it. Score each window by density (total diversity / window size). The window with the highest density is the core lift.

```
1. Smooth diversity with moving average (window=7)
2. Build prefix sum for O(1) window queries
3. For win_size in range(MIN_WIN_SEC*fps, MAX_WIN_SEC*fps, WIN_STEP_SEC*fps):
     For each position: density = (cumul[end] - cumul[start]) / win_size
     Track best (highest density)
4. Reject if best_density < MIN_PEAK_DENSITY
```

**Phase 2.5 — Ankle split recovery extension:**
After a jerk catch, the lifter is in a split stance (wide ankle X gap). If the density window ends during a split, extend until feet come together.

```
1. Check ankle X gap at/near window end (look ahead 0.5s)
2. If max gap >= SPLIT_DETECT_GAP (0.08):
   a. Scan forward up to MAX_RECOVERY_SEC (3.0s)
   b. Smooth raw ankle gaps (window=5)
   c. Extend end to first frame where smoothed gap < SPLIT_CONVERGE_GAP (0.05)
   d. If no convergence found, extend to scan limit
```

**Phase 3 — Convert to seconds + padding:**
```
start_sec = frames[best_start + 1].time_offset_ms / 1000.0
end_sec = frames[end_frame].time_offset_ms / 1000.0
Apply PADDING_SEC (1.25s) on both sides, clamp to video duration
Reject if duration < MIN_DURATION_SEC or > MAX_DURATION_SEC
```

### Constants (must be named constants in Go)

```go
const (
    trimMinKeypointConfidence = 0.5
    trimSmoothWindow          = 7
    trimMinWinSec             = 5.0    // wider min to capture CJ's clean+walk+jerk
    trimMaxWinSec             = 12.0
    trimWinStepSec            = 0.5
    trimPaddingSec            = 1.25
    trimMinDurationSec        = 2.0
    trimMaxDurationSec        = 18.0
    trimMinPeakDensity        = 0.002
    trimSplitDetectGap        = 0.08   // ankle X gap indicating jerk split
    trimSplitConvergeGap      = 0.05   // recovery complete when gap narrows to this
    trimMaxRecoverySec        = 3.0    // max forward extension from window end
    trimAnkleSmoothWindow     = 5      // frames for smoothing noisy ankle gap
)
```

### Key Differences from Current Code

| Aspect | Current (baseline) | New (density-bridged) |
|---|---|---|
| Signal | Frame-to-frame displacement (Euclidean distance) | Motion diversity (deviation from rigid translation) |
| Selection | Threshold → merge runs → best run by total displacement | Sliding window → best by density (avg diversity per frame) |
| Walk handling | None — walking has high displacement, indistinguishable from lift | Diversity is LOW during walking (uniform translation) → naturally excluded |
| CJ support | None — walk between clean and jerk breaks the run | MIN_WIN=5s forces window wide enough to span both phases |
| Jerk recovery | None | Ankle split detection + extension until feet converge |
| Padding | 1.5s symmetric | 1.25s symmetric |

### Functions to Keep (shared utilities)

- `estimateFPS(frames []pose.Frame) float64` — unchanged
- `smoothDisplacements` → rename to `smoothValues(values []float64, window int) []float64` — generalize to accept any signal and window size

### Functions to Remove

- `computeDisplacements` — replaced by `computeMotionDiversity`
- `mergeRuns` — no longer needed (density window replaces run merging)
- `motionRun` struct — no longer needed

### Functions to Add

- `computeMotionDiversity(frames []pose.Frame) []float64` — core signal computation
- `getAnkleGap(frame pose.Frame) (float64, bool)` — for jerk recovery detection
- `detectLiftDensityBridged(keypointsPath string) (startSec, endSec float64, confident bool, err error)` — replaces `detectLiftFromKeypoints`

### Keypoints Reference

COCO-17 keypoints used: all 17 for diversity computation, `left_ankle` + `right_ankle` for split detection. Coordinates are normalized 0-1, Y=0 is top of frame. The `internal/pose` package already defines landmark constants — use `pose.LandmarkLeftAnkle` and `pose.LandmarkRightAnkle` in `getAnkleGap`.

### Test Videos

Old test fixtures (`testdata/keypoints-sample.json`, `testdata/videos/sample-lift.mp4`) were deleted. The 4 new test videos with matching keypoints JSON:

| File | Type | Description |
|---|---|---|
| `testdata/videos/sample-snatch-1.mp4` + `.json` | Snatch | Standard snatch |
| `testdata/videos/sample-snatch-2.mp4` + `.json` | Snatch | Second snatch |
| `testdata/videos/sample-cj-walk-away.mp4` + `.json` | Clean & Jerk | Walk between clean and jerk (walk INCLUDED in trim) |
| `testdata/videos/sample-clean-walk-away.mp4` + `.json` | Clean | Walk after lift (walk EXCLUDED from trim) |

### Architecture Compliance

- Stage interface unchanged: `Name() string`, `Run(ctx, StageInput) (StageOutput, error)`
- Same file I/O: reads `keypoints.json`, writes `trimmed.mp4`
- Same graceful degradation: no keypoints → preserve original, low confidence → preserve original
- Same slog logging: `lift_id`, `stage`, `duration_ms` attributes
- Same FFmpeg integration: `ffmpeg.TrimVideo`, `ffmpeg.GetDuration`
- No new files created — all changes in existing `trim.go` and `trim_test.go`
- No changes to `crop.go`, `pose.go`, or any other package

### Previous Story Intelligence

- Story 2.5 (pose-based trim): introduced `detectLiftFromKeypoints`, `computeDisplacements`, `smoothDisplacements`, `mergeRuns`. This story replaces the core algorithm.
- Story 2.8 (crop centering): status=draft, NOT implemented. Median centering concept will be used in the crop story (2-10), not this one.
- Git commit `0fd519a`: "feat: pose-based video trim using keypoint displacement" — the commit being replaced.
- Story 2.6 spike identified that trim must run AFTER pose (needs keypoints.json). Pipeline order: Pose → Trim → Crop. This is already correct.

### References

- [Source: spikes/crop-rig/trim_rig.py#density-bridged] — spike implementation (lines 482-593)
- [Source: spikes/crop-rig/trim_rig.py#_compute_motion_diversity] — diversity signal (lines 355-383)
- [Source: spikes/crop-rig/trim_rig.py#_get_ankle_gap] — ankle gap helper (lines 386-399)
- [Source: internal/pipeline/stages/trim.go] — current implementation to replace
- [Source: internal/pipeline/stages/trim_test.go] — tests to update
- [Source: internal/pipeline/stage.go] — Stage interface (unchanged)

## Follow-Up

Story 2-10 will port the hybrid crop algorithm from `spikes/crop-rig/crop_spike.py` to Go. It is independent of this story — both can be developed in any order since trim and crop are separate pipeline stages.

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
