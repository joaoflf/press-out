# Story 2.6: Auto-Crop to Lifter

Status: draft

## Story

As a lifter,
I want the system to automatically crop the video to focus on me,
so that I see only my lift without distracting bystanders or background.

## Acceptance Criteria (BDD)

1. **Given** keypoints.json exists from the pose estimation stage, **When** the crop stage runs, **Then** the system uses the per-frame bounding boxes from keypoints.json to determine the lifter's region across all frames, **And** the crop region is expanded with padding and enforced to 9:16 aspect ratio, **And** the video is cropped via FFmpeg and saved as cropped.mp4 in the lift-ID directory, **And** crop parameters (x, y, w, h, source dimensions) are saved as crop-params.json in the lift-ID directory for downstream coordinate transformation

2. **Given** the crop stage successfully produces a cropped video, **When** the crop completes, **Then** a thumbnail is extracted from the cropped video via FFmpeg, **And** the thumbnail is saved as thumbnail.jpg in the lift-ID directory

3. **Given** keypoints.json does not exist (pose estimation was skipped), **When** the crop stage is reached, **Then** the full frame video is preserved as the crop output (FR6), **And** a thumbnail is still extracted from the uncropped video, **And** the stage completes without error (graceful degradation)

## Tasks / Subtasks

- [ ] Create crop stage at `internal/pipeline/stages/crop.go` (AC: 1, 2, 3)
  - [ ] `CropStage` struct implementing `pipeline.Stage` interface
  - [ ] `Name()` returns `"Cropping"` (matches stage name constant)
  - [ ] `Run(ctx context.Context, input StageInput) (StageOutput, error)` implementation

- [ ] Implement bounding box computation from keypoints.json (AC: 1)
  - [ ] Read `keypoints.json` from the lift directory (written by pose estimation stage — always contains a single person, selected by the pose stage)
  - [ ] Use the per-frame `boundingBox` fields to compute a single enclosing box across all frames — find min `left`/`top` and max `right`/`bottom` across all frames. This is simpler and more accurate than deriving from individual keypoints.
  - [ ] Define named constants for tuning:
    - [ ] `cropAspectW = 9` — crop aspect ratio width component
    - [ ] `cropAspectH = 16` — crop aspect ratio height component
    - [ ] `cropPaddingPercent = 0.15` — padding added around the keypoint bounding box (15% of bounding box dimensions on each side)
  - [ ] Expand the bounding box by `cropPaddingPercent` on each side
  - [ ] Enforce 9:16 aspect ratio: expand the smaller dimension to match the ratio, keeping the bounding box centered
  - [ ] Clamp the crop rectangle to the source video dimensions (never exceed frame bounds)
  - [ ] Get source video dimensions via `ffprobe` (use `ffmpeg.RunProbe()`) — needed for clamping and for crop-params.json

- [ ] Write crop parameters file (AC: 1)
  - [ ] Save `crop-params.json` to lift directory: `{"x": int, "y": int, "w": int, "h": int, "sourceWidth": int, "sourceHeight": int}`
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropParams)`
  - [ ] This file is read by the skeleton rendering stage (Story 3.1) for keypoint coordinate transformation

- [ ] Implement graceful degradation when no keypoints available (AC: 3)
  - [ ] Check if keypoints.json exists; if not, skip crop computation
  - [ ] Set `StageOutput.VideoPath` to the input video path (preserve full frame)
  - [ ] Return `StageOutput{VideoPath: input.VideoPath}` (completed successfully, just preserved the original — SSE checklist shows checkmark, not skipped state)
  - [ ] Do NOT write crop-params.json (skeleton stage checks for its absence and skips coordinate transformation)
  - [ ] Log: `slog.Warn("keypoints not available, preserving full frame", "lift_id", input.LiftID)`
  - [ ] Still extract thumbnail from the uncropped video

- [ ] Execute crop via FFmpeg helper (AC: 1)
  - [ ] Use `ffmpeg.CropVideo(ctx, inputPath, outputPath, x, y, w, h)` from Story 2.2
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropped)`
  - [ ] On FFmpeg error: return error to orchestrator

