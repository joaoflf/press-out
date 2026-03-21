package stages

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

func TestCropStage_Name(t *testing.T) {
	stage := &CropStage{}
	if got := stage.Name(); got != "Cropping" {
		t.Errorf("Name() = %q, want %q", got, "Cropping")
	}
}

func TestCropStage_WithKeypoints(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(10)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Copy sample video as input.
	videoData, err := os.ReadFile(video)
	if err != nil {
		t.Fatalf("failed to read sample video: %v", err)
	}
	inputPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	if err := os.WriteFile(inputPath, videoData, 0644); err != nil {
		t.Fatalf("failed to write input video: %v", err)
	}

	// Copy the sample keypoints file.
	sampleKeypoints := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	if _, err := os.Stat(sampleKeypoints); os.IsNotExist(err) {
		t.Skip("keypoints-sample.json not found")
	}
	kpData, err := os.ReadFile(sampleKeypoints)
	if err != nil {
		t.Fatalf("failed to read sample keypoints: %v", err)
	}
	keypointsPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(keypointsPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &CropStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: inputPath,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Verify cropped.mp4 was produced.
	croppedPath := storage.LiftFile(tmpDir, liftID, storage.FileCropped)
	if output.VideoPath != croppedPath {
		t.Errorf("expected output path %q, got %q", croppedPath, output.VideoPath)
	}
	info, err := os.Stat(croppedPath)
	if err != nil {
		t.Fatalf("cropped.mp4 not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("cropped.mp4 is empty")
	}

	// Verify thumbnail.jpg was produced.
	thumbnailPath := storage.LiftFile(tmpDir, liftID, storage.FileThumbnail)
	thumbInfo, err := os.Stat(thumbnailPath)
	if err != nil {
		t.Fatalf("thumbnail.jpg not found: %v", err)
	}
	if thumbInfo.Size() == 0 {
		t.Fatal("thumbnail.jpg is empty")
	}
	// Verify JPEG magic bytes.
	thumbData, err := os.ReadFile(thumbnailPath)
	if err != nil {
		t.Fatalf("failed to read thumbnail: %v", err)
	}
	if len(thumbData) < 2 || thumbData[0] != 0xFF || thumbData[1] != 0xD8 {
		t.Error("thumbnail is not a valid JPEG (missing FF D8 header)")
	}

	// Verify crop-params.json was produced and has expected fields.
	paramsPath := storage.LiftFile(tmpDir, liftID, storage.FileCropParams)
	paramsData, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("crop-params.json not found: %v", err)
	}
	var params CropParams
	if err := json.Unmarshal(paramsData, &params); err != nil {
		t.Fatalf("failed to parse crop-params.json: %v", err)
	}
	if params.W == 0 || params.H == 0 {
		t.Error("crop params have zero width or height")
	}
	if params.SourceWidth == 0 || params.SourceHeight == 0 {
		t.Error("crop params have zero source dimensions")
	}
	t.Logf("crop params: x=%d y=%d w=%d h=%d source=%dx%d", params.X, params.Y, params.W, params.H, params.SourceWidth, params.SourceHeight)
}

func TestCropStage_AspectRatio(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(11)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	videoData, err := os.ReadFile(video)
	if err != nil {
		t.Fatalf("failed to read sample video: %v", err)
	}
	inputPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	if err := os.WriteFile(inputPath, videoData, 0644); err != nil {
		t.Fatalf("failed to write input video: %v", err)
	}

	sampleKeypoints := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	if _, err := os.Stat(sampleKeypoints); os.IsNotExist(err) {
		t.Skip("keypoints-sample.json not found")
	}
	kpData, err := os.ReadFile(sampleKeypoints)
	if err != nil {
		t.Fatalf("failed to read sample keypoints: %v", err)
	}
	keypointsPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(keypointsPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	// Read crop params to check aspect ratio.
	stage := &CropStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: inputPath,
	}); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	paramsPath := storage.LiftFile(tmpDir, liftID, storage.FileCropParams)
	paramsData, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("crop-params.json not found: %v", err)
	}
	var params CropParams
	if err := json.Unmarshal(paramsData, &params); err != nil {
		t.Fatalf("failed to parse crop-params.json: %v", err)
	}

	// Verify 9:16 aspect ratio within rounding tolerance.
	expectedRatio := 9.0 / 16.0
	actualRatio := float64(params.W) / float64(params.H)
	if math.Abs(actualRatio-expectedRatio) > 0.02 {
		t.Errorf("aspect ratio = %.4f, want %.4f (±0.02)", actualRatio, expectedRatio)
	}
}

func TestCropStage_NoKeypoints(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(12)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	videoData, err := os.ReadFile(video)
	if err != nil {
		t.Fatalf("failed to read sample video: %v", err)
	}
	inputPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	if err := os.WriteFile(inputPath, videoData, 0644); err != nil {
		t.Fatalf("failed to write input video: %v", err)
	}

	// No keypoints.json written — graceful degradation.
	stage := &CropStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: inputPath,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Should preserve the original video path.
	if output.VideoPath != inputPath {
		t.Errorf("expected original path %q, got %q", inputPath, output.VideoPath)
	}

	// cropped.mp4 should NOT exist.
	croppedPath := storage.LiftFile(tmpDir, liftID, storage.FileCropped)
	if _, err := os.Stat(croppedPath); err == nil {
		t.Error("cropped.mp4 should not exist when keypoints are absent")
	}

	// crop-params.json should NOT exist.
	paramsPath := storage.LiftFile(tmpDir, liftID, storage.FileCropParams)
	if _, err := os.Stat(paramsPath); err == nil {
		t.Error("crop-params.json should not exist when keypoints are absent")
	}

	// thumbnail.jpg should still be extracted.
	thumbnailPath := storage.LiftFile(tmpDir, liftID, storage.FileThumbnail)
	if _, err := os.Stat(thumbnailPath); err != nil {
		t.Errorf("thumbnail.jpg should be extracted even without keypoints: %v", err)
	}
}

