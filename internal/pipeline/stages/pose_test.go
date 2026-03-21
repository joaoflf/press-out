package stages

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

func skipIfNoUV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not available, skipping test")
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	// From internal/pipeline/stages/ → project root is three levels up.
	root := filepath.Join("..", "..", "..")
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); os.IsNotExist(err) {
		t.Skipf("pyproject.toml not found at %s", root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("failed to resolve project root: %v", err)
	}
	return abs
}

func TestPoseStage_Name(t *testing.T) {
	stage := &PoseStage{}
	if got := stage.Name(); got != "Pose estimation" {
		t.Errorf("Name() = %q, want %q", got, "Pose estimation")
	}
}

func TestPoseStage_SampleLift(t *testing.T) {
	skipIfNoUV(t)
	video := sampleLiftVideo(t)
	// Resolve to absolute path since cmd.Dir changes the working directory.
	absVideo, err := filepath.Abs(video)
	if err != nil {
		t.Fatalf("failed to resolve video path: %v", err)
	}
	video = absVideo
	root := projectRoot(t)

	tmpDir := t.TempDir()
	liftID := int64(10)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	stage := &PoseStage{ProjectRoot: root}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: video,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Pose stage passes video through unchanged.
	if output.VideoPath != video {
		t.Errorf("expected video passthrough %q, got %q", video, output.VideoPath)
	}

	// Verify keypoints.json was written.
	kpPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	data, err := os.ReadFile(kpPath)
	if err != nil {
		t.Fatalf("keypoints.json not found: %v", err)
	}

	var result pose.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid keypoints JSON: %v", err)
	}

	if result.SourceWidth <= 0 || result.SourceHeight <= 0 {
		t.Errorf("invalid source dimensions: %dx%d", result.SourceWidth, result.SourceHeight)
	}
	if len(result.Frames) == 0 {
		t.Fatal("no frames in keypoints output")
	}

	// Check keypoints are normalized 0-1.
	for i, frame := range result.Frames {
		for _, kp := range frame.Keypoints {
			if kp.X < 0 || kp.X > 1 || kp.Y < 0 || kp.Y > 1 {
				t.Errorf("frame %d: keypoint %s coordinates out of 0-1 range: (%.4f, %.4f)", i, kp.Name, kp.X, kp.Y)
			}
		}
	}

	t.Logf("produced %d frames with %dx%d source", len(result.Frames), result.SourceWidth, result.SourceHeight)
}

func TestPoseStage_TestFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("test fixture not found at %s: %v", fixturePath, err)
	}

	var result pose.Result
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.SourceWidth <= 0 {
		t.Error("SourceWidth must be > 0")
	}
	if result.SourceHeight <= 0 {
		t.Error("SourceHeight must be > 0")
	}
	if len(result.Frames) == 0 {
		t.Fatal("no frames")
	}

	// First frame should have 17 keypoints.
	firstFrame := result.Frames[0]
	if len(firstFrame.Keypoints) != 17 {
		t.Errorf("first frame has %d keypoints, want 17", len(firstFrame.Keypoints))
	}

	// Verify keypoint names are valid COCO landmarks.
	validNames := map[string]bool{
		"nose": true, "left_eye": true, "right_eye": true,
		"left_ear": true, "right_ear": true,
		"left_shoulder": true, "right_shoulder": true,
		"left_elbow": true, "right_elbow": true,
		"left_wrist": true, "right_wrist": true,
		"left_hip": true, "right_hip": true,
		"left_knee": true, "right_knee": true,
		"left_ankle": true, "right_ankle": true,
	}
	for _, kp := range firstFrame.Keypoints {
		if !validNames[kp.Name] {
			t.Errorf("invalid keypoint name: %q", kp.Name)
		}
	}

	// Verify all coordinates and bounding boxes are in 0-1 range.
	for i, frame := range result.Frames {
		bb := frame.BoundingBox
		if bb.Left < 0 || bb.Left > 1 || bb.Top < 0 || bb.Top > 1 ||
			bb.Right < 0 || bb.Right > 1 || bb.Bottom < 0 || bb.Bottom > 1 {
			t.Errorf("frame %d: bounding box out of 0-1 range: %+v", i, bb)
		}
		for _, kp := range frame.Keypoints {
			if kp.X < 0 || kp.X > 1 || kp.Y < 0 || kp.Y > 1 {
				t.Errorf("frame %d: keypoint %s out of range: (%.4f, %.4f)", i, kp.Name, kp.X, kp.Y)
			}
		}
	}
}

func TestPoseStage_SubprocessFailure(t *testing.T) {
	skipIfNoUV(t)
	root := projectRoot(t)

	stage := &PoseStage{ProjectRoot: root}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    99,
		DataDir:   t.TempDir(),
		VideoPath: "/nonexistent/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input, got nil")
	}

	// Verify keypoints.json was NOT written.
	kpPath := storage.LiftFile(t.TempDir(), 99, storage.FileKeypoints)
	if _, err := os.Stat(kpPath); err == nil {
		t.Error("keypoints.json should not exist after failure")
	}
}

func TestPoseStage_ContextCancellation(t *testing.T) {
	skipIfNoUV(t)
	root := projectRoot(t)

	stage := &PoseStage{ProjectRoot: root}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    99,
		DataDir:   t.TempDir(),
		VideoPath: "/any/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
