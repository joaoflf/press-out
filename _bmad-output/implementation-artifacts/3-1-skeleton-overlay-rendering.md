# Story 3.1: Skeleton Overlay Rendering

Status: ready-for-dev

## Story

As a lifter,
I want a skeleton overlay rendered on my lift video,
so that I can see my body positions and joint angles visually during the movement.

## Acceptance Criteria

1. **Given** keypoints.json and crop-params.json exist for a lift, **When** the skeleton rendering stage runs, **Then** keypoint coordinates are transformed from the original frame space to cropped frame space using the crop parameters, **And** a skeleton overlay is drawn onto each cropped video frame using the transformed keypoint data, **And** the skeleton-overlay video is rendered via FFmpeg and saved as skeleton.mp4 in the lift-ID directory (FR9), **And** both the clean video and skeleton video are available for the lift (FR10)

2. **Given** crop-params.json does not exist (crop preserved full frame), **When** the skeleton rendering stage runs, **Then** keypoint coordinates are used as-is without transformation, **And** the skeleton is rendered on the full-frame video

3. **Given** keypoints have varying confidence levels across frames, **When** the skeleton is rendered, **Then** the skeleton degrades gracefully on low-confidence frames (e.g., partial skeleton) rather than disappearing entirely, **And** the skeleton overlay remains visually clear against real gym video backgrounds

4. **Given** keypoints.json does not exist (pose estimation was skipped), **When** the skeleton rendering stage is reached, **Then** the stage is skipped since there is no keypoint data to render, **And** the pipeline continues without a skeleton video

5. **Given** the skeleton stage encounters an FFmpeg error, **When** the subprocess fails, **Then** the error is logged with slog, **And** the stage returns an error to the orchestrator

## Tasks / Subtasks

- [ ] Create `internal/pipeline/stages/skeleton.go` with `SkeletonStage` (AC: 1-5)
  - [ ] Implement `Stage` interface: `Name() string` returns `pipeline.StageRenderingSkeleton`
  - [ ] `Run()`: read keypoints.json, read crop-params.json (optional), get video dimensions/FPS
  - [ ] Transform keypoints from original frame space to cropped frame space when crop-params.json exists
  - [ ] Decode input video via `ffmpeg.DecodeFrames`, draw skeleton per frame, encode via `ffmpeg.EncodeFrames`
  - [ ] Save output as `storage.FileSkeleton` (`skeleton.mp4`) in lift directory
  - [ ] Skip stage (return input unchanged) when keypoints.json does not exist

- [ ] Implement skeleton drawing on raw RGB24 pixel buffers (AC: 1, 3)
  - [ ] Define COCO-17 skeleton connections as a constant table
  - [ ] Draw bones (lines between connected joints) using Bresenham's line algorithm with configurable thickness
  - [ ] Draw joint dots at each keypoint position
  - [ ] Use high-visibility colors (bright/contrasting) that work against gym video backgrounds
  - [ ] Skip individual joints/bones where keypoint confidence is below threshold
  - [ ] Handle partial skeletons gracefully (draw whatever is available)

- [ ] Implement keypoint-to-frame time alignment (AC: 1, 2)
  - [ ] Map keypoint frame timestamps to video frame indices
  - [ ] Use nearest-frame matching (keypoints are at pose estimation rate, video is at native FPS)
  - [ ] Handle edge cases: no keypoints for a given video frame (draw no skeleton for that frame)

- [ ] Wire `SkeletonStage` into `main.go` (AC: 1)
  - [ ] Replace stub at index 3: `pipelineStages[3] = &stages.SkeletonStage{}`

- [ ] Create `internal/pipeline/stages/skeleton_test.go` (AC: 1-5)
  - [ ] Test: skeleton stage produces skeleton.mp4 from test video + keypoints
  - [ ] Test: stage skips when keypoints.json does not exist (returns input VideoPath unchanged)
  - [ ] Test: stage works without crop-params.json (no coordinate transformation)
  - [ ] Test: coordinate transformation math is correct (given known crop-params, verify transformed positions)
  - [ ] Test: low-confidence keypoints are skipped (partial skeleton drawn)
  - [ ] Test: context cancellation stops processing

