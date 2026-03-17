# Story 2.5: Progressive Video Availability & Pipeline Re-trigger

Status: ready-for-dev

## Story

As a lifter,
I want to watch my video immediately after upload without waiting for all processing to finish,
so that I can start reviewing while analysis continues in the background.

## Acceptance Criteria (BDD)

1. **Given** a video has been uploaded and the pipeline is still running, **When** the lifter opens the lift detail page, **Then** the original video is available for playback immediately (FR28), **And** video playback begins within 1 second of interaction (NFR3), **And** the pipeline progress checklist is displayed alongside the video

2. **Given** the pipeline has completed some stages, **When** the lifter views the lift detail, **Then** the best available video is served (cropped > trimmed > original, based on what exists), **And** processing state is derived from file existence (no error state in DB)

3. **Given** a pipeline run failed or was interrupted on a previously uploaded lift, **When** the lifter triggers a re-process action, **Then** the pipeline runs again on the existing uploaded video without requiring re-upload (NFR11), **And** existing successfully-produced files are preserved, **And** the SSE progress updates resume for the new pipeline run

## Tasks / Subtasks

- [ ] Implement `HandleGetLift` in `internal/handler/lift.go` (AC: 1, 2)
  - [ ] Parse lift ID from URL path `r.PathValue("id")`
  - [ ] Fetch lift from DB via `s.Queries.GetLift(ctx, id)`
  - [ ] Return 404 if lift not found
  - [ ] Determine best available video by checking file existence in order: cropped.mp4 > trimmed.mp4 > original.mp4
  - [ ] Determine processing state by checking which stage output files exist
  - [ ] Build `LiftDetailData` struct with: lift info, video URL, processing state, pipeline stage states
  - [ ] Render `lift-detail.html` template

- [ ] Create `LiftDetailData` template data struct (AC: 1, 2)
  - [ ] `Lift` — lift record from DB (ID, LiftType, CreatedAt)
  - [ ] `VideoURL` — URL to serve the best available video (e.g., `/static/lifts/{id}/cropped.mp4`)
  - [ ] `IsProcessing` — whether pipeline is currently running (derived from active SSE subscribers or file state)
  - [ ] `Stages` — slice of stage status structs (name, state: pending/active/complete/skipped)
  - [ ] `HasThumbnail` — whether thumbnail.jpg exists

