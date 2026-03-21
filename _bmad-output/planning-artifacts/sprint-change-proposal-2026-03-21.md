# Sprint Change Proposal — Server-Side YOLO26n-Pose

**Date:** 2026-03-21
**Author:** joao
**Status:** Approved

## 1. Issue Summary

A spike (`spikes/yolo-pose/pose_spike.py`) demonstrated that YOLO26n-Pose from ultralytics, running as a standalone Python script, produces superior results to client-side ml5.js MoveNet:

| Metric | YOLO26n-Pose (spike) | ml5.js MoveNet (prior) |
|--------|---------------------|----------------------|
| Speed | 39.3 fps CPU (8.9s for 11.9s/352-frame video) | ~7s for 12s video (browser, device-dependent) |
| Detection rate | 348/350 frames (99.4%) | Good but browser-dependent |
| Model size | 7.5MB (auto-downloaded) | ~10MB via CDN |
| Runs on | Server (consistent) | Browser (variable by device) |

This triggers a pivot from client-side to server-side pose estimation. The output format is identical (`keypoints.json` with normalized coords, 17 COCO keypoints, bounding boxes), so all downstream stages (crop, skeleton, metrics) are unaffected.

**Why this is better:**
- Eliminates browser-side processing bottleneck — no more waiting in the upload modal
- Consistent performance regardless of the lifter's phone hardware
- Upload simplifies to just video + lift type (no keypoints.json multipart field)
- Pose estimation becomes a visible pipeline stage again — the lifter sees progress
- Server-side control over model version and processing parameters

## 2. Impact Analysis

### Epic Impact

- **Epic 2 (Auto-Process Videos):** Story 2.4 rewritten from client-side ml5.js to server-side YOLO. Pipeline returns from 5 to 6 server-side stages. Story 2.1 stage counts updated. Story 2.5 prerequisites updated.
- **Epics 3-5:** No impact. Downstream consumers read the same `keypoints.json` format regardless of origin.

### Story Impact

| Story | Change |
|-------|--------|
| 2.1 (Pipeline Orchestrator) | Stage count 5 -> 6, "Pose estimation" added back to stage list and constants |
| 2.4 (Pose Estimation) | Full rewrite: server-side YOLO via Python subprocess |
| 2.5 (Auto-Crop) | Prerequisites text updated (keypoints from pose stage, not upload) |
| 2.6 (Progressive Video) | No changes needed |

### Artifact Updates Required

| Artifact | Changes |
|----------|---------|
| CLAUDE.md | ml5.js section replaced with YOLO + uv description |
| PRD | FR8 context, NFR6, NFR8, MVP feature set, risk mitigation |
| Architecture | External integrations, pipeline stages, package structure, data flow, system deps, directory tree, validation tables (~18 edits) |
| Epics | Story 2.4 rewritten, stage counts, NFRs, integrations (~8 edits) |
| UX Design | Pipeline stage checklist 5->6, stage lists (~3 edits) |
| Story 2.1 spec | Stage count and constant references (5 edits) |
| Story 2.4 spec | Full rewrite |
| Story 2.5 spec | Prerequisites and dev notes (3 edits) |

### Technical Impact

**Removed (from ml5.js Story 2.4):**
- Client-side pose estimation JavaScript in `web/static/app.js`
- ml5.js CDN script tag in `web/templates/layouts/base.html`
- Pose progress UI in upload modal
- Keypoints multipart field in upload handler
- `web/static/pose-spike.html` (ml5.js spike, superseded)

**Added:**
- `scripts/pose.py` — production YOLO pose script (from spike)
- `pyproject.toml` + `uv.lock` — Python dependency management via uv
- `internal/pipeline/stages/pose.go` — Go pose stage (calls `uv run scripts/pose.py`)
- `internal/pipeline/stages/pose_test.go` — tests