## Dev Notes

### Architecture Compliance

- **Stage interface**: `Name() string`, `Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error)` — same as all other stages
- **Stage name**: `pipeline.StageRenderingSkeleton` (already defined in `internal/pipeline/stage.go:11`)
- **Output file**: `storage.FileSkeleton` = `"skeleton.mp4"` (already defined in `internal/storage/storage.go:14`)
- **Logging**: `slog.With("lift_id", input.LiftID, "stage", pipeline.StageRenderingSkeleton)` — same pattern as crop/trim stages
- **Error handling**: return `(StageOutput{}, error)` on failure, never panic — orchestrator handles skip
- **File paths**: always via `storage.LiftFile()` — never inline path construction
- **FFmpeg calls**: must include `-y` flag (convention enforced across all stages)

### Pipeline Position and Input

The skeleton stage is at index 3 in the pipeline (after Pose, Trim, Crop):

```
Pose → Trim → Crop → **Skeleton** → Metrics → Coaching
```

`input.VideoPath` will be one of:
- `cropped.mp4` — normal case (crop succeeded)
- `trimmed.mp4` — crop was skipped but trim succeeded
- `original.mp4` — both trim and crop were skipped

The stage must work with any of these inputs.

### Keypoint Coordinate Transformation

Keypoints in `keypoints.json` are normalized (0-1) relative to the **original** video dimensions (`result.SourceWidth`, `result.SourceHeight`). When the input is a cropped video, coordinates must be transformed to the cropped frame space.

**Transformation with crop-params.json:**
```
pixelX = keypoint.X * sourceWidth   // denormalize to original pixel space
pixelY = keypoint.Y * sourceHeight

croppedX = pixelX - cropParams.X    // translate to cropped frame origin
croppedY = pixelY - cropParams.Y
```

Where `cropParams` is read from `crop-params.json`:
```json
{"x": 123, "y": 45, "w": 540, "h": 960, "sourceWidth": 1920, "sourceHeight": 1080}
```

**Without crop-params.json (full frame preserved):**
```
croppedX = keypoint.X * videoWidth   // denormalize directly to video dimensions
croppedY = keypoint.Y * videoHeight
```

Points outside the frame bounds (croppedX < 0, croppedX >= frameW, etc.) should be clipped — do not draw that joint/bone.

### CropParams Type

The `CropParams` struct is already defined in `internal/pipeline/stages/crop.go:35-42`:

```go
type CropParams struct {
    X            int `json:"x"`
    Y            int `json:"y"`
    W            int `json:"w"`
    H            int `json:"h"`
    SourceWidth  int `json:"sourceWidth"`
    SourceHeight int `json:"sourceHeight"`
}
```

Read it by unmarshalling `crop-params.json`. If the file doesn't exist, proceed without transformation.

### COCO-17 Skeleton Connections

The COCO pose format defines 17 keypoints. The skeleton connections (bones) to draw:

```go
var skeletonBones = [][2]string{
    // Head
    {pose.LandmarkNose, pose.LandmarkLeftEye},
    {pose.LandmarkNose, pose.LandmarkRightEye},
    {pose.LandmarkLeftEye, pose.LandmarkLeftEar},
    {pose.LandmarkRightEye, pose.LandmarkRightEar},
    // Torso
    {pose.LandmarkLeftShoulder, pose.LandmarkRightShoulder},
    {pose.LandmarkLeftShoulder, pose.LandmarkLeftHip},
    {pose.LandmarkRightShoulder, pose.LandmarkRightHip},
    {pose.LandmarkLeftHip, pose.LandmarkRightHip},
    // Left arm
    {pose.LandmarkLeftShoulder, pose.LandmarkLeftElbow},
    {pose.LandmarkLeftElbow, pose.LandmarkLeftWrist},
    // Right arm
    {pose.LandmarkRightShoulder, pose.LandmarkRightElbow},
    {pose.LandmarkRightElbow, pose.LandmarkRightWrist},
    // Left leg
    {pose.LandmarkLeftHip, pose.LandmarkLeftKnee},
    {pose.LandmarkLeftKnee, pose.LandmarkLeftAnkle},
    // Right leg
    {pose.LandmarkRightHip, pose.LandmarkRightKnee},
    {pose.LandmarkRightKnee, pose.LandmarkRightAnkle},
}
```

