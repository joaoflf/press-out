---
stepsCompleted: ['step-01-init', 'step-02-discovery', 'step-02b-vision', 'step-02c-executive-summary', 'step-03-success', 'step-01b-continue', 'step-04-journeys', 'step-05-domain', 'step-06-innovation', 'step-07-project-type', 'step-08-scoping', 'step-09-functional', 'step-10-nonfunctional', 'step-11-polish', 'step-12-complete']
inputDocuments: ['_bmad-output/brainstorming/brainstorming-session-2026-03-15-1200.md']
workflowType: 'prd'
documentCounts:
  briefs: 0
  research: 0
  brainstorming: 1
  projectDocs: 0
classification:
  projectType: web_app
  domain: sports_fitness
  complexity: medium
  projectContext: greenfield
---

# Product Requirements Document - press-out

**Author:** joao
**Date:** 2026-03-15

## Executive Summary

Press-out is a personal Olympic weightlifting video analysis tool. A lifter records a sagittal-view video of a snatch, clean, or clean & jerk, uploads it via mobile browser, and receives automated visual analysis and coaching feedback between sets. The app auto-trims to the lift, crops to the lifter using barbell-interaction detection, runs pose estimation to generate a skeleton overlay, segments lift phases, computes biomechanical metrics (pull-to-catch ratio, bar path, velocity curve, joint angles, phase durations, key position snapshots), and delivers LLM-generated coaching cues with causal diagnosis. Single-user, no authentication — a private training journal with video intelligence.

### What Makes This Special

Olympic lifts happen in under two seconds. Reviewing raw video at the gym, most lifters can't see what went wrong, and even in slow motion, they don't know what to look for. Press-out solves both problems: the skeleton overlay and metrics make the lift *readable*, and the LLM coaching translates what happened into a concrete cue for the next rep. Existing tools are either generic video players, general fitness apps, or single-metric bar path trackers. Nothing combines auto-processing, pose-aware metrics, lift-phase segmentation, and actionable coaching in one tool purpose-built for the snatch, clean, and jerk. The core insight: the lifter already feels something during the lift — press-out confirms or challenges that feeling with visual evidence and a coaching cue.

## Project Classification

- **Project Type:** Web application — Go backend, HTMX + Tailwind CSS frontend, server-rendered, mobile-first
- **Domain:** Sports/Fitness — Olympic weightlifting video analysis
- **Complexity:** Medium — straightforward domain (no regulatory concerns), non-trivial technical surface (video processing pipeline, ML pose estimation, LLM integration)
- **Project Context:** Greenfield — new product, no existing codebase

## Success Criteria

### User Success

- Upload a video between sets and receive a trimmed, cropped, skeleton-overlaid result fast enough to review before the next attempt
- Metrics (pull-to-catch ratio, bar path, velocity curve, joint angles, phase durations, key position snapshots) are accurate and immediately interpretable — no mental translation needed
- Skeleton overlay is visually clear on real gym video with noisy backgrounds and partial occlusion
- Phase timeline allows jumping directly to any lift phase (setup, first pull, transition, second pull, catch, recovery)
- Playback controls (slow-motion, speed up) work fluidly with both clean and skeleton-overlay videos

### Business Success

- The tool earns a spot in every training session — it's opened as routinely as the camera app
- Processing is fast enough that using it doesn't disrupt training rhythm or rest intervals
- Over time, the lifter can articulate what changed in their technique because of insights from press-out

### Technical Success

- All six metrics computed from keypoint data and producing physically plausible values
- Video processing pipeline (upload → trim → crop → pose estimation → skeleton render → metrics → LLM coaching) completes reliably without manual intervention
- Pose estimation handles real gym conditions: partial occlusion, other people in frame, variable lighting
- LLM coaching produces diagnosis with causal chain and actionable cue, not generic advice

### Measurable Outcomes

- Auto-trim correctly identifies lift start/end on reference test videos
- Auto-crop isolates the lifter (person-barbell interaction) without cutting off the movement
- Skeleton overlay renders on both clean and occluded frames with graceful degradation
- All six metrics fall within physically plausible ranges for known reference lifts
- End-to-end processing completes within a timeframe compatible with gym rest intervals

