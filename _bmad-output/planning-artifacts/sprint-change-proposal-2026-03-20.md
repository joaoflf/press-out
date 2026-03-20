# Sprint Change Proposal — Client-Side Pose Estimation

**Date:** 2026-03-20
**Author:** joao
**Status:** Approved

## 1. Issue Summary

A working spike (`web/static/pose-spike.html`) demonstrated that client-side pose estimation via ml5.js MoveNet (SINGLEPOSE_THUNDER) produces equivalent results to the Google Cloud Video Intelligence API — same 17 COCO keypoints, same JSON format — with zero cloud cost, zero credential infrastructure, and faster processing (~7s for 12s video at 30fps vs 30-120s API round-trip).

This triggered a strategic pivot from server-side to client-side pose estimation. The spike was built after Story 2.4s confirmed the Video Intelligence API works but revealed it as unnecessarily complex and costly for this use case.

## 2. Impact Analysis

### Epic Impact

- **Epic 2 (Auto-Process Videos):** Story 2.4 rewritten from server-side Video Intelligence API to client-side ml5.js. Pipeline drops from 6 to 5 server-side stages. Story 2.5 prerequisites updated.
- **Epics 3-5:** No impact. Downstream consumers read the same `keypoints.json` format regardless of origin.

### Story Impact

| Story | Change |
|-------|--------|
| 2.1 (Pipeline Orchestrator) | Stage count 6 -> 5, "Pose estimation" removed from server stage list and constants |
| 2.4 (Pose Estimation) | Full rewrite: client-side ml5.js + upload modification + trim integration |
| 2.5 (Auto-Crop) | Prerequisites text updated (keypoints from upload, not server stage) |
| 2.6 (Progressive Video) | No changes needed |

### Artifact Updates Applied

| Artifact | Changes |
|----------|---------|
| CLAUDE.md | GCP credentials section replaced with client-side pose note |
| docs/gcp-credentials-setup.md | Deleted (obsolete) |
| PRD | NFR6, NFR8, MVP feature set, risk mitigation updated |
| Architecture | External integrations, config, package structure, data flow, directory tree, validation tables (11 edits) |
| Epics | Story 2.4 rewritten, stage counts, NFRs, config, integrations (7 edits) |
| UX Design | Pipeline stage checklist 6->5, stage list, compact indicator (5 edits) |
| Story 2.4 spec | Full rewrite with verification strategy |
| Story 2.5 spec | Prerequisites updated |
| Story 2.1 spec | Stage count and constant references updated |

### Technical Impact

**Removed:**
- `internal/pose/videointel.go` — Video Intelligence API implementation
- `internal/pose/videointel_test.go` and integration test
- `internal/pipeline/stages/pose.go` and test — server-side pose stage
- `cloud.google.com/go/videointelligence` Go dependency
- `GOOGLE_APPLICATION_CREDENTIALS` env var requirement
- GCP service account setup

**Added:**
- ml5.js CDN script in upload flow
- Client-side pose estimation JavaScript (from spike)
- Upload handler accepts `keypoints` multipart field
- Pose progress UI in upload modal

**Simplified:**
- `internal/pose/` package — reduced to shared types only (`pose.go`)
- Configuration — no required env vars
- Infrastructure — no GCP project/credentials needed

## 3. Recommended Approach

**Direct Adjustment** — rewrite Story 2.4, update supporting artifacts, remove obsolete code.

- **Effort:** Medium
- **Risk:** Low (spike-validated, format-compatible)
- **Timeline impact:** Neutral to positive (simpler infrastructure)

**Rationale:** The change is contained within Epic 2. The keypoints.json format is identical, so downstream stages (crop, skeleton, metrics) are unaffected. The spike validates the approach works. Removing the GCP dependency simplifies deployment and eliminates cost.

## 4. Implementation Handoff

**Scope:** Minor — direct implementation by development team.

**Implementation order:**
1. Implement rewritten Story 2.4 (client-side pose + upload modification + trim integration)
2. Server-side pose code removal happens as part of Story 2.4 tasks
3. Continue with Story 2.5 (crop) which reads keypoints.json from upload

**Success criteria:**
- Upload modal shows pose estimation progress after video selection
- Video + keypoints.json uploaded together as multipart
- Server pipeline runs 5 stages (no server-side pose stage)
- Downstream stages (crop, skeleton, metrics) consume keypoints.json identically
- All verification tests pass (Go unit tests, ChromeDP browser tests)
