# Story 2.4: Client-Side Pose Estimation & Upload

Status: draft

## Story

As a lifter,
I want the system to detect my body positions from the video before uploading,
so that my joint movements can be used for cropping, visualization, and analysis.

## Acceptance Criteria (BDD)

1. **Given** the lifter selects a video in the upload modal, **When** the video is selected, **Then** the browser loads ml5.js bodyPose (MoveNet SINGLEPOSE_THUNDER) and processes the video frame-by-frame at 30fps on a canvas, **And** a progress indicator shows pose estimation progress (e.g., "Processing frame 120 / 360"), **And** 17 COCO-format keypoints are detected per frame (FR8), **And** keypoint coordinates are normalized (0-1 relative to video dimensions), **And** keypoint smoothing (7-frame averaging window) is applied to reduce jitter

2. **Given** client-side pose estimation completes, **When** the lifter selects a lift type and taps submit, **Then** the upload sends both the video file and keypoints.json as multipart form fields, **And** the server stores keypoints.json in the lift-ID directory alongside original.mp4, **And** the server pipeline starts with keypoints.json already available for downstream stages (crop, skeleton, metrics)

3. **Given** client-side pose estimation fails or detects no poses, **When** the upload proceeds, **Then** the video is still uploaded without keypoints.json, **And** downstream stages that depend on keypoints handle the missing file gracefully (FR6), **And** no error screen is shown to the lifter

4. **Given** the upload completes and the pipeline starts, **When** the trim stage runs, **Then** the video is trimmed as in Story 2.3, **And** the pipeline continues with cropping, skeleton, metrics, and coaching stages

## Prerequisites

- Story 1.2 (Upload a Lift Video) must be complete — this story modifies the upload handler and modal.
- Story 2.2 (FFmpeg Integration & Verification) must be complete — the pipeline uses FFmpeg for trimming.
- Story 2.3 (Auto-Trim) must be complete — the pipeline runs the trim stage after upload.

## Tasks / Subtasks

### Task 1: Add ml5.js pose estimation to upload flow

- [ ] Load ml5.js via CDN (`https://unpkg.com/ml5@1/dist/ml5.js`) in `web/templates/layouts/base.html` (or conditionally in upload modal)
- [ ] Create pose estimation JavaScript in `web/static/app.js` (or a dedicated `pose.js`):
  - [ ] On video file selection: load video into a hidden `<video>` element, create an offscreen `<canvas>`
  - [ ] Initialize `ml5.bodyPose("MoveNet", { modelType: "SINGLEPOSE_THUNDER" })`
  - [ ] Process frames sequentially: seek to each frame (1/30s intervals), draw to canvas, call `bodyPose.detect(canvas)`
  - [ ] Normalize keypoint coordinates to 0-1 range (divide by video dimensions)
  - [ ] Apply 7-frame smoothing window (average x, y, confidence across neighboring frames for keypoints with confidence > 0.15)
  - [ ] Compute per-frame bounding box from ml5's `box` property (normalize to 0-1)
  - [ ] Build `keypoints.json` matching `pose.Result` format: `{ sourceWidth, sourceHeight, frames[]{timeOffsetMs, boundingBox{left,top,right,bottom}, keypoints[]{name, x, y, confidence}} }`
  - [ ] Store the JSON blob in memory until form submission
- [ ] Reference implementation: `web/static/pose-spike.html` (working spike with all the above logic)

### Task 2: Add pose estimation progress UI to upload modal

- [ ] After video file selection, show a progress indicator in the upload modal
- [ ] Progress format: "Processing frame N / total" with a progress bar
- [ ] Lift type selector and submit button remain disabled until pose estimation completes (or fails)
- [ ] On completion: enable lift type selector and submit button
- [ ] On failure: enable lift type selector and submit button anyway (video uploads without keypoints)
- [ ] No "pose estimation failed" error message shown to user — silent degradation

### Task 3: Modify upload handler to accept keypoints.json

- [ ] In `internal/handler/lift.go` (POST /lifts handler):
  - [ ] Parse multipart form with existing `video` field + new optional `keypoints` field
  - [ ] If `keypoints` field is present: read the data, validate it is valid JSON, save to `storage.LiftFile(dataDir, liftID, storage.FileKeypoints)`
  - [ ] If `keypoints` field is absent: proceed without it (no error)
  - [ ] Save keypoints.json BEFORE starting the pipeline (like original.mp4 per NFR10)
