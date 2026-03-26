#!/usr/bin/env python3
"""Dynamic crop spike — compare static vs tracking crop with multiple aspect ratios.

Trims each video using density-bridged strategy, then applies crop strategies
on the trimmed portion. Outputs cropped video clips and an HTML comparison report.

Usage:
    uv run spikes/crop-rig/crop_spike.py --test-dir testdata/videos
    uv run spikes/crop-rig/crop_spike.py --test-dir testdata/videos --open
"""

import argparse
import json
import os
import subprocess
import sys
from dataclasses import dataclass

import cv2
import numpy as np

# Import trim logic from sibling module
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from trim_rig import (
    load_keypoints, compute_displacements, smooth, estimate_fps,
    density_bridged_strategy, discover_test_pairs,
)

# --- Constants ---

BAR_TOP_PADDING_PX = 150     # px above min bbox top for bar + plates overhead
FOOT_BOTTOM_PADDING_PX = 40  # px below max bbox bottom for feet
CROP_PADDING_H = 0.30        # horizontal padding around 95th pctile bbox width (bar + plates)
TRACKING_SMOOTH_FRAMES = 31  # ~1s at 30fps for camera pan smoothing

# Hybrid mode: lock crop when lifter is stationary, track when walking
HYBRID_X_VEL_THRESHOLD = 3.0   # px/frame — below this, lifter is "stationary"
HYBRID_VEL_SMOOTH_FRAMES = 15  # smooth velocity signal to debounce


# --- Data types ---

@dataclass
class CropConfig:
    name: str
    aspect_w: int
    aspect_h: int
    mode: str  # "static", "tracking", or "hybrid"


CROP_CONFIGS = [
    CropConfig("static-9x16", 9, 16, "static"),
    CropConfig("track-9x16", 9, 16, "tracking"),
    CropConfig("hybrid-9x16", 9, 16, "hybrid"),
    CropConfig("hybrid-3x4", 3, 4, "hybrid"),
    CropConfig("hybrid-4x5", 4, 5, "hybrid"),
]


@dataclass
class CropResult:
    config: CropConfig
    crop_w: int
    crop_h: int
    output_path: str
    frame_count: int = 0


# --- Crop logic ---

def compute_crop_region(frames, sw, sh, aspect_w, aspect_h):
    """Compute crop (w, h, origin_y) from the full vertical extent of the lifter.

    Sizes the crop to span from above the overhead bar (with plate padding)
    down to below the feet. This ensures the bar + plates are never clipped
    at the top. The Y origin is computed directly — no centering needed.

    Width is the larger of aspect-ratio-derived or body-width + plate padding.
    Returns (w, h, origin_y).
    """
    tops = [f.bounding_box.top * sh for f in frames]
    bottoms = [f.bounding_box.bottom * sh for f in frames]
    widths = [(f.bounding_box.right - f.bounding_box.left) * sw for f in frames]

    # Vertical extent: overhead bar to feet
    # Use min (not percentile) — overhead frames are a minority but must be covered
    crop_top = float(min(tops)) - BAR_TOP_PADDING_PX
    crop_bottom = float(np.percentile(bottoms, 95)) + FOOT_BOTTOM_PADDING_PX
    box_h = crop_bottom - crop_top

    # Width from aspect ratio
    target_ratio = aspect_w / aspect_h
    box_w = box_h * target_ratio

    # Also ensure width covers body + plates
    min_w = float(np.percentile(widths, 95)) * (1 + 2 * CROP_PADDING_H)
    if min_w > box_w:
        box_w = min_w
        box_h = box_w / target_ratio
        # Re-center vertically around the extent midpoint
        mid_y = (crop_top + crop_bottom) / 2
        crop_top = mid_y - box_h / 2

    # Clamp to frame
    crop_top = max(0.0, crop_top)
    if crop_top + box_h > sh:
        box_h = float(sh) - crop_top
        box_w = box_h * target_ratio
    if box_w > sw:
        box_w = float(sw)
        box_h = box_w / target_ratio
        crop_top = max(0.0, (crop_bottom - box_h))

    w = int(round(box_w))
    h = int(round(box_h))
    if w % 2:
        w -= 1
    if h % 2:
        h -= 1
    origin_y = int(round(crop_top))

    return w, h, origin_y


