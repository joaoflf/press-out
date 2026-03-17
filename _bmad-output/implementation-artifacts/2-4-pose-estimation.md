# Story 2.4: Pose Estimation via Video Intelligence API

Status: approved

## Story

As a lifter,
I want the system to detect my body positions from the video,
so that my joint movements can be used for cropping, visualization, and analysis.

## Acceptance Criteria (BDD)

1. **Given** the pipeline reaches the pose estimation stage with a trimmed (or original) video, **When** the pose stage runs, **Then** the system sends the video to a pose estimation provider via a provider-agnostic `PoseClient` interface, **And** keypoint coordinates (17 COCO-format body landmarks) are received per timestamp (FR8), **And** the keypoint data is saved as `keypoints.json` in the lift-ID directory

2. **Given** the pose estimation provider is unavailable or returns an error, **When** the pose stage attempts to call it, **Then** the error is logged with slog (`lift_id`, `stage`, `error` attributes), **And** the stage returns an error to the orchestrator, **And** the orchestrator marks it skipped and continues (NFR6), **And** downstream stages that depend on keypoints (crop, skeleton, metrics) handle the missing `keypoints.json` gracefully, **And** no error screen is shown to the lifter

3. **Given** the video contains partial occlusion or variable lighting, **When** the pose stage processes the video, **Then** keypoints are extracted with the best available confidence, **And** low-confidence keypoints are included in the output with their confidence scores (graceful degradation over omission)

4. **Given** multiple persons are detected in the video, **When** the pose stage processes the results, **Then** the system selects the primary lifter (person with the largest average bounding box area across all frames), **And** only that person's keypoints are written to `keypoints.json`

5. **Given** no persons are detected in the video, **When** the pose stage completes, **Then** the stage returns an error (no keypoints to write), **And** the orchestrator handles it as a skipped stage

## Prerequisites

- Story 2.2 (FFmpeg Integration & Verification) must be complete — this story uses `ffmpeg.RunProbe()` to get source video dimensions.
- Story 2.3 (Auto-Trim) should be complete — pose estimation runs on the trimmed video for lower API cost and better accuracy.
- Google Cloud project with Video Intelligence API enabled.
- Service account credentials configured via `GOOGLE_APPLICATION_CREDENTIALS` environment variable (standard Google Cloud Application Default Credentials).

## Tasks / Subtasks

### Task 1: Add `GetDimensions` helper to ffmpeg package

- [ ] Add `GetDimensions(ctx, input) (width, height int, err error)` to `internal/ffmpeg/ffmpeg.go`
- [ ] Implementation: run `ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=x:p=0 input` and parse `WxH` output
- [ ] Add unit test in `ffmpeg_test.go` with the real test video (`testdata/videos/sample-lift.mp4`)

### Task 2: Define provider-agnostic PoseClient interface and types

- [ ] Create `internal/pose/client.go` with:
  - `Client` interface: `DetectPose(ctx context.Context, videoData []byte) (*Result, error)` + `Close() error`
  - `Result` struct: `SourceWidth int`, `SourceHeight int`, `Frames []Frame`
  - `BoundingBox` struct: `Left float64`, `Top float64`, `Right float64`, `Bottom float64` (all normalized 0-1)
  - `Frame` struct: `TimeOffsetMs int64`, `BoundingBox BoundingBox`, `Keypoints []Keypoint`
  - `Keypoint` struct: `Name string`, `X float64` (normalized 0-1), `Y float64` (normalized 0-1), `Confidence float64` (0-1)
  - JSON struct tags on all fields (these types are serialized to `keypoints.json`)
  - Named constants for the 17 landmark names: `LandmarkNose`, `LandmarkLeftEye`, `LandmarkRightEye`, `LandmarkLeftEar`, `LandmarkRightEar`, `LandmarkLeftShoulder`, `LandmarkRightShoulder`, `LandmarkLeftElbow`, `LandmarkRightElbow`, `LandmarkLeftWrist`, `LandmarkRightWrist`, `LandmarkLeftHip`, `LandmarkRightHip`, `LandmarkLeftKnee`, `LandmarkRightKnee`, `LandmarkLeftAnkle`, `LandmarkRightAnkle`
- [ ] `SourceWidth`/`SourceHeight` are NOT populated by `DetectPose` — the caller (pose stage) fills them in from `ffprobe` before writing JSON. `DetectPose` returns only normalized coordinates and frame data.

### Task 3: Implement Google Cloud Video Intelligence client

