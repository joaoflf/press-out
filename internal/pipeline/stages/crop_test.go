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

func TestMedian(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"single value", []float64{5.0}, 5.0},
		{"odd count", []float64{3.0, 1.0, 2.0}, 2.0},
		{"even count", []float64{4.0, 1.0, 3.0, 2.0}, 2.5},
		{"empty", []float64{}, 0},
		{"with outlier", []float64{0.5, 0.5, 0.5, 0.5, 10.0}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := median(tt.values)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("median(%v) = %f, want %f", tt.values, got, tt.want)
			}
		})
	}
}

func TestComputeCropRegion_MedianCenter(t *testing.T) {
	// Construct frames where one outlier frame shifts the union center significantly.
	// 4 frames with the lifter centered at x=0.5, 1 outlier frame at x=0.9.
	// Union center would be at (0.4+0.95)/2 = 0.675, but median center should be ~0.5.
	frames := []pose.Frame{
		{BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8}},
		// Outlier frame: lifter far right
		{BoundingBox: pose.BoundingBox{Left: 0.8, Top: 0.2, Right: 0.95, Bottom: 0.8}},
	}

	sourceW, sourceH := 1080, 1920
	x, y, w, h := computeCropRegion(frames, sourceW, sourceH)

	// The median horizontal center is 0.5 (from the 4 consistent frames).
	// The crop should be centered near x=540 (0.5 * 1080), not at ~729 (union center).
	cropCenterX := float64(x) + float64(w)/2
	medianExpected := 0.5 * float64(sourceW) // 540
	unionExpected := (0.4 + 0.95) / 2 * float64(sourceW)

	// Crop center should be closer to median than to union center.
	distToMedian := math.Abs(cropCenterX - medianExpected)
	distToUnion := math.Abs(cropCenterX - unionExpected)
	if distToMedian >= distToUnion {
		t.Errorf("crop center X=%.1f is closer to union center (%.1f) than median center (%.1f)",
			cropCenterX, unionExpected, medianExpected)
	}

	// Verify 9:16 aspect ratio.
	expectedRatio := 9.0 / 16.0
	actualRatio := float64(w) / float64(h)
	if math.Abs(actualRatio-expectedRatio) > 0.02 {
		t.Errorf("aspect ratio = %.4f, want %.4f (±0.02)", actualRatio, expectedRatio)
	}

	// Verify within frame bounds.
	if x < 0 || y < 0 || x+w > sourceW || y+h > sourceH {
		t.Errorf("crop out of bounds: x=%d y=%d w=%d h=%d (source %dx%d)", x, y, w, h, sourceW, sourceH)
	}

	t.Logf("crop region: x=%d y=%d w=%d h=%d (center=%.1f, median=%.1f, union=%.1f)",
		x, y, w, h, cropCenterX, medianExpected, unionExpected)
}

