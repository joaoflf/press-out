package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

const (
	minKeypointConfidence = 0.5  // minimum keypoint confidence to include in displacement calculation
	displacementThreshold = 0.01 // normalized displacement above which a frame is considered "high motion"
	minLiftDuration       = 0.8  // minimum seconds for a valid lift cluster
	maxLiftDuration       = 15.0 // maximum seconds for a valid lift cluster
	minHighMotionFrames   = 5    // minimum high-motion frames needed for confidence
	maxGapSec             = 2.0  // maximum seconds of consecutive low-motion frames allowed within a run
	paddingSec            = 1.5  // seconds of padding before and after detected boundaries
	smoothingWindow       = 5    // number of frames for displacement smoothing
)

// TrimParams holds the trim boundaries so downstream stages can filter by trimmed range.
type TrimParams struct {
	TrimStartMs int64 `json:"trimStartMs"`
	TrimEndMs   int64 `json:"trimEndMs"`
}

// TrimStage analyzes keypoint displacement to detect the lift portion and trims the video.
type TrimStage struct{}

func (s *TrimStage) Name() string { return pipeline.StageTrimming }

func (s *TrimStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StageTrimming)

	keypointsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)

	// Check if keypoints.json exists.
	if _, err := os.Stat(keypointsPath); os.IsNotExist(err) {
		logger.Warn("no keypoints file, preserving original video",
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
	}

	// Detect lift boundaries from keypoints.
	liftStart, liftEnd, confident, err := detectLiftFromKeypoints(keypointsPath)
	if err != nil {
		logger.Error("failed to detect lift from keypoints", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: detect lift: %w", err)
	}

	if !confident {
		logger.Warn("trim confidence low, preserving original video",
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
	}

	// Get video duration for clamping.
	duration, err := ffmpeg.GetDuration(ctx, input.VideoPath)
	if err != nil {
		logger.Error("failed to get video duration", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: get duration: %w", err)
	}

	// Apply padding and clamp to video bounds.
	trimStart := liftStart - paddingSec
	if trimStart < 0 {
		trimStart = 0
	}
	trimEnd := liftEnd + paddingSec
	if trimEnd > duration {
		trimEnd = duration
	}
	trimDuration := trimEnd - trimStart

	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimmed)

	if err := ffmpeg.TrimVideo(ctx, input.VideoPath, outputPath, trimStart, trimDuration); err != nil {
		logger.Error("ffmpeg trim failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("trim: ffmpeg: %w", err)
	}

	// Write trim-params.json so downstream stages can filter by trimmed range.
	trimParams := TrimParams{
		TrimStartMs: int64(trimStart * 1000),
		TrimEndMs:   int64(trimEnd * 1000),
	}
	trimParamsData, err := json.Marshal(trimParams)
	if err != nil {
		logger.Error("failed to marshal trim params", "error", err)
		return pipeline.StageOutput{}, fmt.Errorf("trim: marshal params: %w", err)
	}
	trimParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimParams)
	if err := os.WriteFile(trimParamsPath, trimParamsData, 0644); err != nil {
		logger.Error("failed to write trim params", "error", err)
		return pipeline.StageOutput{}, fmt.Errorf("trim: write params: %w", err)
	}

	logger.Info("trim complete",
		"trim_start", trimStart,
		"trim_end", trimEnd,
		"trim_duration", trimDuration,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: outputPath}, nil
}

// detectLiftFromKeypoints analyzes keypoint displacement across frames to find
// the lift's start and end times. It returns the detected boundaries in seconds,
// a confidence flag, and any error.
func detectLiftFromKeypoints(keypointsPath string) (startSec, endSec float64, confident bool, err error) {
	data, err := os.ReadFile(keypointsPath)
	if err != nil {
		return 0, 0, false, fmt.Errorf("read keypoints: %w", err)
	}

	var result pose.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, false, fmt.Errorf("parse keypoints: %w", err)
	}

	if len(result.Frames) < 2 {
		slog.Warn("insufficient frames for displacement analysis", "frames", len(result.Frames))
		return 0, 0, false, nil
	}

	// Compute frame-to-frame keypoint displacement.
	displacements := computeDisplacements(result.Frames)

	// Smooth with moving average to reduce noise.
	smoothed := smoothDisplacements(displacements)

	// Estimate frame rate from time offsets.
	fps := estimateFPS(result.Frames)
	maxGapFrames := int(maxGapSec * fps)
	if maxGapFrames < 1 {
		maxGapFrames = 1
	}

	// Mark high-motion frames.
	highMotion := make([]bool, len(smoothed))
	for i, d := range smoothed {
		highMotion[i] = d > displacementThreshold
	}

	// Merge high-motion frames into runs, bridging gaps.
	runs := mergeRuns(highMotion, smoothed, maxGapFrames)

	if len(runs) == 0 {
		slog.Warn("no high-motion frames detected")
		return 0, 0, false, nil
	}

	// Select the run with the highest total displacement.
	bestRun := runs[0]
	for _, r := range runs[1:] {
		if r.totalDisp > bestRun.totalDisp {
			bestRun = r
		}
	}

	// Convert frame indices to times.
	// Frame index i in displacements corresponds to the transition between
	// result.Frames[i] and result.Frames[i+1]. Use the time of the start frame.
	startSec = float64(result.Frames[bestRun.startIdx+1].TimeOffsetMs) / 1000.0
	endIdx := bestRun.endIdx + 1
	if endIdx >= len(result.Frames) {
		endIdx = len(result.Frames) - 1
	}
	endSec = float64(result.Frames[endIdx].TimeOffsetMs) / 1000.0

	// Validate duration.
	liftDuration := endSec - startSec
	if liftDuration < minLiftDuration {
		slog.Warn("cluster too short",
			"duration", fmt.Sprintf("%.1fs", liftDuration),
			"min", fmt.Sprintf("%.1fs", minLiftDuration),
		)
		return 0, 0, false, nil
	}
	if liftDuration > maxLiftDuration {
		slog.Warn("cluster too long",
			"duration", fmt.Sprintf("%.1fs", liftDuration),
			"max", fmt.Sprintf("%.1fs", maxLiftDuration),
		)
		return 0, 0, false, nil
	}

	// Validate high-motion frame count.
	if bestRun.highMotionCount < minHighMotionFrames {
		slog.Warn("insufficient high-motion frames",
			"count", bestRun.highMotionCount,
			"min", minHighMotionFrames,
		)
		return 0, 0, false, nil
	}

	return startSec, endSec, true, nil
}

// motionRun represents a contiguous run of high-motion frames.
type motionRun struct {
	startIdx        int
	endIdx          int
	totalDisp       float64
	highMotionCount int
}

// computeDisplacements calculates frame-to-frame normalized keypoint displacement.
func computeDisplacements(frames []pose.Frame) []float64 {
	displacements := make([]float64, len(frames)-1)

	for i := 1; i < len(frames); i++ {
		prevKPs := make(map[string]pose.Keypoint, len(frames[i-1].Keypoints))
		for _, kp := range frames[i-1].Keypoints {
			prevKPs[kp.Name] = kp
		}

		var totalDisp float64
		var count int
		for _, kp := range frames[i].Keypoints {
			prev, ok := prevKPs[kp.Name]
			if !ok {
				continue
			}
			if prev.Confidence < minKeypointConfidence || kp.Confidence < minKeypointConfidence {
				continue
			}
			dx := kp.X - prev.X
			dy := kp.Y - prev.Y
			totalDisp += math.Sqrt(dx*dx + dy*dy)
			count++
		}

		if count > 0 {
			displacements[i-1] = totalDisp / float64(count)
		}
	}

	return displacements
}

// smoothDisplacements applies a moving average to reduce noise.
func smoothDisplacements(displacements []float64) []float64 {
	smoothed := make([]float64, len(displacements))
	halfWin := smoothingWindow / 2

	for i := range displacements {
		start := i - halfWin
		if start < 0 {
			start = 0
		}
		end := i + halfWin + 1
		if end > len(displacements) {
			end = len(displacements)
		}
		var sum float64
		for j := start; j < end; j++ {
			sum += displacements[j]
		}
		smoothed[i] = sum / float64(end-start)
	}

	return smoothed
}

// estimateFPS estimates the frame rate from frame time offsets.
func estimateFPS(frames []pose.Frame) float64 {
	if len(frames) < 2 {
		return 30.0 // default
	}
	totalMs := frames[len(frames)-1].TimeOffsetMs - frames[0].TimeOffsetMs
	if totalMs <= 0 {
		return 30.0
	}
	return float64(len(frames)-1) / (float64(totalMs) / 1000.0)
}

// mergeRuns merges high-motion frames into contiguous runs, bridging gaps
// of up to maxGapFrames consecutive low-motion frames.
func mergeRuns(highMotion []bool, displacements []float64, maxGapFrames int) []motionRun {
	var runs []motionRun
	var current *motionRun
	gap := 0

	for i, hm := range highMotion {
		if hm {
			if current == nil {
				current = &motionRun{startIdx: i}
			}
			current.totalDisp += displacements[i]
			current.highMotionCount++
			current.endIdx = i
			gap = 0
		} else if current != nil {
			gap++
			if gap > maxGapFrames {
				runs = append(runs, *current)
				current = nil
				gap = 0
			}
		}
	}

	if current != nil {
		runs = append(runs, *current)
	}

	return runs
}