- [ ] Create `internal/pose/videointel.go` implementing `Client` interface
- [ ] Go dependency: `cloud.google.com/go/videointelligence/apiv1`
- [ ] Constructor: `NewVideoIntelClient(ctx context.Context) (Client, error)` — creates `videointelligence.NewClient(ctx)` using Application Default Credentials (no API key needed)
- [ ] `DetectPose` implementation:
  - Build `AnnotateVideoRequest` with `InputContent: videoData`, `Features: [PERSON_DETECTION]`, `VideoContext.PersonDetectionConfig: {IncludePoseLandmarks: true, IncludeBoundingBoxes: true}`
  - Call `client.AnnotateVideo(ctx, req)` — returns a long-running operation (LRO)
  - Wait for completion: `op.Wait(ctx)` — blocks until the API finishes processing (typically 30-120s for a <60s video)
  - Process response: iterate `AnnotationResults[0].PersonDetectionAnnotations`
  - **Primary person selection:** if multiple persons detected, select the one whose track has the largest average bounding box area (`(right-left) * (bottom-top)` across all `TimestampedObjects`). Log a warning with the person count.
  - For each `TimestampedObject` in the selected person's track: extract `TimeOffset` (convert to milliseconds), extract `NormalizedBoundingBox` into `BoundingBox{Left, Top, Right, Bottom}`, iterate `Landmarks` to build `[]Keypoint` with `Name`, `Point.X`, `Point.Y`, `Confidence`
  - **Landmark name mapping:** normalize API landmark names to COCO-format constants (e.g., if API returns `"LEFT_SHOULDER"`, map to `"left_shoulder"`). This mapping belongs in `videointel.go` — downstream consumers always see consistent names.
  - Return `&Result{Frames: frames}` (caller fills `SourceWidth`/`SourceHeight`)
  - Return error if 0 persons detected or 0 landmarks found
- [ ] `Close` implementation: calls `client.Close()`
- [ ] Add `internal/pose/videointel_test.go` with unit tests using a mock/stub (do NOT call the real API in unit tests)

### Task 4: Implement pose estimation pipeline stage

- [ ] Create `internal/pipeline/stages/pose.go`
- [ ] `PoseStage` struct with `client pose.Client` field
- [ ] Constructor: `NewPoseStage(client pose.Client) *PoseStage`
- [ ] `Name()` returns `pipeline.StagePoseEstimation`
- [ ] Named constants:
  - `maxInlineVideoBytes = 50 * 1024 * 1024` (50MB — inline content limit for Video Intelligence API)
  - `poseAPITimeout = 2 * time.Minute` (stage-level timeout for the API call)
- [ ] `Run()` implementation:
  1. Read input video bytes: `os.ReadFile(input.VideoPath)`
  2. Check `len(videoData) > maxInlineVideoBytes` — if exceeded, return error `"video too large for inline pose estimation (%d bytes, max %d)"` (stage skips gracefully)
  3. Get source dimensions: `ffmpeg.GetDimensions(ctx, input.VideoPath)` — populates `result.SourceWidth`, `result.SourceHeight`
  4. Create a timeout context: `timeoutCtx, cancel := context.WithTimeout(ctx, poseAPITimeout)` + `defer cancel()`
  5. Call `client.DetectPose(timeoutCtx, videoData)`
  6. Set `result.SourceWidth` and `result.SourceHeight` from ffprobe values
  7. Marshal `result` to JSON with `json.MarshalIndent(result, "", "  ")`
  8. Write to `storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)`
  9. Log frame count and landmark count: `slog.Info("pose estimation complete", "lift_id", ..., "stage", ..., "frames", len(result.Frames), "duration_ms", ...)`
  10. Return `StageOutput{VideoPath: input.VideoPath}` — pose stage passes the video through unchanged
- [ ] On any error: return `StageOutput{}, fmt.Errorf("pose: %w", err)` — orchestrator handles skip
- [ ] Create `internal/pipeline/stages/pose_test.go`:
  - Test with mock `pose.Client` returning valid keypoints — verify `keypoints.json` written with correct structure
  - Test with mock `pose.Client` returning error — verify stage returns error
  - Test `keypoints.json` contains expected fields: `sourceWidth`, `sourceHeight`, `frames[].timeOffsetMs`, `frames[].boundingBox.{left, top, right, bottom}`, `frames[].keypoints[].{name, x, y, confidence}`
  - Test video exceeding `maxInlineVideoBytes` — verify stage returns error

