package stages

import (
	"context"
	"math"
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

func sampleStaticVideo(t *testing.T) string {
	t.Helper()
	// sample.mp4 is a short static video good for low-confidence test
	path := filepath.Join("..", "..", "..", "testdata", "videos", "sample.mp4")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("sample video not found at %s", path)
	}
	return path
}

func TestTrimStage_Name(t *testing.T) {
	stage := &TrimStage{}
	if got := stage.Name(); got != "Trimming" {
		t.Errorf("Name() = %q, want %q", got, "Trimming")
	}
}

func TestTrimStage_SampleLift(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleLiftVideo(t)

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

	// Either trimmed.mp4 was produced or original was preserved.
	trimmedPath := storage.LiftFile(tmpDir, liftID, storage.FileTrimmed)
	if output.VideoPath == trimmedPath {
		// Verify trimmed file exists and has reasonable duration.
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
		// Expect trimmed duration roughly ~8s (lift ~6-11s + padding).
		if dur < 2 || dur > 20 {
			t.Errorf("trimmed duration %fs outside expected range [2, 20]", dur)
		}
		t.Logf("trimmed duration: %.2fs", dur)
	} else if output.VideoPath == origPath {
		// Low confidence — original preserved, which is also acceptable.
		t.Log("low confidence: original video preserved (acceptable for this test video)")
	} else {
		t.Errorf("unexpected output path: %s", output.VideoPath)
	}
}

func TestTrimStage_LowConfidence(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)
	video := sampleStaticVideo(t)

	tmpDir := t.TempDir()
	liftID := int64(2)
	if err := storage.CreateLiftDir(tmpDir, liftID); err != nil {
		t.Fatalf("failed to create lift dir: %v", err)
	}

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    liftID,
		DataDir:   tmpDir,
		VideoPath: video,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Low confidence: should return the original path.
	if output.VideoPath != video {
		t.Errorf("expected original path %q, got %q", video, output.VideoPath)
	}

	// trimmed.mp4 should NOT exist.
	trimmedPath := storage.LiftFile(tmpDir, liftID, storage.FileTrimmed)
	if _, err := os.Stat(trimmedPath); err == nil {
		t.Error("trimmed.mp4 should not exist for low-confidence video")
	}
}

func TestTrimStage_FFmpegFailure(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	stage := &TrimStage{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a nonexistent file to trigger FFmpeg failure.
	_, err := stage.Run(ctx, pipeline.StageInput{
		LiftID:    99,
		DataDir:   t.TempDir(),
		VideoPath: "/nonexistent/video.mp4",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent input, got nil")
	}
}

func TestTrimStage_ContextCancellation(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	stage := &TrimStage{}
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

func TestFindMotionCluster(t *testing.T) {
	tests := []struct {
		name       string
		timestamps []float64
		duration   float64
		wantConf   bool
	}{
		{
			name:       "too few timestamps",
			timestamps: []float64{1.0, 2.0, 3.0},
			duration:   30.0,
			wantConf:   false,
		},
		{
			name:       "good cluster",
			timestamps: []float64{5.0, 5.5, 6.0, 6.5, 7.0, 7.5, 8.0},
			duration:   30.0,
			wantConf:   true,
		},
		{
			name:       "sparse timestamps",
			timestamps: []float64{1.0, 5.0, 10.0, 15.0, 20.0},
			duration:   30.0,
			wantConf:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, confident := findMotionCluster(tt.timestamps, tt.duration)
			if confident != tt.wantConf {
				t.Errorf("confident = %v, want %v (start=%.2f, end=%.2f)", confident, tt.wantConf, start, end)
			}
			if confident {
				clusterDur := end - start
				if clusterDur < minClusterDuration || clusterDur > maxClusterDuration {
					t.Errorf("cluster duration %.2f outside [%.1f, %.1f]", clusterDur, minClusterDuration, maxClusterDuration)
				}
			}
		})
	}
}

func TestFindMotionCluster_Padding(t *testing.T) {
	// Verify that padding is applied correctly in the Run method logic.
	timestamps := []float64{2.0, 2.5, 3.0, 3.5, 4.0, 4.5}
	start, end, confident := findMotionCluster(timestamps, 30.0)
	if !confident {
		t.Fatal("expected confident cluster")
	}

	// Apply padding like Run does.
	paddedStart := start - paddingSec
	if paddedStart < 0 {
		paddedStart = 0
	}
	paddedEnd := end + paddingSec

	if math.Abs(paddedStart-0.5) > 0.01 {
		t.Errorf("padded start = %.2f, want 0.50", paddedStart)
	}
	if math.Abs(paddedEnd-6.0) > 0.01 {
		t.Errorf("padded end = %.2f, want 6.00", paddedEnd)
	}
}

func TestFindMotionCluster_ClampToZero(t *testing.T) {
	// Cluster near video start — padding should clamp to 0.
	timestamps := []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.5}
	start, _, confident := findMotionCluster(timestamps, 10.0)
	if !confident {
		t.Fatal("expected confident cluster")
	}

	paddedStart := start - paddingSec
	if paddedStart < 0 {
		paddedStart = 0
	}
	if paddedStart != 0 {
		t.Errorf("expected padded start clamped to 0, got %.2f", paddedStart)
	}
}
