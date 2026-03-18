# Story 2.4s: Verify Video Intelligence API Integration (Spike)

Status: ready-for-dev

## Story

As a developer,
I want to verify that the Google Cloud Video Intelligence API returns valid pose landmarks for the sample lift video,
so that Story 2.4 can be implemented with confidence that the API call works end-to-end.

## Context

A previous dev agent attempted Story 2.4 (Pose Estimation) but did not run the required integration test against the real API. This spike isolates the API verification into a standalone task: add the Go dependency, write a minimal integration test, run it, debug any errors, and confirm the API returns usable pose data. No pipeline wiring, no stage implementation, no `keypoints.json` writing — just prove the API call works.

## Acceptance Criteria (BDD)

1. **Given** the `cloud.google.com/go/videointelligence/apiv1` dependency is added to `go.mod`, **When** `go mod tidy` runs, **Then** the module resolves and compiles without errors

2. **Given** a minimal Video Intelligence client is created using Application Default Credentials, **When** `videointelligence.NewClient(ctx)` is called on the VPS (where `GOOGLE_APPLICATION_CREDENTIALS` is configured), **Then** the client is created successfully without error

3. **Given** the sample video `testdata/videos/sample-lift.mp4` (27MB) is sent to the API as inline content with `PERSON_DETECTION` + `IncludePoseLandmarks: true`, **When** the LRO completes, **Then** the response contains at least 1 person detection annotation, **And** at least 1 frame has landmarks, **And** all landmark coordinates are normalized (0.0-1.0), **And** bounding boxes have `Left < Right` and `Top < Bottom`

4. **Given** the integration test passes, **When** the agent reviews the API response, **Then** the agent logs and documents: (a) the exact landmark names returned by the API, (b) total frame count, (c) average landmarks per frame, (d) whether multiple persons were detected, (e) any unexpected response structure — and records these findings in the Dev Agent Record section of this story

## Prerequisites

- Google Cloud project with Video Intelligence API enabled
- `GOOGLE_APPLICATION_CREDENTIALS` env var pointing to service account JSON key file (pre-configured on VPS per `docs/gcp-credentials-setup.md`)
- `testdata/videos/sample-lift.mp4` exists (27MB snatch video)

## Tasks / Subtasks

### Task 1: Add Video Intelligence Go dependency

- [ ] Run `go get cloud.google.com/go/videointelligence/apiv1` and `go mod tidy`
- [ ] Verify it compiles: `go build ./...`
- [ ] If the package path has changed or the API is deprecated, find the correct current package and document it

### Task 2: Write integration test

