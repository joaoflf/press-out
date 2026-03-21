package stages

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"press-out/internal/pipeline"
	"press-out/internal/storage"
)

// PoseStage runs YOLO26n-Pose via Python subprocess to detect body keypoints.
type PoseStage struct {
	// ProjectRoot is the directory containing pyproject.toml and scripts/.
	// Set in main.go; used as cmd.Dir for subprocess execution.
	ProjectRoot string
}

func (s *PoseStage) Name() string { return pipeline.StagePoseEstimation }

func (s *PoseStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StagePoseEstimation)

	keypointsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)

	cmd := exec.CommandContext(ctx, "uv", "run", "scripts/pose.py", input.VideoPath, "-o", keypointsPath)
	cmd.Dir = s.ProjectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Error("pose estimation failed",
			"error", err,
			"stderr", stderr.String(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return pipeline.StageOutput{}, fmt.Errorf("pose: %w", err)
	}

	logger.Info("pose estimation complete",
		"stderr", stderr.String(),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
}
