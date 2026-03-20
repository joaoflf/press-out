package pose

// Result holds pose estimation output for a video.
type Result struct {
	SourceWidth  int     `json:"sourceWidth"`
	SourceHeight int     `json:"sourceHeight"`
	Frames       []Frame `json:"frames"`
}

// Frame holds pose data for a single video frame.
type Frame struct {
	TimeOffsetMs int64       `json:"timeOffsetMs"`
	BoundingBox  BoundingBox `json:"boundingBox"`
	Keypoints    []Keypoint  `json:"keypoints"`
}

// BoundingBox holds a normalized bounding box (all values 0-1).
type BoundingBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

// Keypoint holds a single body landmark with normalized coordinates.
type Keypoint struct {
	Name       string  `json:"name"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Confidence float64 `json:"confidence"`
}

// COCO-format landmark name constants.
const (
	LandmarkNose          = "nose"
	LandmarkLeftEye       = "left_eye"
	LandmarkRightEye      = "right_eye"
	LandmarkLeftEar       = "left_ear"
	LandmarkRightEar      = "right_ear"
	LandmarkLeftShoulder  = "left_shoulder"
	LandmarkRightShoulder = "right_shoulder"
	LandmarkLeftElbow     = "left_elbow"
	LandmarkRightElbow    = "right_elbow"
	LandmarkLeftWrist     = "left_wrist"
	LandmarkRightWrist    = "right_wrist"
	LandmarkLeftHip       = "left_hip"
	LandmarkRightHip      = "right_hip"
	LandmarkLeftKnee      = "left_knee"
	LandmarkRightKnee     = "right_knee"
	LandmarkLeftAnkle     = "left_ankle"
	LandmarkRightAnkle    = "right_ankle"
)