**Modified:**
- `internal/pipeline/stage.go` — add `StagePoseEstimation` constant, 5 -> 6 stages
- `cmd/press-out/main.go` — add PoseStage to pipeline, remove keypoints upload handling
- `internal/handler/lift.go` — remove keypoints multipart field parsing
- `web/templates/partials/pipeline-stages.html` — 5 -> 6 stages
- `web/templates/partials/upload-modal.html` — remove pose progress UI, keypoints field
- `Makefile` — add check-deps for uv/python

**Simplified:**
- Upload flow — just video + lift type, no client-side processing
- Upload modal — no pose progress bar, no canvas processing
- No CDN dependency for pose estimation

## 3. Recommended Approach

**Direct Adjustment** — rewrite Story 2.4, update supporting artifacts, remove ml5.js code.

- **Effort:** Medium (similar to the ml5.js pivot)
- **Risk:** Low (spike validated with real test video, format compatible)
- **Timeline impact:** Neutral to positive (simpler upload flow, consistent server-side processing)

**Rationale:** The change is contained within Epic 2. The keypoints.json format is identical, so downstream stages (crop, skeleton, metrics) are unaffected. The spike validates the approach works at 39.3 fps with 99.4% detection. Adding a Python subprocess follows the same pattern as FFmpeg — proven in this codebase. The upload flow simplifies (fewer client-side concerns). New system dependency (uv + Python) is a fair trade for eliminating browser-dependent pose processing.

## 4. Detailed Change Proposals

### 4.1 CLAUDE.md

**Section:** Pose Estimation

OLD:
```
## Pose Estimation

Pose estimation runs client-side in the browser via ml5.js (MoveNet SINGLEPOSE_THUNDER).
The browser processes the video frame-by-frame, then uploads both the video file and
`keypoints.json` to the server. No cloud API or credentials needed.
```

NEW:
```
## Pose Estimation

Pose estimation runs server-side via YOLO26n-Pose (ultralytics) as a Python subprocess
managed by uv. The pipeline calls `uv run scripts/pose.py <video> -o <keypoints.json>`.
No cloud API or credentials needed. Model (7.5MB) auto-downloads on first run.

## Python Dependencies

Python dependencies are managed by uv. The project has `pyproject.toml` and `uv.lock`
at the project root. The pose script runs via `uv run`.
```

Rationale: Reflects new pose estimation approach and documents uv dependency management.

---

### 4.2 PRD

**Edit PRD-1: MVP Feature Set (line 164)**

OLD:
```
4. Client-side pose estimation via ml5.js MoveNet (runs in browser before upload)
```

NEW:
```
4. Server-side pose estimation via YOLO26n-Pose (Python subprocess after upload)
```

**Edit PRD-2: Risk Mitigation (line 194)**

OLD:
```
**Technical Risk:** Pose estimation quality on real gym video (occlusion, lighting, bystanders) is the highest-risk component. Mitigation: ml5.js MoveNet runs client-side with no cloud dependency. Spike validated good detection quality on real weightlifting footage. If pose fails, video uploads without keypoints and downstream stages degrade gracefully.
```

NEW:
```
**Technical Risk:** Pose estimation quality on real gym video (occlusion, lighting, bystanders) is the highest-risk component. Mitigation: YOLO26n-Pose runs server-side at 39.3 fps with 99.4% detection rate (spike validated on real weightlifting footage). 7.5MB model auto-downloads on first run. If pose fails, downstream stages degrade gracefully.
```

**Edit PRD-3: NFR6 (line 266)**

OLD:
```
- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped client-side), continuing the pipeline without crashing
```

NEW:
```
- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped), continuing the pipeline without crashing
```

**Edit PRD-4: NFR8 (line 268)**

OLD:
```
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code) and ml5.js CDN
```

NEW:
```
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code). Pose estimation runs locally via YOLO26n-Pose.
```

Rationale: Removes ml5.js references, reflects server-side YOLO approach, removes CDN dependency.

---

### 4.3 Architecture

**Edit ARCH-1: FR8 description (line 27-28)**

