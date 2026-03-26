# Story 3.2: Video Player with Toggle & Speed Controls

Status: ready-for-dev

## Story

As a lifter,
I want to toggle between skeleton and clean video and control playback speed,
so that I can analyze my lift in detail at my own pace.

## Acceptance Criteria

1. **Given** the lifter opens a lift detail page where both skeleton.mp4 and the clean video exist, **When** the page loads, **Then** the skeleton overlay video auto-plays (UX-DR17 — skeleton is the default view), **And** a mode badge in the bottom-right corner displays "Skeleton", **And** the video player is full-width edge-to-edge with sticky positioning while scrolling

2. **Given** the lifter is watching the skeleton video, **When** they tap anywhere on the video surface, **Then** the video source swaps to the clean video (or vice versa), **And** playback continues from the same timestamp and speed, **And** the mode badge updates to reflect the current mode ("Skeleton" or "Clean"), **And** the toggle completes within 500 milliseconds (NFR4)

3. **Given** the video player is visible, **When** the lifter views the speed controls, **Then** a floating speed strip is visible at the bottom of the video with three options: 0.25x, 0.5x, 1x, **And** the strip has a subtle gradient backdrop (bg-gradient-to-t from-black/40), **And** no speed is pre-selected on load (video plays at 1x)

4. **Given** the lifter taps a speed button, **When** the speed is selected, **Then** the video playback rate changes immediately via the HTML5 playbackRate API, **And** the selected button shows sage accent, others lose accent

5. **Given** only the clean video exists (skeleton rendering was skipped), **When** the lifter opens the lift detail page, **Then** the clean video plays without a toggle option, **And** the mode badge is not displayed, **And** the speed controls remain functional

## Tasks / Subtasks

- [ ] Update `LiftDetailData` in `internal/handler/lift.go` to include skeleton video info (AC: 1, 5)
  - [ ] Add `SkeletonSrc string` and `HasSkeleton bool` fields to `LiftDetailData`
  - [ ] In `HandleGetLift`, check if `skeleton.mp4` exists via `os.Stat(storage.LiftFile(...))`
  - [ ] If exists, set `SkeletonSrc` to `/data/lifts/{id}/skeleton.mp4` and `HasSkeleton = true`
  - [ ] When `HasSkeleton`, set `VideoSrc` to the skeleton video (skeleton is the default view per UX-DR17)
  - [ ] Add `CleanSrc string` field — always points to the best clean video (cropped > trimmed > original via existing `bestVideoFile()`)

- [ ] Update `web/templates/partials/video-player.html` (AC: 1, 2, 3, 5)
  - [ ] Add `data-skeleton-src` and `data-clean-src` attributes on the video element (only when `HasSkeleton`)
  - [ ] Set video `src` to `{{.VideoSrc}}` (skeleton when available, clean otherwise)
  - [ ] Add `autoplay muted loop` attributes when skeleton exists (auto-play per UX-DR17; muted required for autoplay on mobile Chrome)
  - [ ] Add floating speed strip: three buttons (0.25x, 0.5x, 1x) absolutely positioned at bottom with `bg-gradient-to-t from-black/40` gradient backdrop
  - [ ] Add mode badge: small DaisyUI badge in bottom-right corner showing "Skeleton" or "Clean" (only rendered when `HasSkeleton`)
  - [ ] Make the video surface the tap target for toggle (overlay div on top of video, below speed controls)
  - [ ] Speed buttons: `h-10`, transparent background, sage accent when active

- [ ] Update `web/static/app.js` with video player logic (AC: 2, 3, 4)
  - [ ] Toggle handler: listen for taps on the video toggle overlay
  - [ ] On toggle: save `currentTime` and `playbackRate`, swap `video.src` between skeleton and clean URLs, restore `currentTime` and `playbackRate` after `loadedmetadata` event, update mode badge text
  - [ ] Speed handler: listen for taps on speed buttons, set `video.playbackRate`, update active button styling (add/remove sage accent classes)
  - [ ] No speed pre-selected on load — all buttons start unstyled, 1x speed button does not have active styling until tapped
  - [ ] Prevent toggle tap from triggering native video play/pause

