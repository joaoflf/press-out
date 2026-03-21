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
	cropAspectW              = 9
	cropAspectH              = 16
	cropPaddingPercent       = 0.10
	noseCenterMinConfidence  = 0.3
	// Fraction of frames from the end used for horizontal centering (standing/lockout position).
	lockoutFraction         = 0.20
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

	// Use display dimensions from keypoints.json. These are the post-rotation
	// dimensions that match what FFmpeg's crop filter sees after auto-rotation.
	// Using ffprobe stream dimensions would be wrong for rotated videos.
	sourceW := result.SourceWidth
	sourceH := result.SourceHeight
	if sourceW == 0 || sourceH == 0 {
		// Fallback to ffprobe if keypoints lack dimensions.
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

	// Compute enclosing bounding box across trimmed frames.
	cropX, cropY, cropW, cropH := computeCropRegion(frames, sourceW, sourceH)

	// Write crop-params.json.
	params := CropParams{
		X:            cropX,
		Y:            cropY,
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

	// Crop the video via FFmpeg.
	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropped)
	if err := ffmpeg.CropVideo(ctx, input.VideoPath, outputPath, cropX, cropY, cropW, cropH); err != nil {
		logger.Error("ffmpeg crop failed", "error", err, "duration_ms", time.Since(start).Milliseconds())
		return pipeline.StageOutput{}, fmt.Errorf("crop: ffmpeg: %w", err)
	}

	// Extract thumbnail from the cropped video.
	extractThumbnail(ctx, logger, input.DataDir, input.LiftID, outputPath)

	logger.Info("crop complete",
		"crop_x", cropX,
		"crop_y", cropY,
		"crop_w", cropW,
		"crop_h", cropH,
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
// It sorts a copy of the input so the original is not modified.
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

// noseCenter extracts the nose keypoint coordinates from a frame if confidence is sufficient.
// Returns the normalized (x, y) and true if found, or (0, 0) and false otherwise.
func noseCenter(f pose.Frame) (float64, float64, bool) {
	for _, kp := range f.Keypoints {
		if kp.Name == pose.LandmarkNose && kp.Confidence >= noseCenterMinConfidence {
			return kp.X, kp.Y, true
		}
	}
	return 0, 0, false
}

// computeCropRegion computes the crop rectangle from per-frame bounding boxes.
// It finds the enclosing box, adds padding, enforces 9:16 aspect ratio, and clamps to frame bounds.
// The crop dimensions are derived from the union bounding box (to ensure no frame clips the lifter).
// Horizontal centering uses the median nose keypoint X from the last ~20% of frames
// (standing/lockout position) for stable centering on the finished lift position.
// Vertical centering always uses the median bounding box center Y to keep the full body in frame.
func computeCropRegion(frames []pose.Frame, sourceW, sourceH int) (x, y, w, h int) {
	// Find the enclosing bounding box across all frames (normalized 0-1 coords).
	minLeft := math.MaxFloat64
	minTop := math.MaxFloat64
	maxRight := -math.MaxFloat64
	maxBottom := -math.MaxFloat64

	// Collect per-frame vertical centers for median computation.
	centersY := make([]float64, 0, len(frames))

	// Determine the start index for lockout frames (last ~20%).
	lockoutStart := len(frames) - int(math.Ceil(float64(len(frames))*lockoutFraction))
	if lockoutStart < 0 {
		lockoutStart = 0
	}

	// Collect horizontal centers only from lockout frames.
	centersX := make([]float64, 0, len(frames)-lockoutStart)

	for i, f := range frames {
		bb := f.BoundingBox
		if bb.Left < minLeft {
			minLeft = bb.Left
		}
		if bb.Top < minTop {
			minTop = bb.Top
		}
		if bb.Right > maxRight {
			maxRight = bb.Right
		}
		if bb.Bottom > maxBottom {
			maxBottom = bb.Bottom
		}
		// Vertical centering always uses bounding box center (preserves full body).
		centersY = append(centersY, (bb.Top+bb.Bottom)/2)
		// Horizontal centering: only use lockout frames (last ~20%).
		if i >= lockoutStart {
			if nx, _, ok := noseCenter(f); ok {
				centersX = append(centersX, nx)
			} else {
				centersX = append(centersX, (bb.Left+bb.Right)/2)
			}
		}
	}

	// Convert normalized coordinates to pixel coordinates.
	sw := float64(sourceW)
	sh := float64(sourceH)

	pxLeft := minLeft * sw
	pxTop := minTop * sh
	pxRight := maxRight * sw
	pxBottom := maxBottom * sh

	boxW := pxRight - pxLeft
	boxH := pxBottom - pxTop

	// Add padding (cropPaddingPercent of box dimension on each side).
	padW := boxW * cropPaddingPercent
	padH := boxH * cropPaddingPercent

	pxLeft -= padW
	pxTop -= padH
	pxRight += padW
	pxBottom += padH

	boxW = pxRight - pxLeft
	boxH = pxBottom - pxTop

	// Enforce 9:16 aspect ratio.
	targetRatio := float64(cropAspectW) / float64(cropAspectH)
	currentRatio := boxW / boxH

	// Center on median per-frame center point (nose keypoint preferred, robust to outlier frames).
	centerX := median(centersX) * sw
	centerY := median(centersY) * sh

	if currentRatio > targetRatio {
		// Too wide — increase height.
		boxH = boxW / targetRatio
	} else {
		// Too tall — increase width.
		boxW = boxH * targetRatio
	}

	// Re-center the box on the median center.
	pxLeft = centerX - boxW/2
	pxTop = centerY - boxH/2

	// Clamp to source frame bounds.
	if pxLeft < 0 {
		pxLeft = 0
	}
	if pxTop < 0 {
		pxTop = 0
	}
	if pxLeft+boxW > sw {
		pxLeft = sw - boxW
	}
	if pxTop+boxH > sh {
		pxTop = sh - boxH
	}

	// Final clamp if box is larger than frame.
	if pxLeft < 0 {
		pxLeft = 0
		boxW = sw
	}
	if pxTop < 0 {
		pxTop = 0
		boxH = sh
	}

	// Round to integers and ensure even dimensions (FFmpeg codec requirement).
	x = int(math.Round(pxLeft))
	y = int(math.Round(pxTop))
	w = int(math.Round(boxW))
	h = int(math.Round(boxH))

	if w%2 != 0 {
		w--
	}
	if h%2 != 0 {
		h--
	}

	return x, y, w, h
}