OLD:
```
Keypoint detection from video frames (client-side via ml5.js MoveNet), skeleton overlay rendering, dual pre-rendered video output. Keypoints arrive via upload; skeleton rendering happens server-side frame-by-frame.
```

NEW:
```
Keypoint detection from video frames (server-side via YOLO26n-Pose Python subprocess), skeleton overlay rendering, dual pre-rendered video output. Keypoints produced by pose pipeline stage; skeleton rendering happens server-side frame-by-frame.
```

**Edit ARCH-2: External APIs (line 52-53)**

OLD:
```
- **External APIs:** LLM API via Claude Code headless (coaching + phase segmentation). Pose estimation runs client-side via ml5.js MoveNet — no external API.
```

NEW:
```
- **External APIs:** LLM API via Claude Code headless (coaching + phase segmentation). Pose estimation runs server-side via YOLO26n-Pose Python subprocess managed by uv — no external API.
```

**Edit ARCH-3: System dependency (line 53)**

OLD:
```
- **System dependency:** FFmpeg (required for video trim, crop, skeleton rendering, and thumbnail extraction — invoked via `exec.Command`)
```

NEW:
```
- **System dependencies:** FFmpeg (required for video trim, crop, skeleton rendering, and thumbnail extraction — invoked via `exec.Command`), uv + Python 3 (required for YOLO26n-Pose pose estimation — invoked via `exec.CommandContext`)
```

**Edit ARCH-4: Pipeline performance budget (line 65)**

OLD:
```
- **Video processing performance budget:** The 3-minute pipeline target requires careful allocation of compute time across 5 server-side stages (pose estimation runs client-side before upload). This may influence whether stages run sequentially or need parallelization, and whether any stages can overlap.
```

NEW:
```
- **Video processing performance budget:** The 3-minute pipeline target requires careful allocation of compute time across 6 server-side stages (Trim -> Pose -> Crop -> Skeleton -> Metrics -> Coaching). Pose estimation at ~9s for a 12s video fits comfortably within the budget. This may influence whether stages run sequentially or need parallelization, and whether any stages can overlap.
```

**Edit ARCH-5: Configuration (line 296)**

OLD:
```
- Claude Code manages its own authentication — no LLM API key needed
- Rationale: Minimal config surface. All config has sensible defaults. No cloud API credentials required — pose estimation runs client-side via ml5.js.
```

NEW:
```
- Claude Code manages its own authentication — no LLM API key needed
- Rationale: Minimal config surface. All config has sensible defaults. No cloud API credentials required — pose estimation runs server-side via YOLO26n-Pose.
```

**Edit ARCH-6: External Integration Architecture (lines 306-311)**

OLD:
```
**ml5.js MoveNet (Client-Side Pose Estimation)**
- MoveNet SINGLEPOSE_THUNDER model loaded via ml5.js CDN in the browser
- Processes video frame-by-frame at 30fps on a canvas element, extracts 17 COCO keypoints per frame
- Keypoints normalized (0-1), smoothed (7-frame averaging window), exported as keypoints.json
- Uploaded alongside the video as a multipart form field — no server-side pose estimation
- Error handling: if pose estimation fails in browser, video uploads without keypoints.json; downstream stages handle missing file gracefully
- Affects: FR8 (keypoint detection)
```

NEW:
```
**YOLO26n-Pose (Server-Side Pose Estimation)**
- YOLO26n-Pose model (7.5MB, auto-downloaded on first run) from ultralytics
- Runs as a Python subprocess via `exec.CommandContext(ctx, "uv", "run", "scripts/pose.py", videoPath, "-o", keypointsPath)`
- Processes video frame-by-frame at ~39 fps on CPU, extracts 17 COCO keypoints per frame
- Keypoints normalized (0-1), exported as keypoints.json in the lift directory
- Python dependency management via uv (pyproject.toml + uv.lock at project root)
- Error handling: if pose estimation fails, keypoints.json is not written; downstream stages handle missing file gracefully
- Affects: FR8 (keypoint detection)
```

