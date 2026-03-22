package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// CropParams holds the crop region and source dimensions for downstream coordinate transformation.
type CropParams struct {
	X            int `json:"x"`
	Y            int `json:"y"`
	W            int `json:"w"`
	H            int `json:"h"`
	SourceWidth  int `json:"sourceWidth"`
	SourceHeight int `json:"sourceHeight"`
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
	trimParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimParams)
	if trimData, err := os.ReadFile(trimParamsPath); err == nil {
		var tp TrimParams
		if err := json.Unmarshal(trimData, &tp); err == nil {
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

	// Compute hybrid per-frame X centers.
	cxList := computeHybridCenters(frames, sourceW)

	// Convert centers to top-left origins.
	xs, ys := centersToOrigins(cxList, cropW, sourceW, originY)

	// Write crop-params.json (use first stationary segment's lock X origin).
	params := CropParams{
		X:            xs[0],
		Y:            ys[0],
		W:            cropW,
		H:            cropH,
		SourceWidth:  sourceW,
		SourceHeight: sourceH,
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

	// Get video FPS for frame piping.
	videoFPS, err := ffmpeg.GetFPS(ctx, input.VideoPath)
	if err != nil {
		logger.Warn("failed to get video FPS, falling back to 30", "error", err)
		videoFPS = 30.0
	}

	// Build keypoint times for interpolation.
	kpTimes := make([]float64, len(frames))
	for i, f := range frames {
		kpTimes[i] = float64(f.TimeOffsetMs) / 1000.0
	}

	// Get trim boundaries for frame mapping.
	trimStart := kpTimes[0]
	trimEnd := kpTimes[len(kpTimes)-1]

	// Interpolate keypoint-rate positions to video frame rate.
	positions := interpolateToVideoFrames(kpTimes, xs, ys, videoFPS, trimStart, trimEnd)

	// Per-frame crop via FFmpeg pipe.
	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropped)
	if err := cropVideoPerFrame(ctx, input.VideoPath, outputPath, sourceW, sourceH, cropW, cropH, videoFPS, positions); err != nil {
		logger.Error("ffmpeg per-frame crop failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: ffmpeg: %w", err)
	}

	// Extract thumbnail from the cropped video.
	extractThumbnail(ctx, logger, input.DataDir, input.LiftID, outputPath)

	walkingPct := computeWalkingPercent(frames, sourceW)
	logger.Info("crop complete",
		"crop_w", cropW,
		"crop_h", cropH,
		"walking_pct", fmt.Sprintf("%.0f%%", walkingPct*100),
		"video_frames", len(positions),
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
		cropTop = sh - boxH
	}
	if cropTop < 0 {
		cropTop = 0
		boxH = sh
		boxW = boxH * (float64(cropAspectW) / float64(cropAspectH))
	}
	if boxW > sw {
		boxW = sw
		boxH = boxW / (float64(cropAspectW) / float64(cropAspectH))
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

// computeHybridCenters computes per-frame X center positions using the hybrid algorithm.
// Stationary frames lock at per-segment median X, walking frames interpolate.
func computeHybridCenters(frames []pose.Frame, sourceW int) []float64 {
	sw := float64(sourceW)
	n := len(frames)
	if n == 0 {
		return nil
	}

	// Pass 1: Compute raw per-frame bbox center X in pixels.
	rawCX := make([]float64, n)
	for i, f := range frames {
		rawCX[i] = ((f.BoundingBox.Left + f.BoundingBox.Right) / 2) * sw
	}

	// Smooth raw CX.
	smoothedCX := smoothValues(rawCX, trackingSmoothFrames)

	// Compute X velocity (absolute difference of smoothed CX).
	xVel := make([]float64, n)
	for i := 1; i < n; i++ {
		xVel[i] = math.Abs(smoothedCX[i] - smoothedCX[i-1])
	}

	// Smooth velocity.
	smoothedVel := smoothValues(xVel, hybridVelSmoothFrames)

	// Classify: walking if velocity > threshold.
	isWalking := make([]bool, n)
	for i := range smoothedVel {
		isWalking[i] = smoothedVel[i] > hybridXVelThreshold
	}

	// Pass 2: Find contiguous segments and compute lock points.
	type segment struct {
		start, end int
		walking    bool
		lockX      float64
	}
	var segments []segment
	segStart := 0
	for i := 1; i <= n; i++ {
		if i == n || isWalking[i] != isWalking[segStart] {
			seg := segment{start: segStart, end: i, walking: isWalking[segStart]}
			if !seg.walking {
				// Compute median raw CX for this stationary segment.
				vals := make([]float64, seg.end-seg.start)
				for j := seg.start; j < seg.end; j++ {
					vals[j-seg.start] = rawCX[j]
				}
				seg.lockX = median(vals)
			}
			segments = append(segments, seg)
			segStart = i
		}
	}

	// Pass 3: Assign X positions.
	cxList := make([]float64, n)
	for si, seg := range segments {
		if !seg.walking {
			for i := seg.start; i < seg.end; i++ {
				cxList[i] = seg.lockX
			}
		} else {
			// Find adjacent stationary segments.
			var prevLock, nextLock float64
			hasPrev, hasNext := false, false
			for pi := si - 1; pi >= 0; pi-- {
				if !segments[pi].walking {
					prevLock = segments[pi].lockX
					hasPrev = true
					break
				}
			}
			for ni := si + 1; ni < len(segments); ni++ {
				if !segments[ni].walking {
					nextLock = segments[ni].lockX
					hasNext = true
					break
				}
			}

			for i := seg.start; i < seg.end; i++ {
				if hasPrev && hasNext {
					// Linear interpolation.
					t := float64(i-seg.start) / float64(seg.end-seg.start)
					cxList[i] = prevLock + t*(nextLock-prevLock)
				} else if hasPrev {
					cxList[i] = prevLock
				} else if hasNext {
					cxList[i] = nextLock
				} else {
					// No adjacent locks — fallback to smoothed tracking.
					cxList[i] = smoothedCX[i]
				}
			}
		}
	}

	return cxList
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
	xVel := make([]float64, n)
	for i := 1; i < n; i++ {
		xVel[i] = math.Abs(smoothedCX[i] - smoothedCX[i-1])
	}
	smoothedVel := smoothValues(xVel, hybridVelSmoothFrames)

	walkCount := 0
	for _, v := range smoothedVel {
		if v > hybridXVelThreshold {
			walkCount++
		}
	}
	return float64(walkCount) / float64(n)
}

// centersToOrigins converts per-frame X centers to top-left crop origins, clamped to frame.
func centersToOrigins(cxList []float64, cropW, sourceW int, originY int) (xs, ys []int) {
	xs = make([]int, len(cxList))
	ys = make([]int, len(cxList))
	for i, cx := range cxList {
		x := int(math.Round(cx)) - cropW/2
		if x < 0 {
			x = 0
		}
		if x+cropW > sourceW {
			x = sourceW - cropW
		}
		if x < 0 {
			x = 0
		}
		xs[i] = x
		ys[i] = originY
	}
	return xs, ys
}

// interpolateToVideoFrames interpolates keypoint-rate positions to video frame rate.
func interpolateToVideoFrames(kpTimes []float64, kpXs, kpYs []int, videoFPS, trimStart, trimEnd float64) [][2]int {
	if len(kpTimes) == 0 {
		return nil
	}

	totalDuration := trimEnd - trimStart
	totalVideoFrames := int(math.Round(totalDuration * videoFPS))
	if totalVideoFrames < 1 {
		totalVideoFrames = 1
	}

	positions := make([][2]int, totalVideoFrames)
	for vi := 0; vi < totalVideoFrames; vi++ {
		t := trimStart + float64(vi)/videoFPS

		// Find bracketing keypoint frames.
		idx := sort.SearchFloat64s(kpTimes, t)
		if idx == 0 {
			positions[vi] = [2]int{kpXs[0], kpYs[0]}
		} else if idx >= len(kpTimes) {
			positions[vi] = [2]int{kpXs[len(kpXs)-1], kpYs[len(kpYs)-1]}
		} else {
			// Linear interpolation between bracketing keypoints.
			t0 := kpTimes[idx-1]
			t1 := kpTimes[idx]
			dt := t1 - t0
			if dt < 1e-9 {
				positions[vi] = [2]int{kpXs[idx], kpYs[idx]}
			} else {
				frac := (t - t0) / dt
				x := float64(kpXs[idx-1]) + frac*float64(kpXs[idx]-kpXs[idx-1])
				y := float64(kpYs[idx-1]) + frac*float64(kpYs[idx]-kpYs[idx-1])
				positions[vi] = [2]int{int(math.Round(x)), int(math.Round(y))}
			}
		}
	}

	return positions
}

// cropVideoPerFrame performs per-frame crop via FFmpeg decode/encode pipes.
func cropVideoPerFrame(ctx context.Context, input, output string, sourceW, sourceH, cropW, cropH int, fps float64, positions [][2]int) error {
	frameSize := sourceW * sourceH * 3
	cropFrameSize := cropW * cropH * 3

	decCmd, decOut, err := ffmpeg.DecodeFrames(ctx, input)
	if err != nil {
		return fmt.Errorf("start decode: %w", err)
	}

	encCmd, encIn, err := ffmpeg.EncodeFrames(ctx, output, cropW, cropH, fps)
	if err != nil {
		decCmd.Process.Kill()
		decCmd.Wait()
		return fmt.Errorf("start encode: %w", err)
	}

	srcBuf := make([]byte, frameSize)
	cropBuf := make([]byte, cropFrameSize)

	frameIdx := 0
	for {
		// Read one full source frame.
		_, err := io.ReadFull(decOut, srcBuf)
		if err != nil {
			break // EOF or error — done reading
		}

		// Determine crop position for this frame.
		var cx, cy int
		if frameIdx < len(positions) {
			cx = positions[frameIdx][0]
			cy = positions[frameIdx][1]
		} else if len(positions) > 0 {
			// Hold last position for any extra frames.
			cx = positions[len(positions)-1][0]
			cy = positions[len(positions)-1][1]
		}

		// Clamp.
		if cx < 0 {
			cx = 0
		}
		if cy < 0 {
			cy = 0
		}
		if cx+cropW > sourceW {
			cx = sourceW - cropW
		}
		if cy+cropH > sourceH {
			cy = sourceH - cropH
		}

		// Crop: extract rectangle from source frame.
		for row := 0; row < cropH; row++ {
			srcOffset := ((cy+row)*sourceW + cx) * 3
			dstOffset := row * cropW * 3
			copy(cropBuf[dstOffset:dstOffset+cropW*3], srcBuf[srcOffset:srcOffset+cropW*3])
		}

		if _, err := encIn.Write(cropBuf); err != nil {
			break
		}

		frameIdx++
	}

	// Clean up pipes and wait for processes.
	encIn.Close()
	encErr := encCmd.Wait()
	decOut.Close()
	decErr := decCmd.Wait()

	if decErr != nil {
		return fmt.Errorf("decode: %w", decErr)
	}
	if encErr != nil {
		return fmt.Errorf("encode: %w", encErr)
	}
	if frameIdx == 0 {
		return fmt.Errorf("decode: no frames read from input")
	}
	return nil
}