def compute_crop_centers(frames, sw, sh, mode):
    """Return (cx_list, cy_list) in pixel coords for each keypoint frame."""
    raw_cx = [(f.bounding_box.left + f.bounding_box.right) / 2 * sw for f in frames]
    raw_cy = [(f.bounding_box.top + f.bounding_box.bottom) / 2 * sh for f in frames]

    if mode == "static":
        med_x = float(np.median(raw_cx))
        med_y = float(np.median(raw_cy))
        return [med_x] * len(frames), [med_y] * len(frames)

    # Y is always static — vertical bbox center movement is body articulation
    # (bend → stand → overhead), not the lifter moving in the frame.
    static_cy = float(np.median(raw_cy))
    cy = [static_cy] * len(frames)

    # Base X tracking trajectory
    track_cx = smooth(raw_cx, window=TRACKING_SMOOTH_FRAMES)

    if mode == "tracking":
        return track_cx, cy

    # --- Hybrid X: lock during lift, track during walk ---
    # Three-pass approach:
    #   1. Classify each frame as walking or stationary via smoothed X velocity
    #   2. Find contiguous segments, compute lock points for stationary segments
    #   3. Walking segments: linearly interpolate between adjacent lock points
    #      (smooth pan, no jitter from raw tracking signal)
    if len(track_cx) < 2:
        return track_cx, cy

    x_vel = [abs(track_cx[i] - track_cx[i - 1]) for i in range(1, len(track_cx))]
    x_vel_smooth = smooth(x_vel, window=HYBRID_VEL_SMOOTH_FRAMES)
    x_vel_smooth = [0.0] + x_vel_smooth  # pad for first frame

    # Pass 1: classify frames
    is_walking = [v > HYBRID_X_VEL_THRESHOLD for v in x_vel_smooth]

    # Pass 2: find contiguous segments
    segments = []  # (type, start, end)
    i = 0
    n = len(is_walking)
    while i < n:
        seg_start = i
        is_walk = is_walking[i]
        while i < n and is_walking[i] == is_walk:
            i += 1
        segments.append(('walking' if is_walk else 'stationary', seg_start, i))

    # Compute lock points for stationary segments
    lock_points = {}  # segment index -> lock_x
    for idx, (seg_type, start, end) in enumerate(segments):
        if seg_type == 'stationary':
            lock_points[idx] = float(np.median(raw_cx[start:end]))

    # Pass 3: assign X positions
    hybrid_cx = [0.0] * n
    for idx, (seg_type, start, end) in enumerate(segments):
        if seg_type == 'stationary':
            lock_x = lock_points[idx]
            for j in range(start, end):
                hybrid_cx[j] = lock_x
            print(f"    stationary seg [{start}-{end}) "
                  f"median_x={lock_x:.0f} ({end - start} frames)",
                  file=sys.stderr)
        else:
            # Walking: linearly interpolate between adjacent lock points
            prev_lock = next((lock_points[k] for k in range(idx - 1, -1, -1)
                              if k in lock_points), None)
            next_lock = next((lock_points[k] for k in range(idx + 1, len(segments))
                              if k in lock_points), None)

            seg_len = end - start
            if prev_lock is not None and next_lock is not None:
                for j in range(start, end):
                    t = (j - start) / max(1, seg_len - 1) if seg_len > 1 else 0
                    hybrid_cx[j] = prev_lock + t * (next_lock - prev_lock)
            elif prev_lock is not None:
                for j in range(start, end):
                    hybrid_cx[j] = prev_lock
            elif next_lock is not None:
                for j in range(start, end):
                    hybrid_cx[j] = next_lock
            else:
                for j in range(start, end):
                    hybrid_cx[j] = track_cx[j]

            print(f"    walking seg [{start}-{end}) ({seg_len} frames) "
                  f"interp {prev_lock:.0f}->{next_lock:.0f}" if prev_lock and next_lock
                  else f"    walking seg [{start}-{end}) ({seg_len} frames)",
                  file=sys.stderr)

    walk_pct = sum(is_walking) / len(is_walking) * 100
    print(f"    walking: {sum(is_walking)}/{len(is_walking)} frames ({walk_pct:.0f}%)",
          file=sys.stderr)

    return hybrid_cx, cy


def cx_to_origins(cx_list, crop_w, sw, origin_y):
    """Convert X centers to top-left (x, origin_y) origins, clamped to frame."""
    xs, ys = [], []
    for cx in cx_list:
        x = int(round(cx - crop_w / 2))
        x = max(0, min(x, sw - crop_w))
        xs.append(x)
        ys.append(origin_y)
    return xs, ys