**Edit ARCH-7: Cross-Component Dependencies (line 340)**

OLD:
```
- Upload handler accepts keypoints.json from client-side pose estimation
```

NEW:
```
- Pose stage produces keypoints.json consumed by crop, skeleton, and metrics stages
```

**Edit ARCH-8: Package Organization — add pose stage (after line 387)**

ADD after `internal/pipeline/stages/trim.go`:
```
internal/pipeline/stages/pose.go   -- pose estimation stage (YOLO26n-Pose via uv subprocess)
```

UPDATE `internal/pose/pose.go` description:
OLD:
```
internal/pose/pose.go              -- pose.Result types and keypoints.json serialization (used by upload handler and downstream stages)
```
NEW:
```
internal/pose/pose.go              -- pose.Result types and keypoints.json serialization (used by pose stage and downstream stages)
```

**Edit ARCH-9: Project Directory Tree (after line 489)**

ADD at project root level (after `sqlc.yaml`):
```
├── pyproject.toml                         -- Python project config (ultralytics, opencv-python)
├── uv.lock                               -- uv lockfile for reproducible Python deps
```

ADD new directory (after `sql/`):
```
├── scripts/
│   └── pose.py                           -- YOLO26n-Pose inference script
```

ADD to `internal/pipeline/stages/` (after trim.go):
```
│   │       ├── pose.go                -- pose estimation stage (YOLO26n-Pose)
│   │       ├── pose_test.go
```

**Edit ARCH-10: FR8 mapping (line 639)**

OLD:
```
- `internal/pipeline/stages/pose.go` — FR8 (keypoint detection, runs before crop)
```

NEW:
```
- `internal/pipeline/stages/pose.go` — FR8 (YOLO26n-Pose keypoint detection via Python subprocess, runs before crop)
```

**Edit ARCH-11: External Integrations in Integration Points (lines 678-679)**

OLD:
```
**External Integrations:**
- **ml5.js MoveNet** (browser): Client-side pose estimation, produces keypoints.json uploaded with video
- **Claude Code** (`internal/claude/runner.go`): Subprocess execution with structured prompt, parses stdout response
```

NEW:
```
**External Integrations:**
- **YOLO26n-Pose** (`scripts/pose.py`): Python subprocess via uv, produces keypoints.json in lift directory
- **Claude Code** (`internal/claude/runner.go`): Subprocess execution with structured prompt, parses stdout response
```

**Edit ARCH-12: Data Flow (lines 683-692)**

OLD:
```
Browser: ml5.js bodyPose -> keypoints.json (client-side, before upload)
Upload (HTTP) -> storage.CreateLift() -> SQLite row + original.mp4 + keypoints.json
  -> pipeline.Run() [goroutine]
    -> trim.Run()     -> trimmed.mp4 (or skip)
    -> crop.Run()     -> cropped.mp4 + crop-params.json + thumbnail.jpg (uses keypoints for bounding box)
    -> skeleton.Run() -> skeleton.mp4 (transforms keypoints to cropped frame via crop-params.json)
    -> metrics.Run()  -> SQLite metrics rows
    -> coaching.Run() -> SQLite phases + coaching rows (via Claude Code)
  -> SSE events emitted at each stage transition
```

NEW:
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

**Edit ARCH-13: Coherence Validation (line 713)**

OLD:
```
ml5.js (client-side pose) and Claude Code (subprocess) integrate independently without interference.
```

NEW:
```
YOLO26n-Pose (Python subprocess via uv) and Claude Code (subprocess) integrate independently without interference.
```

**Edit ARCH-14: Architecture Completeness Checklist (line 782)**

OLD:
```
- [x] Integration patterns defined (ml5.js CDN for pose estimation, subprocess for Claude Code, exec.Command for FFmpeg)
```

NEW:
```
- [x] Integration patterns defined (Python subprocess via uv for pose estimation, subprocess for Claude Code, exec.Command for FFmpeg)
```