### Task 5: Fix pipeline stage ordering

- [ ] In `internal/pipeline/stage.go`, update `DefaultStages()` to put `StagePoseEstimation` BEFORE `StageCropping`:
  ```
  Trimming -> Pose estimation -> Cropping -> Rendering skeleton -> Computing metrics -> Generating coaching
  ```
- [ ] This is a bug fix — the current order has Cropping before Pose estimation, which is wrong after the Epic 2 restructure

### Task 6: Wire pose stage into main.go and update config

- [ ] In `cmd/press-out/main.go`: create `pose.NewVideoIntelClient(ctx)`, pass to `NewPoseStage(client)`, register in pipeline stages list (replacing the `StubStage` for pose estimation)
- [ ] Add `defer poseClient.Close()` for cleanup
- [ ] In `internal/config/config.go`: remove `MediaPipeAPIKey` field and `os.Getenv("MEDIAPIPE_API_KEY")` line (`.env.example` is already updated)
- [ ] Authentication: `GOOGLE_APPLICATION_CREDENTIALS` env var is pre-configured on the machine pointing to a service account JSON key file. The Go client library reads it automatically via Application Default Credentials — no code needed to load or pass credentials. If not set or file missing, `videointelligence.NewClient(ctx)` returns a clear error which propagates as a stage failure.

### Task 7: Integration test with real API

- [ ] Create `internal/pose/videointel_integration_test.go` with build tag `//go:build integration`
- [ ] Test calls the real Video Intelligence API using `testdata/videos/sample-lift.mp4`
- [ ] `GOOGLE_APPLICATION_CREDENTIALS` is pre-configured on the machine — no setup needed
- [ ] Assertions:
  - `Result` has at least 1 frame
  - Each frame has `TimeOffsetMs >= 0`
  - Each frame has a `BoundingBox` with all values in 0.0-1.0 range and `Left < Right`, `Top < Bottom`
  - Each frame has at least 1 keypoint
  - All keypoint coordinates are normalized (0.0-1.0 range)
  - All keypoint confidence values are in 0.0-1.0 range
  - Log the actual landmark names returned by the API (helps verify COCO name mapping)
  - Log total frame count and average keypoints per frame
- [ ] Run with: `go test -tags=integration -v -timeout=3m ./internal/pose/...`
- [ ] The agent MUST run this test and verify it passes before marking the story complete
- [ ] This test costs ~$0.10 per run — acceptable for verification

### Task 8: Update architecture and epics docs

- [ ] In `_bmad-output/planning-artifacts/architecture.md`:
  - Replace `MEDIAPIPE_API_KEY` references with `GOOGLE_APPLICATION_CREDENTIALS`
  - Replace "MediaPipe: HTTP API client" with "Google Cloud Video Intelligence: gRPC client via Go client library"
  - Rename `internal/mediapipe/client.go` to `internal/pose/client.go` + `internal/pose/videointel.go` in the project structure
  - Update the External Integration Architecture section
- [ ] In `_bmad-output/planning-artifacts/epics.md`:
  - Update Story 2.4 title from "Pose Estimation via MediaPipe" to "Pose Estimation via Video Intelligence API"
  - Update AC text to remove MediaPipe references

## Dev Notes

- **Provider choice: Google Cloud Video Intelligence API** — provides `PERSON_DETECTION` feature with `includePoseLandmarks: true`. Returns 17 COCO-format body landmarks per detected person per frame. Async API (long-running operation) — fits naturally since the pipeline already runs in a background goroutine. Authentication via Application Default Credentials (service account key), not API keys.

- **Why not MediaPipe or Roboflow:** MediaPipe's hosted solutions (PoseTracker, etc.) are WebView/iframe-based, not a REST API callable from a Go backend. Roboflow API had authentication issues and couldn't get inference endpoint working. Google Cloud Video Intelligence is a proper server-side API with an official Go client library.

- **Provider-agnostic design:** The `pose.Client` interface allows swapping providers without changing the pipeline stage or downstream consumers. If a better/cheaper provider emerges, implement the interface and swap at wiring time in `main.go`.

- **API pattern:** `videos:annotate` is async. The Go client's `AnnotateVideo()` returns an LRO. Calling `op.Wait(ctx)` blocks the goroutine until completion (30-120s typical for <60s video). Context cancellation propagates correctly for shutdown.