def interpolate_to_video_frames(kp_times_sec, kp_xs, kp_ys, video_fps,
                                 trim_start, trim_end):
    """Interpolate keypoint-rate positions to every video frame in the trim window."""
    start_vf = int(trim_start * video_fps)
    end_vf = int(trim_end * video_fps)

    video_times = [i / video_fps for i in range(start_vf, end_vf + 1)]
    interp_x = np.interp(video_times, kp_times_sec, [float(x) for x in kp_xs])
    interp_y = np.interp(video_times, kp_times_sec, [float(y) for y in kp_ys])

    positions = list(zip(interp_x.astype(int), interp_y.astype(int)))
    return start_vf, positions


# --- Video processing ---

def crop_video(video_path, trim_start, crop_positions, crop_w, crop_h,
               native_fps, output_path):
    """Read video frames in trim window, apply per-frame crop, write via ffmpeg."""
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        return 0

    start_frame_idx = int(trim_start * native_fps)
    cap.set(cv2.CAP_PROP_POS_FRAMES, start_frame_idx)

    proc = subprocess.Popen([
        "ffmpeg", "-y", "-loglevel", "error",
        "-f", "rawvideo", "-pix_fmt", "bgr24",
        "-s", f"{crop_w}x{crop_h}",
        "-r", str(native_fps),
        "-i", "pipe:0",
        "-c:v", "libx264", "-preset", "fast",
        "-pix_fmt", "yuv420p", "-movflags", "+faststart",
        output_path,
    ], stdin=subprocess.PIPE, stderr=subprocess.PIPE)

    count = 0
    for x, y in crop_positions:
        ret, frame = cap.read()
        if not ret:
            break

        cropped = frame[y:y + crop_h, x:x + crop_w]

        # Pad if crop extends beyond frame edge
        if cropped.shape[0] != crop_h or cropped.shape[1] != crop_w:
            padded = np.zeros((crop_h, crop_w, 3), dtype=np.uint8)
            ah = min(cropped.shape[0], crop_h)
            aw = min(cropped.shape[1], crop_w)
            padded[:ah, :aw] = cropped[:ah, :aw]
            cropped = padded

        proc.stdin.write(cropped.tobytes())
        count += 1

    proc.stdin.close()
    proc.wait()
    cap.release()

    return count


# --- Debug frame output ---

