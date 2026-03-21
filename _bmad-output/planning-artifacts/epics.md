---
stepsCompleted: ['step-01-validate-prerequisites', 'step-02-design-epics', 'step-03-create-stories', 'step-04-final-validation']
inputDocuments: ['_bmad-output/planning-artifacts/prd.md', '_bmad-output/planning-artifacts/architecture.md', '_bmad-output/planning-artifacts/ux-design-specification.md']
---

# press-out - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for press-out, decomposing the requirements from the PRD, UX Design, and Architecture into implementable stories.

## Requirements Inventory

### Functional Requirements

- FR1: Lifter can upload a video file from their mobile device
- FR2: System can store uploaded videos persistently for later retrieval
- FR3: Lifter can assign a lift type (Snatch, Clean, or Clean & Jerk) to each upload
- FR4: System can automatically detect and trim a video to the lift portion, removing setup and post-lift footage
- FR5: System can automatically identify and crop to the lifter performing the lift when multiple people are in frame
- FR6: System preserves the full unprocessed video when auto-trim or auto-crop confidence falls below threshold
- FR7: System can process each pipeline stage independently, allowing any stage to be skipped without blocking downstream stages
- FR8: System can detect body keypoints (joints, limbs) from video frames of a lifter
- FR9: System can render a skeleton overlay onto the lift video as a separate viewable version
- FR10: System can produce both a clean video and a skeleton-overlay video for each upload
- FR11: System can compute pull-to-catch ratio from keypoint data
- FR12: System can generate a bar path visualization from keypoint data
- FR13: System can compute a velocity curve for the barbell movement
- FR14: System can measure joint angles at key positions during the lift
- FR15: System can calculate phase durations for each segment of the lift
- FR16: System can capture key position snapshots at phase transition points (first pull start, second pull start, catch, recovery)
- FR17: System can segment a lift into its constituent phases (setup, first pull, transition, second pull, catch, recovery) based on lift type
- FR18: System can display phase markers on a timeline aligned with video playback
- FR19: System can generate a coaching diagnosis that includes at least one identified issue, one causal factor, and references specific metric values from the lift
- FR20: System can produce a physical cue that describes a specific body movement or position change the lifter can attempt on the next rep
- FR21: Coaching feedback can incorporate lift type, keypoint data, phase segmentation, and computed metrics
- FR22: Lifter can view a list of all uploaded lifts with identifying information (date, lift type)
- FR23: Lifter can view the full detail of any individual lift including videos, metrics, phases, and coaching feedback
- FR24: Lifter can delete a lift and all its associated data
- FR25: Lifter can toggle between clean video and skeleton overlay video during playback
- FR26: Lifter can control playback speed (slow-motion and speed up)
- FR27: Lifter can navigate to any lift phase directly from the phase timeline
- FR28: Video playback can start immediately without waiting for all analysis to complete
- FR29: System can provide real-time progress updates to the lifter as each pipeline stage completes
- FR30: Lifter can see which processing stage is currently active during video analysis

### NonFunctional Requirements

- NFR1: End-to-end pipeline (upload through coaching feedback) completes within 3 minutes for videos under 60 seconds at 1080p resolution (under 200MB), as measured by server-side pipeline timing logs
- NFR2: All server-rendered pages load within 1 second under single-user load, as measured by server-side request timing logs
- NFR3: Video playback begins within 1 second of user interaction, as measured by browser performance timing API
- NFR4: Toggle between clean and skeleton overlay video switches playback within 500 milliseconds, as measured by browser performance timing API
- NFR5: Pipeline stage progress updates reach the browser within 1 second of each stage completing, as measured by server-side SSE dispatch timestamps
- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped), continuing the pipeline without crashing
- NFR7: System handles LLM API unavailability gracefully, completing video processing without coaching feedback or phase segmentation
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code). Pose estimation runs locally via YOLO26n-Pose.
- NFR9: No user-facing error screens during video processing — all failures degrade to the best available result
- NFR10: Uploaded videos are persisted before processing begins, ensuring no data loss if processing fails
- NFR11: A failed pipeline run can be re-triggered on a previously uploaded video without re-uploading

### Additional Requirements

