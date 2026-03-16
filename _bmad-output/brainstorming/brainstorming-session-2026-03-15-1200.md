---
stepsCompleted: [1, 2, 3, 4]
inputDocuments: []
session_topic: 'Golang webapp for Olympic weightlifting video analysis - sagittal view upload, auto trim/crop, playback controls, skeleton overlay, lift phase timestamps, LLM coaching feedback'
session_goals: 'MVP feature list that is agent-developable and testable on a headless VPS with programmatic visual validation'
selected_approach: 'ai-recommended'
techniques_used: ['First Principles Thinking', 'Morphological Analysis', 'Chaos Engineering']
ideas_generated: [10 fundamentals, 10 components, 8 validation strategies, 10-step build plan]
context_file: ''
session_active: false
workflow_completed: true
---

# Brainstorming Session Results

**Facilitator:** joao
**Date:** 2026-03-15

## Session Overview

**Topic:** Golang webapp for Olympic weightlifting video analysis - sagittal view upload, auto trim/crop, playback controls, skeleton overlay, lift phase timestamps, LLM coaching feedback
**Goals:** MVP feature list that is agent-developable and testable on a headless VPS with programmatic visual validation

### Session Setup

- Scope: Full product vision, technical architecture, UX, and differentiating features
- Key constraint: Autonomous coding agent on headless VPS must build and verify everything
- Visual validation must be programmatic (no GUI available)

## Technique Selection

**Approach:** AI-Recommended Techniques
**Analysis Context:** Olympic weightlifting video analysis webapp with focus on agent-buildable MVP

**Recommended Techniques:**

- **First Principles Thinking:** Strip assumptions, rebuild from fundamental truths of what a lifter needs from video analysis
- **Morphological Analysis:** Systematically map all system components and explore parameter combinations for architecture and features
- **Chaos Engineering:** Stress-test every MVP feature against headless agent-development constraint and validation strategies

**AI Rationale:** The session requires balancing creative product vision with hard engineering constraints (headless VPS, autonomous agent development). First Principles prevents feature bloat, Morphological Analysis ensures comprehensive architecture coverage, and Chaos Engineering validates feasibility under the unique development constraints.

## Technique Execution Results

### First Principles Thinking

**Interactive Focus:** Strip away assumptions about what a weightlifting analysis app needs and rebuild from the fundamental truths of what a lifter actually requires from video review.

**10 Fundamental Truths:**

**Fundamental #1: Pull-to-Catch Ratio**
The relationship between how high the bar is pulled vs. how deep the lifter catches it. A technically efficient lift has a low pull height relative to catch depth -- you get under the bar rather than muscling it up. This is the single most important metric.

**Fundamental #2: Diagnosis-to-Cue Pipeline**
Useful coaching feedback follows a strict pattern: identify what went wrong, explain why it happened (the causal chain), then give a concrete physical cue for the next rep. Diagnosis without a cue is academic. A cue without diagnosis doesn't build understanding.

**Fundamental #3: Reference Lift Comparison**
A lifter's best lift becomes the benchmark. The app should allow comparing any lift against a personal "reference" lift -- not an abstract ideal, but your own best execution. Your body proportions and movement style are already accounted for.

**Fundamental #4: Two Review Modes**
Lifters review video in two distinct contexts: **Platform mode** (phone in hand, between sets) needs instant, glanceable feedback. **Study mode** (days later) needs deep analysis, comparisons, trends, and coaching insights. These are almost two different products sharing the same data.

**Fundamental #5: Core Processing is Non-Negotiable**
Trim, crop, and pose estimation must happen fast enough for between-sets use. These aren't analysis features -- they're the basic language the lifter uses to read their own lift. Without them, the app is just a video player.

**Fundamental #6: Skeleton as Perceptual Aid, Not Measurement Tool**
The skeleton overlay's primary value isn't numerical (joint angles, degrees) -- it's visual clarity. It makes body positions pop out of noisy gym video. The lifter already knows what they felt; the skeleton confirms or challenges that feeling. Rendering quality and clarity matter more than keypoint precision.

**Fundamental #7: Single-User Personal Tool**
The MVP is a private training journal with video intelligence. No auth complexity, no sharing, no social features. One lifter, their videos, their history. Resisting collaboration features keeps the MVP achievable.

**Fundamental #8: Real Gym Conditions**
The video environment is semi-controlled: roughly perpendicular sagittal view, but with other people potentially in frame and bar/plates occluding joints (especially hip, wrist, and knee on the near side). The app must handle partial skeleton data gracefully.

**Fundamental #9: Lifter = Person Moving the Bar**
The simplest heuristic for "which person is me" -- find the person whose body is connected to a moving barbell. Others in frame are walking, standing, or spotting. More robust than face recognition, works with back to camera.

**Fundamental #10: Lift Type Awareness with Manual Fallback**
The analysis is lift-type-specific -- a snatch has different phases, positions, and coaching cues than a clean or jerk. Auto-detection is ideal, but manual selection (Snatch / Clean / Clean & Jerk) is an acceptable MVP fallback.

### Morphological Analysis

**Interactive Focus:** Systematically map all system components, explore options for each, and select the MVP-viable combination.

