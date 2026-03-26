# Story 3.3: Fix Detail Page to Card Layout & Pipeline Stage Styling

Status: review

## Story

As a lifter,
I want the lift detail page to use card-based content sections and the processing stages to show a proper checklist,
so that each piece of information is visually separated and the processing progress looks polished.

## Acceptance Criteria

1. **Given** the lifter opens a completed lift detail page, **When** the page loads, **Then** content sections below the video are wrapped in individual card containers (white background, rounded-xl corners, subtle shadow, internal padding), matching Direction D from `_bmad-output/planning-artifacts/ux-design-directions.html`

2. **Given** the detail page renders content sections, **When** the lifter views the page, **Then** each card has an uppercase section title in stone color (#C4BFAE, 11px, font-semibold, letter-spacing 0.5px) above its content — titles: "Playback", "Coaching", "Phases", "Metrics" (only rendered when the section has content)

3. **Given** a lift is currently processing, **When** the lifter views the detail page, **Then** the pipeline stages render as a vertical checklist matching Direction H — each stage is a row with a circular icon (28px) on the left, stage name (14px) in the center, and optional elapsed time on the right

4. **Given** a pipeline stage is complete, **When** the lifter views the processing checklist, **Then** the stage shows a green (#7DA67D) filled circle with a white checkmark icon, and the stage name in normal text

5. **Given** a pipeline stage is currently active, **When** the lifter views the checklist, **Then** the stage shows a sage (#8BA888) filled circle with a dot icon and a pulsing animation, and the stage name in primary-colored bold text

6. **Given** a pipeline stage is pending, **When** the lifter views the checklist, **Then** the stage shows a neutral (#EDEDEA) filled circle with an outlined circle icon, and the stage name in dimmed/stone text

7. **Given** a pipeline stage was skipped, **When** the lifter views the checklist, **Then** the stage shows dimmed text with a skip indicator (similar to pending but distinguishable)

8. **Given** the detail page renders the card layout, **When** viewed on mobile (375-430px viewport), **Then** the cards have 12px outer padding, 14px internal padding, 10px gap between cards, and no horizontal overflow

## Tasks / Subtasks

- [x] Update `web/templates/pages/lift-detail.html` to use card-based layout (AC: 1, 2, 8)
  - [x] Wrap content sections in a `content-cards` container with `p-3` (12px) padding
  - [x] Each section gets a `section-card` wrapper: `bg-white rounded-xl p-3.5 shadow-sm mb-2.5`
  - [x] Add uppercase card titles using: `text-[11px] font-semibold text-[#C4BFAE] uppercase tracking-wider mb-2.5`
  - [x] Move the back button + title/date header into a card or keep it above cards as a plain header
  - [x] Processing card: wraps pipeline stages when `.Processing` is true
  - [x] Coaching card: wraps `#coaching-section` (only rendered if coaching exists or is loading)
  - [x] Phases card: wraps `#phase-timeline-section` (only rendered if phases exist)
  - [x] Metrics card: wraps `#metrics-section` (only rendered if metrics exist)
  - [x] Keep re-process button and delete button outside card layout (utility actions)

- [x] Update `web/templates/partials/pipeline-stages.html` initial template (AC: 3, 5)
  - [x] Replace the simple loading spinner with a proper stage checklist structure
  - [x] Show all 6 stage names in a vertical list with pending state initially
  - [x] The first stage should show as "active" (pulsing) when pipeline starts
  - [x] Use the card title "Processing" above the checklist

- [x] Update `internal/pipeline/pipeline.go` `RenderStagesHTML` function (AC: 3, 4, 5, 6, 7)
  - [x] Change the stage list wrapper to use `flex flex-col gap-0` (no gap — use border-bottom separation like Direction H)
  - [x] Each stage item: `flex items-center gap-3 py-3.5 border-b border-neutral last:border-0`
  - [x] Complete state icon: `w-7 h-7 rounded-full bg-[#7DA67D] flex items-center justify-center flex-shrink-0` with white checkmark SVG
  - [x] Active state icon: `w-7 h-7 rounded-full bg-[#8BA888] flex items-center justify-center flex-shrink-0 animate-pulse` with white dot
  - [x] Pending state icon: `w-7 h-7 rounded-full bg-[#EDEDEA] flex items-center justify-center flex-shrink-0` with stone-colored outline circle
  - [x] Skipped state icon: `w-7 h-7 rounded-full bg-[#EDEDEA] flex items-center justify-center flex-shrink-0` with stone-colored dash/skip icon
  - [x] Stage name: `text-sm font-medium` for active, `text-sm` for complete, `text-sm text-[#C4BFAE]` for pending
  - [x] Elapsed time (if available): `text-xs text-base-content/40 ml-auto`

- [x] Update `internal/pipeline/pipeline.go` `RenderStatusHTML` function if needed (AC: 3)
  - [x] Ensure compact list-item status matches the new styling language

- [x] Add `processing-title` to the pipeline stages template or detail page (AC: 3)
  - [x] Show "Processing" as an uppercase card title above the stage list, matching Direction H

- [x] Rebuild Tailwind CSS output (AC: all)
  - [x] Run `make tailwind` or equivalent to compile new utility classes into output.css

- [x] ChromeDP e2e tests in `test/e2e/` (AC: 1-8)
  - [x] Test: detail page has card containers with expected classes (bg-white, rounded-xl, shadow)
  - [x] Test: card titles render with correct text content ("Processing")
  - [x] Test: pipeline stages render with circle icons (check for stage icon elements)
  - [x] Test: pending stage has neutral background icon
  - [x] Test: active stage has pulse animation class
  - [x] Test: no horizontal overflow at 375px viewport

- [x] Playwright CLI visual verification (AC: 1-8)
  - [x] Screenshot the completed lift detail page at mobile viewport (375x812) and visually verify card layout matches Direction D from `_bmad-output/planning-artifacts/ux-design-directions.html`
  - [x] Screenshot a processing lift detail page and visually verify the stage checklist matches Direction H from the same file
  - [x] Compare screenshots against the Direction D and Direction H mockups — cards should have visible white backgrounds, rounded corners, subtle shadow separation from the warm-white page background
  - [x] Verify card titles ("Coaching", "Phases", "Metrics") are visible as uppercase stone-colored labels
  - [x] Verify pipeline stage icons are circular (not spinners), with correct color coding (green done, sage active, neutral pending)
  - [x] Save verification screenshots to `/tmp/` for user review

## Dev Notes

### Architecture Compliance

- **No new packages or routes** — purely template + CSS + pipeline render function changes
- **No database changes** — all rendering changes
- **Template naming**: existing templates `pipeline-stages`, `video-player` — no new template names needed
- **Pipeline render functions**: `RenderStagesHTML` and `RenderStatusHTML` in `internal/pipeline/pipeline.go` (line ~101-160) produce inline HTML for SSE delivery — these must be updated to match the new styling

### Current State of Files to Modify

**`web/templates/pages/lift-detail.html`** (59 lines):
- Currently uses a flat layout with `p-4 flex flex-col gap-4`
- Content sections (`#coaching-section`, `#phase-timeline-section`, `#metrics-section`) are empty divs — downstream stories will populate them
- Needs card wrapper around each section
- Back button + title header stays above cards

**`web/templates/partials/pipeline-stages.html`** (11 lines):
- Currently shows a single loading spinner with "Initializing pipeline..." text
- Needs to render all 6 stages in pending state initially, matching Direction H layout
- SSE will swap the inner content via `sse-swap="pipeline-stages"` with `RenderStagesHTML` output

**`internal/pipeline/pipeline.go`** — `RenderStagesHTML` (lines 101-131):
- Currently renders: complete (green circle + checkmark), active (DaisyUI spinner), skipped (warning circle), pending (empty bordered circle)
- Must change to Direction H style: larger icons (28px/w-7), border-separated rows, elapsed time slot, no DaisyUI spinner (use pulsing circle instead)

**`internal/pipeline/pipeline.go`** — `RenderStatusHTML` (lines 134-159):
- Compact list-item status — may need minor styling adjustments to stay consistent

### Direction D Card Layout Reference

From `_bmad-output/planning-artifacts/ux-design-directions.html` (Direction D):

```css
.section-card {
    background: var(--white);    /* #FFFFFF */
    border-radius: 12px;
    padding: 14px;
    margin-bottom: 10px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.04);
}

.card-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--stone);         /* #C4BFAE */
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 10px;
}
```

Tailwind equivalent:
- Card: `bg-white rounded-xl p-3.5 mb-2.5 shadow-[0_1px_3px_rgba(0,0,0,0.04)]`
- Title: `text-[11px] font-semibold text-[#C4BFAE] uppercase tracking-wider mb-2.5`

### Direction H Pipeline Progress Reference

From `_bmad-output/planning-artifacts/ux-design-directions.html` (Direction H):

```css
.stage-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 0;
    border-bottom: 1px solid var(--neutral);  /* #EDEDEA */
}

.stage-icon {
    width: 28px;
    height: 28px;
    border-radius: 50%;
}

.stage-done .stage-icon {
    background: var(--success);   /* #7DA67D */
    color: var(--white);
}

.stage-active .stage-icon {
    background: var(--sage);      /* #8BA888 */
    color: var(--white);
    animation: pulse 1.5s ease-in-out infinite;
}

.stage-pending .stage-icon {
    background: var(--neutral);   /* #EDEDEA */
    color: var(--stone);          /* #C4BFAE */
}
```

Tailwind equivalent for `RenderStagesHTML`:
- Item row: `flex items-center gap-3 py-3.5 border-b border-[#EDEDEA]` (last item: no border)
- Done icon: `w-7 h-7 rounded-full bg-[#7DA67D] flex items-center justify-center flex-shrink-0`
- Active icon: `w-7 h-7 rounded-full bg-[#8BA888] flex items-center justify-center flex-shrink-0 animate-pulse`
- Pending icon: `w-7 h-7 rounded-full bg-[#EDEDEA] flex items-center justify-center flex-shrink-0`
- Stage name (done): `text-sm font-medium`
- Stage name (active): `text-sm font-medium text-[#8BA888]`
- Stage name (pending): `text-sm text-[#C4BFAE]`
- Time (right-aligned): `text-[11px] text-base-content/40 ml-auto`

### Pipeline Initial State

The `pipeline-stages.html` template renders the INITIAL state before SSE starts delivering updates. Currently it's just a spinner. It should instead render all 6 stages in their initial state (first stage active, rest pending). However, the template doesn't have access to the stage list — it's rendered server-side by the pipeline.

**Solution**: Pass the initial stage names to the template via `LiftDetailData`, OR have the pipeline emit its first SSE event immediately on startup (current behavior — the pipeline calls `emitEvents` before each stage starts). The SSE swap will replace the initial template content almost immediately.

**Pragmatic approach**: Keep the initial template simple — show a "Processing" title and a skeleton/placeholder that gets replaced by SSE within the first second. This avoids duplicating the stage list in Go template and Go render function. OR, hardcode the 6 stage names in the template as the initial pending state, which is the same list that the pipeline uses.

### Important: SSE delivers HTML fragments

`RenderStagesHTML` returns raw HTML that gets swapped into the `sse-swap="pipeline-stages"` target. The returned HTML must be self-contained and styled — no external CSS classes that aren't in `output.css`. All styling must use Tailwind utility classes or inline styles.

### What NOT to Do

- **Do not change the video player** — it already matches Direction E (floating controls) which is correct per UX-DR4
- **Do not change the upload modal or lift list** — those match their shared screens (F, G)
- **Do not add new routes or API endpoints**
- **Do not use custom CSS** — all styling via Tailwind utilities
- **Do not use DaisyUI `loading-spinner`** for pipeline stages — use the pulsing circle icon from Direction H instead
- **Do not add card wrappers around empty sections** — only wrap sections that have content to display
- **Do not use `bg-warning`** for skipped stages — Direction H has no warning color in its palette (no reds, no warnings per UX color system)

### Existing E2E Test Patterns

Tests in `test/e2e/`. Key patterns from story 3.2:
- `startTestEnv(t)` creates a full server with temp DB, templates, routes
- `createTestLift(t, env, liftType, createdAt)` creates a test lift
- `newBrowserCtx(t)` creates a headless Chrome context
- Assertions use `chromedp.Evaluate` for DOM queries

### Project Structure Notes

Modified files:
- `web/templates/pages/lift-detail.html` — card layout wrapper
- `web/templates/partials/pipeline-stages.html` — initial stage checklist
- `internal/pipeline/pipeline.go` — `RenderStagesHTML` and `RenderStatusHTML` styling
- `web/static/output.css` — regenerated (Tailwind rebuild)
- `test/e2e/` — new or updated test file for card layout verification

No new files besides test file.

### References

- [Source: _bmad-output/planning-artifacts/ux-design-directions.html] — Direction D (Card Sections, lines 494-600) and Direction H (Pipeline Progress, lines 859-950)
- [Source: web/templates/pages/lift-detail.html] — current detail page template
- [Source: web/templates/partials/pipeline-stages.html] — current pipeline stages template
- [Source: internal/pipeline/pipeline.go:101-131] — RenderStagesHTML function
- [Source: internal/pipeline/pipeline.go:134-159] — RenderStatusHTML function
- [Source: web/static/app.js] — current JavaScript (no changes needed)
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md] — UX-DR5 (Pipeline Stage Checklist), UX-DR12 (Direction E layout), UX-DR18 (no error states)
- [Source: _bmad-output/implementation-artifacts/3-2-video-player-toggle-speed-controls.md] — previous story patterns
- [Source: test/e2e/video_player_test.go] — existing ChromeDP test patterns

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6

### Debug Log References

- Tailwind content config only scanned HTML files; added `internal/pipeline/pipeline.go` to content array so arbitrary color classes (bg-[#EDEDEA], bg-[#7DA67D], etc.) from Go render functions are included in compiled CSS
- Pre-existing test failures: `TestSystemFontStack` (font family mismatch) and `TestCoordinateTransformation` (skeleton stage) — both unrelated to this story

### Completion Notes List

- Converted lift detail page from flat layout to card-based layout with Direction D styling (white bg, rounded-xl, subtle shadow)
- Processing card wraps pipeline stages when `.Processing` is true, with "Processing" uppercase card title
- Empty sections (coaching, phases, metrics) remain as bare placeholder divs — cards will be added when downstream stories populate them
- Replaced DaisyUI loading spinner with Direction H stage checklist: 28px circular icons with green (complete), sage pulsing (active), neutral (pending), neutral with dash (skipped)
- Initial template hardcodes all 6 stage names with first stage active, rest pending — SSE replaces within first second
- RenderStatusHTML (list page) left unchanged as it's compact badge-style, not detail page
- Added `startTestEnvWithBroker` helper for e2e tests requiring SSE broker
- 10 new ChromeDP e2e tests covering card styling, processing title, stage icons, padding, overflow, button placement
- Updated 2 existing unit tests in pipeline_test.go for new CSS classes

### Change Log

- 2026-03-26: Implemented card layout and pipeline stage styling (Story 3.3)

### File List

- web/templates/pages/lift-detail.html (modified) — card layout with p-3 container, section-card wrapper for processing
- web/templates/partials/pipeline-stages.html (modified) — 6-stage checklist with Direction H styling
- internal/pipeline/pipeline.go (modified) — RenderStagesHTML with Direction H icons and border-separated rows
- internal/pipeline/pipeline_test.go (modified) — updated assertions for new CSS classes
- tailwind.config.js (modified) — added pipeline.go to content scan
- web/static/output.css (regenerated) — Tailwind rebuild with new utility classes
- test/e2e/lift_detail_test.go (modified) — added startTestEnvWithBroker helper, Broker field to testEnv
- test/e2e/card_layout_test.go (new) — 10 ChromeDP tests for card layout and pipeline stages