- [ ] Extract thumbnail (AC: 2, 3)
  - [ ] Use `ffmpeg.ExtractThumbnail(ctx, videoPath, thumbnailPath, timeSec)` from Story 2.2
  - [ ] Extract from the best available video (cropped if crop succeeded, otherwise the input video)
  - [ ] Timestamp: middle of the video (`duration / 2`) — use `ffmpeg.GetDuration()` to compute
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileThumbnail)`
  - [ ] Thumbnail extraction failure should NOT fail the stage — log warning, continue without thumbnail

- [ ] Add storage constants (AC: 1, 2)
  - [ ] Add `FileCropped = "cropped.mp4"` to `internal/storage/storage.go` constants (if not already defined)
  - [ ] Add `FileThumbnail = "thumbnail.jpg"` to storage constants
  - [ ] Add `FileCropParams = "crop-params.json"` to storage constants

- [ ] Register crop stage with pipeline (AC: 1)
  - [ ] In `main.go`, add `&stages.CropStage{}` as the third stage in the pipeline's stage slice (after PoseStage and TrimStage)

- [ ] Write unit tests `internal/pipeline/stages/crop_test.go` (AC: 1, 2, 3)
  - [ ] Test `CropStage.Name()` returns exactly `"Cropping"` (must match `StageCropping` constant from Story 2.1)
  - [ ] Test crop with real keypoints from `testdata/videos/sample-lift.mp4` — produces `cropped.mp4`, `thumbnail.jpg`, and `crop-params.json`
  - [ ] Test crop-params.json contains valid JSON with expected fields (x, y, w, h, sourceWidth, sourceHeight)
  - [ ] Test cropped video has 9:16 aspect ratio (within rounding tolerance)
  - [ ] Test thumbnail extraction — produces `thumbnail.jpg`, verify it is a valid JPEG
  - [ ] Test no-keypoints scenario (keypoints.json absent) — returns input video path unchanged, still extracts thumbnail, no crop-params.json written
  - [ ] Test FFmpeg failure — returns error (not panic)
  - [ ] Test thumbnail extraction failure — stage still succeeds, no thumbnail file
  - [ ] Test with context cancellation — returns error
  - [ ] Tests skip if FFmpeg is not installed (`t.Skip("ffmpeg not available")`)

## Prerequisites

- Story 2.2 (FFmpeg Integration & Verification) must be complete — this story depends on `ffmpeg.CropVideo()`, `ffmpeg.ExtractThumbnail()`, and `ffmpeg.GetDuration()`.
- Story 2.5 (Pose-Based Video Trim) must be complete — this story receives the trimmed (or original) video as input and reads keypoints.json produced by pose estimation.

## Dev Notes

- **Person selection is handled upstream by Story 2.4 (pose estimation).** YOLO26n-Pose selects the first (highest-confidence) person detected. The keypoints.json always contains a single person's data. The crop stage does not need multi-person logic — it computes the bounding box from the single person's keypoints directly.
- The crop stage receives `StageInput.VideoPath` from the trim stage (trimmed.mp4 if trim succeeded, otherwise the original video passed through). It also needs to read `keypoints.json` from the lift directory — this is NOT passed via StageInput but read directly from the filesystem using `storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)`.
- The keypoints.json includes per-frame `boundingBox` data from server-side YOLO26n-Pose detection. The crop stage uses these bounding boxes (not individual keypoints) to determine the crop region — compute the enclosing box across all frames, add padding, enforce 9:16. The skeleton stage (Story 3.1) is responsible for transforming keypoint coordinates to cropped-frame coordinates using crop-params.json.
- For the 9:16 aspect ratio enforcement: compute the bounding box from keypoints, add padding, then adjust to 9:16. If the box is too wide for 9:16, increase height. If too tall, increase width. Center the adjustment around the bounding box center.
- Thumbnail timing: extract at the midpoint of the video duration, which for a trimmed video should capture the lift itself. Use `ffmpeg.GetDuration()` to get the video length.
- crop-params.json is a lightweight metadata file that bridges the crop and skeleton stages. If no crop was applied (full frame preserved), this file is NOT written — the skeleton stage checks for its presence and skips coordinate transformation if absent.
- The `cropPaddingPercent = 0.15` means 15% of the bounding box width/height is added on each side. For a bounding box that's 600px wide, this adds 90px on each side (780px total before aspect ratio enforcement).
- Source video dimensions are needed for two purposes: (1) clamping the crop rectangle to frame bounds, and (2) writing to crop-params.json for downstream coordinate transformation. Use `ffprobe` (via `ffmpeg.RunProbe()`) to get dimensions.

### Architecture Compliance

- Implements `pipeline.Stage` interface: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for all output paths
- Uses `ffmpeg.CropVideo()` and `ffmpeg.ExtractThumbnail()` — never calls `exec.Command` directly
- Returns errors on failure, never panics
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `duration_ms`, `error`
- Graceful degradation: missing keypoints preserves full frame, thumbnail failure doesn't fail stage

### Project Structure Notes

New files to create:
- `internal/pipeline/stages/crop.go` — crop stage implementation
- `internal/pipeline/stages/crop_test.go` — tests

Files to modify:
- `cmd/press-out/main.go` — register CropStage in pipeline stages list (after PoseStage)
- `internal/storage/storage.go` — add `FileCropped`, `FileThumbnail`, `FileCropParams` constants

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition
- [Source: architecture.md#Data Architecture] — cropped.mp4, thumbnail.jpg, crop-params.json in lift directory
- [Source: architecture.md#Process Patterns] — graceful degradation
- [Source: epics.md#Story 2.6] — acceptance criteria
- [Source: epics.md#FR5] — identify and crop to lifter when multiple people in frame
- [Source: epics.md#FR6] — preserve full video on low confidence
- [Source: epics.md#Additional Requirements] — "Thumbnail generation: Extracted from processed video via FFmpeg, stored as thumbnail.jpg"

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