- [ ] Server-side validation of keypoints.json:
  - [ ] Must be valid JSON
  - [ ] Must have `sourceWidth` and `sourceHeight` as positive integers
  - [ ] Must have `frames` array (may be empty)
  - [ ] If validation fails: log warning, discard keypoints, proceed with upload (no error to user)

### Task 4: Simplify pose package to shared types only

- [ ] Rename `internal/pose/client.go` to `internal/pose/pose.go` (or create fresh)
- [ ] Keep only the types needed for keypoints.json deserialization:
  - [ ] `Result` struct: `SourceWidth int`, `SourceHeight int`, `Frames []Frame`
  - [ ] `Frame` struct: `TimeOffsetMs int64`, `BoundingBox BoundingBox`, `Keypoints []Keypoint`
  - [ ] `BoundingBox` struct: `Left float64`, `Top float64`, `Right float64`, `Bottom float64`
  - [ ] `Keypoint` struct: `Name string`, `X float64`, `Y float64`, `Confidence float64`
  - [ ] JSON struct tags on all fields
  - [ ] Named constants for 17 COCO landmark names
- [ ] Remove `Client` interface — no server-side pose provider
- [ ] Remove `internal/pose/videointel.go` and `internal/pose/videointel_test.go`
- [ ] Remove `internal/pose/videointel_integration_test.go` (if exists)

### Task 5: Remove server-side pose pipeline stage

- [ ] Delete `internal/pipeline/stages/pose.go` and `internal/pipeline/stages/pose_test.go`
- [ ] In `internal/pipeline/stage.go`: remove `StagePoseEstimation` from `DefaultStages()` — pipeline is now: Trimming → Cropping → Rendering skeleton → Computing metrics → Generating coaching (5 stages)
- [ ] In `cmd/press-out/main.go`: remove pose client creation, remove `NewPoseStage(client)`, remove `defer poseClient.Close()`
- [ ] In `internal/config/config.go`: remove `MediaPipeAPIKey` field if still present

### Task 6: Remove Google Cloud Video Intelligence dependency

- [ ] Run `go mod tidy` to remove `cloud.google.com/go/videointelligence` from `go.mod` / `go.sum`
- [ ] Verify no remaining imports reference the videointelligence package
- [ ] Remove `.env.example` reference to `GOOGLE_APPLICATION_CREDENTIALS` (if present)

### Task 7: Update upload form to send keypoints as multipart field

- [ ] In `web/templates/partials/upload-modal.html`: form must use `multipart/form-data` (already does for video)
- [ ] JavaScript on form submit: create a `Blob` from the keypoints JSON, append as a form field named `keypoints` with filename `keypoints.json`
- [ ] If pose estimation failed/skipped: do not include the `keypoints` field

### Task 8: Verification tests

- [ ] **Keypoints test fixture:** Generate `testdata/keypoints-sample.json` from the spike using the real test video (`testdata/videos/sample-lift.mp4`). This fixture is used by all server-side tests.

- [ ] **Go test — upload handler accepts keypoints.json:**
  - [ ] POST multipart with `video` (sample video) + `keypoints` (fixture JSON) + `lift_type`
  - [ ] Assert HTTP 200 / redirect
  - [ ] Assert `original.mp4` saved to lift directory
  - [ ] Assert `keypoints.json` saved to lift directory
  - [ ] Assert keypoints.json content matches fixture (valid JSON, has sourceWidth/sourceHeight/frames)

- [ ] **Go test — upload handler without keypoints:**
  - [ ] POST multipart with `video` + `lift_type` only (no keypoints field)
  - [ ] Assert upload succeeds
  - [ ] Assert `keypoints.json` does NOT exist in lift directory

- [ ] **Go test — pipeline runs with pre-existing keypoints.json:**
  - [ ] Create a lift directory with `original.mp4` + `keypoints.json` (fixture)
  - [ ] Run pipeline — trim stage executes, keypoints.json remains available for crop stage
  - [ ] Assert no pose stage runs (stage not in pipeline)

- [ ] **Go test — invalid keypoints.json rejected gracefully:**
  - [ ] POST multipart with `keypoints` field containing invalid JSON
  - [ ] Assert upload succeeds (video saved)
  - [ ] Assert `keypoints.json` NOT saved (invalid data discarded)

- [ ] **ChromeDP test — upload page loads ml5.js:**
  - [ ] Navigate to lift list page
  - [ ] Open upload modal
  - [ ] Assert ml5.js CDN script loaded (no 404 / network error)
  - [ ] Assert no JavaScript console errors
  - [ ] Assert pose progress UI elements exist in DOM (hidden initially)

