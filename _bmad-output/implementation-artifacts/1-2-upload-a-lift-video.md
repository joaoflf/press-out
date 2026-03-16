# Story 1.2: Upload a Lift Video

Status: done

## Story

As a lifter,
I want to upload a video from my phone and assign a lift type,
so that the system stores my lift for later analysis.

## Acceptance Criteria (BDD)

1. **Given** the lifter is on the lift list page, **When** they tap the upload button, **Then** an upload modal opens with dark backdrop overlay (DaisyUI modal), **And** the modal contains a file selector zone, a 3-option lift type selector (Snatch / Clean / C&J as a join group, all visible), and a full-width submit button (h-12, sage accent), **And** the submit button is disabled until both a video file and lift type are selected

2. **Given** the upload modal is open with a video selected and lift type chosen, **When** the lifter taps submit, **Then** the modal closes immediately, **And** the video file is persisted to the filesystem in a lift-ID directory (data/lifts/{id}/original.mp4) before any processing begins (NFR10), **And** a SQLite record is created with the lift type and timestamp, **And** the new lift appears at the top of the lift list

3. **Given** the upload modal is open, **When** the lifter selects a file that is not a video (not mp4/mov) or exceeds ~300MB, **Then** the upload is rejected with appropriate feedback, **And** no file is stored and no database record is created

4. **Given** the upload modal is open, **When** the lifter taps outside the modal or taps X, **Then** the modal closes without uploading

## Tasks / Subtasks

- [ ] Create `web/templates/partials/upload-modal.html` (AC: 1)
  - [ ] DaisyUI modal shell with `<dialog>` element and dark backdrop
  - [ ] Native file input with `accept="video/mp4,video/quicktime,.mp4,.mov"`
  - [ ] 3-option DaisyUI `join` group for lift type: Snatch, Clean, C&J — all visible, radio button behavior
  - [ ] Full-width submit button: h-12, sage accent (#8BA888), white text
  - [ ] Submit button disabled by default with `disabled` attribute
  - [ ] Close button (X) in modal header
  - [ ] Form posts to `POST /lifts` with `enctype="multipart/form-data"`

- [ ] Add client-side validation in `web/static/app.js` (AC: 1, 3)
  - [ ] Enable submit button only when both file and lift type are selected
  - [ ] Listen to file input `change` and radio `change` events
  - [ ] Client-side file type check (mp4/mov extension or MIME type)

- [ ] Create upload handler in `internal/handler/lift.go` — `HandleCreateLift` (AC: 2, 3)
  - [ ] `POST /lifts` handler
  - [ ] Use `http.MaxBytesReader` to limit upload to ~300MB
  - [ ] Parse multipart form: extract video file and lift_type field
  - [ ] Server-side validation: check MIME type/extension for mp4/mov
  - [ ] On validation failure: return appropriate HTTP response (no file stored, no DB record)
  - [ ] On success: create SQLite record via sqlc `CreateLift(lift_type, created_at)` to get lift ID
  - [ ] Create lift directory via `storage.CreateLiftDir(dataDir, liftID)`
  - [ ] Stream uploaded file to `storage.LiftFile(dataDir, liftID, storage.FileOriginal)` — persist BEFORE any processing (NFR10)
  - [ ] Redirect to lift list page (or return HTMX partial for list update)

- [ ] Wire upload button in `lift-list.html` to open modal (AC: 1, 4)
  - [ ] Upload button triggers `<dialog>.showModal()` via onclick or HTMX
  - [ ] Clicking outside modal or X closes it via `<dialog>` native behavior

- [ ] Update `internal/handler/routes.go` to register `POST /lifts` route (AC: 2)

- [ ] Write tests (AC: 1, 2, 3, 4)
  - [ ] `internal/handler/lift_test.go` — test POST /lifts with valid video upload
  - [ ] Test POST /lifts with invalid file type returns error
  - [ ] Test POST /lifts with oversized file returns error
  - [ ] Test that file is persisted to correct path
  - [ ] Test that SQLite record is created with correct lift_type and timestamp

## Dev Notes

- Upload flow: form submit -> server validates -> create DB record -> create lift dir -> stream file to disk -> redirect. File MUST be persisted before any processing (NFR10).
- Use `http.MaxBytesReader(w, r.Body, 300*1024*1024)` to cap upload size. This returns a 413 if exceeded.
- The `<dialog>` element is natively supported in Chrome — no JS library needed for modal behavior. DaisyUI provides styling.
- The lift type join group uses DaisyUI's `join` component with radio inputs styled as buttons. All three options visible at once (not a dropdown).
- After successful upload, the lift appears at the top of the list. This can be done via full page redirect to GET / or via HTMX partial swap.
- File is saved as `original.mp4` regardless of the original filename. The lift-ID directory isolates each upload.
- Pipeline triggering happens in Story 2.1 — this story only handles upload and storage.

### Project Structure Notes

Files to create:
- `web/templates/partials/upload-modal.html` — upload form modal

Files to modify:
- `internal/handler/lift.go` — add `HandleCreateLift` handler
- `internal/handler/routes.go` — register POST /lifts
- `web/templates/pages/lift-list.html` — add upload button -> modal trigger
- `web/static/app.js` — add file/lift-type validation for submit button state

### References

- [Source: architecture.md#Authentication & Security] — file upload validation, MaxBytesReader, MIME check
- [Source: architecture.md#API & Communication Patterns] — POST /lifts route
- [Source: architecture.md#Data Architecture] — lift-ID directory structure, file naming
- [Source: epics.md#Story 1.2] — acceptance criteria
- [Source: ux-design-specification.md] — UX-DR10 (upload modal), UX-DR13 (button hierarchy)

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
