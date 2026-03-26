#!/usr/bin/env python3
"""Spike: YOLO26n-Pose inference on a video file.

Outputs keypoints.json in the same format the press-out Go backend expects:
{
  "sourceWidth": int,
  "sourceHeight": int,
  "frames": [
    {
      "timeOffsetMs": int,
      "boundingBox": {"left": float, "top": float, "right": float, "bottom": float},
      "keypoints": [{"name": str, "x": float, "y": float, "confidence": float}, ...]
    }
  ]
}

All coordinates are normalized to [0, 1].
"""

import argparse
import json
import sys
import time

import cv2
from ultralytics import YOLO

COCO_KEYPOINT_NAMES = [
    "nose",
    "left_eye",
    "right_eye",
    "left_ear",
    "right_ear",
    "left_shoulder",
    "right_shoulder",
    "left_elbow",
    "right_elbow",
    "left_wrist",
    "right_wrist",
    "left_hip",
    "right_hip",
    "left_knee",
    "right_knee",
    "left_ankle",
    "right_ankle",
]


def process_video(video_path: str, model_name: str, target_fps: float) -> dict:
    model = YOLO(model_name)

    cap = cv2.VideoCapture(video_path)
    if not cap.isOpened():
        print(f"Error: cannot open {video_path}", file=sys.stderr)
        sys.exit(1)

    src_w = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
    src_h = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
    native_fps = cap.get(cv2.CAP_PROP_FPS) or 30.0
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    duration_s = total_frames / native_fps

    # Determine frame step to sample at target_fps
    frame_step = max(1, round(native_fps / target_fps))
    sampled_count = total_frames // frame_step

    print(f"Video: {src_w}x{src_h}, {native_fps:.1f}fps, {duration_s:.1f}s, {total_frames} frames")
    print(f"Sampling every {frame_step} frame(s) -> ~{sampled_count} frames at ~{target_fps}fps")
    print(f"Model: {model_name}")

    frames = []
    frame_idx = 0
    processed = 0
    t_start = time.time()

    while True:
        ret, frame = cap.read()
        if not ret:
            break

        if frame_idx % frame_step != 0:
            frame_idx += 1
            continue

        time_offset_ms = round((frame_idx / native_fps) * 1000)

        results = model(frame, verbose=False)
        result = results[0]

        frame_data = {
            "timeOffsetMs": time_offset_ms,
            "boundingBox": {"left": 0, "top": 0, "right": 1, "bottom": 1},
            "keypoints": [],
        }

        if result.keypoints is not None and len(result.keypoints) > 0:
            # Pick the first (highest-confidence) person
            kps = result.keypoints[0]  # shape: (17, 3) — x, y, conf
            xy = kps.xy[0].cpu().numpy()    # (17, 2)
            conf = kps.conf[0].cpu().numpy() if kps.conf is not None else [0.0] * 17  # (17,)

            keypoints_list = []
            for i, name in enumerate(COCO_KEYPOINT_NAMES):
                keypoints_list.append({
                    "name": name,
                    "x": float(xy[i][0]) / src_w,
                    "y": float(xy[i][1]) / src_h,
                    "confidence": float(conf[i]),
                })
            frame_data["keypoints"] = keypoints_list

            # Bounding box from detection boxes
            if result.boxes is not None and len(result.boxes) > 0:
                box = result.boxes[0].xyxy[0].cpu().numpy()  # x1, y1, x2, y2
                frame_data["boundingBox"] = {
                    "left": float(box[0]) / src_w,
                    "top": float(box[1]) / src_h,
                    "right": float(box[2]) / src_w,
                    "bottom": float(box[3]) / src_h,
                }

        frames.append(frame_data)
        processed += 1

        if processed % 50 == 0 or processed == 1:
            elapsed = time.time() - t_start
            fps_actual = processed / elapsed if elapsed > 0 else 0
            print(f"  frame {processed}/{sampled_count} ({100*processed//sampled_count}%) — {fps_actual:.1f} fps")

        frame_idx += 1

    cap.release()
    elapsed = time.time() - t_start
    with_pose = sum(1 for f in frames if len(f["keypoints"]) > 0)
    print(f"\nDone: {processed} frames in {elapsed:.1f}s ({processed/elapsed:.1f} fps)")
    print(f"Frames with pose: {with_pose}/{processed}")

    return {
        "sourceWidth": src_w,
        "sourceHeight": src_h,
        "frames": frames,
    }


def main():
    parser = argparse.ArgumentParser(description="YOLO pose estimation spike")
    parser.add_argument("video", help="Path to video file")
    parser.add_argument("-m", "--model", default="yolo26n-pose", help="YOLO model name (default: yolo26n-pose)")
    parser.add_argument("--fps", type=float, default=30.0, help="Target sampling FPS (default: 30)")
    parser.add_argument("-o", "--output", default="keypoints.json", help="Output JSON path")
    args = parser.parse_args()

    result = process_video(args.video, args.model, args.fps)

    with open(args.output, "w") as f:
        json.dump(result, f, indent=2)
    print(f"Wrote {args.output}")


if __name__ == "__main__":
    main()