func TestComputeCropRegion_SymmetricFrames(t *testing.T) {
	// When all frames are symmetric (same bounding box), median and union center coincide.
	frames := []pose.Frame{
		{BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8}},
		{BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8}},
	}

	sourceW, sourceH := 1080, 1920
	x, y, w, h := computeCropRegion(frames, sourceW, sourceH)

	// Center should be at (0.5*1080, 0.5*1920) = (540, 960).
	cropCenterX := float64(x) + float64(w)/2
	cropCenterY := float64(y) + float64(h)/2
	expectedCX := 0.5 * float64(sourceW)
	expectedCY := 0.5 * float64(sourceH)

	if math.Abs(cropCenterX-expectedCX) > 2 {
		t.Errorf("crop center X=%.1f, want ~%.1f", cropCenterX, expectedCX)
	}
	if math.Abs(cropCenterY-expectedCY) > 2 {
		t.Errorf("crop center Y=%.1f, want ~%.1f", cropCenterY, expectedCY)
	}

	// Verify even dimensions and 9:16 ratio.
	if w%2 != 0 || h%2 != 0 {
		t.Errorf("dimensions not even: w=%d h=%d", w, h)
	}
	expectedRatio := 9.0 / 16.0
	actualRatio := float64(w) / float64(h)
	if math.Abs(actualRatio-expectedRatio) > 0.02 {
		t.Errorf("aspect ratio = %.4f, want %.4f", actualRatio, expectedRatio)
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

func TestNoseCenter(t *testing.T) {
	tests := []struct {
		name    string
		frame   pose.Frame
		wantX   float64
		wantY   float64
		wantOK  bool
	}{
		{
			name: "nose with high confidence",
			frame: pose.Frame{
				Keypoints: []pose.Keypoint{
					{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.9},
				},
			},
			wantX: 0.5, wantY: 0.3, wantOK: true,
		},
		{
			name: "nose with low confidence",
			frame: pose.Frame{
				Keypoints: []pose.Keypoint{
					{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.1},
				},
			},
			wantOK: false,
		},
		{
			name: "no nose keypoint",
			frame: pose.Frame{
				Keypoints: []pose.Keypoint{
					{Name: pose.LandmarkLeftShoulder, X: 0.4, Y: 0.5, Confidence: 0.9},
				},
			},
			wantOK: false,
		},
		{
			name:   "no keypoints at all",
			frame:  pose.Frame{},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY, gotOK := noseCenter(tt.frame)
			if gotOK != tt.wantOK {
				t.Errorf("noseCenter() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && (gotX != tt.wantX || gotY != tt.wantY) {
				t.Errorf("noseCenter() = (%v, %v), want (%v, %v)", gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestComputeCropRegion_NoseCenter(t *testing.T) {
	// Frames with nose keypoints at x=0.5 and BB centers at x=0.5.
	// One outlier frame has BB center at x=0.875 but nose at x=0.5.
	// With nose centering, the crop should stay centered at x=0.5.
	frames := []pose.Frame{
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.9}},
		},
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.9}},
		},
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.9}},
		},
		{
			// Outlier BB, but nose is still at center.
			BoundingBox: pose.BoundingBox{Left: 0.8, Top: 0.2, Right: 0.95, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.3, Confidence: 0.9}},
		},
	}

	sourceW, sourceH := 1080, 1920
	x, _, w, _ := computeCropRegion(frames, sourceW, sourceH)

	cropCenterX := float64(x) + float64(w)/2
	noseExpected := 0.5 * float64(sourceW) // 540

	// Crop center should be very close to the nose position.
	if math.Abs(cropCenterX-noseExpected) > 20 {
		t.Errorf("crop center X=%.1f, want ~%.1f (nose-based centering)", cropCenterX, noseExpected)
	}
}

func TestComputeCropRegion_NoseXOnly(t *testing.T) {
	// Nose keypoint should only drive horizontal centering.
	// Vertical centering should always use bounding box center Y.
	// Here, nose Y=0.15 (near top) but BB center Y=0.5 (mid-frame).
	// If nose Y were used, crop would shift up and cut off legs.
	frames := []pose.Frame{
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.15, Confidence: 0.9}},
		},
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.15, Confidence: 0.9}},
		},
		{
			BoundingBox: pose.BoundingBox{Left: 0.4, Top: 0.2, Right: 0.6, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.5, Y: 0.15, Confidence: 0.9}},
		},
	}

	sourceW, sourceH := 1080, 1920
	_, y, _, h := computeCropRegion(frames, sourceW, sourceH)

	cropCenterY := float64(y) + float64(h)/2
	bbCenterY := 0.5 * float64(sourceH)   // 960 (BB vertical center)
	noseCenterY := 0.15 * float64(sourceH) // 288 (nose Y — too high)

	// Crop center Y should be near BB center, not nose Y.
	distToBB := math.Abs(cropCenterY - bbCenterY)
	distToNose := math.Abs(cropCenterY - noseCenterY)
	if distToBB >= distToNose {
		t.Errorf("crop center Y=%.1f is closer to nose Y (%.1f) than BB center Y (%.1f); "+
			"vertical centering should use BB center, not nose",
			cropCenterY, noseCenterY, bbCenterY)
	}

	t.Logf("crop center Y=%.1f (BB center=%.1f, nose=%.1f)", cropCenterY, bbCenterY, noseCenterY)
}

func TestComputeCropRegion_NoseFallback(t *testing.T) {
	// When nose confidence is low, should fall back to BB center.
	frames := []pose.Frame{
		{
			BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.9, Y: 0.9, Confidence: 0.1}},
		},
		{
			BoundingBox: pose.BoundingBox{Left: 0.3, Top: 0.2, Right: 0.7, Bottom: 0.8},
			Keypoints:   []pose.Keypoint{{Name: pose.LandmarkNose, X: 0.9, Y: 0.9, Confidence: 0.1}},
		},
	}

	sourceW, sourceH := 1080, 1920
	x, _, w, _ := computeCropRegion(frames, sourceW, sourceH)

	cropCenterX := float64(x) + float64(w)/2
	bbCenterExpected := 0.5 * float64(sourceW) // 540 (BB center = (0.3+0.7)/2 = 0.5)

	// Should use BB center since nose confidence is too low.
	if math.Abs(cropCenterX-bbCenterExpected) > 5 {
		t.Errorf("crop center X=%.1f, want ~%.1f (BB center fallback)", cropCenterX, bbCenterExpected)
	}
}
