package ffmpeg

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func sampleVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "videos", "sample.mp4")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("sample video not found at %s", path)
	}
	return path
}

func TestRunVersion(t *testing.T) {
	skipIfNoFFmpeg(t)

	ctx := context.Background()
	stdout, _, err := Run(ctx, "-version")
	if err != nil {
		t.Fatalf("Run -version failed: %v", err)
	}
	if len(stdout) == 0 {
		t.Fatal("expected non-empty stdout from ffmpeg -version")
	}
}

func TestRunInvalidArgs(t *testing.T) {
	skipIfNoFFmpeg(t)

	ctx := context.Background()
	_, _, err := Run(ctx, "-i", "nonexistent_file_that_does_not_exist.mp4")
	if err == nil {
		t.Fatal("expected error for invalid input file")
	}
}

func TestRunContextTimeout(t *testing.T) {
	skipIfNoFFmpeg(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give the context a moment to expire before invoking
	time.Sleep(5 * time.Millisecond)

	_, _, err := Run(ctx, "-version")
	if err == nil {
		t.Fatal("expected error from expired context")
	}
}

func TestProbe(t *testing.T) {
	skipIfNoFFmpeg(t)

	version, err := Probe()
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty version string")
	}
	t.Logf("ffmpeg version: %s", version)
}

func TestTrimVideo(t *testing.T) {
	skipIfNoFFmpeg(t)
	input := sampleVideo(t)

	output := filepath.Join(t.TempDir(), "trimmed.mp4")
	ctx := context.Background()

	err := TrimVideo(ctx, input, output, 0, 1)
	if err != nil {
		t.Fatalf("TrimVideo failed: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestCropVideo(t *testing.T) {
	skipIfNoFFmpeg(t)
	input := sampleVideo(t)

	output := filepath.Join(t.TempDir(), "cropped.mp4")
	ctx := context.Background()

	// Crop to 160x120 from origin (0,0) — sample is 320x240
	err := CropVideo(ctx, input, output, 0, 0, 160, 120)
	if err != nil {
		t.Fatalf("CropVideo failed: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestExtractThumbnail(t *testing.T) {
	skipIfNoFFmpeg(t)
	input := sampleVideo(t)

	output := filepath.Join(t.TempDir(), "thumb.png")
	ctx := context.Background()

	err := ExtractThumbnail(ctx, input, output, 0.5)
	if err != nil {
		t.Fatalf("ExtractThumbnail failed: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestGetDuration(t *testing.T) {
	skipIfNoFFprobe(t)
	input := sampleVideo(t)

	ctx := context.Background()
	dur, err := GetDuration(ctx, input)
	if err != nil {
		t.Fatalf("GetDuration failed: %v", err)
	}

	// sample.mp4 is 2 seconds
	if math.Abs(dur-2.0) > 0.5 {
		t.Fatalf("expected duration ~2s, got %f", dur)
	}
	t.Logf("duration: %fs", dur)
}