- **Inline content vs GCS:** The Video Intelligence API accepts video as inline bytes (`InputContent`) or a GCS URI. For MVP, use inline content — a trimmed weightlifting video (5-15 seconds) is typically 3-20MB, well within gRPC limits. If videos exceed limits, a future enhancement can upload to GCS first. This avoids adding GCS bucket infrastructure (NFR8: no external infrastructure beyond the two APIs).

- **keypoints.json schema:**
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
          {"name": "left_shoulder", "x": 0.45, "y": 0.45, "confidence": 0.92},
          {"name": "right_shoulder", "x": 0.55, "y": 0.45, "confidence": 0.91}
        ]
      },
      {
        "timeOffsetMs": 33,
        "boundingBox": {"left": 0.1, "top": 0.15, "right": 0.75, "bottom": 0.95},
        "keypoints": [...]
      }
    ]
  }
  ```
  - All coordinates are normalized (0.0-1.0 relative to source frame)
  - `sourceWidth`/`sourceHeight` let consumers denormalize to pixel coordinates
  - `boundingBox` per frame is the API's person detection box — used by the crop stage (Story 2.5) as the crop region guide
  - Frames sorted by `timeOffsetMs` ascending
  - Keypoints per frame include only what the API returns (may be fewer than 17 if some landmarks are not detected for that frame — downstream consumers must handle variable counts)

- **17 COCO-format landmarks:** nose, left_eye, right_eye, left_ear, right_ear, left_shoulder, right_shoulder, left_elbow, right_elbow, left_wrist, right_wrist, left_hip, right_hip, left_knee, right_knee, left_ankle, right_ankle. Sufficient for skeleton rendering, crop bounding box, joint angles, and bar path (via wrist tracking).

- **Primary person selection:** When multiple persons are detected, the stage selects the one with the largest average bounding box area. In a typical weightlifting video, the lifter is the most prominent person in frame. This heuristic is simple and sufficient for MVP.

- **Pricing (verify against current docs):** ~$0.10/min with first 1000 min/month free. A 10-second trimmed video costs ~$0.02 (minimum 1 minute charge = $0.10) after the free tier.

- **Coordinate space:** Keypoints are in the coordinate space of the input video (trimmed or original). The crop stage (Story 2.5) reads these to compute a bounding box. The skeleton stage (Story 3.1) transforms them to cropped-frame coordinates using `crop-params.json`.

- This story does NOT produce a video output — it writes `keypoints.json` as a data artifact. The stage returns `StageOutput{VideoPath: input.VideoPath}` to pass the video through unchanged.

### Verification Note

The Google Cloud Video Intelligence API details in this story are based on documentation available through May 2025. Before implementation, verify:
1. The API is still available and not deprecated
2. The Go client library package path: `cloud.google.com/go/videointelligence/apiv1`
3. The exact landmark names returned by the API (may differ slightly from COCO conventions)
4. Current pricing at https://cloud.google.com/video-intelligence/pricing
5. Inline content size limits for the gRPC client (should handle 20MB+ but verify)

### Architecture Compliance

- Implements `pipeline.Stage` interface: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for output path construction
- Returns errors on failure, never panics
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `duration_ms`, `error`
- Graceful degradation: API failure causes stage error, orchestrator skips, downstream stages handle missing `keypoints.json`

### Project Structure Notes

New files to create:
- `internal/pose/client.go` — provider-agnostic PoseClient interface and types
- `internal/pose/videointel.go` — Google Cloud Video Intelligence implementation
- `internal/pose/videointel_test.go` — tests (with mock client, no real API calls)
- `internal/pipeline/stages/pose.go` — pose estimation pipeline stage
- `internal/pipeline/stages/pose_test.go` — tests

Files to modify:
- `internal/ffmpeg/ffmpeg.go` — add `GetDimensions()` helper
- `internal/ffmpeg/ffmpeg_test.go` — test for `GetDimensions()`
- `internal/pipeline/stage.go` — fix stage ordering (pose before crop)
- `internal/config/config.go` — remove `MediaPipeAPIKey`
- `cmd/press-out/main.go` — wire pose client and stage

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition
- [Source: architecture.md#Data Architecture] — keypoints.json in lift directory structure
- [Source: architecture.md#Process Patterns] — graceful degradation
- [Source: architecture.md#External Integration Architecture] — external API client pattern
- [Source: epics.md#Story 2.4] — acceptance criteria
- [Source: epics.md#FR8] — body keypoint detection
- [Source: epics.md#NFR6] — external API unavailability handling

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
