# Story 1.5: Delete a Lift

Status: ready-for-dev

## Story

As a lifter,
I want to remove a lift I no longer need,
so that my lift list stays clean and relevant.

## Acceptance Criteria (BDD)

1. **Given** the lifter is on the lift detail page for a specific lift, **When** they initiate deletion of the lift, **Then** the lift's database record is removed, **And** the entire lift-ID directory is removed (original.mp4 and any other associated files), **And** the lifter is returned to the lift list page, **And** the deleted lift no longer appears in the list

2. **Given** a lift is deleted, **When** the lifter visits the lift list page, **Then** the deleted lift is not present in the list, **And** no orphaned files remain on the filesystem for that lift

## Tasks / Subtasks

- [ ] Add delete button to `web/templates/pages/lift-detail.html` (AC: 1)
  - [ ] Delete button/link on the lift detail page
  - [ ] Use HTMX `hx-delete="/lifts/{id}"` with `hx-confirm` for safety confirmation
  - [ ] Or use a form with `method="POST"` and `_method=DELETE` pattern
  - [ ] After successful deletion, redirect to lift list via `hx-redirect="/"`
  - [ ] Button styling: not primary tier — use text/icon style to avoid accidental taps

- [ ] Add delete handler in `internal/handler/lift.go` — `HandleDeleteLift` (AC: 1, 2)
  - [ ] `DELETE /lifts/{id}` handler
  - [ ] Parse lift ID from URL path
  - [ ] Delete SQLite record via sqlc `DeleteLift(id)`
  - [ ] Remove entire lift directory via `storage.RemoveLiftDir(dataDir, liftID)` — cascading delete of all files
  - [ ] Redirect to lift list page (HTTP 303 See Other or HX-Redirect header)
  - [ ] Log deletion with slog: `slog.Info("lift deleted", "lift_id", id)`

- [ ] Update `internal/handler/routes.go` (AC: 1)
  - [ ] Ensure `DELETE /lifts/{id}` route is registered

- [ ] Write tests (AC: 1, 2)
  - [ ] Test DELETE /lifts/{id} removes database record
  - [ ] Test DELETE /lifts/{id} removes lift directory and all files
  - [ ] Test DELETE /lifts/{id} with invalid ID returns 404
  - [ ] Test that after deletion, GET / does not include the deleted lift
  - [ ] Test no orphaned files remain after deletion

- [ ] Write ChromeDP browser verification tests (AC: 1, 2)
  - [ ] Start the server on a random test port, run ChromeDP against it, tear down after
  - [ ] Verify `output.css` loads successfully (no 404/network errors)
  - [ ] Verify HTMX script loads successfully (no 404/network errors)
  - [ ] Verify `app.js` loads successfully (no 404/network errors)
  - [ ] Verify DaisyUI theme is active: `<html data-theme="press-out">` attribute is present
  - [ ] Verify no JavaScript console errors on page load
  - [ ] Verify delete button is present on the lift detail page
  - [ ] Verify delete button is not primary tier styling (avoids accidental taps)
  - [ ] Verify `hx-confirm` or equivalent confirmation dialog is triggered on delete action
  - [ ] Verify that after deletion, the browser redirects to the lift list page
  - [ ] Verify the deleted lift no longer appears in the lift list

## Dev Notes

- Deletion is a destructive operation. Use HTMX `hx-confirm` to show a browser confirmation dialog before proceeding.
- `storage.RemoveLiftDir` uses `os.RemoveAll` to delete the entire lift directory. This cascades to all files: original.mp4, trimmed.mp4, cropped.mp4, skeleton.mp4, thumbnail.jpg, keypoints.json.
- Delete the DB record first, then the files. If file deletion fails, log the error but don't fail the request — the lift is already removed from the user's view.
- If a pipeline is currently processing this lift, the deletion should still proceed. The pipeline goroutine will encounter missing files and fail gracefully.
- HTMX can handle the redirect after deletion using `HX-Redirect: /` response header.

### Project Structure Notes

Files to modify:
- `internal/handler/lift.go` — add HandleDeleteLift handler
- `internal/handler/routes.go` — register DELETE /lifts/{id}
- `web/templates/pages/lift-detail.html` — add delete button

### References

- [Source: architecture.md#API & Communication Patterns] — DELETE /lifts/{id} route
- [Source: architecture.md#Project Structure & Boundaries] — cascading file deletion via storage package
- [Source: epics.md#Story 1.5] — acceptance criteria

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