Landmark name constants are already defined in `internal/pose/pose.go:34-52`.

### Drawing on Raw RGB24 Pixel Buffers

The frame piping approach (via `ffmpeg.DecodeFrames` / `ffmpeg.EncodeFrames`) gives raw RGB24 pixel data. Each frame is `width * height * 3` bytes, laid out row by row, 3 bytes per pixel (R, G, B).

**Pixel access pattern:**
```go
offset := (y*frameW + x) * 3
frameBuf[offset]   = r  // red
frameBuf[offset+1] = g  // green
frameBuf[offset+2] = b  // blue
```

**Drawing approach — no external libraries needed:**
- **Lines (bones):** Bresenham's line algorithm, draw with thickness by offsetting perpendicular to the line direction. A thickness of 3-4px is visible on mobile without being overpowering.
- **Dots (joints):** Filled circle at each keypoint position. Radius of 5-6px.
- **Colors:** Use bright, high-contrast colors that are visible against typical gym backgrounds (gray walls, black equipment, wood platforms):
  - Bones: bright cyan/lime (`#00FF88` or similar) — high visibility against any background
  - Joints: white dots with a thin darker outline for contrast
  - Consider using different colors for left vs right sides (e.g., left=cyan, right=magenta) to aid analysis

**Constants:**
```go
const (
    skeletonConfidenceThreshold = 0.3  // skip joints below this confidence
    skeletonBoneThickness       = 3    // pixels
    skeletonJointRadius          = 5    // pixels
)
```

### Frame Piping Pattern

Reuse the frame piping approach established in story 2.10 (crop stage). The decode/encode functions already exist in `internal/ffmpeg/ffmpeg.go:198-242`.

**Skeleton rendering loop:**
```
1. Get input video FPS via ffmpeg.GetFPS()
2. Get input video dimensions via ffmpeg.GetDimensions()
3. Start decode: ffmpeg.DecodeFrames(ctx, inputVideoPath)
4. Start encode: ffmpeg.EncodeFrames(ctx, outputPath, frameW, frameH, fps)
5. Build keypoint lookup: map keypoint timestamps to video frame indices
6. For each frame:
   a. Read frameW * frameH * 3 bytes from decode pipe
   b. Find nearest keypoint frame by time
   c. Transform keypoint coordinates (if crop-params exist)
   d. Draw skeleton connections and joints onto the frame buffer
   e. Write frame buffer to encode pipe
7. Close encode stdin, wait for both processes
```

**Keypoint-to-video-frame alignment:**
Keypoints are sampled at the pose estimation rate (may differ from video FPS). For each video frame at time `t = frameIndex / fps`, find the keypoint frame with the nearest `timeOffsetMs`.

```go
// Build sorted list of keypoint times for binary search
kpTimes := make([]float64, len(result.Frames))
for i, f := range result.Frames {
    kpTimes[i] = float64(f.TimeOffsetMs) / 1000.0
}

// For video frame at time t, find nearest keypoint:
func nearestKeypointFrame(kpTimes []float64, frames []pose.Frame, t float64) *pose.Frame
```

### Trim-Range Keypoint Filtering

When the input video is trimmed, keypoints.json still covers the full original video. The skeleton stage needs to align keypoint timestamps to the trimmed video's timeline.

Read `trim-params.json` (written by trim stage) to get `trimStartMs`. The trimmed video starts at time 0, but the keypoints have absolute timestamps. Subtract `trimStartMs` from keypoint timestamps when matching to video frames:

```go
var trimOffset float64 = 0
trimParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimParams)
if trimData, err := os.ReadFile(trimParamsPath); err == nil {
    var tp TrimParams
    if err := json.Unmarshal(trimData, &tp); err == nil {
        trimOffset = float64(tp.TrimStartMs) / 1000.0
    }
}
// Video frame at time t corresponds to keypoint time (t + trimOffset)
```

