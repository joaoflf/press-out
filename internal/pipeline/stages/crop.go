package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

const (
	cropAspectW = 9
	cropAspectH = 16

	// Extent-based Y sizing.
	barTopPaddingPx     = 150  // px above min bbox top for bar + plates overhead
	footBottomPaddingPx = 40   // px below P95 bbox bottom for feet
	cropHorizontalPad   = 0.30 // horizontal padding around P95 body width (bar + plates)

	// Hybrid X tracking.
	trackingSmoothFrames  = 31  // ~1s at 30fps for smoothing raw bbox X
	hybridXVelThreshold   = 3.0 // px/frame — below this, lifter is "stationary"
	hybridVelSmoothFrames = 15  // smooth velocity signal to debounce
)

// CropSegment defines a time range with start/end crop X positions for dynamic crop.
type CropSegment struct {
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	StartX   int     `json:"startX"`
	EndX     int     `json:"endX"`
}

// CropParams holds the crop region and source dimensions for downstream coordinate transformation.
type CropParams struct {
	X            int           `json:"x"`
	Y            int           `json:"y"`
	W            int           `json:"w"`
	H            int           `json:"h"`
	SourceWidth  int           `json:"sourceWidth"`
	SourceHeight int           `json:"sourceHeight"`
	Segments     []CropSegment `json:"segments,omitempty"`
}

// CropXAtTime returns the crop X position at time t (seconds from video start).
// Falls back to static X if no segments are defined.
func (cp *CropParams) CropXAtTime(t float64) int {
	if len(cp.Segments) == 0 {
		return cp.X
	}
	for _, seg := range cp.Segments {
		if t >= seg.StartSec && t <= seg.EndSec {
			if seg.StartX == seg.EndX || seg.EndSec <= seg.StartSec {
				return seg.StartX
			}
			frac := (t - seg.StartSec) / (seg.EndSec - seg.StartSec)
			return int(math.Round(float64(seg.StartX) + frac*float64(seg.EndX-seg.StartX)))
		}
	}
	if t < cp.Segments[0].StartSec {
		return cp.Segments[0].StartX
	}
	return cp.Segments[len(cp.Segments)-1].EndX
}

// CropStage crops the video to focus on the lifter using keypoints bounding boxes.
type CropStage struct{}

func (s *CropStage) Name() string { return pipeline.StageCropping }

