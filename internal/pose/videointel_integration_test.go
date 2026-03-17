//go:build integration

package pose

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVideoIntelClient_Integration(t *testing.T) {
	videoPath := filepath.Join("..", "..", "testdata", "videos", "sample-lift.mp4")
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		t.Skipf("sample-lift video not found at %s", videoPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := NewVideoIntelClient(ctx)
	if err != nil {
		t.Skipf("Video Intelligence client unavailable (credentials not configured?): %v", err)
	}
	defer client.Close()

	videoData, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatalf("failed to read video: %v", err)
	}
	t.Logf("video size: %d bytes", len(videoData))

	start := time.Now()
	result, err := client.DetectPose(ctx, videoData)
	if err != nil {
		t.Fatalf("DetectPose failed (is the Video Intelligence API enabled?): %v", err)
	}
	t.Logf("API call took %v", time.Since(start))

	if len(result.Frames) == 0 {
		t.Fatal("expected at least 1 frame")
	}
	t.Logf("total frames: %d", len(result.Frames))

	var totalKeypoints int
	for i, f := range result.Frames {
		if f.TimeOffsetMs < 0 {
			t.Errorf("frame[%d].TimeOffsetMs = %d, want >= 0", i, f.TimeOffsetMs)
		}

		bb := f.BoundingBox
		if bb.Left < 0 || bb.Left > 1 || bb.Top < 0 || bb.Top > 1 ||
			bb.Right < 0 || bb.Right > 1 || bb.Bottom < 0 || bb.Bottom > 1 {
			t.Errorf("frame[%d] bounding box out of range: %+v", i, bb)
		}
		if bb.Left >= bb.Right {
			t.Errorf("frame[%d] bounding box Left(%f) >= Right(%f)", i, bb.Left, bb.Right)
		}
		if bb.Top >= bb.Bottom {
			t.Errorf("frame[%d] bounding box Top(%f) >= Bottom(%f)", i, bb.Top, bb.Bottom)
		}

		if len(f.Keypoints) == 0 {
			t.Errorf("frame[%d] has 0 keypoints", i)
		}
		totalKeypoints += len(f.Keypoints)

		for j, kp := range f.Keypoints {
			if kp.X < 0 || kp.X > 1 {
				t.Errorf("frame[%d].keypoints[%d].X = %f, want 0-1", i, j, kp.X)
			}
			if kp.Y < 0 || kp.Y > 1 {
				t.Errorf("frame[%d].keypoints[%d].Y = %f, want 0-1", i, j, kp.Y)
			}
			if kp.Confidence < 0 || kp.Confidence > 1 {
				t.Errorf("frame[%d].keypoints[%d].Confidence = %f, want 0-1", i, j, kp.Confidence)
			}
		}

		// Log landmark names from first frame.
		if i == 0 {
			var names []string
			for _, kp := range f.Keypoints {
				names = append(names, kp.Name)
			}
			t.Logf("landmark names (frame 0): %v", names)
		}
	}

	if len(result.Frames) > 0 {
		t.Logf("average keypoints per frame: %.1f", float64(totalKeypoints)/float64(len(result.Frames)))
	}
}
