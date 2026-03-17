# Story 2.3: Auto-Trim Video to Lift

Status: ready-for-dev

## Story

As a lifter,
I want the system to automatically trim my video to just the lift portion,
so that I can review the lift immediately without scrubbing through setup footage.

## Acceptance Criteria (BDD)

1. **Given** a video has been uploaded and the pipeline reaches the trim stage, **When** the trim stage runs, **Then** the system analyzes the video for motion patterns to detect lift start and end, **And** the trimmed video is saved as trimmed.mp4 in the lift-ID directory via FFmpeg, **And** padding is added around the detected lift boundaries

2. **Given** the motion detection confidence falls below the threshold, **When** the trim stage cannot confidently identify the lift boundaries, **Then** the full original video is preserved as the trim output (FR6), **And** the stage completes without error (graceful degradation), **And** downstream stages receive the full video as input

3. **Given** the trim stage encounters an FFmpeg error, **When** the subprocess fails, **Then** the error is logged with slog, **And** the stage returns an error to the orchestrator, **And** the orchestrator skips the stage and passes the original video forward (FR7)

## Tasks / Subtasks

- [ ] Create trim stage at `internal/pipeline/stages/trim.go` (AC: 1, 2, 3)
  - [ ] `TrimStage` struct implementing `pipeline.Stage` interface
  - [ ] `Name()` returns `"Trimming"` (matches the stage name constant from Story 2.1)
  - [ ] `Run(ctx context.Context, input StageInput) (StageOutput, error)` implementation

- [ ] Implement motion-based lift detection (AC: 1, 2)
  - [ ] Use FFmpeg scene change detection to find high-motion frames: `ffprobe -f lavfi -i "movie={input},select='gt(scene,THRESHOLD)'" -show_entries frame=pts_time -of csv=p=0`
  - [ ] Alternative approach: analyze frame differences using FFmpeg's `select` filter with motion threshold
  - [ ] An Olympic lift creates a sharp motion spike (setup is low motion, lift is high motion, post-lift is low motion)
  - [ ] Detect the primary motion cluster — the contiguous period of high motion
  - [ ] Add padding: 1-2 seconds before detected start and 1-2 seconds after detected end
  - [ ] If the motion cluster duration is implausible (< 0.5s or > 15s for a single lift), fall back to full video

- [ ] Implement confidence check and graceful degradation (AC: 2)
  - [ ] Define a confidence threshold: if fewer than N motion spikes detected, or if the motion pattern doesn't match expected lift signature, confidence is low
  - [ ] On low confidence: set `StageOutput.VideoPath` to the input video path (original unchanged) and `StageOutput.Skipped = false` (completed, just preserved the original)
  - [ ] Log the degradation: `slog.Warn("trim confidence low, preserving full video", "lift_id", input.LiftID)`

- [ ] Execute trim via FFmpeg helper (AC: 1, 3)
  - [ ] Use `ffmpeg.TrimVideo(ctx, inputPath, outputPath, startSec, durationSec)` from Story 2.2
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimmed)`
  - [ ] On FFmpeg error: return the error to the orchestrator (orchestrator handles skip logic)

- [ ] Register trim stage with pipeline (AC: 1)
  - [ ] In `main.go` (or pipeline construction), add `&stages.TrimStage{}` as the first stage in the pipeline's stage slice

- [ ] Write unit tests `internal/pipeline/stages/trim_test.go` (AC: 1, 2, 3)
  - [ ] Test trim on `testdata/videos/sample.mp4` — produces `trimmed.mp4` output
  - [ ] Test low-confidence scenario — returns original video path, no error
  - [ ] Test FFmpeg failure — returns error (not panic)
  - [ ] Test with context cancellation — returns error
  - [ ] Tests skip if FFmpeg is not installed (`t.Skip("ffmpeg not available")`)

## Dev Notes

- The trim stage's primary challenge is detecting the lift within the video. Olympic lifts are characterized by a sudden burst of motion (the pull) preceded by relatively static setup (chalking, gripping, breathing) and followed by a recovery/celebration. Scene detection via FFmpeg is a reasonable first approach.
- FFmpeg scene detection approach: `ffmpeg -i input.mp4 -vf "select='gt(scene,0.3)',showinfo" -f null -` prints frame info for frames with scene change above the threshold. Parse the timestamps to find the motion cluster.
- Alternative approach if scene detection is unreliable: use frame-to-frame pixel difference magnitude. Calculate average motion per second, find the peak region.
- Padding values (1-2 seconds) should be conservative — better to include too much than cut off the start of the pull. The lifter can always scrub past extra footage, but a clipped pull is unusable.
- The output path is always `trimmed.mp4` in the lift directory. If the stage falls back to the original video, it should NOT copy the original to `trimmed.mp4` — instead, set `StageOutput.VideoPath` to the original video's path so downstream stages use it directly. This avoids unnecessary disk I/O.
- Context timeout: the trim stage should use the context passed by the orchestrator. If the whole pipeline has a deadline, FFmpeg will be killed when the context expires.

### Architecture Compliance

- Implements `pipeline.Stage` interface exactly: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for output path construction — never inline paths
- Uses `ffmpeg.Run()` or `ffmpeg.TrimVideo()` from the FFmpeg helper — never calls `exec.Command` directly
- Returns errors on failure, never panics — orchestrator handles skip logic
- Logs with `slog`: `slog.Info("stage starting/complete", "lift_id", ..., "stage", "Trimming", "duration_ms", ...)`

### Project Structure Notes

New files to create:
- `internal/pipeline/stages/trim.go` — trim stage implementation
- `internal/pipeline/stages/trim_test.go` — tests

Files to modify:
- `cmd/press-out/main.go` — register TrimStage in pipeline stages list

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition
- [Source: architecture.md#Process Patterns] — graceful degradation, error handling
- [Source: architecture.md#Logging Convention] — slog attribute keys
- [Source: epics.md#Story 2.3] — acceptance criteria
- [Source: epics.md#FR4] — auto-detect and trim to lift portion
- [Source: epics.md#FR6] — preserve full video on low confidence
- [Source: epics.md#FR7] — independent stage processing, any stage skippable

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