func (s *CropStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StageCropping)

	keypointsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)

	// Check if keypoints.json exists.
	keypointsData, err := os.ReadFile(keypointsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("keypoints not available, preserving full frame", "lift_id", input.LiftID)
			extractThumbnail(ctx, logger, input.DataDir, input.LiftID, input.VideoPath)
			return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
		}
		logger.Error("failed to read keypoints", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: read keypoints: %w", err)
	}

	// Parse keypoints.
	var result pose.Result
	if err := json.Unmarshal(keypointsData, &result); err != nil {
		logger.Error("failed to parse keypoints", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: parse keypoints: %w", err)
	}

	if len(result.Frames) == 0 {
		logger.Warn("keypoints has no frames, preserving full frame", "lift_id", input.LiftID)
		extractThumbnail(ctx, logger, input.DataDir, input.LiftID, input.VideoPath)
		return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
	}

	// Use display dimensions from keypoints.json.
	sourceW := result.SourceWidth
	sourceH := result.SourceHeight
	if sourceW == 0 || sourceH == 0 {
		var err error
		sourceW, sourceH, err = ffmpeg.GetDimensions(ctx, input.VideoPath)
		if err != nil {
			logger.Error("failed to get video dimensions", "error", err, "duration_ms", time.Since(start).Milliseconds())
			return pipeline.StageOutput{}, fmt.Errorf("crop: get dimensions: %w", err)
		}
	}

	// Filter frames to trimmed range if trim-params.json exists.
	frames := result.Frames
	var trimStartMs int64
	trimParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimParams)
	if trimData, err := os.ReadFile(trimParamsPath); err == nil {
		var tp TrimParams
		if err := json.Unmarshal(trimData, &tp); err == nil {
			trimStartMs = tp.TrimStartMs
			filtered := make([]pose.Frame, 0, len(frames))
			for _, f := range frames {
				if f.TimeOffsetMs >= tp.TrimStartMs && f.TimeOffsetMs <= tp.TrimEndMs {
					filtered = append(filtered, f)
				}
			}
			if len(filtered) > 0 {
				logger.Info("filtered keypoints to trim range",
					"total_frames", len(frames),
					"trimmed_frames", len(filtered),
					"trim_start_ms", tp.TrimStartMs,
					"trim_end_ms", tp.TrimEndMs,
				)
				frames = filtered
			} else {
				logger.Warn("no keypoints within trim range, using all frames")
			}
		}
	}

	// Compute extent-based crop size.
	cropW, cropH, originY := computeExtentCropRegion(frames, sourceW, sourceH)

	// Compute hybrid X segments (track walks, lock lifts).
	segments := computeHybridSegments(frames, sourceW, cropW, trimStartMs)

	// Determine representative static X (first stationary segment or median fallback).
	cropX := 0
	if len(segments) > 0 {
		cropX = segments[0].StartX
	} else {
		rawCX := make([]float64, len(frames))
		for i, f := range frames {
			rawCX[i] = ((f.BoundingBox.Left + f.BoundingBox.Right) / 2) * float64(sourceW)
		}
		cropX = int(math.Round(median(rawCX))) - cropW/2
		if cropX < 0 {
			cropX = 0
		}
		if cropX+cropW > sourceW {
			cropX = sourceW - cropW
		}
	}

	// Check if dynamic crop is needed (segments have different X positions).
	isDynamic := false
	if len(segments) > 1 {
		firstX := segments[0].StartX
		for _, seg := range segments {
			if seg.StartX != firstX || seg.EndX != firstX {
				isDynamic = true
				break
			}
		}
	}

	// Write crop-params.json.
	params := CropParams{
		X:            cropX,
		Y:            originY,
		W:            cropW,
		H:            cropH,
		SourceWidth:  sourceW,
		SourceHeight: sourceH,
		Segments:     segments,
	}
	paramsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropParams)
	paramsData, err := json.Marshal(params)
	if err != nil {
		logger.Error("failed to marshal crop params", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: marshal params: %w", err)
	}
	if err := os.WriteFile(paramsPath, paramsData, 0644); err != nil {
		logger.Error("failed to write crop params", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: write params: %w", err)
	}

	// Crop via FFmpeg.
	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropped)
	if isDynamic {
		xExpr := buildCropXExpr(segments)
		logger.Info("using dynamic crop", "segments", len(segments))
		if err := ffmpeg.CropVideoExpr(ctx, input.VideoPath, outputPath, xExpr, originY, cropW, cropH); err != nil {
			logger.Error("ffmpeg crop failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
			return pipeline.StageOutput{}, fmt.Errorf("crop: ffmpeg: %w", err)
		}
	} else {
		if err := ffmpeg.CropVideo(ctx, input.VideoPath, outputPath, cropX, originY, cropW, cropH); err != nil {
			logger.Error("ffmpeg crop failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
			return pipeline.StageOutput{}, fmt.Errorf("crop: ffmpeg: %w", err)
		}
	}

	// Extract thumbnail from the cropped video.
	extractThumbnail(ctx, logger, input.DataDir, input.LiftID, outputPath)

	walkingPct := computeWalkingPercent(frames, sourceW)
	logger.Info("crop complete",
		"crop_x", cropX,
		"crop_y", originY,
		"crop_w", cropW,
		"crop_h", cropH,
		"walking_pct", fmt.Sprintf("%.0f%%", walkingPct*100),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: outputPath}, nil
}

// extractThumbnail extracts a thumbnail at the video midpoint. Failure is non-fatal.
func extractThumbnail(ctx context.Context, logger *slog.Logger, dataDir string, liftID int64, videoPath string) {
	duration, err := ffmpeg.GetDuration(ctx, videoPath)
	if err != nil {
		logger.Warn("failed to get duration for thumbnail", "error", err)
		return
	}

	thumbnailPath := storage.LiftFile(dataDir, liftID, storage.FileThumbnail)
	if err := ffmpeg.ExtractThumbnail(ctx, videoPath, thumbnailPath, duration/2); err != nil {
		logger.Warn("thumbnail extraction failed", "error", err)
	}
}

// median returns the median value of a float64 slice.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// percentile returns the p-th percentile (0-100) of a float64 slice.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	k := (p / 100.0) * float64(len(sorted)-1)
	f := math.Floor(k)
	c := math.Ceil(k)
	if f == c {
		return sorted[int(f)]
	}
	return sorted[int(f)]*(c-k) + sorted[int(c)]*(k-f)
}

// computeExtentCropRegion computes the crop rectangle size using extent-based Y sizing.
// Returns width, height, and top-left Y origin (static for all frames).
func computeExtentCropRegion(frames []pose.Frame, sourceW, sourceH int) (w, h, originY int) {
	sw := float64(sourceW)
	sh := float64(sourceH)

	tops := make([]float64, 0, len(frames))
	bottoms := make([]float64, 0, len(frames))
	widths := make([]float64, 0, len(frames))

	for _, f := range frames {
		bb := f.BoundingBox
		tops = append(tops, bb.Top*sh)
		bottoms = append(bottoms, bb.Bottom*sh)
		widths = append(widths, (bb.Right-bb.Left)*sw)
	}

	// Extent-based Y: min(tops) - bar padding to P95(bottoms) + foot padding.
	cropTop := minFloat(tops) - float64(barTopPaddingPx)
	cropBottom := percentile(bottoms, 95) + float64(footBottomPaddingPx)

	boxH := cropBottom - cropTop
	boxW := boxH * (float64(cropAspectW) / float64(cropAspectH))

	// Ensure body + plate width is covered.
	minW := percentile(widths, 95) * (1 + 2*cropHorizontalPad)
	if minW > boxW {
		boxW = minW
		boxH = boxW / (float64(cropAspectW) / float64(cropAspectH))
		// Re-center vertically around extent midpoint.
		midY := (cropTop + cropBottom) / 2
		cropTop = midY - boxH/2
	}

	// Clamp to frame bounds.
	if cropTop < 0 {
		cropTop = 0
	}
	if cropTop+boxH > sh {
		boxH = sh - cropTop
		boxW = boxH * (float64(cropAspectW) / float64(cropAspectH))
	}
	if boxW > sw {
		boxW = sw
		boxH = boxW / (float64(cropAspectW) / float64(cropAspectH))
		// Reposition from crop_bottom so feet stay visible.
		cropTop = math.Max(0, cropBottom-boxH)
	}

	// Round to even dimensions.
	rw := int(math.Round(boxW))
	rh := int(math.Round(boxH))
	if rw%2 != 0 {
		rw--
	}
	if rh%2 != 0 {
		rh--
	}
	ry := int(math.Round(cropTop))

	return rw, rh, ry
}

// minFloat returns the minimum value in a float64 slice.
func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}


