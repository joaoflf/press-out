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
	skeletonConfidenceThreshold = 0.3
	skeletonBoneThickness       = 3
	skeletonJointRadius         = 5
)

// Skeleton bone colors: left side = cyan, right side = magenta, center = lime.
var (
	colorLeft   = [3]byte{0, 255, 255}   // cyan
	colorRight  = [3]byte{255, 0, 255}   // magenta
	colorCenter = [3]byte{0, 255, 136}   // lime-green
	colorJoint  = [3]byte{255, 255, 255} // white
)

// skeletonBone defines a connection between two keypoints and its color.
type skeletonBone struct {
	from, to string
	color    [3]byte
}

var skeletonBones = []skeletonBone{
	// Head
	{pose.LandmarkNose, pose.LandmarkLeftEye, colorCenter},
	{pose.LandmarkNose, pose.LandmarkRightEye, colorCenter},
	{pose.LandmarkLeftEye, pose.LandmarkLeftEar, colorLeft},
	{pose.LandmarkRightEye, pose.LandmarkRightEar, colorRight},
	// Torso
	{pose.LandmarkLeftShoulder, pose.LandmarkRightShoulder, colorCenter},
	{pose.LandmarkLeftShoulder, pose.LandmarkLeftHip, colorLeft},
	{pose.LandmarkRightShoulder, pose.LandmarkRightHip, colorRight},
	{pose.LandmarkLeftHip, pose.LandmarkRightHip, colorCenter},
	// Left arm
	{pose.LandmarkLeftShoulder, pose.LandmarkLeftElbow, colorLeft},
	{pose.LandmarkLeftElbow, pose.LandmarkLeftWrist, colorLeft},
	// Right arm
	{pose.LandmarkRightShoulder, pose.LandmarkRightElbow, colorRight},
	{pose.LandmarkRightElbow, pose.LandmarkRightWrist, colorRight},
	// Left leg
	{pose.LandmarkLeftHip, pose.LandmarkLeftKnee, colorLeft},
	{pose.LandmarkLeftKnee, pose.LandmarkLeftAnkle, colorLeft},
	// Right leg
	{pose.LandmarkRightHip, pose.LandmarkRightKnee, colorRight},
	{pose.LandmarkRightKnee, pose.LandmarkRightAnkle, colorRight},
}

// SkeletonStage renders a skeleton overlay onto the input video using keypoint data.
type SkeletonStage struct{}

func (s *SkeletonStage) Name() string { return pipeline.StageRenderingSkeleton }

func (s *SkeletonStage) Run(ctx context.Context, input pipeline.StageInput) (pipeline.StageOutput, error) {
	start := time.Now()
	logger := slog.With("lift_id", input.LiftID, "stage", pipeline.StageRenderingSkeleton)

	// Read keypoints.json — skip if absent.
	keypointsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileKeypoints)
	keypointsData, err := os.ReadFile(keypointsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("keypoints not available, skipping skeleton rendering")
			return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
		}
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: read keypoints: %w", err)
	}

	var result pose.Result
	if err := json.Unmarshal(keypointsData, &result); err != nil {
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: parse keypoints: %w", err)
	}
	if len(result.Frames) == 0 {
		logger.Info("keypoints has no frames, skipping skeleton rendering")
		return pipeline.StageOutput{VideoPath: input.VideoPath}, nil
	}

	// Read crop-params.json (optional).
	var cropParams *CropParams
	cropParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileCropParams)
	if cpData, err := os.ReadFile(cropParamsPath); err == nil {
		var cp CropParams
		if err := json.Unmarshal(cpData, &cp); err == nil {
			cropParams = &cp
		}
	}

	// Read trim-params.json (optional) for keypoint time offset.
	var trimOffsetSec float64
	trimParamsPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileTrimParams)
	if trimData, err := os.ReadFile(trimParamsPath); err == nil {
		var tp TrimParams
		if err := json.Unmarshal(trimData, &tp); err == nil {
			trimOffsetSec = float64(tp.TrimStartMs) / 1000.0
		}
	}

	// Get input video properties.
	frameW, frameH, err := ffmpeg.GetDimensions(ctx, input.VideoPath)
	if err != nil {
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: get dimensions: %w", err)
	}
	fps, err := ffmpeg.GetFPS(ctx, input.VideoPath)
	if err != nil {
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: get fps: %w", err)
	}

	// Build sorted keypoint times for binary search.
	kpTimes := make([]float64, len(result.Frames))
	for i, f := range result.Frames {
		kpTimes[i] = float64(f.TimeOffsetMs) / 1000.0
	}

	// Start decode and encode pipes.
	outputPath := storage.LiftFile(input.DataDir, input.LiftID, storage.FileSkeleton)
	decCmd, decOut, err := ffmpeg.DecodeFrames(ctx, input.VideoPath)
	if err != nil {
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: decode start: %w", err)
	}
	encCmd, encIn, err := ffmpeg.EncodeFrames(ctx, outputPath, frameW, frameH, fps)
	if err != nil {
		_ = decCmd.Process.Kill()
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: encode start: %w", err)
	}

	// Process frames.
	frameSize := frameW * frameH * 3
	buf := make([]byte, frameSize)
	frameIdx := 0

	for {
		if _, err := io.ReadFull(decOut, buf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			_ = encIn.Close()
			_ = decCmd.Wait()
			_ = encCmd.Wait()
			return pipeline.StageOutput{}, fmt.Errorf("skeleton: read frame %d: %w", frameIdx, err)
		}

		// Find nearest keypoint frame for this video frame.
		videoTimeSec := float64(frameIdx) / fps
		kpTimeSec := videoTimeSec + trimOffsetSec
		kpFrame := nearestKeypointFrame(kpTimes, result.Frames, kpTimeSec)

		if kpFrame != nil {
			drawSkeleton(buf, frameW, frameH, kpFrame, cropParams, &result)
		}

		if _, err := encIn.Write(buf); err != nil {
			_ = encIn.Close()
			_ = decCmd.Wait()
			_ = encCmd.Wait()
			return pipeline.StageOutput{}, fmt.Errorf("skeleton: write frame %d: %w", frameIdx, err)
		}
		frameIdx++
	}

	encIn.Close()
	if err := decCmd.Wait(); err != nil {
		logger.Warn("decode process exited with error", "error", err)
	}
	if err := encCmd.Wait(); err != nil {
		return pipeline.StageOutput{}, fmt.Errorf("skeleton: encode finish: %w", err)
	}

	logger.Info("skeleton rendering complete",
		"frames_processed", frameIdx,
		"keypoint_frames", len(result.Frames),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return pipeline.StageOutput{VideoPath: outputPath}, nil
}

