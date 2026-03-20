# Story 2.1: Pipeline Orchestrator & SSE Progress

Status: ready-for-dev

## Story

As a lifter,
I want to see real-time progress as the system processes my uploaded video,
so that I know the system is working and can estimate when results will be ready.

## Acceptance Criteria (BDD)

1. **Given** a lift has just been uploaded, **When** the upload completes, **Then** the pipeline orchestrator starts processing automatically in a background goroutine, **And** the orchestrator runs stages sequentially using the Stage interface (Name() + Run(ctx, StageInput) (StageOutput, error)), **And** SSE events are emitted via the in-memory broker as each stage starts and completes

2. **Given** the lifter is on the lift list page with a processing lift, **When** a pipeline stage completes, **Then** the compact pipeline indicator on the list item updates via SSE (current stage name + "N of 5"), **And** the update reaches the browser within 1 second of the stage completing (NFR5)

3. **Given** the lifter taps into a processing lift, **When** the lift detail page loads, **Then** a full vertical stage checklist is displayed with 5 stages (Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching) with three states per stage (pending: dimmed, active: pulsing dot, complete: sage checkmark), **And** the checklist updates in real-time via SSE as stages complete

4. **Given** a pipeline stage returns an error, **When** the orchestrator receives the error, **Then** the error is logged server-side with slog (lift_id, stage, error attributes), **And** the stage is marked as skipped, **And** the pipeline continues with the last successful input passed forward unchanged, **And** no error screen is shown to the lifter (NFR9)

## Tasks / Subtasks

- [ ] Define Stage interface and types in `internal/pipeline/stage.go` (AC: 1, 4)
  - [ ] `Stage` interface with `Name() string` and `Run(ctx context.Context, input StageInput) (StageOutput, error)`
  - [ ] `StageInput` struct: `LiftID int64`, `DataDir string`, `VideoPath string` (path to best available video from prior stages)
  - [ ] `StageOutput` struct: `VideoPath string` (path to video produced, empty if no video), `KeypointsPath string`, `Skipped bool`
  - [ ] Stage names as constants: `StageTrimming`, `StageCropping`, `StageRenderingSkeleton`, `StageComputingMetrics`, `StageGeneratingCoaching`

- [ ] Create SSE broker in `internal/sse/broker.go` (AC: 1, 2, 3)
  - [ ] `Broker` struct with map of lift ID to subscriber channels
  - [ ] `Subscribe(liftID int64) (<-chan Event, func())` — returns event channel and unsubscribe function
  - [ ] `Publish(liftID int64, event Event)` — sends event to all subscribers for that lift
  - [ ] `Event` struct: `Type string` (event name), `Data string` (HTML fragment or JSON)
  - [ ] Thread-safe with `sync.RWMutex`
  - [ ] SSE event names follow kebab-case: `stage-start`, `stage-complete`, `pipeline-done`

- [ ] Create pipeline orchestrator in `internal/pipeline/pipeline.go` (AC: 1, 4)
  - [ ] `Pipeline` struct holds: ordered `[]Stage` slice, `*sse.Broker`, `dataDir string`
  - [ ] `Run(ctx context.Context, liftID int64)` method — runs all stages sequentially
  - [ ] For each stage: publish `stage-start` event, call `stage.Run()`, on success publish `stage-complete`, on error log with slog and mark skipped — pass last successful `StageOutput.VideoPath` forward
  - [ ] After all stages: publish `pipeline-done` event
  - [ ] Initial `StageInput.VideoPath` = `storage.LiftFile(dataDir, liftID, storage.FileOriginal)`
  - [ ] Pipeline.Run is called as a goroutine — must not panic, recover from panics

- [ ] Implement SSE endpoint handler in `internal/handler/sse.go` (AC: 2, 3)
  - [ ] Update existing stub `HandleLiftEvents` in `lift.go` (or create `sse.go`)
  - [ ] Parse lift ID from URL path `r.PathValue("id")`
  - [ ] Subscribe to broker for this lift ID
  - [ ] Set headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
  - [ ] Stream events using `fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)`
  - [ ] Flush after each event via `http.Flusher`
  - [ ] Unsubscribe on client disconnect (context cancellation)

- [ ] Wire pipeline into upload handler (AC: 1)
  - [ ] In `HandleCreateLift` (lift.go), after successful upload, start `go pipeline.Run(s.ctx, lift.ID)` in background using the server-level context (NOT the HTTP request context, which is cancelled when the handler returns)
  - [ ] Add `*pipeline.Pipeline` to `Server` struct in `routes.go`
  - [ ] Wire pipeline creation in `main.go` with empty stages initially (stages added in stories 2.2-2.4)

- [ ] Create pipeline stages template `web/templates/partials/pipeline-stages.html` (AC: 2, 3)
  - [ ] Full variant (detail view): vertical list of 5 stages, each with name and state indicator
  - [ ] Pending state: dimmed text (text-gray-400)
  - [ ] Active state: pulsing sage dot (`animate-pulse` with sage color)
  - [ ] Complete state: sage checkmark icon (UX-DR5)
  - [ ] Compact variant (list view): single line showing current stage + "N of 5"
  - [ ] Both wrapped in `div` with `id` for HTMX SSE swap targets