// computeHybridSegments classifies keypoint frames as walking or stationary
// and returns crop segments with per-segment X positions.
// Walking segments linearly interpolate between adjacent stationary lock points.
// Times are relative to the trimmed video start (trimStartMs subtracted).
func computeHybridSegments(frames []pose.Frame, sourceW, cropW int, trimStartMs int64) []CropSegment {
	n := len(frames)
	if n < 2 {
		return nil
	}

	sw := float64(sourceW)
	rawCX := make([]float64, n)
	for i, f := range frames {
		rawCX[i] = ((f.BoundingBox.Left + f.BoundingBox.Right) / 2) * sw
	}

	// Smooth X for tracking trajectory.
	smoothedCX := smoothValues(rawCX, trackingSmoothFrames)

	// Compute velocity from smoothed X (n-1 values), smooth, then prepend 0.0.
	xVelRaw := make([]float64, n-1)
	for i := 1; i < n; i++ {
		xVelRaw[i-1] = math.Abs(smoothedCX[i] - smoothedCX[i-1])
	}
	xVelSmoothed := smoothValues(xVelRaw, hybridVelSmoothFrames)

	// Pad first frame with 0.0 velocity (smooth before pad, matching Python spike).
	smoothedVel := make([]float64, n)
	smoothedVel[0] = 0.0
	copy(smoothedVel[1:], xVelSmoothed)

	// Classify frames as walking/stationary.
	isWalking := make([]bool, n)
	for i, v := range smoothedVel {
		isWalking[i] = v > hybridXVelThreshold
	}

	// Find contiguous segments.
	type segment struct {
		walking bool
		start   int
		end     int
	}
	var segments []segment
	i := 0
	for i < n {
		segStart := i
		walk := isWalking[i]
		for i < n && isWalking[i] == walk {
			i++
		}
		segments = append(segments, segment{walking: walk, start: segStart, end: i})
	}

	// Compute lock points for stationary segments (median of raw bbox center X).
	lockPoints := make(map[int]float64)
	for idx, seg := range segments {
		if !seg.walking {
			lockPoints[idx] = median(rawCX[seg.start:seg.end])
		}
	}

	// Build per-frame X centers.
	hybridCX := make([]float64, n)
	for idx, seg := range segments {
		if !seg.walking {
			lockX := lockPoints[idx]
			for j := seg.start; j < seg.end; j++ {
				hybridCX[j] = lockX
			}
		} else {
			// Walking: linearly interpolate between adjacent lock points.
			var prevLock, nextLock float64
			prevFound, nextFound := false, false
			for k := idx - 1; k >= 0; k-- {
				if lp, ok := lockPoints[k]; ok {
					prevLock = lp
					prevFound = true
					break
				}
			}
			for k := idx + 1; k < len(segments); k++ {
				if lp, ok := lockPoints[k]; ok {
					nextLock = lp
					nextFound = true
					break
				}
			}

			segLen := seg.end - seg.start
			for j := seg.start; j < seg.end; j++ {
				switch {
				case prevFound && nextFound:
					t := 0.0
					if segLen > 1 {
						t = float64(j-seg.start) / float64(segLen-1)
					}
					hybridCX[j] = prevLock + t*(nextLock-prevLock)
				case prevFound:
					hybridCX[j] = prevLock
				case nextFound:
					hybridCX[j] = nextLock
				default:
					hybridCX[j] = smoothedCX[j]
				}
			}
		}
	}

	// Convert centers to clamped origin X.
	clampX := func(cx float64) int {
		x := int(math.Round(cx)) - cropW/2
		if x < 0 {
			x = 0
		}
		if x+cropW > sourceW {
			x = sourceW - cropW
		}
		return x
	}

	// Build output segments with times relative to trim start.
	var cropSegments []CropSegment
	for _, seg := range segments {
		startSec := float64(frames[seg.start].TimeOffsetMs-trimStartMs) / 1000.0
		endSec := float64(frames[seg.end-1].TimeOffsetMs-trimStartMs) / 1000.0
		startX := clampX(hybridCX[seg.start])
		endX := clampX(hybridCX[seg.end-1])
		cropSegments = append(cropSegments, CropSegment{
			StartSec: startSec,
			EndSec:   endSec,
			StartX:   startX,
			EndX:     endX,
		})
	}

	return cropSegments
}

