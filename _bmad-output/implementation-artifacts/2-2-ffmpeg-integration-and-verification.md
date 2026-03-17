# Story 2.2: FFmpeg Integration & Verification

Status: ready-for-dev

## Story

As a developer,
I want FFmpeg subprocess execution to be established and verified,
so that all video processing stages can reliably invoke FFmpeg.

## Acceptance Criteria (BDD)

1. **Given** the application starts, **When** FFmpeg availability is checked, **Then** a clear log message confirms FFmpeg is available (or warns if missing) with the detected version, **And** the application continues to run regardless (FFmpeg absence means processing stages will skip gracefully)

2. **Given** a pipeline stage needs to invoke FFmpeg, **When** it calls the shared FFmpeg helper, **Then** FFmpeg is executed via exec.Command with the provided arguments, **And** stdout and stderr are captured, **And** non-zero exit codes are returned as errors (not panics), **And** execution is bounded by a context timeout

3. **Given** a test video file exists in testdata/, **When** the FFmpeg integration test runs, **Then** FFmpeg is invoked successfully on the sample video, **And** the test verifies that output is produced and is a valid video file

## Tasks / Subtasks

- [ ] Create FFmpeg helper package at `internal/ffmpeg/ffmpeg.go` (AC: 2)
  - [ ] `Run(ctx context.Context, args ...string) (stdout []byte, stderr []byte, err error)` — core execution function
  - [ ] Use `exec.CommandContext(ctx, "ffmpeg", args...)` for timeout support
  - [ ] Capture stdout and stderr into `bytes.Buffer`
  - [ ] On non-zero exit code, return error wrapping stderr output
  - [ ] Never panic — always return errors
  - [ ] Log FFmpeg invocation with `slog.Info("ffmpeg exec", "args", args)`
  - [ ] Log completion with `slog.Info("ffmpeg complete", "duration_ms", elapsed)`

- [ ] Add FFmpeg probe function (AC: 1)
  - [ ] `Probe() (version string, err error)` — runs `ffmpeg -version` and parses version string
  - [ ] Called at application startup in `main.go`
  - [ ] On success: `slog.Info("ffmpeg available", "version", version)`
  - [ ] On failure: `slog.Warn("ffmpeg not available", "error", err)` — application continues

- [ ] Add FFmpeg convenience functions for common operations (AC: 2)
  - [ ] `TrimVideo(ctx context.Context, input, output string, startSec, durationSec float64) error` — trim a video segment
  - [ ] `CropVideo(ctx context.Context, input, output string, x, y, w, h int) error` — crop video to rectangle
  - [ ] `ExtractThumbnail(ctx context.Context, input, output string, timeSec float64) error` — extract a single frame as JPEG
  - [ ] `GetDuration(ctx context.Context, input string) (float64, error)` — get video duration in seconds via ffprobe
  - [ ] Each function builds the correct FFmpeg argument list and calls `Run()`

- [ ] Wire FFmpeg availability check in `cmd/press-out/main.go` (AC: 1)
  - [ ] Call `ffmpeg.Probe()` at startup, after config load
  - [ ] Log result — do not exit on failure

- [ ] Add test video file `testdata/videos/sample.mp4` (AC: 3)
  - [ ] Generate a minimal valid MP4 file via FFmpeg: `ffmpeg -f lavfi -i testsrc=duration=2:size=320x240:rate=24 -c:v libx264 -pix_fmt yuv420p testdata/videos/sample.mp4`
  - [ ] 2-second synthetic video, small file size, sufficient for integration tests
  - [ ] Commit to repo so tests can run without external fixtures

- [ ] Write unit tests `internal/ffmpeg/ffmpeg_test.go` (AC: 2, 3)
  - [ ] Test `Run()` with a simple FFmpeg command (e.g., `-version`)
  - [ ] Test `Run()` with invalid args returns error (non-zero exit code)
  - [ ] Test `Run()` respects context timeout (cancel context mid-execution)
  - [ ] Test `Probe()` returns version string when FFmpeg is installed
  - [ ] Test `TrimVideo()` on `testdata/videos/sample.mp4` produces output file
  - [ ] Test `CropVideo()` on sample video produces output file
  - [ ] Test `ExtractThumbnail()` produces a JPEG file
  - [ ] Test `GetDuration()` returns correct duration for sample video
  - [ ] All tests skip gracefully if FFmpeg is not installed (`t.Skip("ffmpeg not available")`)

## Dev Notes

- FFmpeg is a system dependency invoked via `exec.Command` — it is NOT a Go library. The helper wraps subprocess execution with proper error handling.
- Context timeout is critical for pipeline safety. Each FFmpeg invocation should use a context with timeout (e.g., 60 seconds per stage). This prevents a hung FFmpeg process from blocking the entire pipeline indefinitely.
- The convenience functions (`TrimVideo`, `CropVideo`, etc.) are thin wrappers that build FFmpeg argument lists. The actual trim/crop logic and parameters are determined by the pipeline stages in Stories 2.3 and 2.4.
- stderr is the important output from FFmpeg — it logs progress, warnings, and errors there. stdout is typically empty unless using `-f pipe:1`. Capture both.
- The test video should be committed to the repo. Generate it with FFmpeg's built-in test source generator — no need for a real gym video at this stage.
- This story has no UI component — no ChromeDP tests needed.
- Consider using `ffprobe` (installed alongside FFmpeg) for video metadata queries like duration. Same execution pattern as FFmpeg.

### Architecture Compliance

- FFmpeg helper goes in `internal/ffmpeg/` — a new package, consistent with the `internal/mediapipe/` and `internal/claude/` pattern for external tool integration
- All logging uses `slog` with consistent attributes: `duration_ms`, `error`, `args`
- Never `log.Fatal` or `os.Exit` on FFmpeg failure — return errors for graceful degradation
- The architecture specifies FFmpeg invocation via `exec.Command` — this story establishes the canonical way to do it

### Project Structure Notes

New files to create:
- `internal/ffmpeg/ffmpeg.go` — FFmpeg helper functions
- `internal/ffmpeg/ffmpeg_test.go` — tests
- `testdata/videos/sample.mp4` — synthetic test video (generated via FFmpeg)

Files to modify:
- `cmd/press-out/main.go` — add FFmpeg availability check at startup

### References

- [Source: architecture.md#Technical Constraints & Dependencies] — FFmpeg as system dependency via exec.Command
- [Source: architecture.md#Implementation Handoff] — FFmpeg listed as system dependency
- [Source: architecture.md#Process Patterns] — stages return errors, never panic
- [Source: architecture.md#Logging Convention] — slog with standard attributes
- [Source: epics.md#Story 2.2] — acceptance criteria
- [Source: epics.md#Additional Requirements] — "FFmpeg system dependency: Required for video trim, crop, skeleton rendering, and thumbnail extraction via exec.Command"

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
