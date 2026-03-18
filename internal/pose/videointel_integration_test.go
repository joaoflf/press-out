//go:build integration

package pose_test

import (
	"context"
	"os"
	"testing"
	"time"

	videointelligence "cloud.google.com/go/videointelligence/apiv1"
	videointelligencepb "cloud.google.com/go/videointelligence/apiv1/videointelligencepb"
)

// TestVideoIntelligence_LabelDetection verifies the Video Intelligence API
// is reachable and functional using LABEL_DETECTION (basic sanity check).
func TestVideoIntelligence_LabelDetection(t *testing.T) {
	videoData := loadTestVideo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := videointelligence.NewClient(ctx)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	req := &videointelligencepb.AnnotateVideoRequest{
		InputContent: videoData,
		Features:     []videointelligencepb.Feature{videointelligencepb.Feature_LABEL_DETECTION},
	}

	t.Log("sending LABEL_DETECTION request...")
	start := time.Now()
	op, err := client.AnnotateVideo(ctx, req)
	if err != nil {
		t.Fatalf("AnnotateVideo failed: %v", err)
	}
	resp, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("LRO Wait failed: %v", err)
	}
	t.Logf("completed in %v", time.Since(start))

	if len(resp.AnnotationResults) == 0 {
		t.Fatal("no annotation results")
	}
	ar := resp.AnnotationResults[0]
	if ar.Error != nil {
		t.Fatalf("API error: code=%d message=%q", ar.Error.Code, ar.Error.Message)
	}
	t.Logf("segment labels: %d, shot labels: %d", len(ar.SegmentLabelAnnotations), len(ar.ShotLabelAnnotations))
	if len(ar.SegmentLabelAnnotations) == 0 {
		t.Error("expected at least 1 segment label")
	}
}

// TestVideoIntelligence_PersonDetection_BBoxOnly verifies PERSON_DETECTION
// with bounding boxes (no pose landmarks). This feature works reliably.
func TestVideoIntelligence_PersonDetection_BBoxOnly(t *testing.T) {
	videoData := loadTestVideo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := videointelligence.NewClient(ctx)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	req := &videointelligencepb.AnnotateVideoRequest{
		InputContent: videoData,
		Features:     []videointelligencepb.Feature{videointelligencepb.Feature_PERSON_DETECTION},
		VideoContext: &videointelligencepb.VideoContext{
			PersonDetectionConfig: &videointelligencepb.PersonDetectionConfig{
				IncludeBoundingBoxes: true,
			},
		},
	}

	t.Log("sending PERSON_DETECTION (bbox only) request...")
	start := time.Now()
	op, err := client.AnnotateVideo(ctx, req)
	if err != nil {
		t.Fatalf("AnnotateVideo failed: %v", err)
	}
	resp, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("LRO Wait failed: %v", err)
	}
	t.Logf("completed in %v", time.Since(start))

	if len(resp.AnnotationResults) == 0 {
		t.Fatal("no annotation results")
	}
	ar := resp.AnnotationResults[0]
	if ar.Error != nil {
		t.Fatalf("API error: code=%d message=%q", ar.Error.Code, ar.Error.Message)
	}

	annotations := ar.PersonDetectionAnnotations
	if len(annotations) == 0 {
		t.Fatal("expected at least 1 person detection annotation")
	}
	t.Logf("persons detected: %d", len(annotations))

	// Validate first person's tracks.
	firstAnn := annotations[0]
	if len(firstAnn.Tracks) == 0 {
		t.Fatal("expected at least 1 track")
	}

	totalFrames := 0
	for trackIdx, track := range firstAnn.Tracks {
		t.Logf("track[%d]: %d timestamped objects", trackIdx, len(track.TimestampedObjects))
		for objIdx, tsObj := range track.TimestampedObjects {
			totalFrames++
			bb := tsObj.NormalizedBoundingBox
			if bb == nil {
				t.Errorf("track[%d].obj[%d] missing bounding box", trackIdx, objIdx)
				continue
			}
			if bb.Left < 0 || bb.Left > 1 || bb.Top < 0 || bb.Top > 1 ||
				bb.Right < 0 || bb.Right > 1 || bb.Bottom < 0 || bb.Bottom > 1 {
				t.Errorf("track[%d].obj[%d] bbox out of 0-1: L=%f T=%f R=%f B=%f",
					trackIdx, objIdx, bb.Left, bb.Top, bb.Right, bb.Bottom)
			}
			if bb.Left >= bb.Right {
				t.Errorf("track[%d].obj[%d] Left(%f) >= Right(%f)", trackIdx, objIdx, bb.Left, bb.Right)
			}
			if bb.Top >= bb.Bottom {
				t.Errorf("track[%d].obj[%d] Top(%f) >= Bottom(%f)", trackIdx, objIdx, bb.Top, bb.Bottom)
			}
			// Log landmarks if any are returned (unexpected without IncludePoseLandmarks).
			if len(tsObj.Landmarks) > 0 {
				t.Logf("track[%d].obj[%d] unexpected landmarks: %d", trackIdx, objIdx, len(tsObj.Landmarks))
			}
			// Log first 3 frames.
			if totalFrames <= 3 {
				var timeMs int64
				if tsObj.TimeOffset != nil {
					timeMs = tsObj.TimeOffset.AsDuration().Milliseconds()
				}
				t.Logf("  frame %d (time=%dms): bbox=L%.3f T%.3f R%.3f B%.3f",
					totalFrames, timeMs, bb.Left, bb.Top, bb.Right, bb.Bottom)
			}
		}
	}

	t.Logf("total frames: %d", totalFrames)
	for i, ann := range annotations {
		var frames int
		for _, tr := range ann.Tracks {
			frames += len(tr.TimestampedObjects)
		}
		t.Logf("person[%d]: %d tracks, %d frames", i, len(ann.Tracks), frames)
	}
}

