#!/usr/bin/env python3
"""Server-side pose estimation using YOLO26n-Pose via ultralytics.

Processes a video frame-by-frame, detects 17 COCO-format body keypoints per frame,
and outputs a JSON file compatible with the pose.Result Go type.

Usage:
    uv run scripts/pose.py <video_path> -o <output.json> [--fps 30] [-m yolo26n-pose]
"""

import argparse
import json
import sys
import math

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


def main():
    parser = argparse.ArgumentParser(description="YOLO pose estimation for video")
    parser.add_argument("video_path", help="Path to input video")
    parser.add_argument("-o", "--output", required=True, help="Output keypoints JSON path")
    parser.add_argument("-m", "--model", default="yolo11n-pose", help="YOLO model name")
    parser.add_argument("--fps", type=int, default=30, help="Target FPS for sampling")
    args = parser.parse_args()

    model = YOLO(args.model)

    cap = cv2.VideoCapture(args.video_path)
    if not cap.isOpened():
        print(f"Error: cannot open video {args.video_path}", file=sys.stderr)
        sys.exit(1)

    source_width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
    source_height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
    native_fps = cap.get(cv2.CAP_PROP_FPS)
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))

    frame_step = max(1, round(native_fps / args.fps))

    frames = []
    frame_idx = 0
    processed = 0

    while True:
        ret, frame = cap.read()
        if not ret:
            break

        if frame_idx % frame_step == 0:
            time_offset_ms = round((frame_idx / native_fps) * 1000)

            results = model(frame, verbose=False)
            result = results[0]

            if result.keypoints is not None and len(result.keypoints.data) > 0:
                # First detection (highest confidence — YOLO sorts by confidence)
                kps_data = result.keypoints.data[0]  # shape: (17, 3)
                box_data = result.boxes.xyxy[0]  # shape: (4,)

                keypoints = []
                for i, name in enumerate(COCO_KEYPOINT_NAMES):
                    x = float(kps_data[i][0]) / source_width
                    y = float(kps_data[i][1]) / source_height
                    conf = float(kps_data[i][2])
                    keypoints.append({
                        "name": name,
                        "x": x,
                        "y": y,
                        "confidence": conf,
                    })

                bounding_box = {
                    "left": float(box_data[0]) / source_width,
                    "top": float(box_data[1]) / source_height,
                    "right": float(box_data[2]) / source_width,
                    "bottom": float(box_data[3]) / source_height,
                }
            else:
                # No person detected — empty keypoints, full-frame bounding box
                keypoints = []
                bounding_box = {"left": 0, "top": 0, "right": 1, "bottom": 1}

            frames.append({
                "timeOffsetMs": time_offset_ms,
                "boundingBox": bounding_box,
                "keypoints": keypoints,
            })

            processed += 1
            if processed % 30 == 0:
                pct = round((frame_idx / total_frames) * 100) if total_frames > 0 else 0
                print(f"Processed {processed} frames ({pct}%)", file=sys.stderr)

        frame_idx += 1

    cap.release()

    output = {
        "sourceWidth": source_width,
        "sourceHeight": source_height,
        "frames": frames,
    }

    with open(args.output, "w") as f:
        json.dump(output, f)

    print(f"Done: {processed} frames processed, written to {args.output}", file=sys.stderr)


if __name__ == "__main__":
    main()