- [ ] Update `web/templates/partials/lift-list-item.html` for processing state (AC: 2)
  - [ ] Dual-state component: normal (thumbnail + type + date) vs processing (type + date + compact pipeline indicator)
  - [ ] Processing state uses the compact pipeline-stages partial
  - [ ] SSE-driven transition from processing to normal via HTMX `hx-ext="sse"` and `sse-connect`

- [ ] Add HTMX SSE extension to base template (AC: 2, 3)
  - [ ] Include HTMX SSE extension script in `web/templates/layouts/base.html`
  - [ ] SSE connects to `/lifts/{id}/events`
  - [ ] `hx-swap="innerHTML"` targets for stage updates

- [ ] Write unit tests (AC: 1, 4)
  - [ ] `internal/pipeline/pipeline_test.go` — test sequential execution, error handling/skipping, event publishing
  - [ ] `internal/sse/broker_test.go` — test subscribe/publish/unsubscribe, concurrent access
  - [ ] `internal/handler/sse_test.go` — test SSE endpoint streams events correctly

- [ ] Write ChromeDP browser verification tests (AC: 2, 3)
  - [ ] Start server on random test port with test database
  - [ ] Verify `output.css`, HTMX script, `app.js` load without errors
  - [ ] Verify `<html data-theme="press-out">` attribute is present
  - [ ] Verify no JavaScript console errors on page load
  - [ ] Verify pipeline stage checklist renders with correct 5 stage names
  - [ ] Verify stage states (pending/active/complete) render with correct CSS classes
  - [ ] Verify compact pipeline indicator renders on list item for processing lift
  - [ ] Tear down server and test data after

## Dev Notes

- The pipeline orchestrator runs stages sequentially — no parallelism. The 3-minute budget (NFR1) is met because individual stages are fast enough in sequence for <60s videos.
- SSE events carry HTML fragments for direct HTMX swap — not JSON. The broker publishes rendered HTML that HTMX swaps into the correct target element. This keeps the frontend simple (no client-side rendering of SSE data).
- The pipeline publishes events to the SSE broker, but the broker is in-memory only. If the server restarts mid-pipeline, the pipeline state is lost — but the uploaded video is preserved (NFR10) and can be re-triggered (NFR11, handled in Story 2.5). SSE reconnection is out of scope for the MVP.
- The `Pipeline` struct is initialized in `main.go` with an empty stages slice. Stories 2.2-2.4 will register their stages. This means the pipeline can run with zero stages — it just publishes `pipeline-done` immediately.
- Use HTMX SSE extension (`hx-ext="sse"`) for browser-side SSE handling. This extension is included via CDN alongside HTMX itself.
- The Server struct needs a new field: `Pipeline *pipeline.Pipeline`. This is wired in main.go.
- Existing stub handlers `HandleLiftEvents`, `HandleLiftStatus` should be implemented. `HandleLiftCoaching` remains a stub until Epic 5.

### Architecture Compliance

- Pipeline stages MUST implement the `Stage` interface — no ad-hoc processing functions
- SSE event names MUST use kebab-case: `stage-start`, `stage-complete`, `pipeline-done`
- All logging MUST use `slog` with attributes: `lift_id`, `stage`, `duration_ms`, `error`
- File paths MUST use `storage.LiftDir()` and `storage.LiftFile()` — no inline path construction
- Graceful degradation: stages return errors, never panic. Orchestrator catches errors and continues.
- No error state in DB — processing state derived from file existence

### Project Structure Notes

New files to create:
- `internal/pipeline/stage.go` — Stage interface, StageInput, StageOutput, stage name constants
- `internal/pipeline/pipeline.go` — orchestrator
- `internal/pipeline/pipeline_test.go` — orchestrator tests
- `internal/sse/broker.go` — SSE event broker
- `internal/sse/broker_test.go` — broker tests
- `web/templates/partials/pipeline-stages.html` — stage checklist UI

Files to modify:
- `internal/handler/routes.go` — add Pipeline field to Server struct
- `internal/handler/lift.go` — wire pipeline.Run after upload, implement HandleLiftEvents
- `cmd/press-out/main.go` — create and wire Pipeline, SSE Broker
- `web/templates/layouts/base.html` — add HTMX SSE extension CDN
- `web/templates/partials/lift-list-item.html` — add processing state variant

### References

- [Source: architecture.md#Pipeline Stage Interface] — Stage interface definition with code example
- [Source: architecture.md#SSE Implementation] — per-lift event stream with in-memory broker
- [Source: architecture.md#Error Handling] — graceful degradation, no error screens
- [Source: architecture.md#Package Organization] — pipeline and sse package locations
- [Source: epics.md#Story 2.1] — acceptance criteria
- [Source: ux-design-specification.md#Pipeline Stage Checklist] — UX-DR5 three-state stage indicators
- [Source: ux-design-specification.md#Lift List Item] — UX-DR9 dual-state component
- [Source: ux-design-specification.md#Loading and async state patterns] — UX-DR14 SSE/HTMX swaps

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