**Edit ARCH-15: System Dependencies (lines 818-820)**

OLD:
```
- Go (latest stable)
- FFmpeg (system package — used by trim, crop, skeleton, and thumbnail stages via `exec.Command`)
- Tailwind CSS standalone CLI
```

NEW:
```
- Go (latest stable)
- FFmpeg (system package — used by trim, crop, skeleton, and thumbnail stages via `exec.Command`)
- uv (Python package manager — used to run YOLO26n-Pose pose estimation)
- Python 3 (required by uv for YOLO pose estimation subprocess)
- Tailwind CSS standalone CLI
```

**Edit ARCH-16: FR8 Coverage Table (line 727)**

OLD:
```
| FR8-10 | Pose & Visualization | ml5.js (client-side), stages/skeleton, pose/pose.go | Covered |
```

NEW:
```
| FR8-10 | Pose & Visualization | stages/pose (YOLO26n-Pose), stages/skeleton, pose/pose.go | Covered |
```

**Edit ARCH-17: Static Assets (line 274)**

OLD:
```
- HTMX and DaisyUI via CDN
```

NEW:
```
- HTMX and DaisyUI via CDN (ml5.js CDN removed — pose estimation moved server-side)
```

**Edit ARCH-18: Implementation Sequence (line 331)**

OLD:
```
7. Individual pipeline stages (trim, crop, pose, skeleton, metrics, coaching)
```

This line already includes "pose" which is correct for YOLO. No change needed.

Rationale: Architecture is the most heavily impacted artifact. All references to ml5.js, client-side pose, browser processing, and 5-stage pipeline are updated for server-side YOLO26n-Pose with 6-stage pipeline.

---

### 4.4 Epics

**Edit EPICS-1: NFR6 (line 54)**

OLD:
```
- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped client-side), continuing the pipeline without crashing
```

NEW:
```
- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped), continuing the pipeline without crashing
```

**Edit EPICS-2: NFR8 (line 57)**

OLD:
```
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code) and ml5.js CDN
```

NEW:
```
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code). Pose estimation runs locally via YOLO26n-Pose.
```

**Edit EPICS-3: Additional Requirements — External integrations (line 81)**

OLD:
```
- External integrations: ml5.js via CDN (client-side pose estimation), Claude Code via headless subprocess runner
```

NEW:
```
- External integrations: YOLO26n-Pose via Python subprocess managed by uv (server-side pose estimation), Claude Code via headless subprocess runner
```

**Edit EPICS-4: UX-DR5 (line 90)**

OLD:
```
- UX-DR5: Pipeline Stage Checklist — vertical list of 5 stages (Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching) with three states per stage (pending: dimmed, active: pulsing dot, complete: sage checkmark). Two variants: compact (list item: current stage + "N of 5") and full (detail view: all 5 stages visible). Pose estimation runs client-side before upload and is not part of the server pipeline.
```

NEW:
```
- UX-DR5: Pipeline Stage Checklist — vertical list of 6 stages (Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching) with three states per stage (pending: dimmed, active: pulsing dot, complete: sage checkmark). Two variants: compact (list item: current stage + "N of 6") and full (detail view: all 6 stages visible).
```

**Edit EPICS-5: Story 2.1 AC2 — stage count (line 327)**

OLD:
```
**Then** the compact pipeline indicator on the list item updates via SSE (current stage name + "N of 5")
```

NEW:
```
**Then** the compact pipeline indicator on the list item updates via SSE (current stage name + "N of 6")
```

**Edit EPICS-6: Story 2.1 AC3 — stage list (line 332)**

OLD:
```
**Then** a full vertical stage checklist is displayed with 5 stages (Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching)
```

NEW:
```
**Then** a full vertical stage checklist is displayed with 6 stages (Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching)
```

**Edit EPICS-7: Story 2.4 — full rewrite (lines 398-429)**

