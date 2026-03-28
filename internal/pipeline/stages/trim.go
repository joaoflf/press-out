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
	trimMinKeypointConfidence  = 0.5
	trimMinKeypointsPerFrame   = 3 // require at least 3 confident keypoint pairs
	trimSmoothWindow           = 7
	trimMinWinSec             = 5.0  // wider min to capture CJ's clean+walk+jerk
	trimMaxWinSec             = 12.0
	trimWinStepSec            = 0.5
	trimPaddingSec            = 1.25
	trimMinDurationSec        = 2.0
	trimMaxDurationSec        = 18.0
	trimMinPeakDensity        = 0.002
	trimSplitDetectGap        = 0.08 // ankle X gap indicating jerk split
	trimSplitConvergeGap      = 0.05 // recovery complete when gap narrows to this
	trimMaxRecoverySec        = 3.0  // max forward extension from window end
	trimAnkleSmoothWindow     = 5    // frames for smoothing noisy ankle gap
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
	liftStart, liftEnd, confident, err := detectLiftDensityBridged(keypointsPath)
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
	trimStart := liftStart - trimPaddingSec
	if trimStart < 0 {
		trimStart = 0
	}
	trimEnd := liftEnd + trimPaddingSec
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

// detectLiftDensityBridged analyzes motion diversity across frames to find the lift's
// start and end times. Motion diversity measures how much individual keypoints deviate
// from the mean displacement vector — high during lifting (body parts move in different
// directions), low during walking (uniform translation).
func detectLiftDensityBridged(keypointsPath string) (startSec, endSec float64, confident bool, err error) {
	data, err := os.ReadFile(keypointsPath)
	if err != nil {
		return 0, 0, false, fmt.Errorf("read keypoints: %w", err)
	}

	var result pose.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, 0, false, fmt.Errorf("parse keypoints: %w", err)
	}

	if len(result.Frames) < 2 {
		slog.Warn("insufficient frames for diversity analysis", "frames", len(result.Frames))
		return 0, 0, false, nil
	}

	// Phase 1: Compute motion diversity signal.
	diversity := computeMotionDiversity(result.Frames)

	// Smooth diversity with moving average.
	smoothed := smoothValues(diversity, trimSmoothWindow)

	// Estimate frame rate from time offsets.
	fps := estimateFPS(result.Frames)

	// Phase 2: Densest window search.
	// Build prefix sum for O(1) window sum queries.
	prefixSum := make([]float64, len(smoothed)+1)
	for i, v := range smoothed {
		prefixSum[i+1] = prefixSum[i] + v
	}

	minWinFrames := int(trimMinWinSec * fps)
	maxWinFrames := int(trimMaxWinSec * fps)
	winStepFrames := int(trimWinStepSec * fps)
	if minWinFrames < 1 {
		minWinFrames = 1
	}
	if maxWinFrames > len(smoothed) {
		maxWinFrames = len(smoothed)
	}
	if winStepFrames < 1 {
		winStepFrames = 1
	}

	var bestDensity float64
	var bestStart, bestEnd int
	found := false

	for winSize := minWinFrames; winSize <= maxWinFrames; winSize += winStepFrames {
		for start := 0; start+winSize <= len(smoothed); start++ {
			end := start + winSize
			windowSum := prefixSum[end] - prefixSum[start]
			density := windowSum / float64(winSize)
			if density > bestDensity {
				bestDensity = density
				bestStart = start
				bestEnd = end
				found = true
			}
		}
	}

	if !found || bestDensity < trimMinPeakDensity {
		slog.Warn("no dense lift window found", "best_density", bestDensity, "threshold", trimMinPeakDensity)
		return 0, 0, false, nil
	}

	// Phase 2.5: Ankle split recovery extension.
	// After a jerk catch, the lifter is in a split stance. Extend end until feet converge.
	endFrame := bestEnd
	lookAheadFrames := int(0.5 * fps)
	scanStart := endFrame - 2
	if scanStart < 0 {
		scanStart = 0
	}
	scanEnd := endFrame + lookAheadFrames
	if scanEnd > len(result.Frames)-1 {
		scanEnd = len(result.Frames) - 1
	}

	// Check ankle gap at/near window end (look back 2 frames + forward 0.5s).
	var maxGapNearEnd float64
	for fi := scanStart; fi <= scanEnd && fi < len(result.Frames); fi++ {
		gap, ok := getAnkleGap(result.Frames[fi])
		if ok && gap > maxGapNearEnd {
			maxGapNearEnd = gap
		}
	}

	if maxGapNearEnd >= trimSplitDetectGap {
		// Split stance detected — extend until feet converge.
		maxRecoveryFrames := int(trimMaxRecoverySec * fps)
		recoveryLimit := endFrame + maxRecoveryFrames
		if recoveryLimit > len(result.Frames)-1 {
			recoveryLimit = len(result.Frames) - 1
		}

		// Collect raw ankle gaps for smoothing.
		rawGaps := make([]float64, 0, recoveryLimit-endFrame+1)
		for fi := endFrame; fi <= recoveryLimit; fi++ {
			gap, ok := getAnkleGap(result.Frames[fi])
			if !ok {
				// Carry forward last known gap, or 0.0 if none yet.
				if len(rawGaps) > 0 {
					rawGaps = append(rawGaps, rawGaps[len(rawGaps)-1])
				} else {
					rawGaps = append(rawGaps, 0.0)
				}
			} else {
				rawGaps = append(rawGaps, gap)
			}
		}

		smoothedGaps := smoothValues(rawGaps, trimAnkleSmoothWindow)

		// Find first frame where smoothed gap < convergence threshold.
		for i, sg := range smoothedGaps {
			if sg < trimSplitConvergeGap {
				endFrame = endFrame + i
				break
			}
			if i == len(smoothedGaps)-1 {
				// No convergence found — extend to scan limit.
				endFrame = recoveryLimit
			}
		}
	}

	// Phase 3: Convert frame indices to seconds + padding.
	// diversity[i] represents the transition from frames[i] to frames[i+1].
	// bestStart in diversity corresponds to the transition starting at frames[bestStart].
	// Use frames[bestStart+1] as the start time (the frame after the first transition).
	startFrameIdx := bestStart + 1
	if startFrameIdx >= len(result.Frames) {
		startFrameIdx = len(result.Frames) - 1
	}
	endFrameIdx := endFrame
	if endFrameIdx >= len(result.Frames) {
		endFrameIdx = len(result.Frames) - 1
	}

	startSec = float64(result.Frames[startFrameIdx].TimeOffsetMs) / 1000.0
	endSec = float64(result.Frames[endFrameIdx].TimeOffsetMs) / 1000.0

	// Validate duration.
	liftDuration := endSec - startSec
	if liftDuration < trimMinDurationSec {
		slog.Warn("density window too short",
			"duration", fmt.Sprintf("%.1fs", liftDuration),
			"min", fmt.Sprintf("%.1fs", trimMinDurationSec),
		)
		return 0, 0, false, nil
	}
	if liftDuration > trimMaxDurationSec {
		slog.Warn("density window too long",
			"duration", fmt.Sprintf("%.1fs", liftDuration),
			"max", fmt.Sprintf("%.1fs", trimMaxDurationSec),
		)
		return 0, 0, false, nil
	}

	return startSec, endSec, true, nil
}

