---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8]
inputDocuments: ['_bmad-output/planning-artifacts/prd.md', '_bmad-output/planning-artifacts/prd-validation-report.md', '_bmad-output/planning-artifacts/ux-design-specification.md']
workflowType: 'architecture'
project_name: 'press-out'
user_name: 'joao'
date: '2026-03-15'
lastStep: 8
status: 'complete'
completedAt: '2026-03-15'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**

30 FRs across 8 categories define a video-to-coaching pipeline with a simple CRUD layer around it:

- **Video Upload & Storage (FR1-3):** File upload, persistent storage, manual lift type assignment. Straightforward — the entry point to the pipeline.
- **Video Processing (FR4-7):** Auto-trim (motion detection), auto-crop (person-barbell interaction), independent stage skipping, graceful degradation. This is where architectural complexity lives — each stage must be independently bypassable, and failure at any point passes input through unchanged.
- **Pose Estimation & Visualization (FR8-10):** Keypoint detection from video frames (server-side via YOLO26n-Pose Python subprocess), skeleton overlay rendering, dual pre-rendered video output. Keypoints produced by pose pipeline stage; skeleton rendering happens server-side frame-by-frame.
- **Lift Metrics & Phase Analysis (FR11-18):** Six metrics computed from keypoint data (pull-to-catch ratio, bar path, velocity curve, joint angles, phase durations, key position snapshots), LLM-based phase segmentation with timeline markers. Computation layer that transforms raw keypoints into structured analysis.
- **Coaching Intelligence (FR19-21):** LLM-generated diagnosis with causal chain, physical cue referencing specific metrics, lift-type-aware feedback. Async — may complete after the rest of the pipeline.
- **Lift Management (FR22-24):** List view, detail view, deletion with cascading cleanup of all associated files and data. Simple CRUD with file lifecycle management.
- **Video Playback & Review (FR25-28):** Skeleton/clean toggle, speed control, phase timeline navigation, progressive video availability. Client-side video manipulation with server-rendered controls.
- **Processing Feedback (FR29-30):** Real-time SSE progress updates per pipeline stage. Requires server-side event dispatch and client-side SSE consumption via HTMX.

**Non-Functional Requirements:**

11 NFRs across 3 categories set hard constraints on the architecture:

- **Performance (NFR1-5):** 3-minute end-to-end pipeline for <60s 1080p video, 1s page loads, 1s video playback start, <500ms skeleton/clean toggle, 1s SSE delivery latency. The 3-minute pipeline budget is the primary architectural constraint — it determines whether stages run sequentially or need parallelization, and whether video processing can happen in-process or needs worker separation.
- **Integration (NFR6-8):** Graceful handling of missing keypoints (client-side pose failure) and LLM API unavailability, no external infrastructure beyond LLM API and ml5.js CDN. The system must degrade, not fail, when dependencies are down.
- **Reliability (NFR9-11):** No user-facing error screens, video persisted before processing, failed pipelines re-triggerable without re-upload. The "no error screens" constraint means the architecture must treat degraded results as normal results — no error state in the data model.

**Scale & Complexity:**

- Primary domain: Full-stack web with heavy backend processing (video + ML + LLM)
- Complexity level: Medium
- Estimated architectural components: ~8-10 (HTTP server, template renderer, upload handler, pipeline orchestrator, video processor, pose estimator client, metrics computer, LLM client, SSE broadcaster, storage layer)

### Technical Constraints & Dependencies

- **Language/framework:** Go backend, HTMX + Tailwind CSS + DaisyUI frontend, server-rendered
- **Storage:** SQLite for structured data, filesystem for video files and keypoint data
- **External APIs:** LLM API via Claude Code headless (coaching + phase segmentation). Pose estimation runs server-side via YOLO26n-Pose Python subprocess managed by uv — no external API.
- **System dependencies:** FFmpeg (required for video trim, crop, skeleton rendering, and thumbnail extraction — invoked via `exec.Command`), uv + Python 3 (required for YOLO26n-Pose pose estimation — invoked via `exec.CommandContext`)
- **Deployment:** Single binary, no container orchestration, no external infrastructure
- **Browser:** Chrome-only (mobile primary) — enables modern CSS/HTML features without polyfills
- **Build tooling:** No npm/node — Tailwind standalone CLI, no JavaScript build step
- **User model:** Single user, no authentication, no authorization

### Cross-Cutting Concerns Identified

- **Graceful degradation:** Every pipeline stage must fail silently, pass input through unchanged, and produce a result that the UI treats identically to a full-success result. This affects pipeline design, data model (no error states), and UI rendering (no conditional error displays).
- **Pipeline orchestration:** Sequential stage execution with per-stage skip logic, progress event emission, and re-trigger capability. The orchestrator must track stage completion, emit SSE events, handle partial failures, and allow re-running on existing uploads.
- **File lifecycle management:** Each lift produces multiple file artifacts (original video, trimmed video, cropped video, skeleton-overlay video, keypoint data, thumbnails). Deletion must cascade cleanly. Storage paths must be predictable and organized.
- **SSE event architecture:** Multiple UI contexts (list view compact indicator, detail view full checklist, coaching placeholder) consume pipeline events. The SSE layer must support per-lift event streams that multiple clients can subscribe to.
- **Video processing performance budget:** The 3-minute pipeline target requires careful allocation of compute time across 6 server-side stages (Trim → Pose → Crop → Skeleton → Metrics → Coaching). Pose estimation at ~9s for a 12s video fits comfortably within the budget. This may influence whether stages run sequentially or need parallelization, and whether any stages can overlap.