- [ ] ChromeDP e2e tests in `test/e2e/` (AC: 1-5)
  - [ ] Test: speed strip with three buttons (0.25x, 0.5x, 1x) is visible on detail page
  - [ ] Test: mode badge displays "Skeleton" when skeleton.mp4 exists
  - [ ] Test: mode badge is NOT displayed when only clean video exists
  - [ ] Test: speed strip gradient backdrop is rendered
  - [ ] Test: video auto-plays when skeleton exists (check `autoplay` attribute)

## Dev Notes

### Architecture Compliance

- **No new packages or files** beyond modifying existing ones (handler, template, JS, tests)
- **No new routes** — video files are already served via the static file handler at `/data/lifts/`
- **Template naming**: `video-player` template name is already defined (`{{define "video-player"}}`)
- **Template data struct**: `LiftDetailData` (defined in `internal/handler/lift.go:176-183`)
- **Storage constants**: `storage.FileSkeleton = "skeleton.mp4"` (already in `internal/storage/storage.go:14`)
- **File path construction**: always via `storage.LiftFile()` — never inline

### Current State of Files to Modify

**`internal/handler/lift.go`** — `LiftDetailData` struct (line 176-183):
```go
type LiftDetailData struct {
    ID          int64
    LiftType    string
    DisplayType string
    DisplayDate string
    VideoSrc    string
    Processing  bool
}
```
Add three fields: `SkeletonSrc string`, `CleanSrc string`, `HasSkeleton bool`.

In `HandleGetLift` (line 186-215):
- `bestVideoFile()` already returns best clean video (cropped > trimmed > original)
- Check `storage.LiftFile(s.DataDir, lift.ID, storage.FileSkeleton)` with `os.Stat`
- If skeleton exists: `VideoSrc = skeleton path` (default), `SkeletonSrc = skeleton path`, `CleanSrc = clean path`
- If skeleton doesn't exist: `VideoSrc = clean path`, `HasSkeleton = false`

**`web/templates/partials/video-player.html`** — currently minimal (5 lines):
```html
{{define "video-player"}}
<div class="relative w-full sticky top-0 z-10 bg-black max-h-[50vh]">
    <video class="w-full max-h-[50vh] object-contain" controls playsinline preload="metadata" src="{{.VideoSrc}}"></video>
</div>
{{end}}
```
Expand to include: toggle overlay, speed strip, mode badge. Use `{{if .HasSkeleton}}` for conditional rendering.

**`web/static/app.js`** — currently only handles upload form (57 lines). Add video player logic in a separate section, guarded by checking for the video element's data attributes.

### Video Toggle Implementation

**The swap technique** (NFR4: < 500ms):
```javascript
// 1. Save state
var time = video.currentTime;
var rate = video.playbackRate;
var wasPaused = video.paused;

// 2. Swap src
video.src = isSkeleton ? cleanSrc : skeletonSrc;
isSkeleton = !isSkeleton;

// 3. Restore on loadedmetadata
video.addEventListener('loadedmetadata', function restore() {
    video.removeEventListener('loadedmetadata', restore);
    video.currentTime = time;
    video.playbackRate = rate;
    if (!wasPaused) video.play();
}, {once: true});

// 4. Load new source
video.load();
```

The `loadedmetadata` event fires once enough metadata is loaded to know duration — this is fast because both videos are local files with identical duration. No network round-trip.

### Speed Control Implementation

