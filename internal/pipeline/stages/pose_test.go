package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"press-out/internal/pipeline"
	"press-out/internal/pose"
	"press-out/internal/storage"
)

// mockPoseClient is a test double for pose.Client.
type mockPoseClient struct {
	result *pose.Result
	err    error
}

func (m *mockPoseClient) DetectPose(_ context.Context, _ []byte) (*pose.Result, error) {
	return m.result, m.err
}

func (m *mockPoseClient) Close() error { return nil }

func TestPoseStage_Name(t *testing.T) {
	stage := NewPoseStage(&mockPoseClient{})
	if got := stage.Name(); got != pipeline.StagePoseEstimation {
		t.Errorf("Name() = %q, want %q", got, pipeline.StagePoseEstimation)
	}
}

func TestPoseStage_Success(t *testing.T) {
	skipIfNoFFprobe(t)

	video := sampleLiftVideo(t)
	tmpDir := t.TempDir()
	liftID := int64(1)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatal(err)
	}

	// Copy video to lift directory as original.
	origPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(origPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	mockResult := &pose.Result{
		Frames: []pose.Frame{
			{
				TimeOffsetMs: 0,
				BoundingBox:  pose.BoundingBox{Left: 0.1, Top: 0.15, Right: 0.75, Bottom: 0.95},
				Keypoints: []pose.Keypoint{
					{Name: "nose", X: 0.5, Y: 0.3, Confidence: 0.95},
					{Name: "left_shoulder", X: 0.45, Y: 0.45, Confidence: 0.92},
					{Name: "right_shoulder", X: 0.55, Y: 0.45, Confidence: 0.91},
				},
			},
			{
				TimeOffsetMs: 33,
				BoundingBox:  pose.BoundingBox{Left: 0.1, Top: 0.15, Right: 0.75, Bottom: 0.95},
				Keypoints: []pose.Keypoint{
					{Name: "nose", X: 0.51, Y: 0.31, Confidence: 0.94},
				},
			},
		},
	}

	stage := NewPoseStage(&mockPoseClient{result: mockResult})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: origPath,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Stage passes video through unchanged.
	if output.VideoPath != origPath {
		t.Errorf("VideoPath = %q, want %q", output.VideoPath, origPath)
	}

	// Verify keypoints.json was written.
	kpPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	kpData, err := os.ReadFile(kpPath)
	if err != nil {
		t.Fatalf("keypoints.json not written: %v", err)
	}

	var result pose.Result
	if err := json.Unmarshal(kpData, &result); err != nil {
		t.Fatalf("failed to unmarshal keypoints.json: %v", err)
	}

	// Verify source dimensions were populated.
	if result.SourceWidth == 0 || result.SourceHeight == 0 {
		t.Errorf("source dimensions not set: %dx%d", result.SourceWidth, result.SourceHeight)
	}

	// Verify frames.
	if len(result.Frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(result.Frames))
	}
	if result.Frames[0].TimeOffsetMs != 0 {
		t.Errorf("frame[0].TimeOffsetMs = %d, want 0", result.Frames[0].TimeOffsetMs)
	}
	if len(result.Frames[0].Keypoints) != 3 {
		t.Errorf("frame[0] keypoints = %d, want 3", len(result.Frames[0].Keypoints))
	}

	// Verify bounding box fields.
	bb := result.Frames[0].BoundingBox
	if bb.Left != 0.1 || bb.Top != 0.15 || bb.Right != 0.75 || bb.Bottom != 0.95 {
		t.Errorf("unexpected bounding box: %+v", bb)
	}

	// Verify keypoint fields.
	kp := result.Frames[0].Keypoints[0]
	if kp.Name != "nose" || kp.X != 0.5 || kp.Y != 0.3 || kp.Confidence != 0.95 {
		t.Errorf("unexpected keypoint: %+v", kp)
	}
}

func TestPoseStage_ClientError(t *testing.T) {
	skipIfNoFFprobe(t)

	video := sampleLiftVideo(t)
	tmpDir := t.TempDir()
	liftID := int64(1)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatal(err)
	}

	origPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(origPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	stage := NewPoseStage(&mockPoseClient{err: fmt.Errorf("API unavailable")})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: origPath,
	})
	if err == nil {
		t.Fatal("expected error from client failure")
	}
}

func TestPoseStage_VideoTooLarge(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	tmpDir := t.TempDir()
	liftID := int64(1)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatal(err)
	}

	// Create a file that exceeds the size limit.
	origPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	f, err := os.Create(origPath)
	if err != nil {
		t.Fatal(err)
	}
	// Write just over the limit (sparse file is fine for size check).
	if err := f.Truncate(maxInlineVideoBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	stage := NewPoseStage(&mockPoseClient{result: &pose.Result{}})
	ctx := context.Background()

	_, err = stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: origPath,
	})
	if err == nil {
		t.Fatal("expected error for oversized video")
	}
}