## Starter Template Evaluation

### Primary Technology Domain

Full-stack Go web application with server-rendered HTML (HTMX) and heavy backend video processing. Not a typical SPA or API-first project — the server renders all HTML, manages all state, and orchestrates a compute-heavy processing pipeline.

### Starter Options Considered

**Option A: Manual Scaffold (`go mod init`)**
- Start from scratch with `go mod init`, add dependencies as needed
- Go standard library provides HTTP server, routing (Go 1.22+ enhanced `net/http`), HTML templates, static file serving, SSE support
- Full control over project structure, no generated code to understand or maintain
- Assessment: Best fit — the tech stack is fully specified, and Go's stdlib covers the web layer

**Option B: go-blueprint CLI**
- Scaffolding tool that generates Go web project structure with framework/database choices
- Provides Makefile, Dockerfile, basic middleware, project layout
- Does not support HTMX, Tailwind, or DaisyUI as options — these would be added manually anyway
- Assessment: Marginal value — generates structure and boilerplate that's straightforward to write, and may introduce patterns that don't fit the server-rendered HTMX model

**Option C: templ-based starters**
- Community starters pairing templ (type-safe Go templates) + HTMX + Tailwind
- Replaces Go's `html/template` with compiled, typed templates
- Adds a build step (`templ generate`) and a learning curve
- Assessment: Interesting but unnecessary complexity — Go's `html/template` is well-suited, and the UX spec already defines template partials in `html/template` conventions

### Selected Starter: Manual Scaffold

**Rationale for Selection:**

The tech stack is fully specified (Go stdlib, HTMX, Tailwind CLI, DaisyUI, SQLite), the project has unique architectural needs (video processing pipeline, SSE broadcasting, graceful degradation) that no starter addresses, and Go's standard library covers the entire web layer without frameworks. Adding a scaffolding tool would generate code that needs to be understood, maintained, and potentially removed — negative value for a solo developer who knows the target architecture.

**Initialization Command:**

```bash
mkdir press-out && cd press-out
go mod init press-out
```

**Architectural Decisions Provided by Starter:**

**Language & Runtime:**
- Go (latest stable) with modules enabled
- No CGo dependencies in the web layer (SQLite driver will require CGo)

**Styling Solution:**
- Tailwind CSS via standalone CLI (no npm/node)
- DaisyUI included as Tailwind plugin
- Single compiled CSS output file

**Build Tooling:**
- `go build` for single binary compilation
- Tailwind standalone CLI for CSS compilation
- Makefile for orchestrating build steps (go build + tailwind build)
- No JavaScript build step

**Testing Framework:**
- Go standard `testing` package
- `net/http/httptest` for HTTP handler testing
- ChromeDP (`github.com/chromedp/chromedp`) for headless browser verification tests

**ChromeDP Browser Verification (Required for all stories with UI output):**
- Every story that produces or modifies HTML pages/partials must include ChromeDP tests
- Test setup: start the server on a random test port, run ChromeDP against it, tear down after
- **Asset verification:** Confirm `output.css`, HTMX script, `app.js` all load successfully (no 404/network errors)
- **Theme verification:** Confirm DaisyUI theme is active (`<html data-theme="press-out">` attribute present)
- **Console verification:** Confirm no JavaScript console errors on page load
- **Visual element verification:** Confirm page-specific elements render with correct content, classes, and structure
- ChromeDP tests are co-located with handler tests (e.g., `internal/handler/lift_chromedp_test.go`)

**Code Organization:**
- Flat or shallow package structure following Go conventions
- `cmd/` for application entry point
- `internal/` for private application code
- `web/templates/` for Go HTML templates
- `web/static/` for CSS and static assets
- `data/` for SQLite database and video file storage

**Development Experience:**
- `go run` for development
- Air or similar for hot-reload during development
- SQLite file-based — no database server to manage
- Tailwind CLI watch mode for CSS development

**Note:** Project initialization using this command should be the first implementation story.

## Core Architectural Decisions

### Decision Priority Analysis

**Critical Decisions (Block Implementation):**
- SQLite driver and data access pattern (affects all data operations)
- Video file organization (affects pipeline, storage, deletion)
- SSE implementation pattern (affects real-time updates)
- Error handling strategy (affects every pipeline stage)

**Important Decisions (Shape Architecture):**
- Template organization (affects frontend consistency)
- Route structure (affects all HTTP handlers)
- Configuration management (affects deployment)
- Keypoint data storage (affects metrics computation)

**Deferred Decisions (Post-MVP):**
- Scaling strategy (single user, not needed for MVP)
- Monitoring/alerting (personal tool, logs are sufficient)
- Backup strategy (can be added later)

### Data Architecture

**SQLite Driver: mattn/go-sqlite3**
- Rationale: Battle-tested, full SQLite feature support, the established Go SQLite driver. Requires CGo and a C compiler, both available on the VPS.
- Affects: All data persistence operations

**Data Access: sqlc**
- Rationale: Write SQL queries in `.sql` files, sqlc generates type-safe Go functions. Gets the full power of SQL without ORM abstraction overhead. The domain model is small (lifts, metrics, phases) — SQL files stay manageable.
- Affects: All database interactions, schema management

**Schema Migration: Manual SQL files**
- Rationale: Small schema, solo developer, greenfield project. Numbered migration files (`001_initial.sql`, `002_add_phases.sql`) applied in order at startup. No migration framework needed.
- Affects: Database initialization and evolution