## User Journeys

### Journey 1: Between-Sets Analysis (Happy Path)

Carlos just finished a snatch at 85% of his max. It felt off — the bar crashed on him in the catch and his elbows buckled. He sets his phone on a bench, opens press-out in his mobile browser, and uploads the video he just recorded with his phone camera propped on a plate stack.

He puts the phone down and starts loading his next attempt. A minute later, he picks up his phone. The screen shows the pipeline progress — "Trimming... Cropping... Pose estimation... Rendering skeleton... Computing metrics... Generating coaching..." — and processing is done. He sees the trimmed, cropped video — just his lift, no setup wandering, no rack walk. He toggles to the skeleton overlay and scrubs to the catch. There it is: his elbows are visibly behind the bar at the catch position, and the pull-to-catch ratio confirms he pulled too high instead of getting under it.

He taps the coaching section. The LLM diagnosis reads: "Bar pulled to sternum height but catch depth is only quarter squat. You're muscling the bar up instead of pulling under. The high pull forces a forward catch, causing the elbow buckle." The cue: "Think about punching through the bar and sitting your hips straight down the moment you feel the pull finish."

Carlos re-racks, sets up his next attempt with that cue in mind, and hits record again.

**What this journey reveals:**
- Upload must be fast and minimal-tap (the lifter is sweaty, distracted, resting)
- Processing must complete within a rest interval (~2-3 minutes)
- The trimmed/cropped video must be immediately playable without waiting for all metrics
- Skeleton overlay toggle needs to be instant (pre-rendered, not computed on demand)
- Coaching cue must be concrete and physical, not analytical
- The whole flow is: upload → wait → review video → read cue → lift again

### Journey 2: Graceful Degradation (Edge Cases)

Carlos records a clean & jerk, but the gym is busy — two people walk through the frame during his setup, and someone is stretching behind the platform. He uploads the video.

Auto-trim runs but can't confidently detect the lift start because the background motion from bystanders muddies the signal. The system skips trimming and keeps the full video. Auto-crop detects multiple people but uses the person-barbell interaction heuristic — Carlos is the one whose hands are on the bar that moves. The crop succeeds despite the busy background.

On another day, Carlos records from a bad angle and the crop also fails confidence checks. The system skips cropping too and runs pose estimation on the full frame. The skeleton overlay is noisier — it might briefly flicker onto a bystander — but Carlos's lift is still tracked. The metrics may be slightly less precise, but they're computed and displayed. The LLM coaching still runs on whatever keypoint data is available.

The experience is degraded but never broken. Carlos never sees an error screen or a "processing failed" message. He always gets a video back with whatever analysis the system could produce.

**What this journey reveals:**
- Every pipeline stage must be independently skippable — trim, crop, pose, metrics, coaching are a chain but each link failing just passes the input through unchanged
- No error screens for processing failures — degrade silently and show what you have
- Person-barbell interaction detection is the primary crop strategy, with full-frame as fallback
- The user should never need to re-upload or retry — one upload, one result, always

### Journey Requirements Summary

| Capability | Source Journey |
|---|---|
| Minimal-tap mobile upload | Happy Path |
| Processing within rest interval (~2-3 min) | Happy Path |
| Pre-rendered dual video with instant toggle | Happy Path |
| Skeleton overlay on real gym video | Both |
| Six metrics displayed clearly on mobile | Happy Path |
| LLM coaching with causal diagnosis + physical cue | Happy Path |
| Pipeline stages independently skippable | Degraded Path |
| Silent degradation, never error screens | Degraded Path |
| Person-barbell heuristic with full-frame fallback | Degraded Path |
| Full video fallback when trim fails | Degraded Path |

## Web Application Specific Requirements

### Project-Type Overview

