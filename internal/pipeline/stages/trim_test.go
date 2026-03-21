package stages

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pipeline"
	"press-out/internal/storage"
)

func skipIfNoFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed, skipping test")
	}
}

func skipIfNoFFprobe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed, skipping test")
	}
}

func sampleLiftVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "videos", "sample-lift.mp4")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("sample-lift video not found at %s", path)
	}
	return path
}

func sampleKeypointsPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("keypoints-sample.json not found at %s", path)
	}
	return path
}

func TestTrimStage_Name(t *testing.T) {
	stage := &TrimStage{}
	if got := stage.Name(); got != "Trimming" {
		t.Errorf("Name() = %q, want %q", got, "Trimming")
	}
}

func TestTrimStage_WithKeypoints(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)
	kpSrc := sampleKeypointsPath(t)

	tmpDir := t.TempDir()
	liftID := int64(1)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Copy sample-lift.mp4 to the expected original.mp4 location.
	origPath := storage.LiftFile(tmpDir, liftID, storage.FileOriginal)
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatalf("failed to read sample video: %v", err)
	}
	if err := os.WriteFile(origPath, data, 0644); err != nil {
		t.Fatalf("failed to write original video: %v", err)
	}

	// Copy keypoints-sample.json to the lift directory.
	kpDst := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	kpData, err := os.ReadFile(kpSrc)
	if err != nil {
		t.Fatalf("failed to read keypoints: %v", err)
	}
	if err := os.WriteFile(kpDst, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: origPath,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	trimmedPath := storage.LiftFile(tmpDir, liftID, storage.FileTrimmed)
	if output.VideoPath == trimmedPath {
		// Trimmed video was produced — verify it.
		info, err := os.Stat(trimmedPath)
		if err != nil {
			t.Fatalf("trimmed file not found: %v", err)
		}
		if info.Size() == 0 {
			t.Fatal("trimmed file is empty")
		}

		dur, err := ffmpeg.GetDuration(ctx, trimmedPath)
		if err != nil {
			t.Fatalf("failed to get trimmed duration: %v", err)
		}
		// Expect trimmed duration roughly 6-10s (lift ~4s + padding ~3s).
		if dur < 3 || dur > 14 {
			t.Errorf("trimmed duration %fs outside expected range [3, 14]", dur)
		}
		t.Logf("trimmed duration: %.2fs", dur)
	} else if output.VideoPath == origPath {
		// Low confidence — original preserved, also acceptable.
		t.Log("low confidence: original video preserved (acceptable)")
	} else {
		t.Errorf("unexpected output path: %s", output.VideoPath)
	}
}

func TestTrimStage_NoKeypoints(t *testing.T) {
	skipIfNoFFmpeg(t)

	tmpDir := t.TempDir()
	liftID := int64(2)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// No keypoints.json exists — should return original video, no error.
	videoPath := "/tmp/fake-video.mp4"
	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: videoPath,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.VideoPath != videoPath {
		t.Errorf("expected original path %q, got %q", videoPath, output.VideoPath)
	}
}

func TestTrimStage_LowConfidence(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(3)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Write keypoints with minimal motion (all keypoints at same position).
	kpPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	lowMotionKP := `{
		"sourceWidth": 1080,
		"sourceHeight": 1920,
		"frames": [
			{"timeOffsetMs": 0, "boundingBox": {"left":0.4,"top":0.3,"right":0.6,"bottom":0.8}, "keypoints": [
				{"name":"nose","x":0.5,"y":0.3,"confidence":0.9},
				{"name":"left_shoulder","x":0.45,"y":0.45,"confidence":0.9}
			]},
			{"timeOffsetMs": 33, "boundingBox": {"left":0.4,"top":0.3,"right":0.6,"bottom":0.8}, "keypoints": [
				{"name":"nose","x":0.5,"y":0.3,"confidence":0.9},
				{"name":"left_shoulder","x":0.45,"y":0.45,"confidence":0.9}
			]},
			{"timeOffsetMs": 66, "boundingBox": {"left":0.4,"top":0.3,"right":0.6,"bottom":0.8}, "keypoints": [
				{"name":"nose","x":0.5,"y":0.3,"confidence":0.9},
				{"name":"left_shoulder","x":0.45,"y":0.45,"confidence":0.9}
			]}
		]
	}`
	if err := os.WriteFile(kpPath, []byte(lowMotionKP), 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	videoPath := "/tmp/fake-video.mp4"
	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: videoPath,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.VideoPath != videoPath {
		t.Errorf("expected original path %q, got %q", videoPath, output.VideoPath)
	}

	// trimmed.mp4 should NOT exist.
	trimmedPath := storage.LiftFile(tmpDir, liftID, storage.FileTrimmed)
	if _, err := os.Stat(trimmedPath); err == nil {
		t.Error("trimmed.mp4 should not exist for low-motion keypoints")
	}
}

func TestTrimStage_FFmpegFailure(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(99)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Write keypoints that produce a confident detection, but use a
	// nonexistent video file so FFmpeg fails.
	kpPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	kpSrc := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	if _, err := os.Stat(kpSrc); os.IsNotExist(err) {
		t.Skipf("keypoints-sample.json not found at %s", kpSrc)
	}
	kpData, err := os.ReadFile(kpSrc)
	if err != nil {
		t.Fatalf("failed to read keypoints: %v", err)
	}
	if err := os.WriteFile(kpPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/nonexistent/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input, got nil")
	}
}

func TestTrimStage_ContextCancellation(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	tmpDir := t.TempDir()
	liftID := int64(99)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	// Write keypoints that produce a confident detection.
	kpPath := storage.LiftFile(tmpDir, liftID, storage.FileKeypoints)
	kpSrc := filepath.Join("..", "..", "..", "testdata", "keypoints-sample.json")
	if _, err := os.Stat(kpSrc); os.IsNotExist(err) {
		t.Skipf("keypoints-sample.json not found at %s", kpSrc)
	}
	kpData, err := os.ReadFile(kpSrc)
	if err != nil {
		t.Fatalf("failed to read keypoints: %v", err)
	}
	if err := os.WriteFile(kpPath, kpData, 0644); err != nil {
		t.Fatalf("failed to write keypoints: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: "/any/video.mp4",
	})
	// With a cancelled context, detection itself succeeds (no ctx needed),
	// but ffmpeg.GetDuration or ffmpeg.TrimVideo should fail.
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestDetectLiftFromKeypoints(t *testing.T) {
	kpPath := sampleKeypointsPath(t)

	startSec, endSec, confident, err := detectLiftFromKeypoints(kpPath)
	if err != nil {
		t.Fatalf("detectLiftFromKeypoints() error: %v", err)
	}

	if !confident {
		t.Fatal("expected confident detection from sample keypoints")
	}

	t.Logf("detected lift: %.2fs - %.2fs (duration: %.2fs)", startSec, endSec, endSec-startSec)

	// The sample video has the lift at ~6-11s. Allow tolerance for algorithm variance.
	if startSec < 2 || startSec > 10 {
		t.Errorf("start %.2fs outside expected range [2, 10]", startSec)
	}
	if endSec < 7 || endSec > 12 {
		t.Errorf("end %.2fs outside expected range [7, 12]", endSec)
	}

	// Lift duration should be reasonable.
	dur := endSec - startSec
	if dur < minLiftDuration || dur > maxLiftDuration {
		t.Errorf("lift duration %.2fs outside [%.1f, %.1f]", dur, minLiftDuration, maxLiftDuration)
	}
}