**Video File Organization: Lift-ID directories**
- Structure: `data/lifts/{lift-id}/original.mp4`, `trimmed.mp4`, `cropped.mp4`, `skeleton.mp4`, `thumbnail.jpg`, `keypoints.json`, `crop-params.json`
- Rationale: Self-contained per-lift folders. Deletion is `os.RemoveAll`. Structure is self-documenting. Maps directly to the domain model.
- Affects: Pipeline output, file serving, lift deletion

**Keypoint Data: JSON file per lift**
- Storage: `data/lifts/{lift-id}/keypoints.json`
- Rationale: Write-once, read-many access pattern (written by pose estimation, read by crop, skeleton, and metrics stages). Keeps SQLite lean. Large data (~60K data points per lift) better suited to file storage. Lifecycle managed by lift directory.
- Affects: Pose estimation output, crop stage input (bounding box), skeleton rendering input, metrics computation input

**Crop Parameters: JSON file per lift**
- Storage: `data/lifts/{lift-id}/crop-params.json`
- Contents: `{x, y, w, h, sourceWidth, sourceHeight}` — the crop rectangle applied to the trimmed/original video
- Rationale: Written by crop stage, read by skeleton stage for keypoint coordinate transformation. Small file, simple JSON. Avoids coupling StageOutput to crop-specific fields.
- Affects: Crop stage output, skeleton rendering input (coordinate transformation)

### Authentication & Security

**Authentication: None**
- Rationale: Single-user personal tool per PRD. No auth, no sessions, no user model.

**File Upload Validation:**
- Max upload size: ~300MB via `http.MaxBytesReader` (headroom above 200MB NFR1 target)
- File type validation: Server-side MIME type / extension check for video formats (mp4, mov)
- Rationale: Prevents accidental non-video uploads and protects against oversized files

**No CORS, CSRF, or API Keys:**
- Rationale: Same-origin server-rendered forms. No cross-origin requests. No API consumers. Standard HTML form submissions are inherently safe.

### API & Communication Patterns

**Route Structure:**
```
GET  /                     -> Lift list page
POST /lifts                -> Upload new lift (form submission)
GET  /lifts/{id}           -> Lift detail page
DELETE /lifts/{id}         -> Delete lift
GET  /lifts/{id}/events    -> SSE stream for pipeline progress
```

HTMX partial endpoints (return HTML fragments):
```
GET  /lifts/{id}/coaching  -> Coaching card fragment (SSE target)
GET  /lifts/{id}/status    -> Pipeline status fragment
```

- Rationale: Flat, RESTful resource structure. Clean mapping to Go 1.22's `net/http` route patterns. HTMX partials are separate endpoints that return HTML fragments for swap targets.

**SSE Implementation: Per-lift event stream with in-memory broker**
- Each lift gets its own SSE endpoint (`/lifts/{id}/events`)
- Pipeline goroutine publishes stage events to an in-memory broker (Go channels)
- SSE handler subscribes to the broker and writes events to the HTTP response
- Rationale: Simple, no persistence needed for ephemeral progress events. Go's concurrency primitives (goroutines + channels) make this natural. SSE reconnection handling is deferred to post-MVP.

**Error Handling: Graceful degradation, no error screens**
- Pipeline errors: Logged server-side, stage marked skipped, pipeline continues with last successful input. UI renders degraded results identically to full results.
- HTTP errors: Standard 404/400 for bad requests (these are legitimate user errors, not processing failures)
- External API/subprocess errors: Caught at integration layer, logged, pipeline stage returns skipped result
- Rationale: NFR9 mandates no user-facing error screens during processing. The data model has no error state — a lift either has a result or is still processing.

### Frontend Architecture

**Template Organization:**
```
web/templates/
  layouts/base.html
  pages/lift-list.html, lift-detail.html
  partials/video-player.html, pipeline-stages.html,
           phase-timeline.html, coaching-card.html,
           metric-cell.html, metric-ratio.html,
           metric-barpath.html, metric-velocity.html,
           metric-angles.html, metric-durations.html,
           metric-duration-total.html, lift-list-item.html,
           upload-modal.html
```
- Rationale: 1:1 mapping to UX spec component list. Pages extend base layout. Partials are HTMX-swappable fragments. Go `html/template` `{{template}}` calls for composition.

**Client-Side JavaScript: Single vanilla JS file**
- `web/static/app.js` with event listeners for: video src swap (toggle), playbackRate (speed), currentTime (phase seek), modal triggers (metric expand)
- No modules, no bundling, no build step
- Rationale: Four behaviors, all using native browser APIs. A framework would add complexity for zero value.

**Static Assets:**
- `web/static/output.css` — Tailwind CLI compiled output
- `web/static/app.js` — Vanilla JS
- HTMX and DaisyUI via CDN
- Rationale: No local dependency management for frontend libraries. Tailwind compiled locally because it needs to scan templates.

### Infrastructure & Deployment

**Hosting: VPS (development and production)**
- Same VPS serves as both development environment (coding agents) and production runtime
- Rationale: Simplest possible deployment — rebuild and restart on the same machine

**Deployment: Makefile-driven**
- `make build` — compiles Go binary + Tailwind CSS
- `make test` — runs Go tests
- `make run` — starts the application
- No CI/CD pipeline, no containers, no orchestration
- Rationale: Dev and prod on same machine. Makefile is sufficient for a solo developer with coding agents.

