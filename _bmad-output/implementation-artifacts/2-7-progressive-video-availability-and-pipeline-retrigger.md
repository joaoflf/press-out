# Story 2.7: Progressive Video Availability & Pipeline Re-trigger

Status: ready-for-dev

## Story

As a lifter,
I want to watch my video immediately after upload without waiting for all processing to finish,
so that I can start reviewing while analysis continues in the background.

## Already Implemented (prior stories)

The following AC1/AC2 work is already complete and tested:

- **`HandleGetLift`** (`internal/handler/lift.go:186-215`): parses lift ID, fetches from DB, calls `bestVideoFile()`, builds `LiftDetailData`, renders template. Checks `s.Broker.IsProcessing()` for processing state.
- **`bestVideoFile()`** (`internal/handler/lift.go:219-226`): checks file existence in priority order: `FileCropped > FileTrimmed > FileOriginal`. Returns the first that exists.
- **`LiftDetailData`** (`internal/handler/lift.go:176-183`): struct with `ID`, `LiftType`, `DisplayType`, `DisplayDate`, `VideoSrc`, `Processing`.
- **`HandleListLifts`** (`internal/handler/lift.go:38-67`): includes thumbnail detection and `s.Broker.IsProcessing()` per lift.
- **`lift-detail.html`** (`web/templates/pages/lift-detail.html`): video player, back button, SSE pipeline checklist when processing, placeholder sections for coaching/timeline/metrics.
- **`lift-list-item.html`**: dual-state with processing spinner (SSE-connected) and normal thumbnail/info display.
- **Video serving**: `/data/` file server (`routes.go:42`) with automatic range request support via `http.FileServer`.
- **SSE broker**: `StartProcessing`, `StopProcessing`, `IsProcessing` in `internal/sse/broker.go`. Replays cached state to late subscribers.
- **Pipeline re-runnability**: `pipeline.Run()` calls `StartProcessing` (which clears `lastState`) at entry and `defer StopProcessing`. Safe to call again in a new goroutine.
- **Tests**: `TestHandleGetLift_ValidID`, `_InvalidID`, `_NonNumericID`, `_BestVideoFile`, `_PlaceholderSections` in `internal/handler/lift_test.go`.

## Acceptance Criteria (BDD)

AC1 and AC2 are already satisfied by existing code. This story implements AC3:

1. ~~**Given** a video has been uploaded and the pipeline is still running, **When** the lifter opens the lift detail page, **Then** the original video is available for playback immediately (FR28), **And** video playback begins within 1 second of interaction (NFR3), **And** the pipeline progress checklist is displayed alongside the video~~ **(DONE)**

2. ~~**Given** the pipeline has completed some stages, **When** the lifter views the lift detail, **Then** the best available video is served (cropped > trimmed > original, based on what exists), **And** processing state is derived from file existence (no error state in DB)~~ **(DONE)**

3. **Given** a pipeline run failed or was interrupted on a previously uploaded lift, **When** the lifter triggers a re-process action, **Then** the pipeline runs again on the existing uploaded video without requiring re-upload (NFR11), **And** the SSE progress updates resume for the new pipeline run

## Tasks / Subtasks

- [ ] Add `HandleReprocess` handler in `internal/handler/lift.go` (AC: 3)
  - [ ] `func (s *Server) HandleReprocess(w http.ResponseWriter, r *http.Request)`
  - [ ] Parse lift ID from `r.PathValue("id")`
  - [ ] Fetch lift from DB via `s.Queries.GetLift()` — return 404 if not found
  - [ ] Guard: if `s.Broker.IsProcessing(lift.ID)` is true, return 409 Conflict (pipeline already running)
  - [ ] Guard: if `s.Pipeline` is nil, return 500 (pipeline not configured)
  - [ ] Launch pipeline in background: `go s.Pipeline.Run(context.Background(), lift.ID, s.DataDir)` — same pattern as `HandleCreateLift` at line 137
  - [ ] Return 200 with empty body (HTMX will handle the UI swap)
  - [ ] Log: `slog.Info("pipeline re-triggered", "lift_id", lift.ID)`

- [ ] Register reprocess route in `internal/handler/routes.go` (AC: 3)
  - [ ] Add `mux.HandleFunc("POST /lifts/{id}/reprocess", s.HandleReprocess)` in the Lift CRUD section