def save_debug_frames(video_path, trim_start, crop_positions, crop_w, crop_h,
                      native_fps, output_dir, config_name):
    """Save annotated frames with crop rect overlay at key moments."""
    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        return

    n = len(crop_positions)
    # Sample frames: first, 25%, 50%, 75%, last
    indices = sorted(set([0, n // 4, n // 2, 3 * n // 4, n - 1]))

    start_frame_idx = int(trim_start * native_fps)
    debug_dir = os.path.join(output_dir, f"debug-{config_name}")
    os.makedirs(debug_dir, exist_ok=True)

    for target_idx in indices:
        cap.set(cv2.CAP_PROP_POS_FRAMES, start_frame_idx + target_idx)
        ret, frame = cap.read()
        if not ret:
            continue

        x, y = crop_positions[target_idx]
        # Draw crop rectangle (green)
        cv2.rectangle(frame, (x, y), (x + crop_w, y + crop_h), (0, 255, 0), 3)
        # Draw center crosshair
        cx, cy = x + crop_w // 2, y + crop_h // 2
        cv2.line(frame, (cx - 20, cy), (cx + 20, cy), (0, 255, 0), 2)
        cv2.line(frame, (cx, cy - 20), (cx, cy + 20), (0, 255, 0), 2)
        # Label
        t_sec = (start_frame_idx + target_idx) / native_fps
        label = f"f{target_idx} t={t_sec:.2f}s crop=({x},{y}) {crop_w}x{crop_h}"
        cv2.putText(frame, label, (10, 30), cv2.FONT_HERSHEY_SIMPLEX, 0.7,
                    (0, 255, 0), 2)

        out_path = os.path.join(debug_dir, f"frame-{target_idx:04d}.png")
        cv2.imwrite(out_path, frame)

    cap.release()
    print(f"  debug frames: {debug_dir}/", file=sys.stderr)


# --- HTML report ---

def generate_report(results_by_video, output_dir):
    """Generate HTML report with side-by-side video comparisons."""
    html = """<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>Crop Strategy Comparison</title>
<style>
  body { font-family: -apple-system, system-ui, sans-serif; margin: 20px; background: #1a1a1a; color: #eee; }
  h1 { border-bottom: 2px solid #444; padding-bottom: 10px; }
  h2 { margin-top: 40px; color: #88ccff; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; margin: 16px 0; }
  .card { background: #2a2a2a; border-radius: 8px; padding: 12px; }
  .card h3 { margin: 0 0 8px 0; font-size: 14px; color: #ccc; }
  .card video { width: 100%; border-radius: 4px; background: #000; }
  .card .meta { font-size: 12px; color: #888; margin-top: 4px; }
  .summary { background: #2a2a2a; border-radius: 8px; padding: 16px; margin: 16px 0; }
  table { border-collapse: collapse; width: 100%; }
  th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #444; font-size: 14px; }
  th { color: #88ccff; }
  .tag { display: inline-block; padding: 2px 6px; border-radius: 3px; font-size: 11px; }
  .tag-static { background: #444; }
  .tag-tracking { background: #2d5a2d; }
  .tag-hybrid { background: #4a3d8f; }
  .controls { margin: 16px 0; }
  .controls button { background: #444; color: #eee; border: none; padding: 8px 16px;
                     border-radius: 4px; cursor: pointer; margin-right: 8px; }
  .controls button:hover { background: #555; }
</style>
</head><body>
<h1>Crop Strategy Comparison</h1>
<div class="controls">
  <button onclick="playAll()">Play All</button>
  <button onclick="pauseAll()">Pause All</button>
  <button onclick="restartAll()">Restart All</button>
</div>
"""

    # Summary table
    html += '<div class="summary"><table><tr><th>Video</th><th>Trim</th>'
    config_names = [c.name for c in CROP_CONFIGS]
    for name in config_names:
        html += f"<th>{name}</th>"
    html += "</tr>\n"

    for video_name, (trim_info, results) in results_by_video.items():
        html += f"<tr><td><b>{video_name}</b></td>"
        html += f'<td>{trim_info["start"]:.1f}-{trim_info["end"]:.1f}s</td>'
        for cname in config_names:
            r = next((r for r in results if r.config.name == cname), None)
            if r:
                html += f"<td>{r.crop_w}&times;{r.crop_h}</td>"
            else:
                html += "<td>-</td>"
        html += "</tr>\n"
    html += "</table></div>\n"

    # Per-video grids
    for video_name, (trim_info, results) in results_by_video.items():
        dur = trim_info["end"] - trim_info["start"]
        html += f'<h2>{video_name} <span style="font-size:14px;color:#888">'
        html += f'trim {trim_info["start"]:.1f}-{trim_info["end"]:.1f}s ({dur:.1f}s)</span></h2>\n'
        html += '<div class="grid">\n'
        for r in results:
            rel_path = os.path.relpath(r.output_path, output_dir)
            tag_cls = f"tag-{r.config.mode}"
            tag_label = r.config.mode
            html += f"""<div class="card">
  <h3>{r.config.aspect_w}:{r.config.aspect_h} <span class="tag {tag_cls}">{tag_label}</span></h3>
  <video class="crop-video" src="{rel_path}" controls loop muted playsinline></video>
  <div class="meta">{r.crop_w}&times;{r.crop_h} &middot; {r.frame_count} frames</div>
</div>\n"""
        html += "</div>\n"

    html += """
<script>
function playAll() { document.querySelectorAll('.crop-video').forEach(v => v.play()); }
function pauseAll() { document.querySelectorAll('.crop-video').forEach(v => v.pause()); }
function restartAll() {
  document.querySelectorAll('.crop-video').forEach(v => { v.currentTime = 0; v.play(); });
}
</script>
</body></html>"""

    report_path = os.path.join(output_dir, "crop-report.html")
    with open(report_path, "w") as f:
        f.write(html)
    return report_path


# --- Main processing ---

def process_one_video(video_path, keypoints_path, video_name, output_dir,
                      debug_frames=False):
    """Trim + crop one video with all strategies. Returns (trim_info, results)."""
    pose = load_keypoints(keypoints_path)
    frames = pose.frames
    sw, sh = pose.source_width, pose.source_height

    if len(frames) < 2:
        print(f"  Skipping {video_name}: too few frames", file=sys.stderr)
        return None

    fps = estimate_fps(frames)
    raw_disp = compute_displacements(frames)
    smoothed_disp = smooth(raw_disp)

    # Compute trim boundaries
    trim = density_bridged_strategy(frames, raw_disp, smoothed_disp, fps)
    if not trim.confident:
        print(f"  Skipping {video_name}: trim failed ({trim.label})", file=sys.stderr)
        return None

    trim_info = {"start": trim.start_sec, "end": trim.end_sec}
    print(f"\n{video_name}: {sw}x{sh}, trim {trim.start_sec:.1f}-{trim.end_sec:.1f}s",
          file=sys.stderr)

    # Filter keypoints to trimmed window
    trimmed_frames = [f for f in frames
                      if trim.start_sec * 1000 <= f.time_offset_ms <= trim.end_sec * 1000]

    if len(trimmed_frames) < 5:
        print(f"  Skipping {video_name}: too few trimmed frames", file=sys.stderr)
        return None

    cap = cv2.VideoCapture(video_path)
    native_fps = cap.get(cv2.CAP_PROP_FPS)
    cap.release()

    kp_times = [f.time_offset_ms / 1000.0 for f in trimmed_frames]
    os.makedirs(output_dir, exist_ok=True)
    results = []

    for config in CROP_CONFIGS:
        crop_w, crop_h, origin_y = compute_crop_region(
            trimmed_frames, sw, sh, config.aspect_w, config.aspect_h)

        cx, cy = compute_crop_centers(trimmed_frames, sw, sh, config.mode)
        xs, ys = cx_to_origins(cx, crop_w, sw, origin_y)

        # Interpolate to video frame rate
        _, positions = interpolate_to_video_frames(
            kp_times, xs, ys, native_fps, trim.start_sec, trim.end_sec)

        out_path = os.path.join(output_dir, f"crop-{config.name}.mp4")
        count = crop_video(video_path, trim.start_sec, positions,
                           crop_w, crop_h, native_fps, out_path)

        results.append(CropResult(config=config, crop_w=crop_w, crop_h=crop_h,
                                  output_path=out_path, frame_count=count))
        print(f"  {config.name}: {crop_w}x{crop_h}, {count} frames", file=sys.stderr)

        if debug_frames and config.mode == "hybrid":
            save_debug_frames(video_path, trim.start_sec, positions,
                              crop_w, crop_h, native_fps, output_dir, config.name)

    return trim_info, results


def main():
    parser = argparse.ArgumentParser(description="Dynamic crop comparison spike")
    parser.add_argument("video_path", nargs="?", help="Path to input video")
    parser.add_argument("keypoints_path", nargs="?", help="Path to keypoints.json")
    parser.add_argument("--test-dir", help="Directory with video+keypoints pairs")
    parser.add_argument("--open", action="store_true", help="Open report in browser")
    parser.add_argument("--debug-frames", action="store_true",
                        help="Save debug PNGs with crop rect overlay")
    args = parser.parse_args()

    output_root = os.path.join("spikes", "crop-rig", "output")
    results_by_video = {}

    if args.test_dir:
        pairs = discover_test_pairs(args.test_dir)
        if not pairs:
            print(f"No video+keypoints pairs in {args.test_dir}", file=sys.stderr)
            sys.exit(1)
        for vp, kp, name in pairs:
            out_dir = os.path.join(output_root, name)
            result = process_one_video(vp, kp, name, out_dir,
                                       debug_frames=args.debug_frames)
            if result:
                results_by_video[name] = result
    else:
        if not args.video_path or not args.keypoints_path:
            parser.error("Provide video + keypoints, or --test-dir")
        name = os.path.splitext(os.path.basename(args.video_path))[0]
        out_dir = os.path.join(output_root, name)
        result = process_one_video(args.video_path, args.keypoints_path, name, out_dir,
                                   debug_frames=args.debug_frames)
        if result:
            results_by_video[name] = result

    if results_by_video:
        report_path = generate_report(results_by_video, output_root)
        print(f"\nReport: {report_path}", file=sys.stderr)
        if args.open:
            import webbrowser
            webbrowser.open(f"file://{os.path.abspath(report_path)}")
    else:
        print("No videos processed.", file=sys.stderr)


if __name__ == "__main__":
    main()