func TestCropStage_FFmpegFailure(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(13)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Write minimal keypoints so we reach the FFmpeg crop call.
	result := pose.Result{
		SourceWidth:  1920,
		SourceHeight: 1080,
		Frames: []pose.Frame{
			{BoundingBox: pose.BoundingBox{Left: 0.2, Top: 0.1, Right: 0.8, Bottom: 0.9}},
		},
	}
	kpData, _ := json.Marshal(result)
	keypointsPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(keypointsPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &CropStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a nonexistent video to trigger FFmpeg failure.
	_, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/nonexistent/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input, got nil")
	}
}

func TestCropStage_ThumbnailFailureNonFatal(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(14)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// No keypoints — preserves original. Use nonexistent video to make thumbnail fail.
	stage := &CropStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The no-keypoints path tries to extract thumbnail from a nonexistent video.
	// The stage should still succeed (thumbnail failure is non-fatal).
	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/nonexistent/video.mp4",
	})
	if err != nil {
		t.Fatalf("expected stage to succeed despite thumbnail failure, got: %v", err)
	}

	if output.VideoPath != "/nonexistent/video.mp4" {
		t.Errorf("expected original path preserved, got %q", output.VideoPath)
	}

	// Thumbnail should not exist.
	thumbnailPath := storage.LiftFile(tmpDir, liftID, storage.FileThumbnail)
	if _, err := os.Stat(thumbnailPath); err == nil {
		t.Error("thumbnail should not exist when source video is nonexistent")
	}
}

func TestCropStage_ContextCancellation(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(15)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Write keypoints so we proceed past the no-keypoints path.
	result := pose.Result{
		SourceWidth:  1920,
		SourceHeight: 1080,
		Frames: []pose.Frame{
			{BoundingBox: pose.BoundingBox{Left: 0.2, Top: 0.1, Right: 0.8, Bottom: 0.9}},
		},
	}
	kpData, _ := json.Marshal(result)
	keypointsPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(keypointsPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &CropStage{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/any/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestComputeCropRegion(t *testing.T) {
	frames := []pose.Frame{
		{BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.25, Top: 0.15, Right: 0.75, Bottom: 0.85}},
	}

	x, y, w, h := computeCropRegion(frames, 1080, 1920)

	// Verify 9:16 aspect ratio.
	expectedRatio := 9.0 / 16.0
	actualRatio := float64(w) / float64(h)
	if math.Abs(actualRatio-expectedRatio) > 0.02 {
		t.Errorf("aspect ratio = %.4f, want %.4f (±0.02)", actualRatio, expectedRatio)
	}

	// Verify crop is within frame bounds.
	if x < 0 || y < 0 {
		t.Errorf("crop origin is negative: x=%d y=%d", x, y)
	}
	if x+w > 1080 {
		t.Errorf("crop exceeds frame width: x=%d + w=%d > 1080", x, w)
	}
	if y+h > 1920 {
		t.Errorf("crop exceeds frame height: y=%d + h=%d > 1920", y, h)
	}

	// Verify even dimensions.
	if w%2 != 0 {
		t.Errorf("width %d is not even", w)
	}
	if h%2 != 0 {
		t.Errorf("height %d is not even", h)
	}

	t.Logf("crop region: x=%d y=%d w=%d h=%d", x, y, w, h)
}

func TestComputeCropRegion_EdgeClamp(t *testing.T) {
	// Bounding box near the edge of the frame — should clamp.
	frames := []pose.Frame{
		{BoundingBox: pose.BoundingBox{Left: 0.0, Top: 0.0, Right: 0.3, Bottom: 0.5}},
	}

	x, y, w, h := computeCropRegion(frames, 1080, 1920)

	if x < 0 || y < 0 {
		t.Errorf("crop origin should be clamped to >= 0: x=%d y=%d", x, y)
	}
	if x+w > 1080 {
		t.Errorf("crop exceeds frame width: x=%d + w=%d > 1080", x, w)
	}
	if y+h > 1920 {
		t.Errorf("crop exceeds frame height: y=%d + h=%d > 1920", y, h)
	}
}

func TestComputeCropRegion_FullFrame(t *testing.T) {
	// Bounding box covers entire frame — crop should be the full frame.
	frames := []pose.Frame{
		{BoundingBox: pose.BoundingBox{Left: 0.0, Top: 0.0, Right: 1.0, Bottom: 1.0}},
	}

	x, y, w, h := computeCropRegion(frames, 1080, 1920)

	// With padding and aspect ratio enforcement, should still fit within frame.
	if x < 0 || y < 0 {
		t.Errorf("crop origin should be >= 0: x=%d y=%d", x, y)
	}
	if x+w > 1080 {
		t.Errorf("crop exceeds frame width: x=%d + w=%d > 1080", x, w)
	}
	if y+h > 1920 {
		t.Errorf("crop exceeds frame height: y=%d + h=%d > 1920", y, h)
	}
}
