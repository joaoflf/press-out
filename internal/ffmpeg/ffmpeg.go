package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// CropVideoExpr crops a video using an FFmpeg expression for the X position.
// This enables dynamic per-frame X positioning (e.g., for hybrid track/lock crop).
func CropVideoExpr(ctx context.Context, input, output string, xExpr string, y, w, h int) error {
	filter := fmt.Sprintf("crop=%d:%d:'%s':%d", w, h, xExpr, y)
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

// GetDimensions returns the width and height of the input video via ffprobe.
func GetDimensions(ctx context.Context, input string) (width, height int, err error) {
	stdout, _, err := RunProbe(ctx,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "default=noprint_wrappers=1",
		input,
	)
	if err != nil {
		return 0, 0, err
	}

	// Output format: "width=1920\nheight=1080\n"
	var w, h int
	for _, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "width":
			w, err = strconv.Atoi(parts[1])
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse width %q: %w", parts[1], err)
			}
		case "height":
			h, err = strconv.Atoi(parts[1])
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse height %q: %w", parts[1], err)
			}
		}
	}
	if w == 0 || h == 0 {
		return 0, 0, fmt.Errorf("failed to parse dimensions from %q", string(stdout))
	}
	return w, h, nil
}

// GetFPS returns the video frame rate via ffprobe.
func GetFPS(ctx context.Context, input string) (float64, error) {
	stdout, _, err := RunProbe(ctx,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate",
		"-of", "default=noprint_wrappers=1:nokey=1",
		input,
	)
	if err != nil {
		return 0, err
	}

	s := strings.TrimSpace(string(stdout))
	// r_frame_rate is a fraction like "30/1" or "30000/1001".
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected r_frame_rate format %q", s)
	}
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse r_frame_rate numerator %q: %w", parts[0], err)
	}
	den, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse r_frame_rate denominator %q: %w", parts[1], err)
	}
	if den == 0 {
		return 0, fmt.Errorf("r_frame_rate denominator is zero")
	}
	return num / den, nil
}

// DecodeFrames starts FFmpeg decoding to raw RGB24 frames piped to stdout.
// Caller must read from the returned ReadCloser and call cmd.Wait() when done.
func DecodeFrames(ctx context.Context, input string) (*exec.Cmd, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", input,
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"pipe:1",
	)
	slog.Info("ffmpeg: decode start", "input", input)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("decode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("decode: start: %w", err)
	}
	return cmd, stdout, nil
}

// EncodeFrames starts FFmpeg encoding from raw RGB24 frames piped from stdin.
// Caller must write to the returned WriteCloser, close it, then call cmd.Wait().
func EncodeFrames(ctx context.Context, output string, w, h int, fps float64) (*exec.Cmd, io.WriteCloser, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-r", formatSeconds(fps),
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "fast",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		output,
	)
	slog.Info("ffmpeg: encode start", "output", output, "size", fmt.Sprintf("%dx%d", w, h), "fps", fps)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("encode: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("encode: start: %w", err)
	}
	return cmd, stdin, nil
}

func formatSeconds(sec float64) string {
	return strconv.FormatFloat(sec, 'f', 3, 64)
}