// computeMotionDiversity computes per-frame-transition motion diversity.
// For each consecutive frame pair, it measures how much individual keypoints
// deviate from the mean displacement vector. High values indicate lifting
// (body parts moving in different directions), low values indicate walking
// or stillness (uniform translation).
func computeMotionDiversity(frames []pose.Frame) []float64 {
	diversity := make([]float64, len(frames)-1)

	for i := 1; i < len(frames); i++ {
		prevKPs := make(map[string]pose.Keypoint, len(frames[i-1].Keypoints))
		for _, kp := range frames[i-1].Keypoints {
			prevKPs[kp.Name] = kp
		}

		// Collect per-keypoint displacement vectors for confident keypoints.
		type vec struct{ dx, dy float64 }
		var disps []vec

		for _, kp := range frames[i].Keypoints {
			prev, ok := prevKPs[kp.Name]
			if !ok {
				continue
			}
			if prev.Confidence < trimMinKeypointConfidence || kp.Confidence < trimMinKeypointConfidence {
				continue
			}
			disps = append(disps, vec{dx: kp.X - prev.X, dy: kp.Y - prev.Y})
		}

		if len(disps) < trimMinKeypointsPerFrame {
			continue
		}

		// Compute mean displacement vector (rigid body translation).
		var meanDX, meanDY float64
		for _, d := range disps {
			meanDX += d.dx
			meanDY += d.dy
		}
		meanDX /= float64(len(disps))
		meanDY /= float64(len(disps))

		// Compute per-keypoint deviation from mean vector.
		var totalDeviation float64
		for _, d := range disps {
			devX := d.dx - meanDX
			devY := d.dy - meanDY
			totalDeviation += math.Sqrt(devX*devX + devY*devY)
		}

		diversity[i-1] = totalDeviation / float64(len(disps))
	}

	return diversity
}

// getAnkleGap returns the horizontal distance between left and right ankle keypoints.
// Returns the gap and whether both ankles were found with sufficient confidence.
func getAnkleGap(frame pose.Frame) (float64, bool) {
	var leftX, rightX float64
	var foundLeft, foundRight bool

	for _, kp := range frame.Keypoints {
		if kp.Confidence < trimMinKeypointConfidence {
			continue
		}
		switch kp.Name {
		case pose.LandmarkLeftAnkle:
			leftX = kp.X
			foundLeft = true
		case pose.LandmarkRightAnkle:
			rightX = kp.X
			foundRight = true
		}
	}

	if !foundLeft || !foundRight {
		return 0, false
	}

	return math.Abs(leftX - rightX), true
}

// smoothValues applies a moving average to reduce noise in a signal.
func smoothValues(values []float64, window int) []float64 {
	smoothed := make([]float64, len(values))
	halfWin := window / 2

	for i := range values {
		start := i - halfWin
		if start < 0 {
			start = 0
		}
		end := i + halfWin + 1
		if end > len(values) {
			end = len(values)
		}
		var sum float64
		for j := start; j < end; j++ {
			sum += values[j]
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