// buildCropXExpr builds an FFmpeg expression for dynamic X positioning from segments.
// The expression uses FFmpeg's `t` variable (time in seconds) to compute the crop X
// position as a piecewise linear function.
func buildCropXExpr(segments []CropSegment) string {
	if len(segments) == 0 {
		return "0"
	}

	// Build nested if expression from last segment to first.
	// After all segments, fall back to last segment's EndX.
	expr := fmt.Sprintf("%d", segments[len(segments)-1].EndX)

	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		dur := seg.EndSec - seg.StartSec

		var segExpr string
		if seg.StartX == seg.EndX || dur <= 0 {
			segExpr = fmt.Sprintf("%d", seg.StartX)
		} else {
			segExpr = fmt.Sprintf("(%d+((t-%.4f)/%.4f)*(%d))",
				seg.StartX, seg.StartSec, dur, seg.EndX-seg.StartX)
		}

		expr = fmt.Sprintf("if(lt(t,%.4f),%s,%s)", seg.EndSec, segExpr, expr)
	}

	return expr
}

// computeWalkingPercent returns the fraction of frames classified as walking.
func computeWalkingPercent(frames []pose.Frame, sourceW int) float64 {
	sw := float64(sourceW)
	n := len(frames)
	if n < 2 {
		return 0
	}

	rawCX := make([]float64, n)
	for i, f := range frames {
		rawCX[i] = ((f.BoundingBox.Left + f.BoundingBox.Right) / 2) * sw
	}

	smoothedCX := smoothValues(rawCX, trackingSmoothFrames)
	xVelRaw := make([]float64, n-1)
	for i := 1; i < n; i++ {
		xVelRaw[i-1] = math.Abs(smoothedCX[i] - smoothedCX[i-1])
	}
	xVelSmoothed := smoothValues(xVelRaw, hybridVelSmoothFrames)
	smoothedVel := make([]float64, n)
	smoothedVel[0] = 0.0
	copy(smoothedVel[1:], xVelSmoothed)

	walkCount := 0
	for _, v := range smoothedVel {
		if v > hybridXVelThreshold {
			walkCount++
		}
	}
	return float64(walkCount) / float64(n)
}

