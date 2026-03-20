# Press-Out - Claude Code Context

## Pose Estimation

Pose estimation runs client-side in the browser via ml5.js (MoveNet SINGLEPOSE_THUNDER).
The browser processes the video frame-by-frame, then uploads both the video file and
`keypoints.json` to the server. No cloud API or credentials needed.