**Process Management: systemd**
- systemd service unit to run press-out, auto-restart on failure, capture logs via journal
- Rationale: Standard Linux process management, zero additional tooling

**Configuration: Environment variables with defaults**
- Optional with defaults: `PORT` (8080), `DATA_DIR` (./data), `DB_PATH` (./data/press-out.db)
- Claude Code manages its own authentication — no LLM API key needed
- Rationale: Minimal config surface. All config has sensible defaults. No cloud API credentials required — pose estimation runs server-side via YOLO26n-Pose.

**Logging: Go slog to stdout**
- Structured JSON logging via Go's `slog` package (stdlib)
- Log pipeline stage timing (NFR1 measurement), external API calls, subprocess execution, errors
- Captured by systemd journal
- Rationale: Zero-dependency structured logging. Sufficient for debugging and performance measurement on a personal tool.

### External Integration Architecture

**YOLO26n-Pose (Server-Side Pose Estimation)**
- YOLO26n-Pose model (7.5MB, auto-downloaded on first run) from ultralytics
- Runs as a Python subprocess via `exec.CommandContext(ctx, "uv", "run", "scripts/pose.py", videoPath, "-o", keypointsPath)`
- Processes video frame-by-frame at ~39 fps on CPU, extracts 17 COCO keypoints per frame
- Keypoints normalized (0-1), exported as keypoints.json in the lift directory
- Python dependency management via uv (pyproject.toml + uv.lock at project root)
- Error handling: if pose estimation fails, keypoints.json is not written; downstream stages handle missing file gracefully
- Affects: FR8 (keypoint detection)

**Claude Code Headless: Subprocess runner**
- Invokes Claude Code in headless mode as a subprocess on the VPS
- Constructs prompts with lift type, keypoint data, computed metrics
- Parses structured response for coaching cue + diagnosis and phase segmentation
- Error handling: Subprocess exit codes, stderr, execution timeout → skip coaching/phase segmentation stages gracefully
- Claude Code manages its own authentication
- Affects: FR17 (phase segmentation), FR19-21 (coaching intelligence)

### Decision Impact Analysis

**Implementation Sequence:**
1. Project init (`go mod init`, Makefile, directory structure)
2. SQLite + sqlc setup (schema, queries, generated code)
3. File storage layer (lift directory management)
4. HTTP server + routing + templates
5. Upload handler + video storage
6. Pipeline orchestrator + SSE broker
7. Individual pipeline stages (trim, crop, pose, skeleton, metrics, coaching)
8. Frontend templates + JS

**Cross-Component Dependencies:**
- sqlc generates code from SQL — schema changes require regeneration
- Pipeline orchestrator depends on SSE broker and file storage layer
- Templates depend on route structure (HTMX endpoints) and data models (sqlc types)
- Makefile orchestrates Go build + Tailwind build + sqlc generate
- Claude Code subprocess runner depends on Claude Code being installed and authenticated on the VPS
- Pose stage produces keypoints.json consumed by crop, skeleton, and metrics stages

## Implementation Patterns & Consistency Rules

### Pattern Categories Defined

**Critical Conflict Points Identified:** 8 areas where AI agents could make different choices — naming, SQL conventions, template conventions, SSE events, pipeline stage interface, error handling, file paths, and logging.

### Naming Patterns

**Go Code Naming:**
- Follow standard Go conventions: `PascalCase` for exported identifiers, `camelCase` for unexported
- Package names: lowercase, single word (`pipeline`, `storage`, `handler`, `sse`, `pose`, `claude`)
- Receiver names: short, consistent per type (e.g., `s` for `Server`, `p` for `Pipeline`, `l` for `Lift`)
- No `Get`/`Set` prefixes — `lift.Type()` not `lift.GetType()`
- Interfaces named by behavior: `Stage`, `Broker`, `Store` — not `IStage` or `StageInterface`

**Database Naming (SQL / sqlc):**
- Tables: `snake_case`, plural (`lifts`, `phases`, `metrics`)
- Columns: `snake_case` (`lift_type`, `created_at`, `bar_path_data`)
- Primary key: `id` (integer, autoincrement)
- Foreign keys: `{table_singular}_id` (e.g., `lift_id`)
- Timestamps: `created_at`, `updated_at` (stored as RFC3339 strings)
- sqlc generates PascalCase Go types: `Lift`, `Phase`, `Metric`

**Template Naming:**
- File names: `kebab-case.html` (`video-player.html`, `lift-list-item.html`)
- Template block names: match file name without extension (`video-player`, `coaching-card`)
- Template data structs: `{Page}Data` (`LiftDetailData`, `LiftListData`)

**SSE Event Naming:**
- Event names: `kebab-case` (`stage-complete`, `coaching-ready`, `pipeline-done`)
- Data payloads: HTML fragments for HTMX swap, JSON when structured data is needed

### Structure Patterns

**Package Organization:**
```
cmd/press-out/main.go              -- entry point, wiring
internal/handler/lift.go           -- HTTP handlers for lift CRUD
internal/handler/sse.go            -- SSE endpoint handler
internal/pipeline/pipeline.go      -- orchestrator
internal/pipeline/stages/trim.go   -- trim stage
internal/pipeline/stages/pose.go   -- pose estimation stage (YOLO26n-Pose via uv subprocess)
internal/pipeline/stages/crop.go   -- crop stage
internal/pipeline/stages/skeleton.go -- skeleton rendering stage
internal/pipeline/stages/metrics.go  -- metrics computation stage
internal/pipeline/stages/coaching.go -- coaching stage (Claude Code)
internal/storage/storage.go        -- file storage operations
internal/storage/db.go             -- database operations
internal/sse/broker.go             -- SSE event broker
internal/ffmpeg/ffmpeg.go          -- FFmpeg/ffprobe subprocess helper
internal/pose/pose.go              -- pose.Result types and keypoints.json serialization (used by pose stage and downstream stages)
internal/claude/runner.go          -- Claude Code subprocess runner
```

