# Story 2.5: Pose-Based Video Trim

Status: draft

## Story

As a lifter,
I want the system to trim my video to just the lift using detected body positions,
so that I can review only the relevant portion without setup or post-lift footage.

## Acceptance Criteria (BDD)

1. **Given** keypoints.json exists from the pose estimation stage, **When** the trim stage runs, **Then** the system analyzes frame-to-frame keypoint displacement to detect the lift's start and end, **And** the trimmed video is saved as trimmed.mp4 in the lift-ID directory via FFmpeg, **And** padding is added around the detected lift boundaries

2. **Given** the keypoint-based detection confidence falls below the threshold, **When** the trim stage cannot confidently identify the lift boundaries, **Then** the full original video is preserved as the trim output (FR6), **And** the stage completes without error (graceful degradation), **And** downstream stages receive the full video as input

3. **Given** keypoints.json does not exist (pose estimation was skipped), **When** the trim stage is reached, **Then** the full original video is preserved as the trim output (FR6), **And** the stage completes without error, **And** downstream stages receive the full video as input

4. **Given** the trim stage encounters an FFmpeg error, **When** the subprocess fails, **Then** the error is logged with slog, **And** the stage returns an error to the orchestrator, **And** the orchestrator skips the stage and passes the original video forward (FR7)

## Prerequisites

- Story 2.4 (Server-Side Pose Estimation) must be complete — this story reads keypoints.json produced by the pose stage. Required artifacts from 2.4: `StagePoseEstimation` constant in `stage.go`, `internal/pipeline/stages/pose.go`, `testdata/keypoints-sample.json` test fixture, and `scripts/pose.py`.
- Story 2.2 (FFmpeg Integration & Verification) must be complete — depends on `ffmpeg.TrimVideo()` and `ffmpeg.GetDuration()`.

## Tasks / Subtasks

### Task 1: Reorder pipeline stages

- [ ] In `internal/pipeline/stage.go`:
  - [ ] Update comment to reflect new 6-stage order: Pose estimation -> Trimming -> Cropping -> Rendering skeleton -> Computing metrics -> Generating coaching
  - [ ] Ensure `StagePoseEstimation = "Pose estimation"` constant exists (added by 2.4)
- [ ] In `DefaultStages()` in `internal/pipeline/stage.go`:
  - [ ] Reorder: PoseEstimation first, then Trimming, then remaining stages
- [ ] In `cmd/press-out/main.go`:
  - [ ] Reorder pipeline stages: PoseStage first, then TrimStage, then remaining stages
- [ ] In `web/templates/partials/pipeline-stages.html`: No change needed — stage list is SSE-driven, not hardcoded

### Task 2: Remove scene-change-based trim logic

- [ ] In `internal/pipeline/stages/trim.go`:
  - [ ] Remove `detectSceneChanges()` function entirely
  - [ ] Remove `findMotionCluster()` function entirely
  - [ ] Remove `showInfoTimestamp` regex
  - [ ] Remove scene change constants: `sceneChangeThreshold`, `minClusterDuration`, `maxClusterDuration`, `minHighMotionFrames`
  - [ ] Remove imports only used by scene change detection (`bufio`, `regexp`, `sort`, `strconv`, `strings`)
  - [ ] Add imports needed by keypoint-based detection: `encoding/json`, `math`, `os`

### Task 3: Implement pose-based trim detection

- [ ] In `internal/pipeline/stages/trim.go`:
  - [ ] Add function `detectLiftFromKeypoints(keypointsPath string) (startSec, endSec float64, confident bool, err error)`:
    - [ ] Read and parse keypoints.json into `pose.Result`
    - [ ] Compute frame-to-frame keypoint displacement: for each consecutive frame pair, sum the Euclidean distance of matching keypoints (using only keypoints with confidence above `minKeypointConfidence`)
    - [ ] Normalize displacement by the number of keypoints used
    - [ ] Identify frames where normalized displacement exceeds `displacementThreshold` as "high motion" frames
    - [ ] Merge high-motion frames into runs, bridging gaps of up to `maxGapFrames` (3) consecutive low-motion frames — this tolerates brief pauses (e.g., the transition between first and second pull) without splitting the lift
    - [ ] If multiple runs exist, select the one with the highest total displacement (largest overall motion)
    - [ ] Validate the selected run's duration against `minLiftDuration` and `maxLiftDuration` — if outside range, set `confident = false` and log the reason (e.g., "cluster too short: 0.4s < 0.8s min" or "cluster too long: 18s > 15s max")
    - [ ] If fewer than `minHighMotionFrames` high-motion frames exist in the selected run, set `confident = false` and log the reason (e.g., "insufficient high-motion frames: 3 < 5 min")
    - [ ] Return start/end times from `timeOffsetMs` fields (converted to seconds) and confidence
  - [ ] Update `TrimStage.Run()`:
    - [ ] Read keypoints.json path via `storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)`
    - [ ] If keypoints.json does not exist: log warning, return input video path (graceful degradation, no error)
    - [ ] Call `detectLiftFromKeypoints()` to find lift boundaries
    - [ ] If not confident: log warning, return input video path
    - [ ] Apply padding and clamp to video bounds (same as before)
    - [ ] Trim via `ffmpeg.TrimVideo()` (same as before)
  - [ ] Define named constants:
    - [ ] `minKeypointConfidence = 0.5` — minimum keypoint confidence to include in displacement calculation
    - [ ] `displacementThreshold = 0.02` — normalized displacement above which a frame is considered "high motion"
    - [ ] `minLiftDuration = 0.8` — minimum seconds for a valid lift cluster
    - [ ] `maxLiftDuration = 15.0` — maximum seconds for a valid lift cluster
    - [ ] `minHighMotionFrames = 5` — minimum high-motion frames needed for confidence
    - [ ] `maxGapFrames = 3` — maximum consecutive low-motion frames allowed within a run before splitting
    - [ ] `paddingSec = 1.5` — seconds of padding before and after detected boundaries

