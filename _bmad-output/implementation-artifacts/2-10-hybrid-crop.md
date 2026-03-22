# Story 2.10: Hybrid Crop with Walk Tracking

Status: ready-for-dev

## Story

As a lifter,
I want the crop to track my position when I walk between lifts (clean to jerk) and lock steady when I'm lifting,
so that I stay centered in the frame throughout the entire clean & jerk without the camera jittering during the lift.

## Problem

The current crop algorithm uses a single static rectangle (union bounding box across all frames). This causes two issues:

1. **Poor centering for CJ**: When the lifter walks from clean position to jerk position, the union bbox center lands between the two positions — neither clean nor jerk is centered.
2. **Bar clipped at top**: The union bbox + small padding doesn't account for the barbell + plates extending above the head. The bar gets clipped in overhead positions.

The spike-validated "hybrid" crop solves both:
- **Extent-based Y sizing**: Uses `min(tops) - bar_padding` to `P95(bottoms) + foot_padding`, ensuring bar + plates are never clipped at top
- **Hybrid X centering**: Classifies frames as walking or stationary via smoothed X velocity. Stationary segments lock at per-segment median X. Walking segments linearly interpolate between adjacent lock points — smooth pan, no jitter.

## Acceptance Criteria

1. **Given** keypoints.json exists for a snatch, **When** the crop stage runs, **Then** all frames are classified as stationary (0% walking), the crop locks at a single X position, **And** crop dimensions match spike reference within 20px tolerance

2. **Given** keypoints.json exists for a clean & jerk with walk between lifts, **When** the crop stage runs, **Then** walking frames are detected (~12%), the crop smoothly pans during the walk and locks during each lift segment, **And** the lifter stays centered throughout, **And** crop dimensions match spike reference within 20px tolerance

3. **Given** keypoints.json exists for a clean with post-lift walk, **When** the crop stage runs, **Then** the crop handles any minor walking within the trimmed window, **And** crop dimensions match spike reference within 20px tolerance

4. **Given** the bar is overhead (snatch catch, jerk lockout), **When** the crop is computed, **Then** the bar + plates are fully visible (not clipped at the top of the frame)

5. **Given** keypoints.json does not exist, **When** the crop stage runs, **Then** the original video is preserved and a thumbnail is extracted (unchanged graceful degradation)

6. **Given** the crop stage encounters an FFmpeg error, **When** the subprocess fails, **Then** the stage returns an error to the orchestrator (unchanged)

### Spike-Validated Reference Values (hybrid-9x16)

From `uv run spikes/crop-rig/crop_spike.py --test-dir testdata/videos`:

| Video | Crop W | Crop H | Walking % | Lock Positions (X) | Notes |
|---|---|---|---|---|---|
| sample-cj-walk-away | 642 | 1142 | 12% | 516, 292 | Two segments with pan between |
| sample-clean-walk-away | 548 | 976 | 5% | 550, 407 | Minor walk at trim end |
| sample-snatch-1 | 544 | 968 | 0% | 529 | Single static lock |
| sample-snatch-2 | 622 | 1106 | 0% | 562 | Single static lock |

## Tasks / Subtasks

- [ ] Replace `computeCropRegion` in `internal/pipeline/stages/crop.go` with extent-based sizing (AC: 1-4)
  - [ ] Replace union-bbox sizing with extent-based Y: `crop_top = min(tops) - BAR_TOP_PADDING_PX`, `crop_bottom = P95(bottoms) + FOOT_BOTTOM_PADDING_PX`
  - [ ] Width from 9:16 aspect ratio applied to extent height; also ensure body+plate width covered via P95(widths) * (1 + 2 * CROP_PADDING_H)
  - [ ] Clamp to frame bounds, round to even dimensions
  - [ ] Remove old union-bbox constants (`cropPaddingPercent`); add new extent-based constants

- [ ] Add hybrid X centering in `internal/pipeline/stages/crop.go` (AC: 1-3)
  - [ ] Add `computeCropCenters(frames []pose.Frame, sourceW, sourceH int) (cxList, cyList []float64)` — hybrid mode
  - [ ] Classify frames as walking/stationary via smoothed X velocity (threshold: HYBRID_X_VEL_THRESHOLD px/frame)
  - [ ] Find contiguous segments, compute lock X per stationary segment (median raw bbox center X)
  - [ ] Walking segments: linearly interpolate between adjacent lock points
  - [ ] Y is always static: median of raw bbox center Y across all frames