**Test Location:** Co-located with source files (`pipeline_test.go` next to `pipeline.go`). Table-driven tests. Test fixtures in `testdata/` directories within each package.

**SQL Files:**
```
sql/schema/001_initial.sql         -- initial schema migration
sql/queries/lifts.sql              -- lift queries
sql/queries/metrics.sql            -- metrics queries
sql/queries/phases.sql             -- phase queries
sqlc.yaml                          -- sqlc configuration
```

### Process Patterns

**Pipeline Stage Interface:**
```go
type Stage interface {
    Name() string
    Run(ctx context.Context, input StageInput) (StageOutput, error)
}
```
- `StageInput`: current lift ID, paths to all files produced by prior stages
- `StageOutput`: paths/data produced by this stage
- On error: orchestrator logs, marks stage skipped, passes previous input forward unchanged
- Stages never call other stages — orchestrator controls sequencing

**Graceful Degradation:**
- Stages return `(result, error)` — never panic, never `log.Fatal`
- Orchestrator catches errors and continues with last successful input
- No error state in the database — no `error_message` columns, no `status = "failed"` enums
- Processing state is derived: all outputs exist = complete, some missing = degraded, none + no active goroutine = re-triggerable

**File Path Construction:**
- All paths built through `storage.LiftDir(liftID)` returning `data/lifts/{id}/`
- File names are package-level constants: `FileOriginal = "original.mp4"`, `FileTrimmed = "trimmed.mp4"`, etc.
- Never construct paths via string concatenation outside the storage package

**Logging Convention:**
- `slog` with consistent attribute keys: `lift_id`, `stage`, `duration_ms`, `error`
- Levels: `Info` for stage start/complete, `Warn` for skipped stages, `Error` for unexpected failures
- Example: `slog.Info("stage complete", "lift_id", id, "stage", "trim", "duration_ms", 342)`

### Enforcement Guidelines

**All AI Agents MUST:**
- Implement pipeline stages using the `Stage` interface — no ad-hoc processing functions
- Use the storage package for all file path construction — no inline path building
- Use sqlc-generated types and queries — no raw `database/sql` calls outside the storage package
- Follow the naming conventions above — check existing code for precedent before introducing new names
- Return errors from stages, never panic or fatal — the orchestrator handles all error recovery
- Log with `slog` using the standard attribute keys — no `fmt.Println` or `log.Printf`
- Include ChromeDP browser verification tests for any story that produces or modifies UI pages — verifying asset loading, DaisyUI theme, no console errors, and page-specific visual elements

### Pattern Examples

**Good:**
```go
// Stage follows interface, returns error for orchestrator to handle
func (t *TrimStage) Run(ctx context.Context, input StageInput) (StageOutput, error) {
    slog.Info("stage starting", "lift_id", input.LiftID, "stage", t.Name())
    // ... processing ...
    if err != nil {
        return StageOutput{}, fmt.Errorf("trim: %w", err)
    }
    slog.Info("stage complete", "lift_id", input.LiftID, "stage", t.Name(), "duration_ms", elapsed)
    return StageOutput{VideoPath: storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimmed)}, nil
}
```

**Anti-Patterns:**
```go
// BAD: panics on error instead of returning
func (t *TrimStage) Run(ctx context.Context, input StageInput) (StageOutput, error) {
    if err != nil {
        log.Fatalf("trim failed: %v", err)  // NEVER - kills the whole server
    }
}

// BAD: constructs file paths inline
path := fmt.Sprintf("data/lifts/%d/trimmed.mp4", liftID)  // NEVER - use storage package

// BAD: stores error state in database
db.Exec("UPDATE lifts SET status = 'failed', error = ? WHERE id = ?", err.Error(), id)  // NEVER - no error state
```

## Project Structure & Boundaries

### Complete Project Directory Structure