**MVP Component Matrix:**

| Component | MVP Choice | Details |
|---|---|---|
| **Video Input** | File upload via mobile browser | Simple HTML form, mobile-responsive |
| **Person Detection & Cropping** | Person-barbell interaction detection | Track across frames, user-tap fallback when confidence is low |
| **Video Trimming** | Motion-based auto-trim | Detect barbell movement start/end, add padding before and after |
| **Pose Estimation** | Server-side, Go calls MediaPipe API | Returns keypoints, backend renders skeleton overlay |
| **Lift Classification** | Manual selection | Dropdown: Snatch / Clean / Clean & Jerk |
| **Phase Segmentation** | LLM-based from pose data | Async -- video playable immediately, phase markers appear when LLM responds |
| **Video Player / UI** | Go + HTMX + Tailwind, mobile-first | Two pre-rendered videos (clean + skeleton), HTML5 toggle and speed control, server-rendered phase timeline |
| **Metrics & Analysis** | All six metrics | Pull-to-catch ratio, bar path visualization, phase durations, key position snapshots, joint angles at key moments, velocity curve |
| **LLM Coaching** | Single prompt analysis | Send keypoints + phases + metrics + lift type, receive diagnosis (causal chain) + concrete cues. Iterate to multi-step chain if quality is insufficient |
| **Storage** | Filesystem + SQLite | Videos on disk, metadata in SQLite. No external dependencies |

**Tech Stack:** Go backend, HTMX + Tailwind CSS frontend, MediaPipe API, FFmpeg (video processing), SQLite, LLM API (Claude/OpenAI), Chromedp (testing)

**Key Architecture Decision -- Two Pre-Rendered Videos:**
Instead of real-time canvas compositing, the server renders two versions of each video: one clean, one with skeleton overlay baked in. The user toggles which one plays. This fits perfectly with HTMX's server-side rendering philosophy, works on any mobile browser, and gives the coding agent concrete artifacts to validate.

### Chaos Engineering

**Interactive Focus:** Stress-test every MVP component against the headless VPS constraint -- "How does the coding agent build this, test it, and verify it works without ever seeing the output?"

**Validation Strategies:**

| Component | Validation Approach |
|---|---|
| **Person Detection & Cropping** | Bounding box metadata assertion against pre-defined expected ranges for known reference test videos |
| **Video Trimming** | Pose estimation on first/last frames (setup position at start, completion at end) + motion start/end timestamp assertions against known values |
| **Pose Estimation** | Keypoint sanity checks (within frame bounds, anatomically plausible: head above shoulders above hips) + minimum keypoint count per frame with occlusion tolerance |
| **Phase Segmentation** | Four-layer validation: schema (correct phases for lift type, correct order), temporal ordering (monotonically increasing, within video bounds), phase duration sanity (reasonable ranges), coverage (no large gaps) |
| **Metrics & Analysis** | Range assertions (physically plausible bounds) + consistency checks (durations sum correctly, bar path goes up then down) + snapshot file validation + cross-metric correlation (velocity peak aligns with second pull phase) |
| **LLM Coaching** | No automated validation for MVP -- manual review by user |
| **Video Player / UI** | Headless browser testing with Chromedp (Go-native). Load pages, interact with controls, verify elements render, test HTMX toggles |

**Build Strategy -- Component-by-Component:**
Each component is built and tested in isolation before integration. The agent doesn't move to step N+1 until step N passes its validation suite. This ensures every piece works independently and integration is incremental.

**Prerequisites:** 2-3 reference test videos with hand-verified bounding boxes, trim timestamps, and expected metric ranges.

## Idea Organization and Prioritization

### Theme 1: Product Vision & Core Value

- **Pull-to-catch ratio** as the single most important metric for lift quality
- **Diagnosis-to-cue pipeline** -- LLM gives causal chain + actionable coaching cue, not vague feedback
- **Reference lift comparison** -- compare against your own best, not a textbook ideal
- **Lift type awareness** -- Snatch / Clean / Clean & Jerk each analyzed with specific phases and cues
- **Skeleton as perceptual aid** -- visual clarity for confirming/challenging what the lifter felt

### Theme 2: User Experience & Workflow

- **Mobile-first** -- phone browser is the primary interface, used at the gym between sets
- **Two review modes** -- platform (instant, glanceable) and study (deep, analytical)
- **Two pre-rendered videos** with toggle -- clean and skeleton overlay
- **Async phase markers** -- video playable immediately, phase timeline populates when LLM responds
- **HTML5 speed control** for slow-motion review
- **Manual lift type selection** via dropdown
- **Six metrics displayed** -- pull-to-catch ratio, bar path, phase durations, key position snapshots, joint angles, velocity curve

### Theme 3: Technical Architecture

- **Go backend** handles all processing -- no client-side ML or JS framework
- **HTMX + Tailwind CSS** -- server-rendered, no build tools, minimal JS
- **MediaPipe API** called from Go for pose estimation
- **FFmpeg** for video trimming, cropping, and skeleton overlay rendering
- **SQLite + filesystem** -- zero external infrastructure dependencies
- **LLM API** for phase segmentation and coaching
- **Chromedp** for headless browser testing
- **Single-user, no auth** -- maximum simplicity