// nearestKeypointFrame finds the keypoint frame nearest to target time t.
// Returns nil if no frame is within 0.5s of t.
func nearestKeypointFrame(kpTimes []float64, frames []pose.Frame, t float64) *pose.Frame {
	if len(kpTimes) == 0 {
		return nil
	}
	idx := sort.SearchFloat64s(kpTimes, t)

	bestIdx := -1
	bestDist := math.MaxFloat64

	for _, i := range []int{idx - 1, idx} {
		if i >= 0 && i < len(kpTimes) {
			d := math.Abs(kpTimes[i] - t)
			if d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}
	}

	if bestIdx < 0 || bestDist > 0.5 {
		return nil
	}
	return &frames[bestIdx]
}

// drawSkeleton draws the COCO-17 skeleton onto an RGB24 frame buffer.
func drawSkeleton(buf []byte, frameW, frameH int, frame *pose.Frame, cropParams *CropParams, result *pose.Result) {
	// Build keypoint lookup by name.
	kpMap := make(map[string]pose.Keypoint, len(frame.Keypoints))
	for _, kp := range frame.Keypoints {
		kpMap[kp.Name] = kp
	}

	// Transform keypoint to pixel coordinates in the current frame.
	toPixel := func(kp pose.Keypoint) (int, int, bool) {
		if kp.Confidence < skeletonConfidenceThreshold {
			return 0, 0, false
		}
		var px, py float64
		if cropParams != nil {
			// Denormalize to original pixel space, then translate to crop space.
			px = kp.X*float64(result.SourceWidth) - float64(cropParams.X)
			py = kp.Y*float64(result.SourceHeight) - float64(cropParams.Y)
		} else {
			// No crop — denormalize directly to video dimensions.
			px = kp.X * float64(frameW)
			py = kp.Y * float64(frameH)
		}
		ix, iy := int(math.Round(px)), int(math.Round(py))
		if ix < 0 || ix >= frameW || iy < 0 || iy >= frameH {
			return 0, 0, false
		}
		return ix, iy, true
	}

	// Draw bones.
	for _, bone := range skeletonBones {
		kpFrom, okFrom := kpMap[bone.from]
		kpTo, okTo := kpMap[bone.to]
		if !okFrom || !okTo {
			continue
		}
		x0, y0, v0 := toPixel(kpFrom)
		x1, y1, v1 := toPixel(kpTo)
		if !v0 || !v1 {
			continue
		}
		drawThickLine(buf, frameW, frameH, x0, y0, x1, y1, bone.color, skeletonBoneThickness)
	}

	// Draw joints on top of bones.
	for _, kp := range frame.Keypoints {
		x, y, visible := toPixel(kp)
		if !visible {
			continue
		}
		drawFilledCircle(buf, frameW, frameH, x, y, skeletonJointRadius, colorJoint)
	}
}

// drawThickLine draws a line with thickness using Bresenham's algorithm.
func drawThickLine(buf []byte, frameW, frameH, x0, y0, x1, y1 int, color [3]byte, thickness int) {
	halfT := thickness / 2

	// Compute perpendicular offsets for thickness.
	dx := x1 - x0
	dy := y1 - y0
	length := math.Sqrt(float64(dx*dx + dy*dy))
	if length == 0 {
		setPixel(buf, frameW, frameH, x0, y0, color)
		return
	}

	// Perpendicular unit vector.
	px := -float64(dy) / length
	py := float64(dx) / length

	for t := -halfT; t <= halfT; t++ {
		ox := int(math.Round(float64(t) * px))
		oy := int(math.Round(float64(t) * py))
		bresenham(buf, frameW, frameH, x0+ox, y0+oy, x1+ox, y1+oy, color)
	}
}

// bresenham draws a one-pixel-wide line using Bresenham's algorithm.
func bresenham(buf []byte, frameW, frameH, x0, y0, x1, y1 int, color [3]byte) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy

	for {
		setPixel(buf, frameW, frameH, x0, y0, color)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// drawFilledCircle draws a filled circle at (cx, cy) with the given radius.
func drawFilledCircle(buf []byte, frameW, frameH, cx, cy, radius int, color [3]byte) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				setPixel(buf, frameW, frameH, cx+dx, cy+dy, color)
			}
		}
	}
}

// setPixel sets a pixel in an RGB24 buffer with bounds checking.
func setPixel(buf []byte, frameW, frameH, x, y int, color [3]byte) {
	if x < 0 || x >= frameW || y < 0 || y >= frameH {
		return
	}
	offset := (y*frameW + x) * 3
	buf[offset] = color[0]
	buf[offset+1] = color[1]
	buf[offset+2] = color[2]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