Press-out is a server-rendered multi-page application (MPA) built with Go, HTMX, and Tailwind CSS. HTMX handles partial page updates and real-time interactions without a JavaScript framework. The app is mobile-first, designed for use on a phone at the gym, targeting Chrome as the sole supported browser.

### Technical Architecture Considerations

- **Rendering model:** Server-rendered HTML with HTMX for partial updates and interactivity — no client-side framework, no build step
- **Browser target:** Chrome (mobile primary, desktop secondary) — no cross-browser compatibility concerns
- **Real-time pipeline updates:** Server-Sent Events (SSE) via HTMX to push processing stage progress to the browser (e.g., "Trimming... Cropping... Running pose estimation... Rendering skeleton... Computing metrics... Generating coaching feedback")
- **SEO:** Not applicable — personal tool, no public pages, no indexing needed
- **Accessibility:** Not required — single-user personal tool

### Responsive Design

- **Mobile-first layout:** All UI designed for phone screen widths first
- **Touch targets:** Buttons and controls sized for gym use (sweaty hands, quick taps)
- **Video player:** Full-width on mobile, HTML5 native controls with custom speed and toggle overlays
- **Metrics display:** Stacked/scrollable on mobile, no multi-column layouts required

### Implementation Considerations

- **No build tooling:** Tailwind CSS via CDN or standalone CLI, no npm/webpack
- **No JavaScript framework:** All interactivity through HTMX attributes and SSE
- **Single binary deployment:** Go compiles to one binary, serves static assets, runs the pipeline
- **Chrome-only:** Can use modern CSS and HTML features without polyfills (e.g., `<dialog>`, CSS grid, container queries)

## Project Scoping & Phased Development

### MVP Strategy & Philosophy

**MVP Approach:** Problem-solving MVP — deliver the complete video-to-coaching pipeline so every upload produces actionable feedback. No partial product; the value is in the full chain from video to coaching cue.

**Resource:** Solo developer. The 10-step build plan is designed for sequential, component-by-component development with validation gates between steps.

### MVP Feature Set (Phase 1)

**Core Journey Supported:** Between-sets analysis — upload, process, review, apply cue, lift again.

**Must-Have Capabilities:**

1. Video upload + storage (filesystem + SQLite)
2. Motion-based auto-trim with padding (full video fallback)
3. Person-barbell interaction detection + crop (full frame fallback)
4. Client-side pose estimation via ml5.js MoveNet (runs in browser before upload)
5. Skeleton overlay rendering (dual pre-rendered videos)
6. Six metrics computation from keypoint data
7. Video player UI (HTMX + Tailwind, mobile-first, toggle, speed control)
8. Manual lift type selection (Snatch / Clean / Clean & Jerk)
9. LLM-based phase segmentation with timeline markers
10. LLM coaching feedback (diagnosis + cues)
11. Lift list view — browse all uploaded lifts with basic info (date, lift type, thumbnail)
12. Lift detail view — tap any lift to view its videos (clean/skeleton toggle), metrics, phase timeline, and coaching feedback
13. Lift deletion — remove a lift and its associated videos, keypoints, and metrics

**Also in MVP:**
- SSE real-time pipeline progress updates
- Graceful degradation at every pipeline stage
- Chrome-only, mobile-first

### Post-MVP Features

**Phase 2 (Growth):**
- Reference lift comparison — compare any lift against a personal best for the same lift type
- Historical tracking — metric trends over time
- Lift library with search and filtering

**Phase 3 (Vision):**
- Multi-angle support (front view in addition to sagittal)
- Auto lift-type detection replacing manual selection
- Session-level analysis (patterns across multiple lifts in one session)

### Risk Mitigation Strategy

**Technical Risk:** Pose estimation quality on real gym video (occlusion, lighting, bystanders) is the highest-risk component. Mitigation: ml5.js MoveNet runs client-side with no cloud dependency. Spike validated good detection quality on real weightlifting footage. If pose fails, video uploads without keypoints and downstream stages degrade gracefully.

**Resource Risk:** Solo developer building a 13-capability pipeline. Mitigation: each step is independently testable with clear validation criteria — no step depends on future steps being complete. Build sequentially, validate before proceeding.

