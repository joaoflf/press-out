# Story 2.4: Auto-Crop to Lifter

Status: ready-for-dev

## Story

As a lifter,
I want the system to automatically crop the video to focus on me,
so that I see only my lift without distracting bystanders or background.

## Acceptance Criteria (BDD)

1. **Given** the pipeline reaches the crop stage with a trimmed (or original) video, **When** the crop stage runs, **Then** the system uses person-barbell interaction detection to identify the lifter, **And** the video is cropped to isolate the lifter via FFmpeg, **And** the cropped video is saved as cropped.mp4 in the lift-ID directory

2. **Given** the crop stage successfully produces a cropped video, **When** the crop completes, **Then** a thumbnail is extracted from the cropped video via FFmpeg, **And** the thumbnail is saved as thumbnail.jpg in the lift-ID directory

3. **Given** the person-barbell detection confidence falls below the threshold, **When** the crop stage cannot confidently identify the lifter, **Then** the full frame video is preserved as the crop output (FR6), **And** a thumbnail is still extracted from the uncropped video, **And** the stage completes without error (graceful degradation)

4. **Given** multiple people are in the frame, **When** the crop stage runs, **Then** the system identifies the person interacting with the barbell as the lifter (FR5), **And** other people in the frame are excluded by the crop

## Tasks / Subtasks

- [ ] Create crop stage at `internal/pipeline/stages/crop.go` (AC: 1, 2, 3, 4)
  - [ ] `CropStage` struct implementing `pipeline.Stage` interface
  - [ ] `Name()` returns `"Cropping"` (matches stage name constant)
  - [ ] `Run(ctx context.Context, input StageInput) (StageOutput, error)` implementation

- [ ] Implement person-barbell interaction detection (AC: 1, 3, 4)
  - [ ] Strategy: analyze video frames to detect the region of primary motion activity (the person performing the lift with the barbell)
  - [ ] Approach A — Motion-based ROI: use FFmpeg to calculate motion vectors or frame diffs, identify the region with the most significant vertical motion (the lift), and center the crop around that region
  - [ ] Approach B — FFmpeg cropdetect: use `ffmpeg -i input.mp4 -vf cropdetect -f null -` to detect the active region of the frame, then apply the detected crop
  - [ ] The barbell creates a distinct horizontal element; the lifter creates vertical motion beneath it. The intersection of these patterns identifies the lifter
  - [ ] Maintain aspect ratio suitable for mobile viewing (portrait-ish or 3:4)
  - [ ] Add margin around the detected region (10-15% padding on each side) to avoid cutting off the movement

- [ ] Implement confidence check and graceful degradation (AC: 3)
  - [ ] If motion analysis cannot isolate a clear region (e.g., motion is evenly distributed, or no significant motion found), fall back to full frame
  - [ ] On low confidence: set `StageOutput.VideoPath` to input video path, still extract thumbnail
  - [ ] Log degradation: `slog.Warn("crop confidence low, preserving full frame", "lift_id", input.LiftID)`

- [ ] Execute crop via FFmpeg helper (AC: 1)
  - [ ] Use `ffmpeg.CropVideo(ctx, inputPath, outputPath, x, y, w, h)` from Story 2.2
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropped)`
  - [ ] On FFmpeg error: return error to orchestrator

- [ ] Extract thumbnail (AC: 2, 3)
  - [ ] Use `ffmpeg.ExtractThumbnail(ctx, videoPath, thumbnailPath, timeSec)` from Story 2.2
  - [ ] Extract from the best available video (cropped if crop succeeded, otherwise the input video)
  - [ ] Timestamp: 0.5 seconds into the video (or middle of video if shorter)
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileThumbnail)`
  - [ ] Thumbnail extraction failure should NOT fail the stage — log warning, continue without thumbnail

- [ ] Register crop stage with pipeline (AC: 1)
  - [ ] In `main.go`, add `&stages.CropStage{}` as the second stage in the pipeline's stage slice (after TrimStage)

- [ ] Write unit tests `internal/pipeline/stages/crop_test.go` (AC: 1, 2, 3)
  - [ ] Test crop on `testdata/videos/sample.mp4` — produces `cropped.mp4`
  - [ ] Test thumbnail extraction — produces `thumbnail.jpg`
  - [ ] Test low-confidence scenario — returns input video path, still extracts thumbnail
  - [ ] Test FFmpeg failure — returns error (not panic)
  - [ ] Test thumbnail extraction failure — stage still succeeds, no thumbnail file
  - [ ] Tests skip if FFmpeg is not installed

## Dev Notes

- The crop stage receives the video path from the trim stage's output. If trim succeeded, it gets `trimmed.mp4`. If trim was skipped, it gets `original.mp4`. The crop stage does not need to know which — it just processes whatever `StageInput.VideoPath` points to.
- Person-barbell interaction detection is the most technically ambitious part of this story. A pragmatic first approach: analyze motion intensity across the frame using FFmpeg, find the region with the most vertical motion (characteristic of a lift), and crop to that region. This doesn't require ML or pose estimation (that's in Epic 3).
- Crop dimensions should maintain a reasonable aspect ratio for mobile viewing. A square or slightly portrait crop (3:4 or 9:16) works best on mobile screens.
- Thumbnail timing matters: for a trimmed video, the middle of the video likely shows the lift itself. For an untrimmed video, 0.5s in might still be setup. Consider using 30-40% into the video duration as the thumbnail extraction point.
- Thumbnail extraction is a secondary concern within this stage. It should never block or fail the stage. If FFmpeg can't extract the thumbnail (e.g., corrupt video), log the warning and return success — the lift list will simply not show a thumbnail for this lift.
- The cropped video is what downstream stages (pose estimation, skeleton rendering) will work with. A well-cropped video improves pose estimation accuracy significantly.

### Architecture Compliance

- Implements `pipeline.Stage` interface: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for all output paths
- Uses `ffmpeg.CropVideo()` and `ffmpeg.ExtractThumbnail()` — never calls `exec.Command` directly
- Returns errors on failure, never panics
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `duration_ms`, `error`
- Graceful degradation: low confidence preserves full frame, thumbnail failure doesn't fail stage

### Project Structure Notes

New files to create:
- `internal/pipeline/stages/crop.go` — crop stage implementation
- `internal/pipeline/stages/crop_test.go` — tests

Files to modify:
- `cmd/press-out/main.go` — register CropStage in pipeline stages list (after TrimStage)

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition
- [Source: architecture.md#Data Architecture] — `thumbnail.jpg` in lift directory structure
- [Source: architecture.md#Process Patterns] — graceful degradation
- [Source: epics.md#Story 2.4] — acceptance criteria
- [Source: epics.md#FR5] — identify and crop to lifter when multiple people in frame
- [Source: epics.md#FR6] — preserve full video on low confidence
- [Source: epics.md#Additional Requirements] — "Thumbnail generation: Extracted from processed video via FFmpeg, stored as thumbnail.jpg"
- [Source: prd.md#Journey 2] — person-barbell interaction heuristic, full-frame fallback

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