- [ ] Add re-process button to `web/templates/pages/lift-detail.html` (AC: 3)
  - [ ] Show button only when `{{if not .Processing}}` — pipeline is not currently running
  - [ ] Place in the section below the pipeline checklist area (between the SSE block and coaching section)
  - [ ] Use interactive control tier (UX-DR13): transparent background, sage when active, h-10
  - [ ] Use `hx-post="/lifts/{{.ID}}/reprocess"` to trigger re-processing
  - [ ] On success, the page should reload to show the pipeline checklist — use `hx-on::after-request="window.location.reload()"` or respond with `HX-Redirect` header
  - [ ] Button text: "Re-process" with a refresh icon

- [ ] Write unit tests in `internal/handler/lift_test.go` (AC: 3)
  - [ ] Test `HandleReprocess` with valid lift ID — returns 200, pipeline starts (verify `Broker.IsProcessing()` becomes true)
  - [ ] Test `HandleReprocess` with non-existent lift — returns 404
  - [ ] Test `HandleReprocess` when already processing — returns 409
  - [ ] Test `HandleReprocess` with invalid ID — returns 404

- [ ] Write ChromeDP browser verification tests (AC: 1, 2, 3)
  - [ ] Start server on random test port with test database and a test lift with video file
  - [ ] Verify `output.css`, HTMX script, `app.js` load without errors
  - [ ] Verify `<html data-theme="press-out">` attribute is present
  - [ ] Verify no JavaScript console errors on page load
  - [ ] Verify video element is present and has a valid `src` attribute
  - [ ] Verify video player is full-width (no horizontal padding)
  - [ ] Verify back button is present
  - [ ] Verify lift type and date are displayed
  - [ ] Verify re-process button is visible when pipeline is not running
  - [ ] Tear down server and test data after

## Prerequisites

- Story 2.6 (Auto-Crop to Lifter) must be complete — the best-video logic depends on cropped.mp4 existing.
- Stories 2.1 (Pipeline & SSE), 2.4 (Pose), 2.5 (Trim) must be complete — the pipeline must have real stages to re-trigger.

## Dev Notes

- **Re-trigger is safe by design.** `pipeline.Run()` calls `Broker.StartProcessing()` which clears cached SSE state (`lastState`), so subscribers get a fresh stream. `defer StopProcessing()` ensures cleanup even on panic. Stages overwrite their output files (e.g., `cropped.mp4`), so re-running produces fresh results.
- **No concurrent run protection beyond the button guard.** The `IsProcessing` check in the handler prevents double-triggering from the UI. There is no mutex in `pipeline.Run()` itself — two concurrent calls for the same lift could race. The UI guard is sufficient for single-user MVP.
- **Pipeline uses `context.Background()`**, not the HTTP request context. This is intentional — the pipeline must outlive the HTTP request (same pattern as `HandleCreateLift` at line 137).
- **HTMX response pattern**: The simplest approach is to reload the page after the POST succeeds. This re-renders `lift-detail.html` with `Processing: true`, which triggers the SSE connection and pipeline checklist. No need for partial swaps.
- **No file deletion on re-trigger.** The pipeline runs all stages from scratch on the original video. Stages overwrite their outputs. This means "existing files are preserved" only in the sense that no explicit delete happens — stages will produce new outputs that replace old ones.

### Existing Patterns to Follow

Follow `HandleCreateLift` (`internal/handler/lift.go:69-141`) for the pipeline launch pattern:
- `go s.Pipeline.Run(context.Background(), lift.ID, s.DataDir)` (line 137)
- Nil-check `s.Pipeline` before calling (line 136)

Follow `HandleDeleteLift` for the ID parsing and lift lookup pattern.

### Architecture Compliance

- No new DB columns — processing state is from `Broker.IsProcessing()`
- File paths use `storage.LiftFile()` (existing pattern)
- Pipeline launch uses `context.Background()` (existing pattern)
- No error states in UI — re-process button simply disappears while processing
- Route follows existing pattern: `POST /lifts/{id}/reprocess`

### References

- [Source: internal/handler/lift.go:136-137] — existing pipeline launch pattern in HandleCreateLift
- [Source: internal/pipeline/pipeline.go:30-87] — Pipeline.Run implementation
- [Source: internal/sse/broker.go:84-101] — StartProcessing/StopProcessing/IsProcessing
- [Source: internal/handler/routes.go] — existing route registration
- [Source: web/templates/pages/lift-detail.html] — existing detail page template
- [Source: epics.md#Story 2.7] — acceptance criteria
- [Source: epics.md#NFR11] — failed pipeline re-triggerable without re-upload

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