- [ ] Filter keypoints to trimmed video window (AC: 1-3)
  - [ ] Call `detectLiftDensityBridged` (from story 2-9) to get trim boundaries from keypoints.json
  - [ ] Filter frames to `[trimStartMs, trimEndMs]`
  - [ ] Use filtered frames for crop size and position computation

- [ ] Add per-frame crop via FFmpeg frame piping (AC: 1-3)
  - [ ] Add `DecodeFrames(ctx, input string, fps float64) (*exec.Cmd, io.ReadCloser, error)` to `internal/ffmpeg/ffmpeg.go` — starts FFmpeg decode to raw RGB24 pipe
  - [ ] Add `EncodeFrames(ctx, output string, w, h int, fps float64) (*exec.Cmd, io.WriteCloser, error)` to `internal/ffmpeg/ffmpeg.go` — starts FFmpeg encode from raw RGB24 pipe
  - [ ] Convert keypoint-rate positions to video frame rate via linear interpolation
  - [ ] Per-frame loop: read source frame from decode pipe, crop in Go (slice pixel buffer), write to encode pipe
  - [ ] Handle pipe cleanup and process wait on error/completion

- [ ] Update crop-params.json output (AC: 1-3)
  - [ ] `x`: first stationary segment's lock X origin (crop_cx - crop_w/2, clamped)
  - [ ] `y`: origin_y from extent-based computation
  - [ ] `w`, `h`: constant crop dimensions
  - [ ] `sourceWidth`, `sourceHeight`: unchanged
  - [ ] Format compatible with skeleton stage (Story 3.1)

- [ ] Update tests in `internal/pipeline/stages/crop_test.go` (AC: 1-6)
  - [ ] Replace `sampleKeypointsPath` and `sampleLiftVideo` to use `testdata/videos/sample-snatch-1.*` (old fixtures deleted)
  - [ ] Add multi-video validation test: load all 4 test video keypoints, verify crop dimensions within 20px of reference table
  - [ ] Add test: CJ video produces walking segments with interpolated X
  - [ ] Add test: snatch produces 0% walking (all stationary)
  - [ ] Keep existing tests: `TestCropStage_NoKeypoints`, `TestCropStage_FFmpegFailure`, `TestCropStage_ThumbnailFailureNonFatal`, `TestCropStage_ContextCancellation`
  - [ ] Update `TestComputeCropRegion` for new function signature (extent-based)
  - [ ] Remove obsolete `TestComputeCropRegion_EdgeClamp`, `TestComputeCropRegion_FullFrame` if they test union-bbox-specific behavior

## Dev Notes

### Algorithm: Hybrid Crop (source: `spikes/crop-rig/crop_spike.py`)

The crop has two independent computations: **sizing** (how big is the crop rectangle) and **positioning** (where is it placed per frame).

#### Crop Sizing: Extent-Based Y (`compute_crop_region`, lines 71-122)

Sizes the crop to span from above the overhead bar down to below the feet:

```
1. tops = [f.bounding_box.top * sourceH for f in trimmed_frames]
2. bottoms = [f.bounding_box.bottom * sourceH for f in trimmed_frames]
3. widths = [(f.bounding_box.right - f.bounding_box.left) * sourceW for f in trimmed_frames]

4. crop_top = min(tops) - BAR_TOP_PADDING_PX          // bar + plates overhead
5. crop_bottom = P95(bottoms) + FOOT_BOTTOM_PADDING_PX // feet

6. box_h = crop_bottom - crop_top
7. box_w = box_h * (9/16)                              // 9:16 aspect ratio

8. min_w = P95(widths) * (1 + 2 * CROP_PADDING_H)     // body + plates width
9. if min_w > box_w:
     box_w = min_w
     box_h = box_w / (9/16)
     re-center vertically around extent midpoint

10. Clamp to frame, round to even
```

Key insight: uses `min(tops)` not a percentile — overhead frames are rare but MUST be covered. Uses `P95(bottoms)` because outlier bottom values are usually noise.

#### Crop Positioning: Hybrid X (`compute_crop_centers`, lines 125-220)