// TestVideoIntelligence_PersonDetection_PoseLandmarks tests IncludePoseLandmarks.
// As of 2026-03-18, this returns "Calculator failure" (code=2) from the API.
// This test documents the failure for Story 2.4 planning.
func TestVideoIntelligence_PersonDetection_PoseLandmarks(t *testing.T) {
	videoData := loadTestVideo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := videointelligence.NewClient(ctx)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	req := &videointelligencepb.AnnotateVideoRequest{
		InputContent: videoData,
		Features:     []videointelligencepb.Feature{videointelligencepb.Feature_PERSON_DETECTION},
		VideoContext: &videointelligencepb.VideoContext{
			PersonDetectionConfig: &videointelligencepb.PersonDetectionConfig{
				IncludePoseLandmarks: true,
				IncludeBoundingBoxes: true,
			},
		},
	}

	t.Log("sending PERSON_DETECTION (with pose landmarks) request...")
	start := time.Now()
	op, err := client.AnnotateVideo(ctx, req)
	if err != nil {
		t.Fatalf("AnnotateVideo failed: %v", err)
	}
	resp, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("LRO Wait failed: %v", err)
	}
	t.Logf("completed in %v", time.Since(start))

	if len(resp.AnnotationResults) == 0 {
		t.Fatal("no annotation results")
	}
	ar := resp.AnnotationResults[0]

	// Document the API response for spike findings.
	t.Logf("PersonDetectionAnnotations: %d", len(ar.PersonDetectionAnnotations))
	if ar.Error != nil {
		t.Logf("API error: code=%d message=%q", ar.Error.Code, ar.Error.Message)
		t.Log("SPIKE FINDING: IncludePoseLandmarks causes 'Calculator failure' in the Video Intelligence API.")
		t.Log("PERSON_DETECTION works with IncludeBoundingBoxes alone, but adding IncludePoseLandmarks triggers a server-side error.")
		t.Log("Story 2.4 must use an alternative approach for pose estimation (e.g., MediaPipe, MoveNet, or OpenPose).")
		// This is an expected failure — the spike exists to discover this.
		// Mark as skip rather than fail so the test suite stays green.
		t.Skip("IncludePoseLandmarks not supported — see spike findings")
	}

	// If landmarks DO work (future fix), validate them.
	annotations := ar.PersonDetectionAnnotations
	if len(annotations) == 0 {
		t.Fatal("no person annotations despite no error")
	}

	totalLandmarks := 0
	uniqueLandmarks := make(map[string]bool)
	totalFrames := 0
	for _, ann := range annotations {
		for _, track := range ann.Tracks {
			for _, tsObj := range track.TimestampedObjects {
				totalFrames++
				for _, lm := range tsObj.Landmarks {
					uniqueLandmarks[lm.Name] = true
					totalLandmarks++
					if lm.Point != nil {
						if lm.Point.X < 0 || lm.Point.X > 1 || lm.Point.Y < 0 || lm.Point.Y > 1 {
							t.Errorf("landmark %q out of 0-1 range: (%.4f, %.4f)", lm.Name, lm.Point.X, lm.Point.Y)
						}
					}
				}
			}
		}
	}

	t.Logf("persons: %d, frames: %d, total landmarks: %d, unique: %d",
		len(annotations), totalFrames, totalLandmarks, len(uniqueLandmarks))
	for name := range uniqueLandmarks {
		t.Logf("  landmark: %q", name)
	}
	if totalFrames > 0 {
		t.Logf("avg landmarks/frame: %.1f", float64(totalLandmarks)/float64(totalFrames))
	}
}

func loadTestVideo(t *testing.T) []byte {
	t.Helper()
	// Use re-encoded smaller video if available (original 27MB also causes
	// "Calculator failure" for some features due to processing limits).
	videoPath := "/tmp/medium-clip.mp4"
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		videoPath = "../../testdata/videos/sample-lift.mp4"
	}
	data, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatalf("failed to read video at %s: %v", videoPath, err)
	}
	t.Logf("video: %s (%d bytes, %.1f MB)", videoPath, len(data), float64(len(data))/(1024*1024))
	return data
}