- Starter: Manual scaffold via `go mod init` — no starter template, no scaffolding tool
- SQLite driver: mattn/go-sqlite3 (requires CGo and C compiler)
- Data access: sqlc — write SQL queries in .sql files, generates type-safe Go functions
- Schema migration: Manual numbered SQL files (001_initial.sql, etc.) applied at startup
- Video file organization: Lift-ID directories (`data/lifts/{lift-id}/original.mp4`, `trimmed.mp4`, `cropped.mp4`, `skeleton.mp4`, `thumbnail.jpg`, `keypoints.json`)
- Keypoint data: JSON file per lift (write-once, read-once pattern)
- No authentication: Single-user personal tool, no auth, no sessions, no user model
- File upload validation: ~300MB max via `http.MaxBytesReader`, server-side MIME type/extension check for mp4/mov
- Route structure: 5 main routes (GET /, POST /lifts, GET /lifts/{id}, DELETE /lifts/{id}, GET /lifts/{id}/events) + 2 HTMX partial endpoints (GET /lifts/{id}/coaching, GET /lifts/{id}/status)
- SSE implementation: Per-lift event stream with in-memory broker (Go channels). SSE reconnection handling deferred to post-MVP
- Graceful degradation: Pipeline errors logged server-side, stage marked skipped, pipeline continues with last successful input; no error state in data model
- FFmpeg system dependency: Required for video trim, crop, skeleton rendering, and thumbnail extraction via `exec.Command`
- Pipeline Stage interface: All stages implement `Stage` interface with `Name()` and `Run(ctx, StageInput) (StageOutput, error)`
- Package organization: `cmd/press-out/`, `internal/` (handler, pipeline, storage, sse, pose, claude), `sql/`, `web/`
- Configuration: Environment variables with defaults (PORT=8080, DATA_DIR=./data, DB_PATH=./data/press-out.db)
- Deployment: Makefile-driven (build, test, run), systemd process management, single VPS
- Logging: Go slog to stdout, structured JSON, captured by systemd journal
- Build tooling: `go build` + Tailwind standalone CLI + sqlc generate, no npm/node, no JavaScript build step
- External integrations: YOLO26n-Pose via Python subprocess managed by uv (server-side pose estimation), Claude Code via headless subprocess runner
- Thumbnail generation: Extracted from processed video via FFmpeg, stored as `thumbnail.jpg` in lift directory

### UX Design Requirements