- [ ] Add video file serving route (AC: 1, 2)
  - [ ] Serve lift video files via a route like `GET /lifts/{id}/video/{filename}`
  - [ ] Or use the static file server with a path mapping to data directory
  - [ ] Ensure video files are served with correct `Content-Type: video/mp4` headers
  - [ ] Support HTTP range requests for video seeking (Go's `http.ServeFile` handles this automatically)

- [ ] Implement best-video-available logic (AC: 2)
  - [ ] `storage.BestVideo(dataDir string, liftID int64) string` — returns the path to the best available video file
  - [ ] Check file existence in priority order: `FileCropped` > `FileTrimmed` > `FileOriginal`
  - [ ] Return the first file that exists
  - [ ] This function is also useful for the pipeline (determining what video downstream stages should work with)

- [ ] Implement processing state derivation (AC: 2)
  - [ ] `storage.LiftProcessingState(dataDir string, liftID int64) []StageState` — returns state of each pipeline stage
  - [ ] For each stage, check if the corresponding output file exists:
    - Trimming → `trimmed.mp4`
    - Cropping → `cropped.mp4` (+ `thumbnail.jpg`)
    - Pose estimation → `keypoints.json`
    - Rendering skeleton → `skeleton.mp4`
    - Computing metrics → (check DB for metrics rows)
    - Generating coaching → (check DB for coaching columns)
  - [ ] Returns a slice of stage name + state (complete/pending) — no "active" or "error" states from disk inspection

- [ ] Create/update `web/templates/pages/lift-detail.html` (AC: 1, 2)
  - [ ] Full-width edge-to-edge video player with the best available video
  - [ ] Video auto-plays the clean video (skeleton isn't available yet in Epic 2)
  - [ ] Below video: pipeline progress checklist (if processing) or result sections (if done)
  - [ ] Back button for returning to the list (navigation tier: no background, charcoal, 44px target)
  - [ ] Lift type and date displayed
  - [ ] SSE connection to `/lifts/{id}/events` for live updates
  - [ ] When SSE `pipeline-done` event received, reload the page or swap in result view

- [ ] Implement pipeline re-trigger (AC: 3)
  - [ ] Add a "Re-process" button on the lift detail page (visible when pipeline is not currently running)
  - [ ] `POST /lifts/{id}/reprocess` endpoint in handler
  - [ ] Re-triggers `pipeline.Run(ctx, liftID)` in a new background goroutine
  - [ ] Does NOT delete existing files — pipeline stages check for existing outputs
  - [ ] SSE events resume for the new pipeline run
  - [ ] Register the new route in `routes.go`

- [ ] Update `HandleListLifts` to include processing state (AC: 1)
  - [ ] For each lift in the list, check if it has a thumbnail
  - [ ] Determine if pipeline is still running (check if broker has active subscribers or check file state)
  - [ ] Pass thumbnail URL and processing state to the list template

- [ ] Update `web/templates/partials/lift-list-item.html` (AC: 1)
  - [ ] Show thumbnail when available (from Story 2.4)
  - [ ] Show processing indicator when pipeline is running
  - [ ] Tap row navigates to lift detail page

- [ ] Write unit tests (AC: 1, 2, 3)
  - [ ] Test `HandleGetLift` returns correct page with video URL
  - [ ] Test `BestVideo()` returns cropped > trimmed > original based on file existence
  - [ ] Test `LiftProcessingState()` returns correct stage states
  - [ ] Test `HandleGetLift` with non-existent lift returns 404
  - [ ] Test reprocess endpoint starts new pipeline run
  - [ ] Test video file serving with range request support

- [ ] Write ChromeDP browser verification tests (AC: 1, 2)
  - [ ] Start server on random test port with test database and a test lift with video file
  - [ ] Verify `output.css`, HTMX script, `app.js` load without errors
  - [ ] Verify `<html data-theme="press-out">` attribute is present
  - [ ] Verify no JavaScript console errors on page load
  - [ ] Verify video element is present and has a valid `src` attribute
  - [ ] Verify video player is full-width (no horizontal padding)
  - [ ] Verify back button is present with correct styling
  - [ ] Verify lift type and date are displayed
  - [ ] Verify pipeline progress checklist is displayed for a processing lift
  - [ ] Tear down server and test data after

## Dev Notes

- This story fully implements `HandleGetLift`, which was a stub returning 501 from Story 1.2. It becomes the most important page in the app.
- Best-video-available logic uses file existence checks: `os.Stat()` on each candidate file in priority order. This is fast (filesystem metadata) and aligns with the "no error state in DB" architecture principle.
- Video serving must support HTTP range requests for seeking. Go's `http.ServeFile()` handles this automatically — use it instead of manually reading and writing the file.
- The video URL construction needs care. Options:
  1. Serve via a dedicated handler: `GET /lifts/{id}/video` that calls `http.ServeFile` with the best available video
  2. Serve via file server with the data directory mapped
  Option 1 is cleaner because it handles best-video selection server-side.
- The lift detail page in Epic 2 shows the clean video only (no skeleton toggle yet — that's Epic 3). The video player partial should be structured to support skeleton/clean toggle later but only shows clean video now.
- Pipeline re-trigger: the re-process button should only appear when the pipeline is NOT currently running. Detecting "currently running" is tricky without state in the DB. Options: (a) check if the SSE broker has active publishers for this lift, (b) add a simple in-memory set of "currently processing" lift IDs to the pipeline. Option (b) is simpler.
- The `LiftDetailData` struct will grow over epics. In Epic 2, it has: lift info, video URL, processing state. Epic 3 adds skeleton video. Epic 4 adds metrics and phases. Epic 5 adds coaching.

### Architecture Compliance

- Processing state MUST be derived from file existence — no `status` column in the DB
- Video serving MUST use `http.ServeFile` for proper range request support
- File paths MUST use `storage.LiftFile()` and `storage.BestVideo()`
- The lift detail page follows Direction E layout (UX-DR12): video at top, content below
- No error states in UI — if a stage was skipped, its output simply doesn't exist, and the UI omits it

### Project Structure Notes

New files to create:
- `web/templates/pages/lift-detail.html` — lift detail page (or update if stub exists)

Files to modify:
- `internal/handler/lift.go` — implement HandleGetLift, add video serving handler, add reprocess handler
- `internal/handler/routes.go` — add video serving route, reprocess route
- `internal/storage/storage.go` — add BestVideo() and LiftProcessingState() functions
- `web/templates/partials/lift-list-item.html` — add thumbnail and processing state

### References

- [Source: architecture.md#Error Handling] — processing state derived from file existence, no error state in DB
- [Source: architecture.md#Route Structure] — GET /lifts/{id} for lift detail
- [Source: architecture.md#Video File Organization] — lift directory structure with all artifact filenames
- [Source: epics.md#Story 2.5] — acceptance criteria
- [Source: epics.md#FR28] — video playback starts immediately without waiting for analysis
- [Source: epics.md#NFR3] — video playback begins within 1 second
- [Source: epics.md#NFR11] — failed pipeline re-triggerable without re-upload
- [Source: ux-design-specification.md#Direction E] — UX-DR12 layout: video top, content below
- [Source: ux-design-specification.md#Processing to Result Flow] — SSE-driven transition

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