Three-pass approach:

```
Pass 1 — Classify frames:
  1. raw_cx = per-frame bbox center X in pixels
  2. Smooth raw_cx with TRACKING_SMOOTH_FRAMES (31) window
  3. x_vel = abs(smoothed_cx[i] - smoothed_cx[i-1]) for each frame
  4. Smooth x_vel with HYBRID_VEL_SMOOTH_FRAMES (15) window
  5. is_walking[i] = x_vel_smooth[i] > HYBRID_X_VEL_THRESHOLD (3.0 px/frame)

Pass 2 — Find segments and lock points:
  6. Find contiguous segments of walking/stationary
  7. For each stationary segment: lock_x = median(raw_cx[start:end])

Pass 3 — Assign X positions:
  8. Stationary frames: x = lock_x
  9. Walking frames: linearly interpolate between prev_lock and next_lock
  10. If only one adjacent lock exists, hold that value
  11. If no adjacent locks, fall back to smoothed tracking
```

Y is always static: `median(raw_cy)` across all frames. Body articulation (bend → stand → overhead) moves the bbox center vertically, but that's body motion, not the lifter translating in frame.

#### Frame Piping

Convert keypoint-rate positions to video frame rate, then pipe through FFmpeg:

```
Decode:  ffmpeg -i input.mp4 -f rawvideo -pix_fmt rgb24 pipe:1
Encode:  ffmpeg -y -f rawvideo -pix_fmt rgb24 -s WxH -r FPS -i pipe:0
         -c:v libx264 -preset fast -pix_fmt yuv420p -movflags +faststart output.mp4

Per frame:
  1. Read sourceW * sourceH * 3 bytes from decode stdout
  2. For each row in [y, y+cropH): copy cropW*3 bytes starting at x*3
  3. Write cropW * cropH * 3 bytes to encode stdin
```

#### Keypoint Frame Filtering

The crop stage receives `trimmed.mp4` but keypoints.json covers the full original video. To filter:

```go
// Re-compute trim boundaries from keypoints (same function from story 2-9)
trimStart, trimEnd, confident, err := detectLiftDensityBridged(keypointsPath)

// Filter to trimmed window
var trimmedFrames []pose.Frame
for _, f := range result.Frames {
    tSec := float64(f.TimeOffsetMs) / 1000.0
    if tSec >= trimStart && tSec <= trimEnd {
        trimmedFrames = append(trimmedFrames, f)
    }
}
```

If trim detection is not confident (original video preserved), use all frames.

### Constants (must be named constants in Go)

```go
const (
    cropAspectW        = 9    // unchanged
    cropAspectH        = 16   // unchanged

    // Extent-based Y sizing
    barTopPaddingPx      = 150   // px above min bbox top for bar + plates overhead
    footBottomPaddingPx  = 40    // px below P95 bbox bottom for feet
    cropHorizontalPad    = 0.30  // horizontal padding around P95 body width (bar + plates)

    // Hybrid X tracking
    trackingSmoothFrames    = 31    // ~1s at 30fps for smoothing raw bbox X
    hybridXVelThreshold     = 3.0   // px/frame — below this, lifter is "stationary"
    hybridVelSmoothFrames   = 15    // smooth velocity signal to debounce
)
```

### Key Differences from Current Code

| Aspect | Current (union bbox) | New (hybrid extent-based) |
|---|---|---|
| Y sizing | Union bbox + 2% padding | min(tops) - 150px to P95(bottoms) + 40px |
| X centering | Center of union bbox (or median per story 2.8 draft) | Per-frame hybrid: lock during lift, interpolate during walk |
| Aspect ratio | 9:16 enforced by expanding | 9:16 from extent height, with body+plate width minimum |
| Walk handling | None — single static position | Detects walking via X velocity, smooth pan |
| FFmpeg approach | Single static crop filter | Per-frame: decode pipe → Go crop → encode pipe |
| Bar clipping | Possible with tight bbox | BAR_TOP_PADDING_PX prevents clipping |

### New FFmpeg Functions (`internal/ffmpeg/ffmpeg.go`)

Two new functions for frame decode/encode piping:

