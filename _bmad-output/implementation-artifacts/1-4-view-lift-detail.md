# Story 1.4: View Lift Detail

Status: ready-for-dev

## Story

As a lifter,
I want to tap any lift and see its video with basic information,
so that I can review what I recorded.

## Acceptance Criteria (BDD)

1. **Given** the lifter is on the lift list page, **When** they tap a lift row, **Then** the lift detail page loads showing the video player with the clean video, **And** the page displays the lift type and date, **And** the video player is full-width edge-to-edge with no horizontal padding, **And** a back button is visible for returning to the list (navigation tier: no background, charcoal, 44px target), **And** the page loads within 1 second (NFR2)

2. **Given** the lifter is on the lift detail page, **When** they tap the video, **Then** video playback begins within 1 second (NFR3)

3. **Given** the lifter is on the lift detail page, **When** they tap the back button or use browser back, **Then** they return to the lift list page

## Tasks / Subtasks

- [ ] Create `web/templates/pages/lift-detail.html` (AC: 1, 2, 3)
  - [ ] Extends base layout
  - [ ] Back button at top: navigation tier styling (no background, charcoal #2D2D2D text, 44px touch target), links to GET /
  - [ ] Lift type and date displayed (text-xl semibold for title)
  - [ ] Video player container: full-width, edge-to-edge (no horizontal padding on video, remove p-4 for this element)
  - [ ] HTML5 `<video>` element with `controls`, `playsinline`, `preload="metadata"`
  - [ ] Video src points to the best available video file (clean video at this stage = original.mp4)
  - [ ] Placeholder sections for future content: coaching card, phase timeline, metrics grid (empty divs with IDs for HTMX swap targets)
  - [ ] Direction E layout order: video -> coaching -> phase timeline -> metrics

- [ ] Create `web/templates/partials/video-player.html` — basic video player (AC: 1, 2)
  - [ ] `<div class="relative w-full">` container with position relative
  - [ ] `<video>` element: full width, `playsinline` for mobile, `preload="metadata"` for fast start
  - [ ] Video source determined server-side: serve best available (original.mp4 for now)
  - [ ] Sticky positioning while scrolling (`sticky top-0 z-10`)

- [ ] Add detail handler in `internal/handler/lift.go` — `HandleGetLift` (AC: 1)
  - [ ] `GET /lifts/{id}` handler
  - [ ] Parse lift ID from URL path using Go 1.22 `r.PathValue("id")`
  - [ ] Query lift by ID via sqlc `GetLift`
  - [ ] Return 404 if lift not found
  - [ ] Determine best available video: check file existence (cropped > trimmed > original)
  - [ ] Pass lift data + video path to template
  - [ ] Render `lift-detail.html`

- [ ] Update `internal/handler/routes.go` (AC: 1)
  - [ ] Ensure `GET /lifts/{id}` route is registered

- [ ] Write tests (AC: 1, 2, 3)
  - [ ] Test GET /lifts/{id} with valid ID returns 200 and renders detail page
  - [ ] Test GET /lifts/{id} with invalid ID returns 404
  - [ ] Test that video source points to correct file
  - [ ] Test back button link points to /

## Dev Notes

- The video player at this stage is a basic HTML5 video element showing the clean (original) video. Toggle and speed controls are added in Story 3.3.
- Use `playsinline` attribute to prevent fullscreen on iOS/mobile Chrome. Use `preload="metadata"` for fast initial load (NFR3).
- Best available video logic: check file existence in priority order (cropped.mp4 > trimmed.mp4 > original.mp4). At this stage in development, only original.mp4 will exist.
- The lift detail page serves as the container for all future sections (coaching, timeline, metrics). Set up HTMX swap target divs now.
- Video is served as a static file from the data directory (file serving established in Story 1.3).
- Direction E layout (UX-DR12): video at top (edge-to-edge), then coaching section, then phase timeline, then metrics grid — all single column with gap-4 between sections.

### Project Structure Notes

Files to create:
- `web/templates/pages/lift-detail.html` — lift detail page
- `web/templates/partials/video-player.html` — basic video player partial

Files to modify:
- `internal/handler/lift.go` — add HandleGetLift handler
- `internal/handler/routes.go` — register GET /lifts/{id}

### References

- [Source: architecture.md#API & Communication Patterns] — GET /lifts/{id} route
- [Source: architecture.md#Frontend Architecture] — template organization, video-player partial
- [Source: epics.md#Story 1.4] — acceptance criteria
- [Source: ux-design-specification.md] — UX-DR4 (video player), UX-DR12 (Direction E layout), UX-DR13 (button hierarchy), UX-DR16 (spacing)

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
