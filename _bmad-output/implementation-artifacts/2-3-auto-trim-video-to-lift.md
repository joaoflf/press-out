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
  - [ ] Use FFmpeg scene change detection as the primary approach: `ffmpeg -i input.mp4 -vf "select='gt(scene,0.3)',showinfo" -f null -` — parse timestamps from the output to find high-motion frames
  - [ ] An Olympic lift creates a sharp motion spike (setup is low motion, lift is high motion, post-lift is low motion)
  - [ ] Detect the primary motion cluster — the contiguous period of high motion
  - [ ] Define these as named constants for easy tuning:
    - [ ] `sceneChangeThreshold = 0.3` — FFmpeg scene change score above which a frame counts as high-motion
    - [ ] `minClusterDuration = 0.8` — minimum seconds for a valid lift cluster (below this it's noise)
    - [ ] `maxClusterDuration = 15.0` — maximum seconds for a valid lift cluster (above this it's not a single lift)
    - [ ] `minHighMotionFrames = 5` — minimum high-motion frames needed to be confident a lift was detected
    - [ ] `paddingSec = 1.5` — seconds of padding before and after the detected lift boundaries
  - [ ] Clamp the padded start time to 0 and the padded end time to the video's total duration to avoid FFmpeg receiving out-of-bounds timestamps
  - [ ] If the motion cluster duration is outside the plausible range, or fewer than `minHighMotionFrames` are detected, fall back to full video

- [ ] Implement confidence check and graceful degradation (AC: 2)
  - [ ] Confidence is low when: fewer than 5 high-motion frames detected, no contiguous cluster found, or cluster duration is outside the 0.8s–15s range
  - [ ] On low confidence: set `StageOutput.VideoPath` to the input video path (original unchanged) and `StageOutput.Skipped = false` (completed successfully, just preserved the original — SSE checklist shows a sage checkmark, not a skipped state)
  - [ ] Log the degradation: `slog.Warn("trim confidence low, preserving full video", "lift_id", input.LiftID)`

- [ ] Execute trim via FFmpeg helper (AC: 1, 3)
  - [ ] Use `ffmpeg.TrimVideo(ctx, inputPath, outputPath, startSec, durationSec)` from Story 2.2
  - [ ] Output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimmed)`
  - [ ] On FFmpeg error: return the error to the orchestrator (orchestrator handles skip logic)

- [ ] Register trim stage with pipeline (AC: 1)
  - [ ] In `main.go` (or pipeline construction), add `&stages.TrimStage{}` as the first stage in the pipeline's stage slice

- [ ] Write unit tests `internal/pipeline/stages/trim_test.go` (AC: 1, 2, 3)
  - [ ] Test `TrimStage.Name()` returns exactly `"Trimming"` (must match `StageTrimming` constant from Story 2.1)
  - [ ] Test trim on `testdata/videos/sample-lift.mp4` (real snatch video, lift at ~6-11s) — verify `trimmed.mp4` is produced and its duration is approximately 8s (5s lift + 1.5s padding on each side, clamped to video bounds)
  - [ ] Test low-confidence scenario (use a static synthetic video or mock) — returns original video path, `Skipped = false`, no error
  - [ ] Test FFmpeg failure — returns error (not panic)
  - [ ] Test with context cancellation — returns error
  - [ ] Tests skip if FFmpeg is not installed (`t.Skip("ffmpeg not available")`)

## Prerequisites

- Story 2.2 (FFmpeg Integration & Verification) must be complete — this story depends on `ffmpeg.TrimVideo()` and `ffmpeg.GetDuration()` from the FFmpeg helper package.

## Dev Notes

- The trim stage's primary challenge is detecting the lift within the video. Olympic lifts are characterized by a sudden burst of motion (the pull) preceded by relatively static setup (chalking, gripping, breathing) and followed by a recovery/celebration.
- **Primary approach:** Use FFmpeg scene detection: `ffmpeg -i input.mp4 -vf "select='gt(scene,0.3)',showinfo" -f null -` — parse frame timestamps from stderr output to find the motion cluster. If scene detection proves unreliable, a fallback approach is frame-to-frame pixel difference magnitude (calculate average motion per second, find the peak region).
- All detection thresholds are defined as named constants in the trim stage for easy tuning. Getting these wrong is low-risk — the fallback is keeping the full video.
- The `durationSec` parameter in `ffmpeg.TrimVideo()` is the segment length (not an end timestamp), matching FFmpeg's `-t` flag. The trim stage must compute `durationSec = endSec - startSec`.
- The output path is always `trimmed.mp4` in the lift directory. If the stage falls back to the original video, it should NOT copy the original to `trimmed.mp4` — instead, set `StageOutput.VideoPath` to the original video's path so downstream stages use it directly. This avoids unnecessary disk I/O.
- A real snatch video is available at `testdata/videos/sample-lift.mp4` (~27MB, single lifter, no bystanders). Lift occurs at approximately 6-11s. Expected trimmed output is ~8s (5s lift + 1.5s padding). Use this for happy-path trim tests. For low-confidence fallback tests, generate a static synthetic video via FFmpeg or use a mock.
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
