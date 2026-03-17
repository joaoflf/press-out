package pose

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	videointelligence "cloud.google.com/go/videointelligence/apiv1"
	videointelligencepb "cloud.google.com/go/videointelligence/apiv1/videointelligencepb"
)

// VideoIntelClient implements Client using Google Cloud Video Intelligence API.
type VideoIntelClient struct {
	client *videointelligence.Client
}

// NewVideoIntelClient creates a new Video Intelligence API client.
// Credentials are read automatically from the GOOGLE_APPLICATION_CREDENTIALS env var.
func NewVideoIntelClient(ctx context.Context) (*VideoIntelClient, error) {
	c, err := videointelligence.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("videointel: create client: %w", err)
	}
	return &VideoIntelClient{client: c}, nil
}

func (v *VideoIntelClient) DetectPose(ctx context.Context, videoData []byte) (*Result, error) {
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

	op, err := v.client.AnnotateVideo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("videointel: annotate: %w", err)
	}

	resp, err := op.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("videointel: wait: %w", err)
	}

	if len(resp.AnnotationResults) == 0 {
		return nil, fmt.Errorf("videointel: no annotation results")
	}

	annotations := resp.AnnotationResults[0].PersonDetectionAnnotations
	if len(annotations) == 0 {
		return nil, fmt.Errorf("videointel: no persons detected")
	}

	// Select primary person: largest average bounding box area.
	bestIdx := 0
	bestAvgArea := 0.0
	for i, ann := range annotations {
		var totalArea float64
		var count int
		for _, ts := range ann.Tracks {
			for _, obj := range ts.TimestampedObjects {
				bb := obj.NormalizedBoundingBox
				if bb != nil {
					area := float64(bb.Right-bb.Left) * float64(bb.Bottom-bb.Top)
					totalArea += area
					count++
				}
			}
		}
		if count > 0 {
			avgArea := totalArea / float64(count)
			if avgArea > bestAvgArea {
				bestAvgArea = avgArea
				bestIdx = i
			}
		}
	}

	if len(annotations) > 1 {
		slog.Warn("multiple persons detected, selecting primary",
			"person_count", len(annotations),
			"selected_avg_area", bestAvgArea,
		)
	}

	// Extract frames from the selected person's tracks.
	var frames []Frame
	selectedAnn := annotations[bestIdx]
	for _, track := range selectedAnn.Tracks {
		for _, obj := range track.TimestampedObjects {
			var f Frame

			if obj.TimeOffset != nil {
				f.TimeOffsetMs = obj.TimeOffset.AsDuration().Milliseconds()
			}

			if bb := obj.NormalizedBoundingBox; bb != nil {
				f.BoundingBox = BoundingBox{
					Left:   float64(bb.Left),
					Top:    float64(bb.Top),
					Right:  float64(bb.Right),
					Bottom: float64(bb.Bottom),
				}
			}

			for _, lm := range obj.Landmarks {
				if lm.Point == nil {
					continue
				}
				f.Keypoints = append(f.Keypoints, Keypoint{
					Name:       normalizeLandmarkName(lm.Name),
					X:          float64(lm.Point.X),
					Y:          float64(lm.Point.Y),
					Confidence: float64(lm.Confidence),
				})
			}

			frames = append(frames, f)
		}
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("videointel: no frames with landmarks found")
	}

	return &Result{Frames: frames}, nil
}

func (v *VideoIntelClient) Close() error {
	return v.client.Close()
}

// normalizeLandmarkName maps API landmark names to COCO-format lowercase names.
func normalizeLandmarkName(name string) string {
	n := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	// Map known API names to COCO-format constants.
	mapping := map[string]string{
		"nose":           LandmarkNose,
		"left_eye":       LandmarkLeftEye,
		"right_eye":      LandmarkRightEye,
		"left_ear":       LandmarkLeftEar,
		"right_ear":      LandmarkRightEar,
		"left_shoulder":  LandmarkLeftShoulder,
		"right_shoulder": LandmarkRightShoulder,
		"left_elbow":     LandmarkLeftElbow,
		"right_elbow":    LandmarkRightElbow,
		"left_wrist":     LandmarkLeftWrist,
		"right_wrist":    LandmarkRightWrist,
		"left_hip":       LandmarkLeftHip,
		"right_hip":      LandmarkRightHip,
		"left_knee":      LandmarkLeftKnee,
		"right_knee":     LandmarkRightKnee,
		"left_ankle":     LandmarkLeftAnkle,
		"right_ankle":    LandmarkRightAnkle,
	}
	if mapped, ok := mapping[n]; ok {
		return mapped
	}
	return n
}
