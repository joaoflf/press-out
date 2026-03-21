# Story 2.4: Server-Side Pose Estimation (YOLO)

Status: draft

## Story

As a lifter,
I want the system to detect my body positions from the video after uploading,
so that my joint movements can be used for cropping, visualization, and analysis.

## Acceptance Criteria (BDD)

1. **Given** a video has been uploaded and the pipeline reaches the pose stage, **When** the pose stage runs, **Then** YOLO26n-Pose detects body keypoints via Python subprocess (`uv run scripts/pose.py`), **And** 17 COCO-format keypoints are detected per frame (FR8), **And** keypoint coordinates are normalized (0-1 relative to video dimensions), **And** per-frame bounding boxes are computed from detection boxes, **And** keypoints.json is saved to the lift-ID directory

2. **Given** the pose stage completes successfully, **When** the pipeline continues to downstream stages, **Then** keypoints.json is available for crop, skeleton, and metrics stages

3. **Given** the pose stage fails (Python subprocess error, model download failure, timeout), **When** the orchestrator handles the error, **Then** the error is logged with slog, **And** keypoints.json is not written, **And** the pipeline continues — downstream stages handle missing keypoints gracefully (FR6), **And** no error screen is shown to the lifter

4. **Given** a frame contains no detected person, **When** the pose stage processes that frame, **Then** the frame is included in keypoints.json with an empty keypoints array and a full-frame bounding box `{left:0, top:0, right:1, bottom:1}`

## Prerequisites

- Story 1.2 (Upload a Lift Video) must be complete — this story modifies the upload handler to remove keypoints multipart field parsing.
- Story 2.2 (FFmpeg Integration & Verification) must be complete — establishes the subprocess execution pattern.
- Story 2.3 (Auto-Trim) must be complete — the trim stage runs before pose in the pipeline.
- `uv` and Python 3 must be installed on the system (checked via `make check-deps`).

## Tasks / Subtasks

### Task 1: Create Python pose estimation script

- [ ] Create `scripts/pose.py` — production version based on spike (`spikes/yolo-pose/pose_spike.py`)
  - [ ] Accept CLI args: `video_path` (positional), `-o/--output` (keypoints.json path), `-m/--model` (default: `yolo26n-pose`), `--fps` (default: 30)
  - [ ] Load YOLO26n-Pose model via ultralytics
  - [ ] Process video frame-by-frame using OpenCV, sample at target fps using `frame_step = max(1, round(native_fps / target_fps))` — for a 30fps source this processes every frame, for 60fps every other frame
  - [ ] For each frame: run YOLO inference, extract 17 COCO keypoints + bounding box from the first (highest-confidence) detection — YOLO sorts detections by confidence by default, so index 0 is always highest
  - [ ] For frames with no person detected: output an empty keypoints array and a full-frame bounding box `{left:0, top:0, right:1, bottom:1}`
  - [ ] Normalize all coordinates to 0-1 range (divide by source dimensions)
  - [ ] Output keypoints.json matching `pose.Result` format: `{ sourceWidth, sourceHeight, frames[]{timeOffsetMs, boundingBox{left,top,right,bottom}, keypoints[]{name, x, y, confidence}} }`
  - [ ] Exit code 0 on success, non-zero on failure
  - [ ] Print progress to stderr (Go stage can log it)

### Task 2: Create Python project configuration

- [ ] Create `pyproject.toml` at project root:
  ```toml
  [project]
  name = "press-out-scripts"
  version = "0.1.0"
  requires-python = ">=3.10"
  dependencies = [
      "ultralytics>=8.0",
      "opencv-python-headless>=4.0",
  ]
  ```
  Note: use `opencv-python-headless` (not `opencv-python`) — same API without Qt/GUI deps, much smaller install on a server.
- [ ] Run `uv lock` to generate `uv.lock`
- [ ] Add `uv.lock` to git (reproducible deps)
- [ ] Add `.venv/` to `.gitignore` (uv creates local venv)

### Task 3: Create Go pose estimation pipeline stage