```
press-out/
├── README.md
├── Makefile
├── go.mod
├── go.sum
├── sqlc.yaml
├── pyproject.toml                         -- Python project config (ultralytics, opencv-python)
├── uv.lock                               -- uv lockfile for reproducible Python deps
├── tailwind.config.js
├── .env.example
├── .gitignore
│
├── cmd/
│   └── press-out/
│       └── main.go                    -- entry point, wiring, server startup
│
├── internal/
│   ├── config/
│   │   └── config.go                  -- env var loading, defaults
│   │
│   ├── handler/
│   │   ├── lift.go                    -- GET /, POST /lifts, GET /lifts/{id}, DELETE /lifts/{id}
│   │   ├── lift_test.go
│   │   ├── sse.go                     -- GET /lifts/{id}/events, SSE streaming
│   │   ├── sse_test.go
│   │   └── routes.go                  -- route registration on mux
│   │
│   ├── pipeline/
│   │   ├── pipeline.go                -- orchestrator: runs stages, emits SSE events
│   │   ├── pipeline_test.go
│   │   ├── stage.go                   -- Stage interface, StageInput, StageOutput types
│   │   └── stages/
│   │       ├── trim.go                -- auto-trim via motion detection
│   │       ├── trim_test.go
│   │       ├── pose.go                -- pose estimation stage (YOLO26n-Pose)
│   │       ├── pose_test.go
│   │       ├── crop.go                -- auto-crop via person-barbell interaction
│   │       ├── crop_test.go
│   │       ├── skeleton.go            -- skeleton overlay rendering
│   │       ├── skeleton_test.go
│   │       ├── metrics.go             -- six metrics computation from keypoints
│   │       ├── metrics_test.go
│   │       ├── coaching.go            -- coaching + phase segmentation (calls Claude Code)
│   │       └── coaching_test.go
│   │
│   ├── storage/
│   │   ├── storage.go                 -- LiftDir(), LiftFile(), file constants, directory ops
│   │   ├── storage_test.go
│   │   ├── db.go                      -- SQLite connection, migration runner
│   │   └── db_test.go
│   │
│   ├── sse/
│   │   ├── broker.go                  -- in-memory event broker (channels)
│   │   └── broker_test.go
│   │
│   ├── pose/
│   │   └── pose.go                    -- Result/Frame/Keypoint types, keypoints.json serialization
│   │
│   └── claude/
│       ├── runner.go                  -- Claude Code headless subprocess runner
│       └── runner_test.go
│
├── sql/
│   ├── schema/
│   │   └── 001_initial.sql            -- lifts, metrics, phases tables
│   └── queries/
│       ├── lifts.sql                  -- CRUD queries for lifts
│       ├── metrics.sql                -- insert/select metrics per lift
│       └── phases.sql                 -- insert/select phases per lift
│
├── scripts/
│   └── pose.py                           -- YOLO26n-Pose inference script
│
├── web/
│   ├── templates/
│   │   ├── layouts/
│   │   │   └── base.html              -- HTML shell: head, body, CDN links, scripts
│   │   ├── pages/
│   │   │   ├── lift-list.html         -- lift list page (extends base)
│   │   │   └── lift-detail.html       -- lift detail page (extends base)
│   │   └── partials/
│   │       ├── video-player.html      -- video element + floating controls + toggle
│   │       ├── pipeline-stages.html   -- stage checklist (compact + full variants)
│   │       ├── phase-timeline.html    -- segmented bar, tap to seek
│   │       ├── coaching-card.html     -- cue + diagnosis, SSE placeholder
│   │       ├── metric-cell.html       -- metric dispatcher partial
│   │       ├── metric-ratio.html      -- pull-to-catch ratio cell
│   │       ├── metric-barpath.html    -- bar path cell
│   │       ├── metric-velocity.html   -- velocity curve cell
│   │       ├── metric-angles.html     -- joint angles cell
│   │       ├── metric-durations.html  -- phase durations cell
│   │       ├── metric-duration-total.html -- total lift duration cell
│   │       ├── lift-list-item.html    -- list row (normal + processing states)
│   │       └── upload-modal.html      -- upload form modal
│   └── static/
│       ├── app.js                     -- vanilla JS: toggle, speed, seek, modal
│       ├── input.css                  -- Tailwind directives (@tailwind base, etc.)
│       └── output.css                 -- Tailwind CLI compiled output (gitignored)
│
├── data/                              -- runtime data directory (gitignored)
│   ├── press-out.db                   -- SQLite database
│   └── lifts/
│       └── {lift-id}/
│           ├── original.mp4
│           ├── trimmed.mp4
│           ├── cropped.mp4
│           ├── skeleton.mp4
│           ├── thumbnail.jpg
│           ├── keypoints.json
│           └── crop-params.json
│
└── testdata/                          -- shared test fixtures
    └── videos/
        └── sample-snatch.mp4          -- reference video for integration tests
```

### Architectural Boundaries

**HTTP Boundary (handler package):**
- Handlers receive HTTP requests, call storage/pipeline, return rendered templates or SSE streams
- Handlers never access SQLite directly — always through storage package (sqlc-generated code)
- Handlers never construct file paths — always through storage package functions
- One-way dependency: handler -> storage, handler -> pipeline, handler -> sse

**Pipeline Boundary (pipeline package):**
- Orchestrator runs stages sequentially, emits events via SSE broker
- Stages receive `StageInput`, return `StageOutput` — no access to HTTP layer, no direct DB access
- Stages call external integrations (pose, claude) through injected clients
- One-way dependency: pipeline -> stages, pipeline -> sse, stages -> pose/claude, stages -> storage (file paths only)

**Storage Boundary (storage package):**
- Sole owner of SQLite access (via sqlc-generated code) and file path construction
- Exposes typed functions: `CreateLift()`, `GetLift()`, `DeleteLift()`, `LiftDir()`, `LiftFile()`
- No knowledge of HTTP, pipeline, or SSE concerns
- Independent: storage depends on nothing internal

**SSE Boundary (sse package):**
- In-memory broker with publish/subscribe via Go channels
- Pipeline publishes events, HTTP handler subscribes and streams to client
- No knowledge of what events mean — just routing messages by lift ID
- Independent: sse depends on nothing internal

**External Integration Boundary (pose, claude packages):**
- Each external integration is isolated in its own package
- Exposes a single client type with methods matching what stages need
- Handles its own error wrapping, timeout, and retry logic
- No knowledge of pipeline orchestration — called by stages, returns results

### Requirements to Structure Mapping

**Video Upload & Storage (FR1-3):**
- `internal/handler/lift.go` — upload endpoint, lift type assignment
- `internal/storage/` — file persistence, SQLite CRUD
- `sql/queries/lifts.sql` — lift queries
- `web/templates/partials/upload-modal.html` — upload UI

