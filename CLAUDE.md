# Press-Out - Claude Code Context

## Pose Estimation

Pose estimation runs server-side via YOLO26n-Pose (ultralytics) as a Python subprocess
managed by uv. The pipeline calls `uv run scripts/pose.py <video> -o <keypoints.json>`.
No cloud API or credentials needed. Model (7.5MB) auto-downloads on first run.

## Python Dependencies

Python dependencies are managed by uv. The project has `pyproject.toml` and `uv.lock`
at the project root. The pose script runs via `uv run`.