OLD:
```
### Story 2.4: Client-Side Pose Estimation & Upload

As a lifter,
I want the system to detect my body positions from the video before uploading,
So that my joint movements can be used for cropping, visualization, and analysis.

**Acceptance Criteria:**

**Given** the lifter selects a video in the upload modal
**When** the video is selected
**Then** the browser loads ml5.js bodyPose (MoveNet SINGLEPOSE_THUNDER) and processes the video frame-by-frame at 30fps on a canvas
**And** a progress indicator shows pose estimation progress (e.g., "Processing frame 120 / 360")
**And** 17 COCO-format keypoints are detected per frame (FR8)
**And** keypoint coordinates are normalized (0-1 relative to video dimensions)
**And** keypoint smoothing (7-frame averaging window) is applied to reduce jitter

**Given** client-side pose estimation completes
**When** the lifter selects a lift type and taps submit
**Then** the upload sends both the video file and keypoints.json as multipart form fields
**And** the server stores keypoints.json in the lift-ID directory alongside original.mp4
**And** the server pipeline starts with keypoints.json already available for downstream stages (crop, skeleton, metrics)

**Given** client-side pose estimation fails or detects no poses
**When** the upload proceeds
**Then** the video is still uploaded without keypoints.json
**And** downstream stages that depend on keypoints handle the missing file gracefully (FR6)
**And** no error screen is shown to the lifter

**Given** the upload completes and the pipeline starts
**When** the trim stage runs
**Then** the video is trimmed as in Story 2.3
**And** the pipeline continues with cropping, skeleton, metrics, and coaching stages
```

NEW:
```
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
```

Rationale: Epic 2 story structure and FR coverage unchanged. Only the mechanism for FR8 changes.

---

### 4.5 UX Design Specification

**Edit UX-1: Experience Mechanics — Processing stages (line 248-249)**

OLD:
```
- Screen shows pipeline stage checklist (Trimming → Cropping → Rendering skeleton → Computing metrics → Generating coaching)
```

NEW:
```
- Screen shows pipeline stage checklist (Trimming → Pose estimation → Cropping → Rendering skeleton → Computing metrics → Generating coaching)
```

**Edit UX-2: Pipeline Stage Checklist component (line 576-577)**

OLD:
```
- Vertical list of 5 stages: Trimming → Cropping → Rendering skeleton → Computing metrics → Generating coaching
```

NEW:
```
- Vertical list of 6 stages: Trimming → Pose estimation → Cropping → Rendering skeleton → Computing metrics → Generating coaching
```

**Edit UX-3: Upload Modal component — no pose progress UI**

The upload modal description (UX-DR10) does not mention ml5.js pose progress directly — it describes the simple upload flow (file selector, lift type, submit). No text change needed. The dev story handles removing ml5.js pose code from the implementation.

Rationale: UX changes are minimal — the upload flow simplifies (no pose progress UI), and the pipeline checklist gains the "Pose estimation" stage back.

---

### 4.6 Story 2.1 Spec

**Edit STORY-2.1-1: AC2 — stage count (line 15)**

OLD: `(current stage name + "N of 5")`
NEW: `(current stage name + "N of 6")`

**Edit STORY-2.1-2: AC3 — stage list (line 17)**

OLD: `5 stages (Trimming, Cropping, Rendering skeleton, Computing metrics, Generating coaching)`
NEW: `6 stages (Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching)`

**Edit STORY-2.1-3: Stage name constants (line 27)**

OLD: `Stage names as constants: \`StageTrimming\`, \`StageCropping\`, \`StageRenderingSkeleton\`, \`StageComputingMetrics\`, \`StageGeneratingCoaching\``
NEW: `Stage names as constants: \`StageTrimming\`, \`StagePoseEstimation\`, \`StageCropping\`, \`StageRenderingSkeleton\`, \`StageComputingMetrics\`, \`StageGeneratingCoaching\``

**Edit STORY-2.1-4: Pipeline stages template (line 60)**

OLD: `Full variant (detail view): vertical list of 5 stages`
NEW: `Full variant (detail view): vertical list of 6 stages`