**Video Processing (FR4-8):**
- `internal/pipeline/pipeline.go` — orchestrator with skip logic
- `internal/pipeline/stages/trim.go` — FR4 (auto-trim)
- `internal/pipeline/stages/pose.go` — FR8 (YOLO26n-Pose keypoint detection via Python subprocess, runs before crop)
- `internal/pipeline/stages/crop.go` — FR5 (auto-crop using keypoint bounding box)
- `internal/pose/` — pose.Result types and keypoints.json serialization
- `internal/pipeline/stage.go` — FR7 (independent stage interface)

**Skeleton Visualization (FR9-10):**
- `internal/pipeline/stages/skeleton.go` — FR9-10 (skeleton rendering on cropped video, with coordinate transformation via crop-params.json)

**Lift Metrics & Phase Analysis (FR11-18):**
- `internal/pipeline/stages/metrics.go` — FR11-16 (six metrics)
- `internal/pipeline/stages/coaching.go` — FR17-18 (phase segmentation via Claude Code)
- `sql/queries/metrics.sql`, `sql/queries/phases.sql` — persistence

**Coaching Intelligence (FR19-21):**
- `internal/pipeline/stages/coaching.go` — FR19-21 (coaching generation)
- `internal/claude/runner.go` — Claude Code subprocess execution

**Lift Management (FR22-24):**
- `internal/handler/lift.go` — FR22-24 (list, detail, delete)
- `web/templates/pages/lift-list.html`, `lift-detail.html` — UI
- `internal/storage/storage.go` — cascading file deletion

**Video Playback & Review (FR25-28):**
- `web/templates/partials/video-player.html` — FR25-27 (toggle, speed, phase nav)
- `web/static/app.js` — FR25-26 (client-side video control)
- `web/templates/partials/phase-timeline.html` — FR27-28 (phase navigation)

**Processing Feedback (FR29-30):**
- `internal/handler/sse.go` — SSE endpoint
- `internal/sse/broker.go` — event routing
- `web/templates/partials/pipeline-stages.html` — progress UI

### Integration Points

**Internal Communication:**
- `handler` -> `storage`: typed function calls for data access
- `handler` -> `pipeline`: starts pipeline goroutine for a lift
- `pipeline` -> `sse.Broker`: publishes stage events
- `handler/sse` -> `sse.Broker`: subscribes to events, streams to HTTP response
- `pipeline/stages` -> `pose`, `claude`: external service calls

**External Integrations:**
- **YOLO26n-Pose** (`scripts/pose.py`): Python subprocess via uv, produces keypoints.json in lift directory
- **Claude Code** (`internal/claude/runner.go`): Subprocess execution with structured prompt, parses stdout response

**Data Flow:**
```
Upload (HTTP) -> storage.CreateLift() -> SQLite row + original.mp4
  -> pipeline.Run() [goroutine]
    -> trim.Run()     -> trimmed.mp4 (or skip)
    -> pose.Run()     -> keypoints.json (YOLO26n-Pose via uv subprocess)
    -> crop.Run()     -> cropped.mp4 + crop-params.json + thumbnail.jpg (uses keypoints for bounding box)
    -> skeleton.Run() -> skeleton.mp4 (transforms keypoints to cropped frame via crop-params.json)
    -> metrics.Run()  -> SQLite metrics rows
    -> coaching.Run() -> SQLite phases + coaching rows (via Claude Code)
  -> SSE events emitted at each stage transition
```

### Development Workflow Integration

**Build Process:**
```makefile
build: sqlc-generate tailwind-build go-build
sqlc-generate:    sqlc generate
tailwind-build:   tailwindcss -i web/static/input.css -o web/static/output.css --minify
go-build:         go build -o press-out ./cmd/press-out
test:             go test ./...
run:              go run ./cmd/press-out
dev:              air  # hot-reload for development
```

**Deployment:** `make build && systemctl restart press-out`

## Architecture Validation Results

### Coherence Validation

**Decision Compatibility:** All decisions are compatible. Go stdlib `net/http` (1.22+) + HTMX + Tailwind/DaisyUI is an established pattern. mattn/go-sqlite3 + sqlc work natively together. SSE via Go stdlib pairs with HTMX's SSE extension. YOLO26n-Pose (Python subprocess via uv) and Claude Code (subprocess) integrate independently without interference. FFmpeg invoked via `exec.Command` from pipeline stages. No conflicting decisions found.

**Pattern Consistency:** Naming conventions are unambiguous across layers (Go conventions for code, snake_case for SQL, kebab-case for templates/SSE). The Stage interface provides a single uniform pattern for all pipeline processing. The storage package as sole owner of paths and DB access prevents cross-agent inconsistency.

**Structure Alignment:** One-way dependency flow with no circular dependencies. handler -> pipeline -> stages -> integrations, handler -> storage, pipeline -> sse. All boundaries are respected and enforceable through Go's package visibility rules.

### Requirements Coverage Validation

**Functional Requirements:** 30/30 covered. Every FR maps to specific files in the project structure. No architectural gaps.