`TrimParams` is defined in `internal/pipeline/stages/trim.go:35-38`.

### Wiring in main.go

Currently `main.go:94-97` replaces stubs at indices 0-2. Add skeleton at index 3:

```go
pipelineStages[3] = &stages.SkeletonStage{}  // Replace stub with real skeleton stage
```

### Existing FFmpeg Functions Used

All in `internal/ffmpeg/ffmpeg.go`:
- `GetFPS(ctx, input) (float64, error)` — line 164
- `GetDimensions(ctx, input) (width, height int, error)` — line 125
- `DecodeFrames(ctx, input) (*exec.Cmd, io.ReadCloser, error)` — line 198
- `EncodeFrames(ctx, output, w, h int, fps float64) (*exec.Cmd, io.WriteCloser, error)` — line 219

### Pose Types Used

All in `internal/pose/pose.go`:
- `pose.Result` — top-level keypoints.json structure
- `pose.Frame` — per-frame data with `TimeOffsetMs`, `BoundingBox`, `Keypoints`
- `pose.Keypoint` — `Name`, `X`, `Y`, `Confidence` (all normalized 0-1)
- Landmark constants: `pose.LandmarkNose`, `pose.LandmarkLeftShoulder`, etc.

### Storage Constants Used

In `internal/storage/storage.go`:
- `storage.FileKeypoints` = `"keypoints.json"`
- `storage.FileCropParams` = `"crop-params.json"`
- `storage.FileTrimParams` = `"trim-params.json"`
- `storage.FileSkeleton` = `"skeleton.mp4"`

### Performance Considerations

- Frame piping processes frames sequentially — same approach as the crop stage
- RGB24 buffers are allocated once and reused per frame (don't allocate per frame)
- Bresenham line drawing is O(max(dx,dy)) per line — negligible compared to FFmpeg decode/encode
- A 10s video at 30fps = 300 frames. With 16 bones + 17 joints per frame, drawing is fast.
- The encode uses `-preset fast` (same as crop stage) for reasonable speed/quality trade-off

### What NOT to Do

- **No external Go image libraries** (image/draw, gg, etc.) — draw directly on RGB24 buffers. Avoids dependencies and the overhead of converting between formats.
- **No per-frame FFmpeg invocations** — use the pipe-based approach (DecodeFrames/EncodeFrames)
- **No changes to the Stage interface** — use existing StageInput/StageOutput
- **No audio handling** — skeleton video has no audio (same as cropped video from FFmpeg pipe)
- **No changes to keypoints.json or crop-params.json** — read-only consumer
- **Do not modify trim.go, crop.go, pose.go, or pipeline.go** — this is a new, self-contained stage

### Project Structure Notes

New files:
- `internal/pipeline/stages/skeleton.go` — SkeletonStage implementation
- `internal/pipeline/stages/skeleton_test.go` — tests

Modified files:
- `cmd/press-out/main.go` — wire SkeletonStage at pipeline index 3

No other files modified.

### References

- [Source: internal/pipeline/stage.go] — Stage interface, StageRenderingSkeleton constant
- [Source: internal/pose/pose.go] — Keypoint types and COCO-17 landmark constants
- [Source: internal/storage/storage.go] — FileSkeleton, FileKeypoints, FileCropParams, FileTrimParams constants
- [Source: internal/pipeline/stages/crop.go:35-42] — CropParams struct definition
- [Source: internal/pipeline/stages/trim.go:35-38] — TrimParams struct definition
- [Source: internal/ffmpeg/ffmpeg.go:198-242] — DecodeFrames, EncodeFrames (frame piping)
- [Source: internal/ffmpeg/ffmpeg.go:125-161] — GetDimensions
- [Source: internal/ffmpeg/ffmpeg.go:164-194] — GetFPS
- [Source: cmd/press-out/main.go:94-97] — pipeline stage wiring
- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.1] — acceptance criteria
- [Source: _bmad-output/planning-artifacts/architecture.md] — Stage interface pattern, file organization

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