### Theme 4: Video Processing Pipeline

- **Motion-based auto-trim** with padding before and after the lift
- **Person-barbell interaction detection** with tracking + user-tap fallback
- **Server-side pose estimation** via MediaPipe API, keypoints stored
- **Dual video rendering** -- FFmpeg produces clean and skeleton overlay versions
- **Bar path, joint angles, velocity** computed from keypoint data
- **Phase segmentation** via LLM analysis of sampled keyframes

### Theme 5: Agent Development & Validation

- **Component-by-component build** -- test each before integrating
- **Reference test videos** with hand-verified expected values as ground truth
- **Bounding box assertions** for crop validation
- **Keypoint sanity + count checks** for pose validation
- **Four-layer phase validation** -- schema, ordering, duration, coverage
- **Multi-dimensional metric validation** -- range, consistency, correlation
- **Chromedp headless browser tests** for UI verification
- **No LLM coaching validation** for MVP -- manual review

### Breakthrough Concepts

- **Two pre-rendered videos** -- sidesteps real-time rendering complexity, fits HTMX perfectly, agent can validate file existence and pixel differences
- **Headless constraint improved the architecture** -- forcing programmatic verification led to a more modular, testable system than if a human were building it
- **Lifter = person moving the bar** -- simple, robust detection heuristic that avoids face recognition or manual region selection

## MVP Build Plan (10 Steps)

Each step is independently testable. The coding agent does not proceed to step N+1 until step N passes validation.

### Step 1: Video Upload + Storage
- **Build:** HTTP upload endpoint, save video to filesystem, create SQLite record
- **Validate:** File exists on disk at expected path, DB record created with correct metadata

### Step 2: Video Trimming
- **Build:** Motion detection on uploaded video, FFmpeg trim with padding
- **Validate:** Pose estimation on first/last frames (setup position at start, completion at end), timestamps match expected values for reference videos

### Step 3: Person Detection + Cropping
- **Build:** Person-barbell interaction detection, track across frames, FFmpeg crop. User-tap fallback when confidence is low
- **Validate:** Bounding box coordinates fall within pre-defined expected ranges for reference test videos

### Step 4: Pose Estimation
- **Build:** Go backend calls MediaPipe API on cropped/trimmed video, stores keypoints
- **Validate:** Keypoints within frame bounds, anatomically plausible proportions (head above shoulders above hips), minimum keypoint count per frame with occlusion tolerance

### Step 5: Skeleton Overlay Rendering
- **Build:** FFmpeg draws skeleton from keypoints onto video, produces second video file
- **Validate:** Two video files exist, correct duration, both are valid video files

### Step 6: Metrics Computation
- **Build:** Compute all six metrics from keypoint data -- pull-to-catch ratio, bar path, phase durations, key position snapshots, joint angles, velocity curve
- **Validate:** Range assertions (physically plausible), consistency checks (durations sum correctly), snapshot files exist, cross-metric correlation (velocity peak aligns with second pull)

### Step 7: Video Player UI
- **Build:** Go templates + HTMX + Tailwind. Serve both videos, toggle between them, speed control, metrics display panel
- **Validate:** Chromedp loads page, video elements present, toggle switches video source, speed control works, metrics panel renders

### Step 8: Lift Classification UI
- **Build:** Dropdown selector (Snatch / Clean / Clean & Jerk), stored in SQLite with lift record
- **Validate:** Chromedp selects each option, verifies selection persists

### Step 9: Phase Segmentation
- **Build:** Send keypoint data + lift type to LLM, parse response into phase timestamps, render phase markers on timeline (async)
- **Validate:** Schema validation (correct phases for lift type), temporal ordering (monotonically increasing), phase duration sanity, coverage check (no large gaps)

### Step 10: LLM Coaching
- **Build:** Single prompt with keypoints + phases + metrics + lift type. Parse response into diagnosis section + cues section. Display in UI
- **Validate:** No automated validation -- manual review by user. Iterate prompt engineering or switch to multi-step chain if quality is insufficient

## Session Summary and Insights

**Key Achievements:**
- Produced a complete, actionable MVP specification from first principles -- no feature was included without tracing back to a fundamental lifter need
- Every component has a concrete validation strategy for headless agent development
- The tech stack (Go + HTMX + Tailwind + SQLite) is deliberately simple -- no moving parts that a coding agent can't handle
- The build order respects data dependencies and allows incremental integration

**Breakthrough Insight:** The constraint of building on a headless VPS with an autonomous coding agent didn't limit the product -- it forced a cleaner, more modular architecture. Every component being independently testable is a design virtue, not just an agent-development necessity.

**Creative Facilitation Narrative:**
The session moved from abstract ("what does a lifter need?") to concrete (10-step build plan with validation) through three complementary techniques. First Principles prevented feature bloat by anchoring every decision to real lifting needs. Morphological Analysis ensured no system component was overlooked. Chaos Engineering ruthlessly filtered the MVP through the headless development constraint. The result is a specification that a coding agent can execute step-by-step with clear success criteria at every stage.