- [ ] Create `internal/pose/videointel_integration_test.go` with build tag `//go:build integration`
- [ ] The test should be a single self-contained function — no interfaces, no abstractions, no production code. This is a minimal verification test; Story 2.4 will extend it.
- [ ] Use `package pose_test` as the package declaration (external test — no production code exists in this directory yet)
- [ ] Implementation:
  1. Imports — use these exact paths:
     ```go
     import (
         videointelligence "cloud.google.com/go/videointelligence/apiv1"
         videointelligencepb "cloud.google.com/go/videointelligence/apiv1/videointelligencepb"
     )
     ```
  2. Create client: `videointelligence.NewClient(ctx)` — fail test if error, then `defer client.Close()`
  3. Read video bytes: `os.ReadFile("../../testdata/videos/sample-lift.mp4")` — fail test if error
  4. Build request:
     ```go
     req := &videointelligencepb.AnnotateVideoRequest{
         InputContent: videoData,
         Features:     []videointelligencepb.Feature{videointelligencepb.Feature_PERSON_DETECTION},
         VideoContext: &videointelligencepb.VideoContext{
             PersonDetectionConfig: &videointelligencepb.PersonDetectionConfig{
                 IncludePoseLandmarks: true,
                 IncludeBoundingBoxes: true,
             },
         },
     }
     ```
  5. Call `client.AnnotateVideo(ctx, req)` to get LRO, then `op.Wait(ctx)` — fail test if error
  6. Assert: `len(resp.AnnotationResults) > 0`
  7. Assert: `len(resp.AnnotationResults[0].PersonDetectionAnnotations) > 0`
  8. **Response traversal** — the structure has an intermediate `Tracks` level:
     ```
     resp.AnnotationResults[0].PersonDetectionAnnotations[i].Tracks[j].TimestampedObjects[k]
     ```
     Assert the first annotation has at least 1 track (`len(.Tracks) > 0`), then iterate `Tracks[0].TimestampedObjects`:
     - Log `TimeOffset` converted to ms via `tsObj.TimeOffset.AsDuration().Milliseconds()` (it's a `*durationpb.Duration`, not a plain int)
     - Log `NormalizedBoundingBox` values
     - Assert bounding box values in 0.0-1.0 range
     - Assert `Left < Right` and `Top < Bottom`
     - Log each `Landmark`: `Name`, `Point.X`, `Point.Y`, `Confidence`
     - Assert landmark coordinates in 0.0-1.0 range
  9. Log summary: total persons detected (len of `PersonDetectionAnnotations`), total frames (`TimestampedObjects`), unique landmark names, average landmarks per frame
- [ ] If multiple persons are detected, log a count and iterate only the first person's track (just for verification)

### Task 3: Run the test and debug

- [ ] Run: `go test -tags=integration -v -timeout=5m -run TestVideoIntelligence ./internal/pose/...`
- [ ] **This is the critical task.** The agent MUST actually run this test on the VPS and observe the output.
- [ ] If the test fails:
  - **Auth error:** Check `echo $GOOGLE_APPLICATION_CREDENTIALS` and `cat "$GOOGLE_APPLICATION_CREDENTIALS" | head -3`. Verify the env var is set and the file contains `"type": "service_account"`.
  - **API not enabled:** The error message will say "API not enabled". The agent should report this — it requires manual GCP console action.
  - **Package import error:** The Go client library path may have changed. Search for the current package: `go doc cloud.google.com/go/videointelligence` or check Go package registry.
  - **Proto field name mismatch:** The protobuf field names (e.g., `IncludePoseLandmarks`) may differ from documentation. Check the actual generated proto struct fields. Also verify the response traversal path — `PersonDetectionAnnotations[i].Tracks[j].TimestampedObjects[k]` — the `Tracks` intermediate level is critical.
  - **Wrong protobuf import:** If `videointelligencepb` doesn't resolve, the import path may have changed. The expected path is `cloud.google.com/go/videointelligence/apiv1/videointelligencepb`. Do NOT use the legacy genproto path (`google.golang.org/genproto/googleapis/cloud/videointelligence/v1`).
  - **Inline content too large:** 27MB should be within gRPC limits but if rejected, try with a shorter test clip extracted via `ffmpeg -y -ss 6 -t 5 -i testdata/videos/sample-lift.mp4 -c copy /tmp/short-clip.mp4`
  - **Timeout:** The LRO can take 30-120s. The 5-minute test timeout should be sufficient. If it times out, increase to `-timeout=10m`.
  - **Any other error:** Log the full error, examine the response structure, and fix the test code accordingly
- [ ] Iterate until the test passes. Do not move on until green.
- [ ] Cost: ~$0.10 per run (acceptable for verification)

### Task 4: Document findings

- [ ] In the Dev Agent Record section of THIS story file, record:
  - Exact landmark names returned by the API (list all unique names observed)
  - Total frame count for the sample video
  - Number of persons detected
  - Average landmarks per frame
  - Any surprises or deviations from what Story 2.4 assumes
  - The exact Go import path that worked
  - The exact protobuf field names used (in case they differ from docs)
- [ ] These findings will be used by the dev agent implementing the full Story 2.4

## Dev Notes

- **This is a spike, not production code.** The integration test file is the only deliverable. No interfaces, no types, no pipeline stage. Story 2.4 will create the production code using the findings from this spike.
- **The integration test file will be reused.** Story 2.4 Task 7 requires `internal/pose/videointel_integration_test.go` — the file created here becomes the foundation for that test. The dev agent for 2.4 can extend it.
- **Do NOT create `internal/pose/client.go` or `internal/pose/videointel.go`.** Those belong to Story 2.4. This spike only creates the integration test file.
- **API pattern:** `AnnotateVideo` returns a long-running operation (LRO). `op.Wait(ctx)` blocks until the API finishes processing. For a 27MB/~27s video, expect 30-120 seconds.
- **Authentication:** The Go client library reads `GOOGLE_APPLICATION_CREDENTIALS` automatically via Application Default Credentials. No API key, no code to load credentials. If the env var isn't set, `NewClient` returns a clear error.
- **Inline content limit:** The Video Intelligence API accepts video as inline bytes (`InputContent` field). The gRPC message size limit is typically ~20MB for REST but higher for gRPC. At 27MB, the sample video might hit a limit — if so, trim it first (see Task 3 debugging notes).
- **Landmark names:** Story 2.4 assumes 17 COCO-format landmarks (nose, left_eye, right_eye, etc.) but the API may return different names (e.g., `"LEFT_SHOULDER"` vs `"left_shoulder"`). This spike discovers the actual names.

### Architecture Compliance

- This is a spike — no production code, no pipeline stage, no interface implementation.
- The only file created is an integration test with `//go:build integration` tag, so it never runs in normal `go test ./...`.
- The file lives at `internal/pose/videointel_integration_test.go` — same location Story 2.4 expects, so it's already in the right place.

### Project Structure Notes

New files to create:
- `internal/pose/videointel_integration_test.go` — integration test (build tag: `integration`)

Files modified:
- `go.mod` / `go.sum` — new dependency: `cloud.google.com/go/videointelligence/apiv1`

No other files should be created or modified.

### References

- [Source: docs/gcp-credentials-setup.md] — credential file location and verification
- [Source: 2-4-pose-estimation.md#Task 7] — integration test requirements this spike fulfills
- [Source: 2-4-pose-estimation.md#Task 3] — Video Intelligence API call pattern
- [Source: 2-4-pose-estimation.md#Dev Notes] — API details, landmark format, inline content

## Dev Agent Record

### Agent Model Used
Claude Opus 4.6 (1M context)

### Completion Notes List
- 2026-03-18: Spike completed. API integration verified with 3 feature configurations.
- CRITICAL FINDING: `IncludePoseLandmarks: true` causes "Calculator failure" (code=2) server-side.
- PERSON_DETECTION with bounding boxes only WORKS. LABEL_DETECTION WORKS.
- Story 2.4 CANNOT rely on Video Intelligence API for pose landmarks. Needs alternative approach.

### Change Log
- Rewrote `internal/pose/videointel_integration_test.go` as self-contained external test (`package pose_test`)
- Split into 3 test functions: LabelDetection, PersonDetection_BBoxOnly, PersonDetection_PoseLandmarks
- Each test directly calls the API without production code wrappers

### API Findings

**Go import path:** `cloud.google.com/go/videointelligence/apiv1` (client) and `cloud.google.com/go/videointelligence/apiv1/videointelligencepb` (proto types). Both resolve correctly from go.mod.

**Landmark names returned by API:** NONE — `IncludePoseLandmarks: true` causes a server-side "Calculator failure" (gRPC code=2, UNKNOWN). No landmarks are returned.

**Frame count for sample video:** N/A for landmarks. With bbox-only person detection on a 4.1MB re-encoded version of sample-lift.mp4: 1 person annotation returned (tracks and timestamped objects available).

**Persons detected:** 1 (with IncludeBoundingBoxes only). 0 with IncludePoseLandmarks (Calculator failure returns empty results).

**Average landmarks per frame:** N/A — no landmarks returned.

**Protobuf field names used:**
- Request: `AnnotateVideoRequest{InputContent, Features: []Feature{Feature_PERSON_DETECTION}, VideoContext: &VideoContext{PersonDetectionConfig: &PersonDetectionConfig{IncludePoseLandmarks, IncludeBoundingBoxes}}}`
- Response: `AnnotateVideoResponse.AnnotationResults[0].PersonDetectionAnnotations[i].Tracks[j].TimestampedObjects[k]`
  - `.TimeOffset` — `*durationpb.Duration`, use `.AsDuration().Milliseconds()`
  - `.NormalizedBoundingBox` — `{Left, Top, Right, Bottom}` (all float32, normalized 0-1)
  - `.Landmarks[]` — `{Name string, Point *NormalizedVertex{X,Y}, Confidence float32}` (never populated due to Calculator failure)
- Error field: `AnnotationResults[0].Error` — `*status.Status{Code int32, Message string}`

**Surprises / deviations from Story 2.4 assumptions:**

1. **CRITICAL: Pose landmarks are broken.** `IncludePoseLandmarks: true` triggers "Calculator failure" (code=2) on the server. This is NOT a client-side issue — the request is accepted, the LRO runs for ~1-2 minutes, then returns an error in `AnnotationResults[0].Error`. This was tested with multiple video sizes (317KB, 4.1MB, 27MB) — all fail identically.

2. **Person detection bounding boxes DO work.** `IncludeBoundingBoxes: true` (without landmarks) returns valid person tracking with normalized bounding boxes. The response structure matches the expected `Tracks[].TimestampedObjects[]` pattern.

3. **Label detection works.** Confirms the API is enabled, credentials are valid, and the service account has access. Returns segment and shot labels correctly.

4. **Original video (27MB) also causes Calculator failure.** Both the original and re-encoded versions fail with landmarks enabled. The 27MB version is slower (~10 min LRO) but the error is the same.

5. **GOOGLE_APPLICATION_CREDENTIALS env var was not set in shell** but the credential file exists at `~/.config/press-out/gcp-sa.json`. The test requires setting it explicitly.

6. **LRO latency is high.** Even a 4.1MB video takes 60-100 seconds for person detection. The original 27MB video takes 2-10 minutes.

**Recommendation for Story 2.4:** The Video Intelligence API cannot provide pose landmarks. Story 2.4 must either:
- (a) Use only bounding boxes from Video Intelligence API (no body keypoints)
- (b) Switch to a local pose estimation library (MediaPipe Pose, MoveNet via TensorFlow Lite, or OpenPose)
- (c) Use a different cloud API that supports pose estimation (e.g., AWS Rekognition, Azure AI Vision)

### File List
- `internal/pose/videointel_integration_test.go` — rewritten as external test with 3 diagnostic subtests
