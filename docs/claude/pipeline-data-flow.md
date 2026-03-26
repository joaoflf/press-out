# Pipeline Data Flow

## File Artifacts Per Lift

All stored in `data/lifts/<id>/`:

```
original.mp4      ← uploaded video
keypoints.json    ← pose estimation output (used by trim + crop)
trimmed.mp4       ← trim stage output
cropped.mp4       ← crop stage output
crop-params.json  ← crop stage output (coordinate transform for skeleton)
skeleton.mp4      ← skeleton stage output (not yet implemented)
thumbnail.jpg     ← thumbnail
```

File name constants: `internal/storage/storage.go`

## Data Dependencies Between Stages

```
original.mp4
    │
    ├──▶ [Pose] ──▶ keypoints.json
    │                    │
    │    ┌───────────────┤
    │    │               │
    │    ▼               ▼
    └──▶ [Trim] ──▶ trimmed.mp4
                        │
                        ▼
                   [Crop] ──▶ cropped.mp4 + crop-params.json
                                  │              │
                                  ▼              ▼
                             [Skeleton] ──▶ skeleton.mp4
```

- Pose runs on `original.mp4`, outputs `keypoints.json`
- Trim reads `keypoints.json` + processes current video path
- Crop reads `keypoints.json` + processes current video path, outputs transform params
- Skeleton will use `crop-params.json` to map original keypoint coords to cropped frame

## keypoints.json Format

```json
{
  "sourceWidth": 1080,
  "sourceHeight": 1920,
  "frames": [
    {
      "timeOffsetMs": 0,
      "boundingBox": { "left": 100, "top": 200, "right": 500, "bottom": 1800 },
      "keypoints": [
        { "name": "nose", "x": 300.5, "y": 250.1, "confidence": 0.95 },
        ...
      ]
    }
  ]
}
```

17 COCO keypoints per frame. Coordinates are in original video pixel space.