### Task 4: Update tests

- [ ] Rewrite `internal/pipeline/stages/trim_test.go`:
  - [ ] Remove `TestFindMotionCluster` and related scene-change tests
  - [ ] Remove `TestFindMotionCluster_Padding` and `TestFindMotionCluster_ClampToZero`
  - [ ] Test `TrimStage.Name()` still returns `"Trimming"`
  - [ ] Test trim with keypoints.json from `testdata/keypoints-sample.json` + sample video — verify trimmed.mp4 is produced with duration between 6-10s (lift ~5s + padding ~3s, allowing tolerance)
  - [ ] Test no-keypoints scenario (keypoints.json absent) — returns original video path, no error
  - [ ] Test low-confidence scenario (keypoints with minimal motion) — returns original video path, no error
  - [ ] Test FFmpeg failure — returns error (not panic)
  - [ ] Test with context cancellation — returns error
  - [ ] Test `detectLiftFromKeypoints()` with real keypoints fixture — assert detected start is between 4-7s and end is between 10-13s (lift at ~6-11s in sample video, allowing tolerance for algorithm variance)
  - [ ] Tests skip if FFmpeg is not installed (`t.Skip("ffmpeg not available")`)
- [ ] Update `internal/pipeline/pipeline_test.go`:
  - [ ] Assert stage order: Pose estimation, Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching

## Dev Notes

- **What this replaces:** Story 2.3 implemented trim using FFmpeg scene change detection (`select='gt(scene,0.3)',showinfo`). That was a pixel-level motion heuristic. Now that pose estimation (Story 2.4) runs first and produces per-frame keypoint data, we use keypoint displacement instead — a much more reliable signal for detecting the lift portion.

- **Pipeline order change:** The pipeline was Trimming -> Pose -> Crop -> .... Now it's Pose -> Trimming -> Crop -> .... Pose runs on the full original video. Trim uses keypoints.json to find the lift, then cuts with FFmpeg. Pose processes some frames that will be trimmed away, but the extra time is negligible (~39fps) and the accuracy gain is significant.

- **Displacement algorithm:** For each consecutive frame pair, sum the Euclidean distance (`math.Sqrt(dx*dx + dy*dy)`) of each keypoint from its position in the previous frame, using only keypoints above the confidence threshold in both frames. Normalize by the number of keypoints used. An Olympic lift creates a clear spike in this signal — setup/post-lift are near-zero displacement, the pull/catch creates large displacement.

- **Cluster detection:** Mark frames as "high motion" when displacement exceeds threshold. Merge consecutive high-motion frames into runs, bridging gaps of up to `maxGapFrames` low-motion frames (tolerates brief pauses like the first-to-second pull transition). If multiple runs exist, pick the one with the highest total displacement. Validate the winner against `minLiftDuration`/`maxLiftDuration`/`minHighMotionFrames`.

- **Low-confidence logging:** When confidence is low, log the specific reason via `logger.Warn()` so operators can tune thresholds. Examples: "no keypoints file", "insufficient high-motion frames: 3 < 5 min", "cluster too short: 0.4s < 0.8s min", "cluster too long: 18s > 15s max", "no high-motion frames detected".

- **Coordinate space:** Keypoints in keypoints.json are normalized (0-1). Displacement is computed in this normalized space, so the threshold is resolution-independent.

- **Optional smoothing:** Raw frame-to-frame displacement can be noisy (camera shake, compression artifacts). A 3-5 frame moving average on the displacement signal before thresholding would produce cleaner detection. Not required for initial implementation but worth considering if detection quality is poor on real videos.

- **Graceful degradation:** Two fallback paths: (1) keypoints.json missing entirely — preserve full video, no error. (2) keypoints exist but no confident motion cluster — preserve full video, no error. Both result in a sage checkmark in the pipeline checklist (completed successfully, just preserved the original).

- **Reuse from current trim:** The FFmpeg trim execution (`ffmpeg.TrimVideo()`), padding logic, and duration clamping remain identical. Only the detection method changes (keypoints instead of scene changes).

- **Test fixture:** `testdata/keypoints-sample.json` (generated by Story 2.4 from the real sample video) provides the keypoint data for trim tests. The sample video has the lift at ~6-11s, so the trim should detect boundaries in that range.

### Architecture Compliance

- Implements `pipeline.Stage` interface: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for all path construction
- Uses `ffmpeg.TrimVideo()` and `ffmpeg.GetDuration()` — never calls `exec.Command` directly
- Returns errors on failure, never panics
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `duration_ms`, `error`
- Reads `pose.Result` type for keypoints deserialization

### Project Structure Notes

Files to modify:
- `internal/pipeline/stages/trim.go` — rewrite detection logic
- `internal/pipeline/stages/trim_test.go` — rewrite tests
- `internal/pipeline/stage.go` — reorder stages, update comments
- `internal/pipeline/pipeline_test.go` — update stage order assertions
- `cmd/press-out/main.go` — reorder pipeline stages

No new files needed.

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition
- [Source: architecture.md#Process Patterns] — graceful degradation, error handling
- [Source: epics.md#Story 2.5] — acceptance criteria
- [Source: epics.md#FR4] — auto-detect and trim to lift portion
- [Source: epics.md#FR6] — preserve full video on low confidence
- [Source: epics.md#FR7] — independent stage processing, any stage skippable

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
