package pipeline

import "context"

// Stage name constants for the 5-stage pipeline.
// Pose estimation is handled client-side via ml5.js before upload.
const (
	StageTrimming           = "Trimming"
	StageCropping           = "Cropping"
	StageRenderingSkeleton  = "Rendering skeleton"
	StageComputingMetrics   = "Computing metrics"
	StageGeneratingCoaching = "Generating coaching"
)

// StageState represents the current processing state of a pipeline stage.
type StageState string

const (
	StatePending  StageState = "pending"
	StateActive   StageState = "active"
	StateComplete StageState = "complete"
	StateSkipped  StageState = "skipped"
)

// StageInput holds input data for a pipeline stage.
type StageInput struct {
	LiftID    int64
	DataDir   string
	VideoPath string
}

// StageOutput holds the result of a pipeline stage.
type StageOutput struct {
	VideoPath string
}

// Stage defines the interface for a pipeline processing stage.
type Stage interface {
	Name() string
	Run(ctx context.Context, input StageInput) (StageOutput, error)
}

// StubStage is a no-op stage used when real processing is not yet implemented.
type StubStage struct {
	StageName string
}

func (s *StubStage) Name() string { return s.StageName }

func (s *StubStage) Run(_ context.Context, input StageInput) (StageOutput, error) {
	return StageOutput{VideoPath: input.VideoPath}, nil
}

// DefaultStages returns the ordered set of pipeline stages with stub implementations.
func DefaultStages() []Stage {
	return []Stage{
		&StubStage{StageName: StageTrimming},
		&StubStage{StageName: StageCropping},
		&StubStage{StageName: StageRenderingSkeleton},
		&StubStage{StageName: StageComputingMetrics},
		&StubStage{StageName: StageGeneratingCoaching},
	}
}