**Edit STORY-2.1-5: ChromeDP tests (line 87)**

OLD: `Verify pipeline stage checklist renders with correct 5 stage names`
NEW: `Verify pipeline stage checklist renders with correct 6 stage names`

Rationale: Story 2.1 defines the pipeline infrastructure. Stage count and stage name list must match the new 6-stage pipeline.

---

### 4.7 Story 2.4 Spec — Full Rewrite

The story file at `_bmad-output/implementation-artifacts/2-4-pose-estimation.md` requires a complete rewrite from client-side ml5.js to server-side YOLO26n-Pose. Key changes:

- Title: "Client-Side Pose Estimation & Upload" -> "Server-Side Pose Estimation (YOLO)"
- User story: "before uploading" -> "after uploading"
- All ml5.js/browser/canvas references removed
- New tasks: Python script, pyproject.toml, Go pose stage, uv integration
- Remove tasks: ml5.js CDN, pose progress UI, multipart keypoints field, Video Intelligence cleanup
- Verification tests: test the Go stage + Python subprocess, not browser behavior

Full rewrite will be applied on approval.

---

### 4.8 Story 2.5 Spec

**Edit STORY-2.5-1: Prerequisites (line 86-87)**

OLD:
```
- Story 2.4 (Client-Side Pose Estimation & Upload) must be complete — this story reads keypoints.json uploaded from the browser.
```

NEW:
```
- Story 2.4 (Server-Side Pose Estimation) must be complete — this story reads keypoints.json produced by the pose pipeline stage.
```

**Edit STORY-2.5-2: Dev notes — person selection (line 90)**

OLD:
```
**Person selection is handled upstream by Story 2.4 (pose estimation).** The keypoints.json always contains a single person's data (the primary lifter, selected by largest average bounding box area).
```

NEW:
```
**Person selection is handled upstream by Story 2.4 (pose estimation).** YOLO26n-Pose selects the first (highest-confidence) person detected. The keypoints.json always contains a single person's data.
```

**Edit STORY-2.5-3: Dev notes — bounding box source (line 92)**

OLD:
```
The keypoints.json includes per-frame `boundingBox` data from client-side ml5.js MoveNet detection.
```

NEW:
```
The keypoints.json includes per-frame `boundingBox` data from server-side YOLO26n-Pose detection.
```

Rationale: Story 2.5 references are updated to reflect server-side pose. Logic and acceptance criteria unchanged.

---

## 5. Implementation Handoff

**Scope:** Minor — direct implementation by development team.

**Implementation order:**
1. Apply all artifact updates from this proposal (planning docs, stories, CLAUDE.md)
2. Implement rewritten Story 2.4 (server-side YOLO pose stage):
   a. Create `pyproject.toml` with ultralytics + opencv-python deps
   b. Create `scripts/pose.py` (production version from spike)
   c. Create `internal/pipeline/stages/pose.go` (Go stage calling `uv run`)
   d. Remove ml5.js code from app.js, base.html, upload-modal.html, upload handler
   e. Add `StagePoseEstimation` constant, update pipeline stages list
3. Continue with Story 2.5 (crop) which reads keypoints.json from pose stage

**Success criteria:**
- `uv run scripts/pose.py testdata/videos/sample-lift.mp4 -o /tmp/test-keypoints.json` produces valid keypoints.json
- Pipeline runs 6 stages (Trimming, Pose estimation, Cropping, Rendering skeleton, Computing metrics, Generating coaching)
- Downstream stages (crop, skeleton, metrics) consume keypoints.json identically
- Upload modal is simple: video + lift type, no pose processing UI
- All verification tests pass (Go unit tests, ChromeDP browser tests)

**Spike evidence:**
- Location: `spikes/yolo-pose/pose_spike.py`
- Test video: `testdata/videos/sample-lift.mp4`
- Results: 39.3 fps, 348/350 frames with pose (99.4%), 8.9s for 11.9s video