## Functional Requirements

### Video Upload & Storage

- FR1: Lifter can upload a video file from their mobile device
- FR2: System can store uploaded videos persistently for later retrieval
- FR3: Lifter can assign a lift type (Snatch, Clean, or Clean & Jerk) to each upload

### Video Processing

- FR4: System can automatically detect and trim a video to the lift portion, removing setup and post-lift footage
- FR5: System can automatically identify and crop to the lifter performing the lift when multiple people are in frame
- FR6: System preserves the full unprocessed video when auto-trim or auto-crop confidence falls below threshold
- FR7: System can process each pipeline stage independently, allowing any stage to be skipped without blocking downstream stages

### Pose Estimation & Visualization

- FR8: System can detect body keypoints (joints, limbs) from video frames of a lifter
- FR9: System can render a skeleton overlay onto the lift video as a separate viewable version
- FR10: System can produce both a clean video and a skeleton-overlay video for each upload

### Lift Metrics & Phase Analysis

- FR11: System can compute pull-to-catch ratio from keypoint data
- FR12: System can generate a bar path visualization from keypoint data
- FR13: System can compute a velocity curve for the barbell movement
- FR14: System can measure joint angles at key positions during the lift
- FR15: System can calculate phase durations for each segment of the lift
- FR16: System can capture key position snapshots at phase transition points (first pull start, second pull start, catch, recovery)
- FR17: System can segment a lift into its constituent phases (setup, first pull, transition, second pull, catch, recovery) based on lift type
- FR18: System can display phase markers on a timeline aligned with video playback

### Coaching Intelligence

- FR19: System can generate a coaching diagnosis that includes at least one identified issue, one causal factor, and references specific metric values from the lift
- FR20: System can produce a physical cue that describes a specific body movement or position change the lifter can attempt on the next rep
- FR21: Coaching feedback can incorporate lift type, keypoint data, phase segmentation, and computed metrics

### Lift Management

- FR22: Lifter can view a list of all uploaded lifts with identifying information (date, lift type)
- FR23: Lifter can view the full detail of any individual lift including videos, metrics, phases, and coaching feedback
- FR24: Lifter can delete a lift and all its associated data

### Video Playback & Review

- FR25: Lifter can toggle between clean video and skeleton overlay video during playback
- FR26: Lifter can control playback speed (slow-motion and speed up)
- FR27: Lifter can navigate to any lift phase directly from the phase timeline
- FR28: Video playback can start immediately without waiting for all analysis to complete

### Processing Feedback

- FR29: System can provide real-time progress updates to the lifter as each pipeline stage completes
- FR30: Lifter can see which processing stage is currently active during video analysis

## Non-Functional Requirements

### Performance

- NFR1: End-to-end pipeline (upload through coaching feedback) completes within 3 minutes for videos under 60 seconds at 1080p resolution (under 200MB), as measured by server-side pipeline timing logs
- NFR2: All server-rendered pages load within 1 second under single-user load, as measured by server-side request timing logs
- NFR3: Video playback begins within 1 second of user interaction, as measured by browser performance timing API
- NFR4: Toggle between clean and skeleton overlay video switches playback within 500 milliseconds, as measured by browser performance timing API
- NFR5: Pipeline stage progress updates reach the browser within 1 second of each stage completing, as measured by server-side SSE dispatch timestamps

### Integration

- NFR6: System handles missing keypoints.json gracefully (pose estimation failed or was skipped client-side), continuing the pipeline without crashing
- NFR7: System handles LLM API unavailability gracefully, completing video processing without coaching feedback or phase segmentation
- NFR8: System operates with no external infrastructure dependencies beyond the LLM API (Claude Code) and ml5.js CDN

### Reliability

- NFR9: No user-facing error screens during video processing — all failures degrade to the best available result
- NFR10: Uploaded videos are persisted before processing begins, ensuring no data loss if processing fails
- NFR11: A failed pipeline run can be re-triggered on a previously uploaded video without re-uploading