```go
// DecodeFrames starts FFmpeg decoding to raw RGB24 frames piped to stdout.
// Caller must read from the returned ReadCloser and call cmd.Wait() when done.
func DecodeFrames(ctx context.Context, input string) (*exec.Cmd, io.ReadCloser, error)

// EncodeFrames starts FFmpeg encoding from raw RGB24 frames piped from stdin.
// Caller must write to the returned WriteCloser, close it, then call cmd.Wait().
func EncodeFrames(ctx context.Context, output string, w, h int, fps float64) (*exec.Cmd, io.WriteCloser, error)
```

Both must include `-y` flag (convention). The encode command uses `-c:v libx264 -preset fast -pix_fmt yuv420p -movflags +faststart`.

### Functions to Remove

- `computeCropRegion(frames []pose.Frame, sourceW, sourceH int) (x, y, w, h int)` — replaced entirely
- `median` helper (if it was added from story 2.8 draft — 2.8 was never implemented, check first)

### Functions to Add

- `computeExtentCropRegion(frames []pose.Frame, sourceW, sourceH int) (w, h, originY int)` — extent-based sizing
- `computeHybridCenters(frames []pose.Frame, sourceW int) []float64` — per-frame X centers
- `smoothValues(values []float64, window int) []float64` — from story 2-9 (shared)
- `centersToOrigins(cxList []float64, cropW, sourceW int, originY int) (xs, ys []int)` — convert centers to top-left origins
- `interpolateToVideoFrames(kpTimes []float64, kpXs, kpYs []int, videoFPS float64, trimStart, trimEnd float64) (startFrame int, positions [][2]int)` — interpolate to video frame rate
- `percentile(values []float64, p float64) float64` — P95 for bottoms and widths

### Dependency on Story 2-9

This story uses `detectLiftDensityBridged` from story 2-9 to filter keypoints to the trimmed window. Both functions live in the `stages` package (unexported). **Story 2-9 must be completed first.**

Also reuses `smoothValues` (the generalized smoothing function from 2-9) and `estimateFPS`.

### Test Videos

Same 4 test videos as story 2-9 (in `testdata/videos/`). All have matching `.json` keypoints files.

### Architecture Compliance

- Stage interface unchanged: `Name() string`, `Run(ctx, StageInput) (StageOutput, error)`
- Same file outputs: `cropped.mp4`, `crop-params.json`, `thumbnail.jpg`
- crop-params.json format unchanged (x, y, w, h, sourceWidth, sourceHeight)
- Same graceful degradation: no keypoints → preserve original + extract thumbnail
- Same slog logging: `lift_id`, `stage`, `duration_ms` attributes
- New FFmpeg functions in `internal/ffmpeg/ffmpeg.go` follow existing conventions (`-y` flag, context-based, slog logging)
- No changes to `trim.go`, `pose.go`, or `pipeline/stage.go`
- Aspect ratio remains 9:16

### Previous Story Intelligence

- Story 2.6: Original crop implementation — `computeCropRegion` with union bbox. This story replaces the entire crop algorithm.
- Story 2.8: Proposed median centering — status=draft, never implemented. The hybrid approach supersedes it.
- Story 2.5/2.9: Trim detection — the crop stage calls the same detection function to filter keypoints.
- `CropParams` struct and `extractThumbnail` helper are kept unchanged.
- `CropVideo` in `internal/ffmpeg/ffmpeg.go` is kept (not removed) — other code may use it. The crop stage just won't call it anymore.

### References

- [Source: spikes/crop-rig/crop_spike.py#compute_crop_region] — extent-based sizing (lines 71-122)
- [Source: spikes/crop-rig/crop_spike.py#compute_crop_centers] — hybrid X centering (lines 125-220)
- [Source: spikes/crop-rig/crop_spike.py#cx_to_origins] — center to origin conversion (lines 223-231)
- [Source: spikes/crop-rig/crop_spike.py#interpolate_to_video_frames] — frame rate interpolation (lines 234-245)
- [Source: spikes/crop-rig/crop_spike.py#crop_video] — per-frame crop via FFmpeg pipe (lines 250-294)
- [Source: internal/pipeline/stages/crop.go] — current implementation to replace
- [Source: internal/pipeline/stages/crop_test.go] — tests to update
- [Source: internal/ffmpeg/ffmpeg.go] — where to add DecodeFrames/EncodeFrames

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
