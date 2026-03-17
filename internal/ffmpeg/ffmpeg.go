package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Run executes ffmpeg with the given arguments and returns captured stdout, stderr, and any error.
func Run(ctx context.Context, args ...string) (stdout, stderr []byte, err error) {
	return run(ctx, "ffmpeg", args...)
}

// RunProbe executes ffprobe with the given arguments and returns captured stdout, stderr, and any error.
func RunProbe(ctx context.Context, args ...string) (stdout, stderr []byte, err error) {
	return run(ctx, "ffprobe", args...)
}

func run(ctx context.Context, binary string, args ...string) (stdout, stderr []byte, err error) {
	slog.Info("ffmpeg: invoking", "binary", binary, "args", args)
	start := time.Now()

	cmd := exec.CommandContext(ctx, binary, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	stdout = outBuf.Bytes()
	stderr = errBuf.Bytes()

	if err != nil {
		slog.Info("ffmpeg: completed with error",
			"binary", binary,
			"duration_ms", durationMs,
			"error", err,
		)
		return stdout, stderr, fmt.Errorf("%s failed: %w (stderr: %s)", binary, err, string(stderr))
	}

	slog.Info("ffmpeg: completed",
		"binary", binary,
		"duration_ms", durationMs,
	)
	return stdout, stderr, nil
}

// Probe runs "ffmpeg -version" and returns the version string.
func Probe() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stdout, _, err := Run(ctx, "-version")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not available: %w", err)
	}

	line := strings.SplitN(string(stdout), "\n", 2)[0]
	return line, nil
}

// TrimVideo trims a video from startSec for durationSec seconds.
func TrimVideo(ctx context.Context, input, output string, startSec, durationSec float64) error {
	_, _, err := Run(ctx, "-y",
		"-ss", formatSeconds(startSec),
		"-i", input,
		"-t", formatSeconds(durationSec),
		"-c", "copy",
		output,
	)
	return err
}

// CropVideo crops a video to the rectangle defined by x, y, w, h.
func CropVideo(ctx context.Context, input, output string, x, y, w, h int) error {
	filter := fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y)
	_, _, err := Run(ctx, "-y",
		"-i", input,
		"-vf", filter,
		output,
	)
	return err
}

// ExtractThumbnail extracts a single frame at timeSec as an image.
func ExtractThumbnail(ctx context.Context, input, output string, timeSec float64) error {
	_, _, err := Run(ctx, "-y",
		"-ss", formatSeconds(timeSec),
		"-i", input,
		"-frames:v", "1",
		output,
	)
	return err
}

// GetDuration returns the duration of the input file in seconds via ffprobe.
func GetDuration(ctx context.Context, input string) (float64, error) {
	stdout, _, err := RunProbe(ctx,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		input,
	)
	if err != nil {
		return 0, err
	}

	s := strings.TrimSpace(string(stdout))
	d, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration %q: %w", s, err)
	}
	return d, nil
}

func formatSeconds(sec float64) string {
	return strconv.FormatFloat(sec, 'f', 3, 64)
}