- UX-DR1: Custom DaisyUI theme with specified color system — warm white base (#FAFAF8), dark charcoal text (#2D2D2D), muted sage primary (#8BA888), soft stone secondary (#C4BFAE), neutral (#EDEDEA), info/coaching (#9BB0BA), success (#7DA67D). No reds, no warnings, no error colors.
- UX-DR2: System font stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif`) — no web fonts, no loading delay
- UX-DR3: Typography scale — metric values text-2xl bold (largest on screen), page title text-xl semibold, section headers text-lg medium, body text-sm normal, metric labels text-xs medium, UI chrome text-xs medium
- UX-DR4: Video Player with Floating Controls — full-width edge-to-edge video element, position relative container, floating speed strip (0.25x/0.5x/1x) at bottom with gradient backdrop (`bg-gradient-to-t from-black/40`), mode badge in bottom-right ("Skeleton"/"Clean"), entire video surface as tap target for toggle, sticky positioning while scrolling
- UX-DR5: Pipeline Stage Checklist — vertical list of 6 stages (Pose estimation, Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching) with three states per stage (pending: dimmed, active: pulsing dot, complete: sage checkmark). Two variants: compact (list item: current stage + "N of 6") and full (detail view: all 6 stages visible).
- UX-DR6: Phase Timeline Bar — horizontal segmented bar, full-width, rounded corners, segments proportional to phase duration, sage gradient palette colors. Tap segment to seek video to phase start, label appears above selected segment (e.g., "2nd Pull — 0.31s"), selected segment highlighted at full opacity with slight scale, others dimmed
- UX-DR7: Metric Cells in 3x2 grid (grid-cols-3) — six metric-specific variants: pull-to-catch ratio (vertical ratio bar + numeric value, no expand), bar path (mini X-Y trajectory, tap-to-expand with axis labels and phase markers), velocity curve (mini sparkline + peak value, tap-to-expand with phase shading), joint angles (mini stick figure at catch + angle values, tap-to-expand all four positions), phase durations (mini stacked bar + total duration, tap-to-expand with individual ms values), total lift duration (large numeric value, no expand)
- UX-DR8: Coaching Card — no card container, coaching cue in `text-lg font-semibold` as hero text (first content below video), divider, diagnosis in `text-sm text-gray-500`, left border accent in info color (#9BB0BA). DaisyUI skeleton placeholder while loading, SSE swap when ready
- UX-DR9: Lift List Item — dual-state component. Normal: thumbnail (left) + lift type + date. Processing: lift type + date + compact pipeline indicator. SSE-driven transition from processing to normal state (thumbnail appears, indicator removed)
- UX-DR10: Upload Modal — DaisyUI modal shell with dark backdrop, file selector zone (native file picker), 3-option `join` group for lift type (Snatch/Clean/C&J, all visible), full-width submit button (h-12, 48px, sage accent). Submit disabled until both video and lift type selected. Modal over list — never navigates away
- UX-DR11: Metric Expanded View — modal overlay for detailed visualizations of bar path, velocity curve, joint angles, and phase durations. Tap outside or X to dismiss, return to scroll position
- UX-DR12: Direction E (Minimal Float) layout for lift detail — floating speed controls over video bottom, coaching cue as hero text immediately below video, coaching diagnosis below cue, phase timeline below coaching, metrics 3x2 grid below timeline, generous whitespace between sections
- UX-DR13: Button hierarchy — three tiers: primary action (sage fill, white text, full-width, h-12), interactive control (transparent, sage when active, h-10), navigation (no background, charcoal, standard 44px target). Only one primary action per screen context
- UX-DR14: Loading and async state patterns — content appears progressively via SSE/HTMX swaps, DaisyUI skeleton placeholders at correct dimensions, no loading labels, no completion announcements, no alerts
- UX-DR15: Mobile-only responsive strategy — 375-430px target viewport, portrait only, no breakpoints, no responsive prefixes, single-column layout at all widths
- UX-DR16: Spacing and layout — 4px base unit (Tailwind default), p-4 (16px) page padding, gap-4 (16px) between major sections, gap-3 (12px) for metric grid, minimum 44x44px touch targets, video edge-to-edge with no horizontal padding
- UX-DR17: Skeleton video auto-plays on detail load — skeleton overlay is the default view, not clean video. No tap required to see primary analysis
- UX-DR18: No error states in UI vocabulary — degraded results presented identically to full results. If a metric couldn't be computed, it's absent, not "N/A" or "Error"

### Cross-Cutting Testing Requirement: ChromeDP Browser Verification

Every story that produces or modifies HTML pages or partials **must** include ChromeDP headless browser verification tests. These tests ensure that the rendered pages are fully functional in a real browser environment.

**Required checks for every UI story:**
1. **Asset loading:** `output.css`, HTMX script, and `app.js` load without 404 or network errors
2. **DaisyUI theme:** `<html data-theme="press-out">` attribute is present and active
3. **No console errors:** No JavaScript errors on page load
4. **Page-specific element verification:** Key visual elements render with correct content, classes, and structure (defined per story)

**Test pattern:**
- Start the server on a random test port with a test database and test data
- Run ChromeDP against the running server
- Assert asset loading, theme, console errors, and page-specific elements
- Tear down server and test data after

**Stories requiring ChromeDP verification:**
- Epic 1: Stories 1.1, 1.2, 1.3, 1.4, 1.5 (all have UI)
- Epic 2: Stories 2.1 (pipeline progress UI), 2.7 (progressive video availability)
- Epic 3: Story 3.2 (video player with toggle & speed controls)
- Epic 4: Stories 4.3 (phase timeline), 4.4 (metrics display grid)
- Epic 5: Story 5.2 (coaching card display)

### FR Coverage Map

- FR1: Epic 1 - Video upload from mobile device
- FR2: Epic 1 - Persistent video storage
- FR3: Epic 1 - Lift type assignment
- FR4: Epic 2 - Auto-trim to lift portion
- FR5: Epic 2 - Auto-crop to lifter
- FR6: Epic 2 - Full video fallback on low confidence
- FR7: Epic 2 - Independent pipeline stage processing
- FR8: Epic 2 - Body keypoint detection
- FR9: Epic 3 - Skeleton overlay rendering
- FR10: Epic 3 - Dual video production (clean + skeleton)
- FR11: Epic 4 - Pull-to-catch ratio computation
- FR12: Epic 4 - Bar path visualization
- FR13: Epic 4 - Velocity curve computation
- FR14: Epic 4 - Joint angle measurement
- FR15: Epic 4 - Phase duration calculation
- FR16: Epic 4 - Key position snapshots
- FR17: Epic 4 - Lift phase segmentation
- FR18: Epic 4 - Phase timeline markers
- FR19: Epic 5 - Coaching diagnosis generation
- FR20: Epic 5 - Physical cue generation
- FR21: Epic 5 - Lift-aware coaching feedback
- FR22: Epic 1 - Lift list view
- FR23: Epic 1 - Lift detail view
- FR24: Epic 1 - Lift deletion
- FR25: Epic 3 - Skeleton/clean video toggle
- FR26: Epic 3 - Playback speed control
- FR27: Epic 4 - Phase timeline navigation
- FR28: Epic 2 - Progressive video playback
- FR29: Epic 2 - Real-time pipeline progress updates
- FR30: Epic 2 - Active processing stage indicator

## Epic List

### Epic 1: Upload & Manage Lift Videos
The lifter can upload a video from their phone, assign a lift type, browse all their lifts, view any lift's video, and delete lifts they no longer want. Includes project initialization, SQLite schema, file storage, basic templates, and routes.
**FRs covered:** FR1, FR2, FR3, FR22, FR23, FR24

### Epic 2: Auto-Process Videos with Live Progress
After uploading, the system detects body keypoints via pose estimation, trims the video to just the lift using keypoint data, and crops to the lifter. The lifter sees real-time progress as each processing stage completes. Video is viewable immediately even while processing continues. Includes pipeline orchestrator, SSE broker, pose/trim/crop stages, graceful degradation.
**FRs covered:** FR4, FR5, FR6, FR7, FR8, FR28, FR29, FR30

### Epic 3: Skeleton Overlay & Video Review
The system renders a skeleton overlay as a separate video using keypoint data from Epic 2. The lifter can toggle between clean and skeleton views instantly and control playback speed for frame-by-frame analysis.
**FRs covered:** FR9, FR10, FR25, FR26

### Epic 4: Lift Metrics & Phase Analysis
The system computes six biomechanical metrics (pull-to-catch ratio, bar path, velocity curve, joint angles, phase durations, key position snapshots), segments the lift into phases, and displays a navigable phase timeline.
**FRs covered:** FR11, FR12, FR13, FR14, FR15, FR16, FR17, FR18, FR27

### Epic 5: Coaching Intelligence
The system generates an LLM-powered coaching diagnosis that identifies issues with causal explanation referencing specific metrics, plus a concrete physical cue for the next rep.
**FRs covered:** FR19, FR20, FR21

## Epic 1: Upload & Manage Lift Videos

The lifter can upload a video from their phone, assign a lift type, browse all their lifts, view any lift's video, and delete lifts they no longer want.

### Story 1.1: Project Initialization & Landing Page

As a lifter,
I want to open press-out in my mobile browser and see the lift list home page,
So that I have a starting point for managing my training videos.

**Acceptance Criteria:**

**Given** the project is built and running
**When** the lifter navigates to the root URL in Chrome mobile
**Then** the lift list page renders with the DaisyUI custom theme (warm white background #FAFAF8, dark charcoal text #2D2D2D, system font stack)
**And** the page displays an empty state when no lifts exist
**And** an upload button is visible (sage accent #8BA888, white text, full-width, h-12)
**And** the page loads within 1 second (NFR2)

**Given** the project structure
**When** the developer runs `make build`
**Then** the Go binary compiles, Tailwind CSS compiles via standalone CLI, and sqlc generates type-safe code from SQL queries
**And** the project follows the defined package organization (cmd/press-out/, internal/, sql/, web/)

**Given** the SQLite database does not exist
**When** the application starts
**Then** the database is created at the configured DB_PATH with the lifts table schema applied via migration files
**And** configuration loads from environment variables with defaults (PORT=8080, DATA_DIR=./data, DB_PATH=./data/press-out.db)

### Story 1.2: Upload a Lift Video

As a lifter,
I want to upload a video from my phone and assign a lift type,
So that the system stores my lift for later analysis.

**Acceptance Criteria:**

**Given** the lifter is on the lift list page
**When** they tap the upload button
**Then** an upload modal opens with dark backdrop overlay (DaisyUI modal)
**And** the modal contains a file selector zone, a 3-option lift type selector (Snatch / Clean / C&J as a join group, all visible), and a full-width submit button (h-12, sage accent)
**And** the submit button is disabled until both a video file and lift type are selected

**Given** the upload modal is open with a video selected and lift type chosen
**When** the lifter taps submit
**Then** the modal closes immediately
**And** the video file is persisted to the filesystem in a lift-ID directory (data/lifts/{id}/original.mp4) before any processing begins (NFR10)
**And** a SQLite record is created with the lift type and timestamp
**And** the new lift appears at the top of the lift list

**Given** the upload modal is open
**When** the lifter selects a file that is not a video (not mp4/mov) or exceeds ~300MB
**Then** the upload is rejected with appropriate feedback
**And** no file is stored and no database record is created

**Given** the upload modal is open
**When** the lifter taps outside the modal or taps X
**Then** the modal closes without uploading

### Story 1.3: Browse Lift History

As a lifter,
I want to see a list of all my uploaded lifts with their date and lift type,
So that I can find and review any previous lift.

**Acceptance Criteria:**

**Given** the lifter has uploaded one or more lifts
**When** they visit the lift list page
**Then** all lifts are displayed in reverse chronological order (newest first)
**And** each lift row shows the lift type (Snatch, Clean, or Clean & Jerk) and date
**And** each lift row shows a thumbnail image when available
**And** each row is tappable (full row is the tap target)
**And** the page loads within 1 second (NFR2)

**Given** the lifter has no uploaded lifts
**When** they visit the lift list page
**Then** an empty state is displayed (no error, no broken layout)
**And** the upload button remains visible and functional

### Story 1.4: View Lift Detail

As a lifter,
I want to tap any lift and see its video with basic information,
So that I can review what I recorded.

**Acceptance Criteria:**

**Given** the lifter is on the lift list page
**When** they tap a lift row
**Then** the lift detail page loads showing the video player with the clean video
**And** the page displays the lift type and date
**And** the video player is full-width edge-to-edge with no horizontal padding
**And** a back button is visible for returning to the list (navigation tier: no background, charcoal, 44px target)
**And** the page loads within 1 second (NFR2)

**Given** the lifter is on the lift detail page
**When** they tap the video
**Then** video playback begins within 1 second (NFR3)

**Given** the lifter is on the lift detail page
**When** they tap the back button or use browser back
**Then** they return to the lift list page

### Story 1.5: Delete a Lift

As a lifter,
I want to remove a lift I no longer need,
So that my lift list stays clean and relevant.

**Acceptance Criteria:**

**Given** the lifter is on the lift detail page for a specific lift
**When** they initiate deletion of the lift
**Then** the lift's database record is removed
**And** the entire lift-ID directory is removed (original.mp4 and any other associated files)
**And** the lifter is returned to the lift list page
**And** the deleted lift no longer appears in the list

**Given** a lift is deleted
**When** the lifter visits the lift list page
**Then** the deleted lift is not present in the list
**And** no orphaned files remain on the filesystem for that lift

## Epic 2: Auto-Process Videos with Live Progress

After uploading, the system detects body keypoints via pose estimation, trims the video to just the lift using keypoint data, and crops to the lifter. The lifter sees real-time progress as each processing stage completes. Video is viewable immediately even while processing continues.

### Story 2.1: Pipeline Orchestrator & SSE Progress

As a lifter,
I want to see real-time progress as the system processes my uploaded video,
So that I know the system is working and can estimate when results will be ready.

**Acceptance Criteria:**

**Given** a lift has just been uploaded
**When** the upload completes
**Then** the pipeline orchestrator starts processing automatically in a background goroutine
**And** the orchestrator runs stages sequentially using the Stage interface (Name() + Run(ctx, StageInput) (StageOutput, error))
**And** SSE events are emitted via the in-memory broker as each stage starts and completes

**Given** the lifter is on the lift list page with a processing lift
**When** a pipeline stage completes
**Then** the compact pipeline indicator on the list item updates via SSE (current stage name + "N of 6")
**And** the update reaches the browser within 1 second of the stage completing (NFR5)

**Given** the lifter taps into a processing lift
**When** the lift detail page loads
**Then** a full vertical stage checklist is displayed with 6 stages (Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching)
**And** completed stages show a sage checkmark, the active stage shows a pulsing dot, and pending stages are dimmed
**And** the checklist updates in real-time via SSE as stages complete

**Given** a pipeline stage returns an error
**When** the orchestrator receives the error
**Then** the error is logged server-side with slog (lift_id, stage, error attributes)
**And** the stage is marked as skipped
**And** the pipeline continues with the last successful input passed forward unchanged
**And** no error screen is shown to the lifter (NFR9)

**Given** all pipeline stages have completed (or been skipped)
**When** the lifter is viewing the lift detail page
**Then** the stage checklist is replaced by the result view automatically via SSE

### Story 2.2: FFmpeg Integration & Verification

As a developer,
I want FFmpeg subprocess execution to be established and verified,
So that all video processing stages can reliably invoke FFmpeg.

**Acceptance Criteria:**

**Given** the application starts
**When** FFmpeg availability is checked
**Then** a clear log message confirms FFmpeg is available (or warns if missing) with the detected version
**And** the application continues to run regardless (FFmpeg absence means processing stages will skip gracefully)

**Given** a pipeline stage needs to invoke FFmpeg
**When** it calls the shared FFmpeg helper
**Then** FFmpeg is executed via exec.Command with the provided arguments
**And** stdout and stderr are captured
**And** non-zero exit codes are returned as errors (not panics)
**And** execution is bounded by a context timeout

**Given** a test video file exists in testdata/
**When** the FFmpeg integration test runs
**Then** FFmpeg is invoked successfully on the sample video
**And** the test verifies that output is produced and is a valid video file

### Story 2.3: Auto-Trim Video to Lift

As a lifter,
I want the system to automatically trim my video to just the lift portion,
So that I can review the lift immediately without scrubbing through setup footage.

**Acceptance Criteria:**

**Given** a video has been uploaded and the pipeline reaches the trim stage
**When** the trim stage runs
**Then** the system analyzes the video for motion patterns to detect lift start and end
**And** the trimmed video is saved as trimmed.mp4 in the lift-ID directory via FFmpeg
**And** padding is added around the detected lift boundaries

**Given** the motion detection confidence falls below the threshold
**When** the trim stage cannot confidently identify the lift boundaries
**Then** the full original video is preserved as the trim output (FR6)
**And** the stage completes without error (graceful degradation)
**And** downstream stages receive the full video as input

**Given** the trim stage encounters an FFmpeg error
**When** the subprocess fails
**Then** the error is logged with slog
**And** the stage returns an error to the orchestrator
**And** the orchestrator skips the stage and passes the original video forward (FR7)

### Story 2.4: Server-Side Pose Estimation (YOLO)

As a lifter,
I want the system to detect my body positions from the video after uploading,
So that my joint movements can be used for cropping, visualization, and analysis.

**Acceptance Criteria:**

**Given** a video has been uploaded and the pipeline reaches the pose stage
**When** the pose stage runs
**Then** YOLO26n-Pose detects body keypoints via Python subprocess (`uv run scripts/pose.py`)
**And** 17 COCO-format keypoints are detected per frame (FR8)
**And** keypoint coordinates are normalized (0-1 relative to video dimensions)
**And** per-frame bounding boxes are computed from detection boxes
**And** keypoints.json is saved to the lift-ID directory

**Given** the pose stage completes successfully
**When** the pipeline continues to downstream stages
**Then** keypoints.json is available for crop, skeleton, and metrics stages

**Given** the pose stage fails (Python subprocess error, model download failure, timeout)
**When** the orchestrator handles the error
**Then** the error is logged with slog
**And** keypoints.json is not written
**And** the pipeline continues — downstream stages handle missing keypoints gracefully (FR6)
**And** no error screen is shown to the lifter

### Story 2.5: Pose-Based Video Trim

As a lifter,
I want the system to trim my video to just the lift using detected body positions,
So that I can review only the relevant portion without setup or post-lift footage.

**Acceptance Criteria:**

**Given** keypoints.json exists from the pose estimation stage
**When** the trim stage runs
**Then** the system analyzes frame-to-frame keypoint displacement to detect the lift's start and end
**And** the trimmed video is saved as trimmed.mp4 in the lift-ID directory via FFmpeg
**And** padding is added around the detected lift boundaries

**Given** the keypoint-based detection confidence falls below the threshold
**When** the trim stage cannot confidently identify the lift boundaries
**Then** the full original video is preserved as the trim output (FR6)
**And** the stage completes without error (graceful degradation)
**And** downstream stages receive the full video as input

**Given** keypoints.json does not exist (pose estimation was skipped)
**When** the trim stage is reached
**Then** the full original video is preserved as the trim output (FR6)
**And** the stage completes without error
**And** downstream stages receive the full video as input

**Given** the trim stage encounters an FFmpeg error
**When** the subprocess fails
**Then** the error is logged with slog
**And** the stage returns an error to the orchestrator
**And** the orchestrator skips the stage and passes the original video forward (FR7)

### Story 2.6: Auto-Crop to Lifter

As a lifter,
I want the system to automatically crop the video to focus on me,
So that I see only my lift without distracting bystanders or background.

**Acceptance Criteria:**

**Given** keypoints.json exists from the pose estimation stage
**When** the crop stage runs
**Then** the system computes a bounding box from the keypoint data to identify the lifter's region
**And** the crop region is expanded with padding and enforced to 9:16 aspect ratio
**And** the video is cropped via FFmpeg and saved as cropped.mp4 in the lift-ID directory
**And** crop parameters (x, y, w, h, source dimensions) are saved as crop-params.json in the lift-ID directory for downstream coordinate transformation

**Given** the crop stage successfully produces a cropped video
**When** the crop completes
**Then** a thumbnail is extracted from the cropped video via FFmpeg
**And** the thumbnail is saved as thumbnail.jpg in the lift-ID directory

**Given** keypoints.json does not exist (pose estimation was skipped)
**When** the crop stage is reached
**Then** the full frame video is preserved as the crop output (FR6)
**And** a thumbnail is still extracted from the uncropped video
**And** the stage completes without error (graceful degradation)

**Given** multiple people are in the frame
**When** the crop stage runs
**Then** the system identifies the person with the most vertical movement in the keypoint data as the lifter (FR5)
**And** other people in the frame are excluded by the crop

### Story 2.7: Progressive Video Availability & Pipeline Re-trigger

As a lifter,
I want to watch my video immediately after upload without waiting for all processing to finish,
So that I can start reviewing while analysis continues in the background.

**Acceptance Criteria:**

**Given** a video has been uploaded and the pipeline is still running
**When** the lifter opens the lift detail page
**Then** the original video is available for playback immediately (FR28)
**And** video playback begins within 1 second of interaction (NFR3)
**And** the pipeline progress checklist is displayed alongside the video

**Given** the pipeline has completed some stages
**When** the lifter views the lift detail
**Then** the best available video is served (cropped > trimmed > original, based on what exists)
**And** processing state is derived from file existence (no error state in DB)

**Given** a pipeline run failed or was interrupted on a previously uploaded lift
**When** the lifter triggers a re-process action
**Then** the pipeline runs again on the existing uploaded video without requiring re-upload (NFR11)
**And** existing successfully-produced files are preserved
**And** the SSE progress updates resume for the new pipeline run

## Epic 3: Skeleton Overlay & Video Review

The system renders a skeleton overlay as a separate video using keypoint data from Epic 2. The lifter can toggle between clean and skeleton views instantly and control playback speed for frame-by-frame analysis.

### Story 3.1: Skeleton Overlay Rendering

As a lifter,
I want a skeleton overlay rendered on my lift video,
So that I can see my body positions and joint angles visually during the movement.

**Acceptance Criteria:**

**Given** keypoints.json and crop-params.json exist for a lift
**When** the skeleton rendering stage runs
**Then** keypoint coordinates are transformed from the original frame space to cropped frame space using the crop parameters
**And** a skeleton overlay is drawn onto each cropped video frame using the transformed keypoint data
**And** the skeleton-overlay video is rendered via FFmpeg and saved as skeleton.mp4 in the lift-ID directory (FR9)
**And** both the clean video and skeleton video are available for the lift (FR10)

**Given** crop-params.json does not exist (crop preserved full frame)
**When** the skeleton rendering stage runs
**Then** keypoint coordinates are used as-is without transformation
**And** the skeleton is rendered on the full-frame video

**Given** keypoints have varying confidence levels across frames
**When** the skeleton is rendered
**Then** the skeleton degrades gracefully on low-confidence frames (e.g., partial skeleton) rather than disappearing entirely
**And** the skeleton overlay remains visually clear against real gym video backgrounds

**Given** keypoints.json does not exist (pose estimation was skipped)
**When** the skeleton rendering stage is reached
**Then** the stage is skipped since there is no keypoint data to render
**And** the pipeline continues without a skeleton video

### Story 3.2: Video Player with Toggle & Speed Controls

As a lifter,
I want to toggle between skeleton and clean video and control playback speed,
So that I can analyze my lift in detail at my own pace.

**Acceptance Criteria:**

**Given** the lifter opens a lift detail page where both skeleton.mp4 and the clean video exist
**When** the page loads
**Then** the skeleton overlay video auto-plays (UX-DR17 — skeleton is the default view)
**And** a mode badge in the bottom-right corner displays "Skeleton"
**And** the video player is full-width edge-to-edge with sticky positioning while scrolling

**Given** the lifter is watching the skeleton video
**When** they tap anywhere on the video surface
**Then** the video source swaps to the clean video (or vice versa)
**And** playback continues from the same timestamp and speed
**And** the mode badge updates to reflect the current mode ("Skeleton" or "Clean")
**And** the toggle completes within 500 milliseconds (NFR4)

**Given** the video player is visible
**When** the lifter views the speed controls
**Then** a floating speed strip is visible at the bottom of the video with three options: 0.25x, 0.5x, 1x
**And** the strip has a subtle gradient backdrop (bg-gradient-to-t from-black/40)
**And** no speed is pre-selected on load (video plays at 1x)

**Given** the lifter taps a speed button
**When** the speed is selected
**Then** the video playback rate changes immediately via the HTML5 playbackRate API
**And** the selected button shows sage accent, others lose accent

**Given** only the clean video exists (skeleton rendering was skipped)
**When** the lifter opens the lift detail page
**Then** the clean video plays without a toggle option
**And** the mode badge is not displayed
**And** the speed controls remain functional

## Epic 4: Lift Metrics & Phase Analysis

The system computes six biomechanical metrics (pull-to-catch ratio, bar path, velocity curve, joint angles, phase durations, key position snapshots), segments the lift into phases, and displays a navigable phase timeline.

### Story 4.1: Metrics Computation from Keypoints

As a lifter,
I want the system to compute biomechanical metrics from my lift,
So that I can see quantified analysis of my technique.

**Acceptance Criteria:**

**Given** keypoints.json exists for a lift
**When** the metrics computation stage runs
**Then** pull-to-catch ratio is computed from keypoint data (FR11)
**And** bar path data is generated from keypoint data (FR12)
**And** a velocity curve is computed for the barbell movement (FR13)
**And** joint angles are measured at key positions during the lift (FR14)
**And** phase durations are calculated for each segment of the lift (FR15)
**And** key position snapshots are captured at phase transition points — first pull start, second pull start, catch, recovery (FR16)
**And** all six metrics are stored in the SQLite metrics table via sqlc-generated queries
**And** all metric values fall within physically plausible ranges

**Given** keypoints.json does not exist (pose estimation was skipped)
**When** the metrics stage is reached
**Then** the stage is skipped since there is no keypoint data
**And** the pipeline continues without metrics
**And** no error screen is shown to the lifter

**Given** keypoint data has low-confidence regions
**When** metrics are computed
**Then** the system produces best-effort metrics from available data
**And** metrics that cannot be reliably computed are omitted rather than showing incorrect values

### Story 4.2: LLM-Based Phase Segmentation

As a lifter,
I want the system to identify the phases of my lift,
So that I can navigate directly to any part of the movement.

**Acceptance Criteria:**

**Given** keypoints.json and computed metrics exist for a lift
**When** the coaching stage runs (phase segmentation is part of the coaching stage)
**Then** Claude Code headless is invoked as a subprocess with a structured prompt containing lift type, keypoint data, and computed metrics
**And** the lift is segmented into its constituent phases based on lift type (FR17):
  - Snatch: setup, first pull, transition, second pull, catch, recovery
  - Clean: setup, first pull, transition, second pull, catch, recovery
  - Clean & Jerk: setup, first pull, transition, second pull, catch, recovery, jerk setup, jerk drive, jerk catch
**And** each phase has a start time and end time
**And** phase data is stored in the SQLite phases table via sqlc-generated queries

**Given** Claude Code headless is unavailable or returns an error
**When** the coaching stage attempts phase segmentation
**Then** the error is logged with slog
**And** the stage is skipped gracefully (NFR7)
**And** the lift detail page renders without phase data (no timeline, no error message)

**Given** the subprocess execution exceeds the timeout
**When** the context deadline is reached
**Then** the subprocess is terminated
**And** the stage returns an error to the orchestrator for graceful handling

### Story 4.3: Phase Timeline & Navigation

As a lifter,
I want to see a phase timeline and tap any phase to jump the video there,
So that I can instantly review any part of my lift without scrubbing.

**Acceptance Criteria:**

**Given** phase data exists for a lift
**When** the lifter views the lift detail page
**Then** a phase timeline bar is displayed as a horizontal segmented bar (full-width, rounded corners)
**And** each segment is proportional to the phase duration
**And** segments are colored using the sage gradient palette, distinguishable but harmonious
**And** the timeline is positioned below the coaching section per Direction E layout (UX-DR12)
**And** phase markers are aligned with video playback (FR18)

**Given** the phase timeline is displayed
**When** the lifter taps a phase segment
**Then** the video seeks to the phase start time via the HTML5 currentTime API (FR27)
**And** the tapped segment highlights (full opacity, slight scale)
**And** a label appears above the selected segment (e.g., "2nd Pull — 0.31s")
**And** other segments dim

**Given** a phase segment is selected
**When** the video plays past the phase end or another segment is tapped
**Then** the previous highlight clears
**And** the new selection (if any) is highlighted

**Given** phase data does not exist (segmentation was skipped)
**When** the lifter views the lift detail page
**Then** the phase timeline is not rendered
**And** the layout adjusts without leaving a gap or showing an error

### Story 4.4: Metrics Display Grid

As a lifter,
I want to see my lift metrics displayed clearly on my phone,
So that I can quickly understand my technique at a glance.

**Acceptance Criteria:**

**Given** metrics exist for a lift
**When** the lifter views the lift detail page
**Then** six metrics are displayed in a 3x2 grid (grid-cols-3, gap-3) below the phase timeline
**And** each cell uses a DaisyUI card with neutral background (#EDEDEA), rounded-lg, p-3
**And** the grid layout is:
  | Pull-to-Catch Ratio | Bar Path | Velocity Curve |
  | Joint Angles | Phase Durations | Total Lift Duration |

**Given** the pull-to-catch ratio metric cell is displayed
**When** the lifter views it
**Then** it shows a vertical ratio bar (two proportional segments: pull height vs. catch depth) plus a large numeric value (text-2xl, font-bold)
**And** the metric label "Pull-to-Catch Ratio" is below (text-xs, font-medium)
**And** tapping the cell has no expand action

**Given** the bar path metric cell is displayed
**When** the lifter taps it
**Then** a modal opens showing an expanded plot with axis labels, phase transition markers, and numeric drift values
**And** tapping outside or X dismisses the modal and returns to the scroll position

**Given** the velocity curve metric cell is displayed
**When** the lifter taps it
**Then** a modal opens showing an expanded chart with phase regions shaded and numeric values at key points
**And** tapping outside or X dismisses the modal

**Given** the joint angles metric cell is displayed
**When** the lifter taps it
**Then** a modal opens showing all four key positions side by side with full angle annotations
**And** tapping outside or X dismisses the modal

**Given** the phase durations metric cell is displayed
**When** the lifter taps it
**Then** a modal opens showing each phase labeled with individual duration in milliseconds
**And** tapping outside or X dismisses the modal

**Given** the total lift duration metric cell is displayed
**When** the lifter views it
**Then** it shows a large numeric value (e.g., "1.84s") with label below
**And** tapping the cell has no expand action

**Given** some metrics could not be computed (partial data)
**When** the lifter views the metrics grid
**Then** only the successfully computed metrics are displayed
**And** missing metrics are absent from the grid (not shown as "N/A" or "Error") (UX-DR18)
**And** the grid adjusts without broken layout

## Epic 5: Coaching Intelligence

The system generates an LLM-powered coaching diagnosis that identifies issues with causal explanation referencing specific metrics, plus a concrete physical cue for the next rep.

### Story 5.1: LLM Coaching Generation

As a lifter,
I want the system to generate coaching feedback based on my lift data,
So that I know what went wrong and what to try on the next rep.

**Acceptance Criteria:**

**Given** keypoints, phase segmentation, and computed metrics exist for a lift
**When** the coaching generation portion of the coaching stage runs
**Then** Claude Code headless is invoked as a subprocess with a structured prompt containing lift type, keypoint data, phase segmentation, and computed metrics (FR21)
**And** the response includes a coaching diagnosis that identifies at least one issue, one causal factor, and references specific metric values from the lift (FR19)
**And** the response includes a physical cue describing a specific body movement or position change for the next rep (FR20)
**And** the coaching diagnosis and cue are stored in the SQLite database via sqlc-generated queries

**Given** the coaching diagnosis is generated
**When** the lifter reads the diagnosis
**Then** the diagnosis references specific data from this lift (e.g., "your elbows were at X degrees at the catch") — not generic advice
**And** the causal chain explains why the issue occurred, not just what happened

**Given** Claude Code headless is unavailable or returns an error
**When** the coaching stage attempts to generate coaching
**Then** the error is logged with slog (lift_id, stage, error attributes)
**And** the stage is skipped gracefully (NFR7)
**And** the lift detail page renders without coaching content (no error message, no placeholder text)

**Given** the coaching stage completes successfully
**When** results are stored
**Then** an SSE event is emitted to notify the browser that coaching content is ready

### Story 5.2: Coaching Card Display

As a lifter,
I want to see the coaching cue prominently below the video,
So that I can quickly absorb the key takeaway for my next rep.

**Acceptance Criteria:**

**Given** the lifter opens a lift detail page where coaching is still being generated
**When** the page loads
**Then** a DaisyUI skeleton placeholder is displayed in the coaching section matching the card dimensions (subtle pulse animation)
**And** no loading label or "generating coaching..." text is shown (UX-DR14)
**And** the coaching section is positioned immediately below the video per Direction E layout (UX-DR12)

**Given** coaching generation completes while the lifter is on the detail page
**When** the SSE event for coaching-ready arrives
**Then** the skeleton placeholder is replaced with the coaching content via HTMX hx-swap
**And** the swap happens silently with no animation or notification
**And** the lifter may already be scrolled past the coaching area — no disruption to their current view

**Given** coaching content exists for a lift
**When** the lifter views the coaching section
**Then** the coaching cue is displayed as hero text (text-lg, font-semibold) — the first readable content below the video (UX-DR8)
**And** a subtle divider separates the cue from the diagnosis
**And** the coaching diagnosis is displayed in muted text (text-sm, text-gray-500) below the divider
**And** a left border accent in info color (#9BB0BA) visually distinguishes the coaching section as generated analysis
**And** the coaching section has generous padding (p-4)

**Given** coaching was skipped (LLM unavailable)
**When** the lifter views the lift detail page
**Then** the coaching section is not rendered
**And** no placeholder, no error, no gap — the layout flows from video to the next available section (UX-DR18)
