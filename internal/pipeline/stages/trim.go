package stages

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pipeline"
	"press-out/internal/storage"
)

const (
	sceneChangeThreshold = 0.3
	minClusterDuration   = 0.8
	maxClusterDuration   = 15.0
	minHighMotionFrames  = 5
	paddingSec           = 1.5
)

// TrimStage analyzes video for motion patterns and trims to the lift portion.
type TrimStage struct{}

func (s *TrimStage) Name() string { return pipeline.StageTrimming }

func (s *TrimStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StageTrimming)

	// Get video duration for clamping.
	duration, err := ffmpeg.GetDuration(ctx, input.VideoPath)
	if err != nil {
		logger.Error("failed to get video duration", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: get duration: %w", err)
	}

	// Detect scene changes via FFmpeg.
	timestamps, err := detectSceneChanges(ctx, input.VideoPath)
	if err != nil {
		logger.Error("failed to detect scene changes", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: scene detection: %w", err)
	}

	// Find the best motion cluster.
	clusterStart, clusterEnd, confident := findMotionCluster(timestamps, duration)

	if !confident {
		logger.Warn("trim confidence low, preserving original video",
			"scene_changes", len(timestamps),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
	}

	// Apply padding and clamp to video bounds.
	trimStart := clusterStart - paddingSec
	if trimStart < 0 {
		trimStart = 0
	}
	trimEnd := clusterEnd + paddingSec
	if trimEnd > duration {
		trimEnd = duration
	}
	trimDuration := trimEnd - trimStart

	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimmed)

	if err := ffmpeg.TrimVideo(ctx, input.VideoPath, outputPath, trimStart, trimDuration); err != nil {
		logger.Error("ffmpeg trim failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: ffmpeg: %w", err)
	}

	logger.Info("trim complete",
		"trim_start", trimStart,
		"trim_end", trimEnd,
		"trim_duration", trimDuration,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: outputPath}, nil
}

// showInfoTimestamp matches the pts_time field from FFmpeg showinfo filter output.
var showInfoTimestamp = regexp.MustCompile(`pts_time:\s*([0-9.]+)`)

// detectSceneChanges runs FFmpeg scene change detection and returns timestamps.
func detectSceneChanges(ctx context.Context, videoPath string) ([]float64, error) {
	filter := fmt.Sprintf("select='gt(scene,%s)',showinfo", strconv.FormatFloat(sceneChangeThreshold, 'f', 1, 64))
	_, stderr, err := ffmpeg.Run(ctx, "-i", videoPath, "-vf", filter, "-f", "null", "-")
	if err != nil {
		return nil, err
	}

	var timestamps []float64
	scanner := bufio.NewScanner(strings.NewReader(string(stderr)))
	for scanner.Scan() {
		line := scanner.Text()
		matches := showInfoTimestamp.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		ts, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			continue
		}
		timestamps = append(timestamps, ts)
	}

	return timestamps, nil
}

// findMotionCluster finds the densest cluster of scene change timestamps
// and returns its start/end times and whether confidence is sufficient.
func findMotionCluster(timestamps []float64, videoDuration float64) (start, end float64, confident bool) {
	if len(timestamps) < minHighMotionFrames {
		return 0, 0, false
	}

	sort.Float64s(timestamps)

	// Sliding window to find the densest cluster.
	bestStart := 0
	bestEnd := 0
	bestCount := 0

	for i := 0; i < len(timestamps); i++ {
		// Find the furthest timestamp within maxClusterDuration from timestamps[i].
		for j := i; j < len(timestamps); j++ {
			windowDuration := timestamps[j] - timestamps[i]
			if windowDuration > maxClusterDuration {
				break
			}
			count := j - i + 1
			if count > bestCount {
				bestCount = count
				bestStart = i
				bestEnd = j
			}
		}
	}

	if bestCount < minHighMotionFrames {
		return 0, 0, false
	}

	clusterStart := timestamps[bestStart]
	clusterEnd := timestamps[bestEnd]
	clusterDuration := clusterEnd - clusterStart

	if clusterDuration < minClusterDuration || clusterDuration > maxClusterDuration {
		return 0, 0, false
	}

	return clusterStart, clusterEnd, true
}