- [ ] Create `internal/pipeline/stages/pose.go`:
  - [ ] `PoseStage` struct with `ProjectRoot string` field — set in `main.go` to the project root directory, used as `cmd.Dir` for subprocess execution
  - [ ] `PoseStage` implements `pipeline.Stage` interface
  - [ ] `Name()` returns `"Pose estimation"` (matches `StagePoseEstimation` constant)
  - [ ] `Run(ctx context.Context, input StageInput) (StageOutput, error)` implementation:
    - [ ] Construct output path: `storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)`
    - [ ] Build command: `exec.CommandContext(ctx, "uv", "run", "scripts/pose.py", input.VideoPath, "-o", keypointsPath)`
    - [ ] Set `cmd.Dir = s.ProjectRoot` — ensures `uv run` finds `pyproject.toml` and the script path resolves correctly regardless of where the Go binary is invoked from
    - [ ] Capture stdout/stderr — log stderr lines via slog at Info level (contains progress), log stdout only on error
    - [ ] On success: return `StageOutput{VideoPath: input.VideoPath}` (pose doesn't produce a video, passes through)
    - [ ] On error: return error to orchestrator for graceful handling
  - [ ] Log with slog: `lift_id`, `stage`, `duration_ms`, `error`

### Task 4: Register pose stage in pipeline

- [ ] In `internal/pipeline/stage.go`:
  - [ ] Add `StagePoseEstimation = "Pose estimation"` constant
  - [ ] Update stage count references from 5 to 6
- [ ] In `cmd/press-out/main.go`:
  - [ ] Add `&stages.PoseStage{ProjectRoot: projectRoot}` as the second stage (after TrimStage, before CropStage)
  - [ ] Derive `projectRoot` — the directory containing `pyproject.toml` (typically the working directory, or derived from the executable path)
  - [ ] Pipeline order: Trim → Pose → Crop → Skeleton → Metrics → Coaching
- [ ] In `web/templates/partials/pipeline-stages.html`:
  - [ ] Add "Pose estimation" as the second stage in the hardcoded stage list (after "Trimming", before "Cropping")
  - [ ] Update both full and compact variants to reflect 6 stages

### Task 5: Remove client-side ml5.js pose estimation code

- [ ] In `web/templates/layouts/base.html`: remove ml5.js CDN script tag
- [ ] In `web/static/app.js`: remove all pose estimation JavaScript (ml5 model loading, canvas processing, frame extraction, smoothing, keypoints JSON generation)
- [ ] In `web/templates/partials/upload-modal.html`: remove pose progress UI elements (progress bar, frame counter), remove keypoints hidden form field
- [ ] In `internal/handler/lift.go`: remove keypoints multipart field parsing from POST /lifts handler — upload now accepts only `video` and `lift_type` fields
- [ ] Delete `web/static/pose-spike.html` (ml5.js spike, superseded by YOLO spike)
- [ ] Verify removal is complete: `grep -r "ml5" web/ internal/` must return zero matches

### Task 6: Update Makefile

- [ ] Add `check-deps` target: verify `uv` and `python3` are installed (alongside existing `ffmpeg` check)
- [ ] Add `uv-sync` target: `uv sync` to install Python deps

### Task 7: Generate test fixture

Depends on Tasks 1 and 2 being complete.

- [ ] Run `uv run scripts/pose.py testdata/videos/sample-lift.mp4 -o testdata/keypoints-sample.json` to generate the test fixture from YOLO output
- [ ] Commit `testdata/keypoints-sample.json` — used by all downstream stage tests (crop, skeleton, metrics)

### Task 8: Verification tests

- [ ] **Go test — pose stage produces keypoints.json:**
  - [ ] Create a lift directory with the sample video
  - [ ] Run PoseStage.Run() with valid input (set ProjectRoot to project root)
  - [ ] Assert keypoints.json is written to lift directory
  - [ ] Assert keypoints.json is valid JSON with sourceWidth, sourceHeight, frames
  - [ ] Assert frames have keypoints with normalized coordinates (0-1)
  - [ ] Skip if `uv` is not installed (`t.Skip("uv not available")`)

- [ ] **Go test — test fixture deserializes into pose.Result:**
  - [ ] Read `testdata/keypoints-sample.json`
  - [ ] Deserialize into `pose.Result`
  - [ ] Assert `SourceWidth > 0` and `SourceHeight > 0`
  - [ ] Assert `len(Frames) > 0`
  - [ ] Assert first frame has 17 keypoints
  - [ ] Assert all keypoint coordinates are in 0-1 range
  - [ ] Assert all keypoint names are valid COCO landmark names
  - [ ] Assert bounding box values are in 0-1 range

- [ ] **Go test — pose stage handles subprocess failure:**
  - [ ] Run PoseStage with a non-existent video path
  - [ ] Assert error is returned (not panic)
  - [ ] Assert keypoints.json is NOT written

- [ ] **Go test — pose stage respects context cancellation:**
  - [ ] Run PoseStage with a cancelled context
  - [ ] Assert error is returned promptly

- [ ] **Go test — pipeline runs 6 stages with pose:**
  - [ ] Create pipeline with all 6 stages
  - [ ] Assert stage names and order: Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching

- [ ] **Go test — upload handler ignores keypoints field:**
  - [ ] POST multipart with `video` + `lift_type` + `keypoints` (include a keypoints field)
  - [ ] Assert upload succeeds (field is ignored, not rejected)
  - [ ] Assert `keypoints.json` was NOT saved to the lift directory (handler no longer reads this field)

- [ ] **Integration test — upload sample video through HTTP endpoint, verify pose estimation:**
  - [ ] Start server on random test port with test database and test data dir
  - [ ] POST multipart upload with `testdata/videos/sample-lift.mp4` + lift type "Snatch"
  - [ ] Wait for pipeline to complete (poll for `keypoints.json` existence in lift directory, with timeout)
  - [ ] Assert `keypoints.json` exists in the lift directory
  - [ ] Deserialize into `pose.Result` — assert valid structure (sourceWidth, sourceHeight, frames with keypoints)
  - [ ] Assert frame count is reasonable for the sample video (~350 frames at 30fps)
  - [ ] Assert detection rate is high (>95% of frames have non-empty keypoints)
  - [ ] Tear down server and test data
  - [ ] Skip if `uv` or `ffmpeg` not installed

- [ ] **Go test — ml5.js removal verification:**
  - [ ] Walk `web/` and `internal/` directories
  - [ ] Assert no file contains the string "ml5" (case-insensitive)
  - [ ] This prevents regressions and confirms Task 5 is complete

- [ ] **ChromeDP test — upload modal has no pose progress UI:**
  - [ ] Navigate to lift list, open upload modal
  - [ ] Assert no JavaScript console errors
  - [ ] Assert no pose progress elements in DOM (no elements with IDs/classes containing "pose-progress" or similar)
  - [ ] Assert upload form has only video selector, lift type selector, and submit button

- [ ] **ChromeDP test — pipeline stages show 6 stages including pose:**
  - [ ] Upload a video to trigger processing
  - [ ] Navigate to lift detail page while processing
  - [ ] Assert pipeline stage checklist contains "Pose estimation" as the second stage
  - [ ] Assert 6 total stages are displayed

## Dev Notes

- **Spike reference:** `spikes/yolo-pose/pose_spike.py` is the working prototype. The production `scripts/pose.py` should be a cleaned-up version — same core logic, same CLI interface, same output format.

- **YOLO26n-Pose:** The `yolo26n-pose` model from ultralytics. 7.5MB, auto-downloads to `~/.cache/` on first run. Uses COCO 17-keypoint format. The spike validated 39.3 fps on CPU and 99.4% frame detection rate on the sample weightlifting video. YOLO sorts detections by confidence descending — `result.keypoints[0]` / `result.boxes[0]` is always the highest-confidence person.

- **uv integration:** Go calls `exec.CommandContext(ctx, "uv", "run", "scripts/pose.py", videoPath, "-o", keypointsPath)` with `cmd.Dir` set to the project root. The `uv run` command automatically creates/uses a venv from `pyproject.toml` and installs dependencies on first run. No manual venv activation needed.

- **Working directory:** The `PoseStage` struct has a `ProjectRoot` field set in `main.go`. This is used as `cmd.Dir` for the subprocess, ensuring `uv run` finds `pyproject.toml` and resolves `scripts/pose.py` correctly regardless of where the Go binary is invoked from (dev, systemd, tests).

- **Subprocess pattern:** Same as FFmpeg — Go manages the subprocess lifecycle, captures output, handles timeouts via context. The contract is: video path in, keypoints.json file out. Zero coupling between Go and Python.

- **opencv-python-headless:** Use `opencv-python-headless` instead of `opencv-python`. Same API, no Qt/GUI dependencies, significantly smaller install (~30MB vs ~80MB). Server has no display — headless is the correct choice.

- **keypoints.json format:** Identical to what ml5.js produced — same structure, same field names, same normalized coordinate system. Downstream stages (crop, skeleton, metrics) consume it identically:
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

- **17 COCO landmarks:** nose, left_eye, right_eye, left_ear, right_ear, left_shoulder, right_shoulder, left_elbow, right_elbow, left_wrist, right_wrist, left_hip, right_hip, left_knee, right_knee, left_ankle, right_ankle.

- **Frames with no detection:** When YOLO detects no person in a frame, output an empty keypoints array `[]` and a full-frame bounding box `{left:0, top:0, right:1, bottom:1}`. This matches the spike behavior and ensures the frames array has consistent entries for every sampled frame.

- **Frame sampling:** The `--fps` flag (default 30) controls sampling density. For a 30fps source, every frame is processed. For a 60fps source, every other frame is sampled (`frame_step = max(1, round(native_fps / target_fps))`). This keeps processing time proportional to video duration, not frame count.

- **Pipeline stage count:** Server pipeline is now 6 stages (was 5 with ml5.js). Pose estimation runs after trim and before crop: Trimming → Pose estimation → Cropping → Rendering skeleton → Computing metrics → Generating coaching.

- **What this replaces:** This story replaces the client-side ml5.js MoveNet approach. The ml5.js CDN script, browser-side pose JavaScript, pose progress UI, and keypoints multipart upload field are all removed.

- **Coordinate space:** Keypoints are in the coordinate space of the input video (trimmed if trim succeeded, otherwise original). The crop stage reads these to compute a bounding box, and the skeleton stage transforms them to cropped-frame coordinates using `crop-params.json`.

- **Model auto-download:** YOLO26n-Pose model downloads automatically on first run (~7.5MB to `~/.cache/`). For CI/testing, the model can be pre-cached. The script handles this transparently.

### Architecture Compliance

- Implements `pipeline.Stage` interface: `Name() string` + `Run(ctx, StageInput) (StageOutput, error)`
- Uses `storage.LiftFile()` for keypoints.json output path
- Subprocess execution via `exec.CommandContext` (not `exec.Command`) — context version is required for timeout/cancellation support
- Sets `cmd.Dir` to project root for reliable path resolution
- Graceful degradation: missing keypoints.json means crop/skeleton/metrics stages skip or preserve full frame
- Logs with `slog` using standard attributes: `lift_id`, `stage`, `duration_ms`, `error`
- Returns errors, never panics — orchestrator handles all error recovery

### Project Structure Notes

New files to create:
- `scripts/pose.py` — YOLO pose estimation script
- `pyproject.toml` — Python project config
- `uv.lock` — Python dependency lockfile
- `internal/pipeline/stages/pose.go` — Go pose stage
- `internal/pipeline/stages/pose_test.go` — tests
- `testdata/keypoints-sample.json` — test fixture (generated from YOLO)

Files to modify:
- `internal/pipeline/stage.go` — add `StagePoseEstimation` constant, update stage count
- `cmd/press-out/main.go` — add PoseStage to pipeline stages list, derive project root
- `internal/handler/lift.go` — remove keypoints multipart field parsing
- `web/static/app.js` — remove ml5.js pose estimation code
- `web/templates/layouts/base.html` — remove ml5.js CDN script tag
- `web/templates/partials/upload-modal.html` — remove pose progress UI, keypoints field
- `web/templates/partials/pipeline-stages.html` — add "Pose estimation" as second stage, update both full and compact variants to 6 stages
- `Makefile` — add check-deps for uv and python3, add uv-sync target

Files to delete:
- `web/static/pose-spike.html` — ml5.js spike (superseded)

### References

- [Source: architecture.md#External Integration Architecture] — YOLO26n-Pose integration
- [Source: architecture.md#Data Architecture] — keypoints.json in lift directory structure
- [Source: epics.md#Story 2.4] — acceptance criteria
- [Source: epics.md#FR8] — body keypoint detection
- [Source: spikes/yolo-pose/pose_spike.py] — working spike prototype

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