| FR Range | Category | Architectural Support | Status |
|---|---|---|---|
| FR1-3 | Upload & Storage | handler/lift.go, storage/, lifts table | Covered |
| FR4-7 | Video Processing | pipeline orchestrator, stages/trim, stages/crop, Stage interface | Covered |
| FR8-10 | Pose & Visualization | stages/pose (YOLO26n-Pose), stages/skeleton, pose/pose.go | Covered |
| FR11-18 | Metrics & Phases | stages/metrics, stages/coaching, metrics + phases tables | Covered |
| FR19-21 | Coaching | stages/coaching, claude/runner | Covered |
| FR22-24 | Lift Management | handler/lift CRUD, templates, cascading delete | Covered |
| FR25-28 | Playback & Review | video-player.html, phase-timeline.html, app.js | Covered |
| FR29-30 | Processing Feedback | handler/sse, sse/broker, pipeline-stages.html | Covered |

**Non-Functional Requirements:** 11/11 covered.

| NFR | Requirement | Architectural Support | Status |
|---|---|---|---|
| NFR1 | 3-min pipeline | Pipeline orchestrator with slog timing | Covered |
| NFR2 | 1s page loads | Server-rendered Go templates, single user | Covered |
| NFR3 | 1s video start | Pre-rendered videos served as static files | Covered |
| NFR4 | <500ms toggle | Dual pre-rendered videos, JS src swap | Covered |
| NFR5 | 1s SSE delivery | In-memory broker, Go channels | Covered |
| NFR6 | Missing keypoints handling | Upload handler + downstream stage skip | Covered |
| NFR7 | LLM failure | claude/runner error handling + stage skip | Covered |
| NFR8 | No external infra | SQLite + filesystem, single binary | Covered |
| NFR9 | No error screens | Graceful degradation, no error state in DB | Covered |
| NFR10 | Persist before processing | Upload saves first, then starts pipeline | Covered |
| NFR11 | Re-triggerable pipeline | Derived state from file existence | Covered |

### Implementation Readiness Validation

**Decision Completeness:** All critical decisions documented with specific technologies. Implementation patterns include code examples and anti-patterns. Consistency rules are explicit and enforceable.

**Structure Completeness:** Complete project tree with every file and directory defined. All 30 FRs mapped to specific source files. Integration points specified with data flow diagram.

**Pattern Completeness:** All 8 identified conflict points addressed with concrete conventions. Pipeline Stage interface defined with code example. Enforcement guidelines listed for AI agents.

### Gap Analysis Results

**Critical Gaps:** 0

**Important Gaps Addressed:**
1. **FFmpeg system dependency:** Added to Technical Constraints. Required for video trim, crop, skeleton rendering, and thumbnail extraction. Invoked via `exec.Command` from pipeline stages.
2. **Thumbnail generation:** Added to pipeline data flow. Thumbnail extracted from processed video (after crop/trim) via FFmpeg. Stored as `thumbnail.jpg` in lift directory.
3. **Tailwind input.css:** Added to project structure under `web/static/input.css`.

**Deferred to Implementation:**
- Claude Code prompt structure for coaching/phase segmentation stages
- Detailed FFmpeg command patterns for each pipeline stage

### Architecture Completeness Checklist

**Requirements Analysis**
- [x] Project context thoroughly analyzed
- [x] Scale and complexity assessed
- [x] Technical constraints identified (including FFmpeg)
- [x] Cross-cutting concerns mapped

**Architectural Decisions**
- [x] Critical decisions documented with specific technologies
- [x] Technology stack fully specified
- [x] Integration patterns defined (Python subprocess via uv for pose estimation, subprocess for Claude Code, exec.Command for FFmpeg)
- [x] Performance considerations addressed

**Implementation Patterns**
- [x] Naming conventions established (Go, SQL, templates, SSE)
- [x] Structure patterns defined (package organization, test location)
- [x] Communication patterns specified (SSE events, HTMX partials)
- [x] Process patterns documented (Stage interface, graceful degradation, logging)

**Project Structure**
- [x] Complete directory structure defined
- [x] Component boundaries established
- [x] Integration points mapped
- [x] Requirements to structure mapping complete

### Architecture Readiness Assessment

**Overall Status:** READY FOR IMPLEMENTATION

**Confidence Level:** High — all requirements covered, no coherence issues, clean boundaries, comprehensive patterns for agent consistency.

**Key Strengths:**
- Pipeline Stage interface guarantees consistent implementation across all processing stages
- Graceful degradation is architecturally enforced (no error state in DB, derived processing state)
- Clean one-way dependency flow prevents circular dependencies
- Storage package as single source of truth for paths and data access
- Every FR mapped to specific files — no ambiguity for implementing agents

**Areas for Future Enhancement:**
- Claude Code prompt templates (deferred to implementation)
- Post-MVP: scaling considerations if processing load increases
- Post-MVP: backup strategy for SQLite and lift data

### Implementation Handoff

**System Dependencies:**
- Go (latest stable)
- FFmpeg (system package — used by trim, crop, skeleton, and thumbnail stages via `exec.Command`)
- uv (Python package manager — used to run YOLO26n-Pose pose estimation)
- Python 3 (required by uv for YOLO pose estimation subprocess)
- Tailwind CSS standalone CLI
- sqlc CLI
- Claude Code CLI (installed and authenticated on VPS)
- C compiler (for mattn/go-sqlite3 CGo)

**AI Agent Guidelines:**
- Follow all architectural decisions exactly as documented
- Use implementation patterns consistently across all components
- Respect project structure and package boundaries
- Implement pipeline stages using the Stage interface — no exceptions
- Use the storage package for all file path construction and data access
- Refer to this document for all architectural questions

**First Implementation Priority:**
```bash
mkdir press-out && cd press-out
go mod init press-out
```
Then: directory structure, Makefile, sqlc config, initial schema, and base template layout.
