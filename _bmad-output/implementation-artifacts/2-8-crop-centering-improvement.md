# Story 2.8: Crop Centering Improvement

Status: draft

## Story

As a lifter,
I want the crop to be tight around me and properly centered,
so that I am magnified and centered in the cropped video rather than offset to one side.

## Problem

The current crop algorithm (`computeCropRegion` in `internal/pipeline/stages/crop.go`) centers the crop rectangle on the center of the **union bounding box** across all frames. This causes two issues:

1. **Insufficient magnification** (fixed): `cropPaddingPercent` was reduced from 0.15 to 0.02 (2% of box dimension on each side).

2. **Off-center lifter**: The union bounding box center (`(minLeft+maxRight)/2, (minTop+maxBottom)/2`) does not represent where the lifter typically is. If the lifter's bounding box shifts asymmetrically in a few frames (e.g., barbell swing during the catch, leaning during recovery), the union box center skews away from the lifter's typical position. After 9:16 aspect ratio enforcement and frame-edge clamping, the lifter ends up off-center (currently too far left).

## Acceptance Criteria (BDD)

1. **Given** keypoints.json exists with per-frame bounding boxes, **When** the crop stage computes the crop region, **Then** the crop rectangle is centered on the **median** per-frame bounding box center (median of per-frame horizontal centers, median of per-frame vertical centers) rather than the center of the union bounding box, **And** the crop dimensions still use the union bounding box extents (to ensure no frame clips the lifter)

2. **Given** the lifter is positioned near one edge of the source frame, **When** the crop region is clamped to frame bounds, **Then** the lifter remains as centered as physically possible within the crop (clamping shifts the box only as much as needed to stay within frame bounds)

3. **Given** the crop padding is 2% (`cropPaddingPercent = 0.02`), **When** the crop stage runs, **Then** the lifter is visibly magnified compared to the original frame, **And** the lifter is not clipped in any frame

4. **Given** all existing crop tests pass, **When** the centering logic is updated, **Then** the crop still produces a valid 9:16 video, **And** crop-params.json is still written with correct values, **And** graceful degradation (no keypoints) still works unchanged

## Tasks / Subtasks

- [ ] Refactor `computeCropRegion` in `internal/pipeline/stages/crop.go` (AC: 1, 2)
  - [ ] Compute per-frame bounding box centers: for each frame, `cx = (bb.Left + bb.Right) / 2`, `cy = (bb.Top + bb.Bottom) / 2`
  - [ ] Compute median of all `cx` values and median of all `cy` values (in normalized 0-1 coords)
  - [ ] Convert median center to pixel coordinates (`medianCX * sourceW`, `medianCY * sourceH`)
  - [ ] Keep existing union bounding box logic for crop **dimensions** (the union extents + padding determine how large the crop is)
  - [ ] Replace the centering step (currently lines 196-209): instead of centering the aspect-ratio-enforced box on `(pxLeft+pxRight)/2, (pxTop+pxBottom)/2` (union box center), center it on the median center. This includes the center computation at lines 196-197 and the re-centering assignment at lines 208-209.
  - [ ] Keep existing clamping logic (lines 212-233) and even-dimension rounding (lines 236-246) unchanged

- [ ] Add a `median` helper function in `crop.go` (AC: 1)
  - [ ] `func median(values []float64) float64` — sorts a copy of the slice, returns the middle value (or average of two middle values for even-length slices)
  - [ ] Only used by `computeCropRegion`, keep unexported

- [ ] Update unit tests in `internal/pipeline/stages/crop_test.go` (AC: 4)
  - [ ] Existing tests must still pass (9:16 aspect ratio, crop-params.json, graceful degradation)
  - [ ] Add test: crop centers on median frame center, not union box center — construct frames where one outlier frame shifts the union center significantly, verify the crop center tracks the median instead
  - [ ] Add test: symmetric bounding boxes produce a centered crop (median and union center coincide)

## Prerequisites

- Story 2.6 (Auto-Crop to Lifter) must be complete — this story modifies the crop region computation introduced in 2.6.

## Dev Notes

- **Dimensions from union, position from median.** The key insight is to decouple the crop's SIZE (which must encompass all frames) from its POSITION (which should track where the lifter typically is). The union bounding box determines the minimum crop size. The median per-frame center determines where to place that crop.
- **Why median, not mean?** The median is robust to outlier frames (e.g., a single frame where YOLO's bounding box jumps due to occlusion or a second person entering frame). The mean would be pulled by outliers.
- **Clamping still applies.** If the lifter is near the frame edge, the crop will still be shifted to stay within bounds. This is physically unavoidable — we can't crop outside the frame. But the median center minimizes how often this happens compared to the union center.
- **No changes to pose.py or keypoints.json format.** This is purely a change to how the crop stage interprets the existing bounding box data.
- The `computeCropRegion` function signature stays the same: `func computeCropRegion(frames []pose.Frame, sourceW, sourceH int) (x, y, w, h int)`.
- **Even-dimension rounding** (lines 236-246) ensures FFmpeg codec compatibility. This step runs after clamping and must be preserved as-is.

### Existing Code Reference

The change is isolated to `computeCropRegion` in `internal/pipeline/stages/crop.go` (lines 145-249). The function currently:
1. Computes union bounding box (lines 147-166)
2. Converts to pixel coords and computes box dimensions (lines 169-178)
3. Adds padding and recomputes box dimensions (lines 180-190)
4. Enforces 9:16 aspect ratio and centers crop (lines 193-209) — center computed at lines 196-197 from union box edges, aspect ratio adjusted at lines 199-205, re-centering applied at lines 208-209
5. Clamps to frame bounds (lines 212-233)
6. Rounds to integers and enforces even dimensions (lines 236-246)

Step 4 changes: replace center computation (lines 196-197) with median per-frame center; keep aspect ratio logic and re-centering assignment unchanged.

### Architecture Compliance

- No new files created
- No interface changes
- No changes to keypoints.json format or pose.py
- crop-params.json format unchanged
- All existing downstream consumers (skeleton stage) unaffected

### References

- [Source: internal/pipeline/stages/crop.go] — current implementation
- [Source: epics.md#Story 2.6] — original crop acceptance criteria
- [Source: internal/pipeline/stages/crop_test.go] — existing tests

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