- Three buttons with `data-speed` attribute: `0.25`, `0.5`, `1`
- On tap: `video.playbackRate = parseFloat(btn.dataset.speed)`
- Active state: toggle class `text-sage` (or appropriate Tailwind class for the sage accent color #8BA888)
- No default active — video starts at 1x but 1x button is NOT highlighted until tapped

### Tap Target for Toggle

The video surface itself is the toggle target (UX-DR4). However, the native `<video controls>` element intercepts taps for play/pause. Two approaches:

**Approach: Overlay div** — Place a transparent div over the video that captures taps for toggle. Leave the native controls bar at the bottom untouched. The overlay div sits between the video and the speed strip.

```html
<div class="absolute inset-0 z-10 cursor-pointer" id="video-toggle-overlay"></div>
```

The overlay captures taps on the video surface. The native controls bar at the bottom is still accessible because it has higher z-index by default in Chrome. The speed strip also needs a higher z-index.

**Important**: The overlay must NOT cover the native controls bar. Use `bottom: 60px` or similar to leave room for the controls, OR remove native controls entirely and rely on the speed strip + tap for play/pause. Per UX spec, native scrub bar is kept ("Standard browser behavior — frame-accurate seeking").

**Recommended**: Keep `controls` attribute on video. The overlay div covers the video above the controls area. Speed strip floats at the bottom, above the controls. The toggle fires on the overlay, not on the video element itself.

### Auto-play Behavior

Per UX-DR17: skeleton video auto-plays on detail load. Chrome mobile requires `muted` attribute for autoplay to work without user gesture. Add `autoplay muted` to the video element when skeleton exists. The `loop` attribute is also appropriate since lifts are short (5-20s) and the lifter reviews repeatedly.

When only clean video exists, do NOT auto-play — use current behavior (`preload="metadata"`, manual play).

### Graceful Degradation (AC: 5)

When `HasSkeleton` is false:
- `VideoSrc` = best clean video (existing `bestVideoFile()` logic)
- No `data-skeleton-src` / `data-clean-src` attributes
- No mode badge rendered
- No toggle overlay rendered
- Speed strip still rendered and functional
- Video element does NOT have `autoplay` — plays on tap

### Existing E2E Test Patterns

Tests are in `test/e2e/`. Key patterns:
- `startTestEnv(t)` creates a full server with temp DB, templates, routes (defined in `test/e2e/lift_detail_test.go:29-109`)
- `createTestLift(t, env, liftType, createdAt)` creates a lift with a fake video file (line 112-129)
- `newBrowserCtx(t)` creates a headless Chrome context (defined in `test/e2e/theme_test.go:130-149`)
- Assertions use `chromedp.Evaluate` for DOM queries and `chromedp.AttributeValue` for attribute checks

For tests needing a skeleton video, write a fake `skeleton.mp4` file alongside the `original.mp4`:
```go
skeletonPath := storage.LiftFile(env.DataDir, liftID, storage.FileSkeleton)
os.WriteFile(skeletonPath, []byte("fake skeleton data"), 0644)
```

### What NOT to Do

- **No custom video player** — use native HTML5 `<video>` with `controls` attribute
- **No JavaScript framework or library** — vanilla JS only, added to existing `app.js`
- **No changes to storage.go, pipeline, or any backend processing** — this is purely handler + template + JS
- **No new routes** — video files are already served at `/data/lifts/{id}/filename`
- **No preloading of both videos** — only load the active video; swap on toggle
- **No CSS transitions on video swap** — instant swap per UX spec ("no CSS transitions between content states")
- **Do not remove native video controls** — UX spec explicitly says "native scrub bar works well enough on Chrome mobile"
- **Do not add play/pause toggle on tap** — tap on video surface is exclusively for skeleton/clean toggle (when skeleton exists)

### Project Structure Notes

Modified files:
- `internal/handler/lift.go` — add fields to `LiftDetailData`, update `HandleGetLift`
- `web/templates/partials/video-player.html` — expand with toggle, speed strip, mode badge
- `web/static/app.js` — add video toggle and speed control logic
- `test/e2e/` — new test file for video player e2e tests (e.g., `test/e2e/video_player_test.go`)

No new files besides test file.

### References

- [Source: internal/handler/lift.go:176-183] — LiftDetailData struct
- [Source: internal/handler/lift.go:186-215] — HandleGetLift handler
- [Source: internal/handler/lift.go:219-226] — bestVideoFile() function
- [Source: internal/storage/storage.go:14] — FileSkeleton constant
- [Source: web/templates/partials/video-player.html] — current video player template
- [Source: web/static/app.js] — current JavaScript
- [Source: web/templates/pages/lift-detail.html] — lift detail page
- [Source: web/templates/layouts/base.html] — base layout with HTMX + app.js
- [Source: test/e2e/lift_detail_test.go] — existing ChromeDP test patterns
- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.2] — acceptance criteria
- [Source: _bmad-output/planning-artifacts/architecture.md] — frontend architecture, app.js, template organization
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md] — UX-DR4 (video player), UX-DR17 (skeleton default), video interaction patterns

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