- [ ] **Spike as reference:** `web/static/pose-spike.html` validates that ml5.js MoveNet detects all 17 COCO keypoints with good confidence on the sample weightlifting video. The story integrates this validated logic — it does not re-invent pose detection.

## Dev Notes

- **Spike reference:** `web/static/pose-spike.html` is a working prototype. Copy the core logic (model loading, frame-by-frame extraction, smoothing, JSON export) — do not start from scratch.

- **ml5.js MoveNet:** MoveNet SINGLEPOSE_THUNDER is the model. It's more accurate than SINGLEPOSE_LIGHTNING but still fast (~7s for 12s video at 30fps on desktop). Loaded via `ml5.bodyPose("MoveNet", { modelType: "SINGLEPOSE_THUNDER" })`.

- **Canvas requirement:** The video element must be drawn to a canvas for ml5 detection. A hidden `<video>` element doesn't expose pixel data. The spike uses an offscreen canvas for processing.

- **Smoothing:** 7-frame window (3 frames each side). Averages x, y, confidence for keypoints with confidence > 0.15. Reduces jitter significantly in the skeleton overlay.

- **keypoints.json format:** Identical to what the Video Intelligence API produced — downstream stages (crop, skeleton, metrics) consume it the same way:
  ```json
  {
    "sourceWidth": 1920,
    "sourceHeight": 1080,
    "frames": [
      {
        "timeOffsetMs": 0,
        "boundingBox": {"left": 0.1, "top": 0.15, "right": 0.75, "bottom": 0.95},
        "keypoints": [
          {"name": "nose", "x": 0.5, "y": 0.3, "confidence": 0.95},
          {"name": "left_shoulder", "x": 0.45, "y": 0.45, "confidence": 0.92}
        ]
      }
    ]
  }
  ```

- **17 COCO landmarks:** nose, left_eye, right_eye, left_ear, right_ear, left_shoulder, right_shoulder, left_elbow, right_elbow, left_wrist, right_wrist, left_hip, right_hip, left_knee, right_knee, left_ankle, right_ankle. Same as what Video Intelligence API returned.

- **Primary person:** MoveNet SINGLEPOSE_THUNDER detects only one person by design — no multi-person selection logic needed (unlike the Video Intelligence API which could return multiple person tracks).

- **Pipeline stage count:** Server pipeline is now 5 stages (was 6). "Pose estimation" is removed. Update any hardcoded "6" references to "5" and "N of 6" to "N of 5".

- **What this replaces:** This story replaces the server-side Video Intelligence API approach. The old `internal/pose/videointel.go`, `internal/pipeline/stages/pose.go`, and the `cloud.google.com/go/videointelligence` Go dependency are all removed.

- **Coordinate space:** Keypoints are in the coordinate space of the original video (before trim/crop). This is the same as before — crop stage reads these to compute a bounding box, skeleton stage transforms them to cropped-frame coordinates using `crop-params.json`.

### Architecture Compliance

- Upload handler uses `storage.LiftFile()` for keypoints.json output path
- Pipeline runs without a pose stage — trim → crop → skeleton → metrics → coaching
- Graceful degradation: missing keypoints.json means crop/skeleton/metrics stages skip or preserve full frame
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `error`
- ChromeDP browser verification tests for upload page changes

### Project Structure Notes

New files to create:
- `testdata/keypoints-sample.json` — test fixture generated from spike

Files to modify:
- `web/static/app.js` — add pose estimation logic (from spike)
- `web/templates/partials/upload-modal.html` — add progress UI, keypoints form field
- `web/templates/layouts/base.html` — add ml5.js CDN script
- `internal/handler/lift.go` — accept keypoints multipart field
- `internal/handler/lift_test.go` — upload handler tests with keypoints
- `internal/pose/client.go` → rename to `internal/pose/pose.go`, keep types only
- `internal/pipeline/stage.go` — remove StagePoseEstimation from DefaultStages
- `cmd/press-out/main.go` — remove pose client wiring

Files to delete:
- `internal/pose/videointel.go`
- `internal/pose/videointel_test.go`
- `internal/pose/videointel_integration_test.go` (if exists)
- `internal/pipeline/stages/pose.go`
- `internal/pipeline/stages/pose_test.go`

### References

- [Source: architecture.md#External Integration Architecture] — ml5.js MoveNet integration
- [Source: architecture.md#Data Architecture] — keypoints.json in lift directory structure
- [Source: epics.md#Story 2.4] — acceptance criteria
- [Source: epics.md#FR8] — body keypoint detection
- [Source: web/static/pose-spike.html] — working spike prototype

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
