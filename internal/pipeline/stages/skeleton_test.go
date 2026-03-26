package stages

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

func TestSkeletonStage_Name(t *testing.T) {
	stage := &SkeletonStage{}
	if got := stage.Name(); got != pipeline.StageRenderingSkeleton {
		t.Errorf("Name() = %q, want %q", got, pipeline.StageRenderingSkeleton)
	}
}

func TestSkeletonStage_SkipsWhenNoKeypoints(t *testing.T) {
	tmpDir := t.TempDir()
	liftID := int64(20)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	stage := &SkeletonStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/nonexistent/video.mp4",
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if output.VideoPath != "/nonexistent/video.mp4" {
		t.Errorf("expected input path passed through, got %q", output.VideoPath)
	}
}

func TestSkeletonStage_ProducesSkeletonVideo(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(21)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Copy sample video as input.
	videoData, err := os.ReadFile(video)
	if err != nil {
		t.Fatalf("failed to read sample video: %v", err)
	}
	inputPath := storage.LiftFile(tmpDir, liftID, storage.FileCropped)
	if err := os.WriteFile(inputPath, videoData, 0644); err != nil {
		t.Fatalf("failed to write input video: %v", err)
	}

	// Write synthetic keypoints.
	result := pose.Result{
		SourceWidth:  1920,
		SourceHeight: 1080,
		Frames: []pose.Frame{
			{
				TimeOffsetMs: 0,
				Keypoints:    makeTestKeypoints(0.5, 0.3),
			},
			{
				TimeOffsetMs: 33,
				Keypoints:    makeTestKeypoints(0.5, 0.31),
			},
			{
				TimeOffsetMs: 66,
				Keypoints:    makeTestKeypoints(0.5, 0.32),
			},
		},
	}
	kpData, _ := json.Marshal(result)
	keypointsPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	if err := os.WriteFile(keypointsPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &SkeletonStage{}
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

	skeletonPath := storage.LiftFile(tmpDir, liftID, storage.FileSkeleton)
	if output.VideoPath != skeletonPath {
		t.Errorf("expected output path %q, got %q", skeletonPath, output.VideoPath)
	}
	info, err := os.Stat(skeletonPath)
	if err != nil {
		t.Fatalf("skeleton.mp4 not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("skeleton.mp4 is empty")
	}
}

func TestSkeletonStage_WorksWithoutCropParams(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(22)
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

	// Write keypoints without crop-params.json.
	result := pose.Result{
		SourceWidth:  1920,
		SourceHeight: 1080,
		Frames: []pose.Frame{
			{
				TimeOffsetMs: 0,
				Keypoints:    makeTestKeypoints(0.5, 0.3),
			},
		},
	}
	kpData, _ := json.Marshal(result)
	if err := os.WriteFile(storage.LiftFile(tmpDir, liftID, storage.FileKeypoints), kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &SkeletonStage{}
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

	skeletonPath := storage.LiftFile(tmpDir, liftID, storage.FileSkeleton)
	if output.VideoPath != skeletonPath {
		t.Errorf("expected output path %q, got %q", skeletonPath, output.VideoPath)
	}
	if _, err := os.Stat(skeletonPath); err != nil {
		t.Fatalf("skeleton.mp4 not found: %v", err)
	}
}

func TestCoordinateTransformation(t *testing.T) {
	kp := pose.Keypoint{
		Name:       pose.LandmarkLeftShoulder,
		X:          0.25,
		Y:          0.25,
		Confidence: 0.9,
	}
	result := &pose.Result{
		SourceWidth:  1920,
		SourceHeight: 1080,
	}
	cp := &CropParams{
		X:            200,
		Y:            50,
		W:            540,
		H:            960,
		SourceWidth:  1920,
		SourceHeight: 1080,
	}

	// pixelX = 0.25 * 1920 = 480, croppedX = 480 - 200 = 280
	// pixelY = 0.25 * 1080 = 270, croppedY = 270 - 50 = 220
	frame := &pose.Frame{
		Keypoints: []pose.Keypoint{kp},
	}

	frameW, frameH := cp.W, cp.H
	buf := make([]byte, frameW*frameH*3)

	drawSkeleton(buf, frameW, frameH, frame, cp, result)

	// Check that the joint was drawn near (280, 220).
	offset := (220*frameW + 280) * 3
	if buf[offset] == 0 && buf[offset+1] == 0 && buf[offset+2] == 0 {
		t.Error("expected pixel at transformed coordinate to be drawn, but it's black")
	}
}

func TestLowConfidenceKeypointsSkipped(t *testing.T) {
	frame := &pose.Frame{
		Keypoints: []pose.Keypoint{
			{Name: pose.LandmarkNose, X: 0.5, Y: 0.5, Confidence: 0.1}, // below threshold
		},
	}
	result := &pose.Result{SourceWidth: 100, SourceHeight: 100}

	frameW, frameH := 100, 100
	buf := make([]byte, frameW*frameH*3)

	drawSkeleton(buf, frameW, frameH, frame, nil, result)

	// Verify nothing was drawn (all zeros).
	allZero := true
	for _, b := range buf {
		if b != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("expected no drawing for low-confidence keypoints, but pixels were set")
	}
}

func TestNearestKeypointFrame(t *testing.T) {
	frames := []pose.Frame{
		{TimeOffsetMs: 0},
		{TimeOffsetMs: 100},
		{TimeOffsetMs: 200},
		{TimeOffsetMs: 300},
	}
	kpTimes := []float64{0, 0.1, 0.2, 0.3}

	tests := []struct {
		t       float64
		wantIdx int
		wantNil bool
	}{
		{0.0, 0, false},
		{0.05, 0, false},
		{0.09, 1, false},
		{0.15, 1, false},
		{0.25, 2, false},
		{0.3, 3, false},
		{1.0, -1, true}, // too far from any frame
	}

	for _, tt := range tests {
		got := nearestKeypointFrame(kpTimes, frames, tt.t)
		if tt.wantNil {
			if got != nil {
				t.Errorf("nearestKeypointFrame(%.2f) = frame, want nil", tt.t)
			}
			continue
		}
		if got == nil {
			t.Errorf("nearestKeypointFrame(%.2f) = nil, want frame[%d]", tt.t, tt.wantIdx)
			continue
		}
		if got.TimeOffsetMs != frames[tt.wantIdx].TimeOffsetMs {
			t.Errorf("nearestKeypointFrame(%.2f) = frame at %dms, want %dms",
				tt.t, got.TimeOffsetMs, frames[tt.wantIdx].TimeOffsetMs)
		}
	}
}

// makeTestKeypoints creates a full COCO-17 keypoint set centered at (cx, cy) with high confidence.
func makeTestKeypoints(cx, cy float64) []pose.Keypoint {
	spread := 0.05
	names := []string{
		pose.LandmarkNose,
		pose.LandmarkLeftEye, pose.LandmarkRightEye,
		pose.LandmarkLeftEar, pose.LandmarkRightEar,
		pose.LandmarkLeftShoulder, pose.LandmarkRightShoulder,
		pose.LandmarkLeftElbow, pose.LandmarkRightElbow,
		pose.LandmarkLeftWrist, pose.LandmarkRightWrist,
		pose.LandmarkLeftHip, pose.LandmarkRightHip,
		pose.LandmarkLeftKnee, pose.LandmarkRightKnee,
		pose.LandmarkLeftAnkle, pose.LandmarkRightAnkle,
	}
	kps := make([]pose.Keypoint, len(names))
	for i, name := range names {
		dx := spread * float64(i%3-1)
		dy := spread * float64(i/3-2)
		kps[i] = pose.Keypoint{
			Name:       name,
			X:          cx + dx,
			Y:          cy + dy,
			Confidence: 0.9,
		}
	}
	return kps
}
