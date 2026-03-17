package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

const (
	maxInlineVideoBytes = 50 * 1024 * 1024 // 50MB inline content limit
	poseAPITimeout      = 2 * time.Minute
)

// PoseStage detects body keypoints via a pose estimation provider.
type PoseStage struct {
	Client pose.Client
}

// NewPoseStage creates a new PoseStage with the given pose client.
func NewPoseStage(client pose.Client) *PoseStage {
	return &PoseStage{Client: client}
}

func (s *PoseStage) Name() string { return pipeline.StagePoseEstimation }

func (s *PoseStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StagePoseEstimation)

	videoData, err := os.ReadFile(input.VideoPath)
	if err != nil {
		logger.Error("failed to read video", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: read video: %w", err)
	}

	if len(videoData) > maxInlineVideoBytes {
		logger.Error("video too large for inline pose estimation",
			"bytes", len(videoData), "max_bytes", maxInlineVideoBytes,
			"duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: video too large for inline pose estimation (%d bytes, max %d)", len(videoData), maxInlineVideoBytes)
	}

	width, height, err := ffmpeg.GetDimensions(ctx, input.VideoPath)
	if err != nil {
		logger.Error("failed to get video dimensions", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: get dimensions: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, poseAPITimeout)
	defer cancel()

	result, err := s.Client.DetectPose(timeoutCtx, videoData)
	if err != nil {
		logger.Error("pose estimation failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: %w", err)
	}

	result.SourceWidth = width
	result.SourceHeight = height

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal keypoints", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: marshal: %w", err)
	}

	outPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		logger.Error("failed to write keypoints", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("pose: write keypoints: %w", err)
	}

	logger.Info("pose estimation complete",
		"frames", len(result.Frames),
		"source_width", width,
		"source_height", height,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
}
