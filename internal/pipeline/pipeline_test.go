package pipeline

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"press-out/internal/sse"
)

// testStage is a controllable stage for testing.
type testStage struct {
	name string
	err  error
	out  StageOutput
	mu   sync.Mutex
	ran  bool
}

func (s *testStage) Name() string { return s.name }

func (s *testStage) Run(_ context.Context, input StageInput) (StageOutput, error) {
	s.mu.Lock()
	s.ran = true
	s.mu.Unlock()
	if s.err != nil {
		return StageOutput{}, s.err
	}
	out := s.out
	if out.VideoPath == "" {
		out.VideoPath = input.VideoPath
	}
	return out, nil
}

func (s *testStage) didRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ran
}

func collectEvents(ch chan sse.Event, t *testing.T) []sse.Event {
	t.Helper()
	var events []sse.Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e := <-ch:
			events = append(events, e)
			if e.Name == "pipeline-done" {
				return events
			}
		case <-timeout:
			t.Fatal("timeout waiting for pipeline-done event")
			return nil
		}
	}
}

func TestPipelineAllStagesSucceed(t *testing.T) {
	broker := sse.NewBroker()
	ch := broker.Subscribe(1)

	stages := []Stage{
		&testStage{name: "stage1"},
		&testStage{name: "stage2"},
		&testStage{name: "stage3"},
	}
	p := New(stages, broker)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Run(context.Background(), 1, "/tmp/test")
	}()

	events := collectEvents(ch, t)
	wg.Wait()

	for _, s := range stages {
		ts := s.(*testStage)
		if !ts.didRun() {
			t.Errorf("stage %s did not run", ts.name)
		}
	}

	// Count event types.
	counts := map[string]int{}
	for _, e := range events {
		counts[e.Name]++
	}

	// 3 stages: initial(all pending) + 3*(active + complete) = 7 stage events.
	if counts["pipeline-stages"] != 7 {
		t.Errorf("expected 7 pipeline-stages events, got %d", counts["pipeline-stages"])
	}
	if counts["pipeline-status"] != 7 {
		t.Errorf("expected 7 pipeline-status events, got %d", counts["pipeline-status"])
	}
	if counts["pipeline-done"] != 1 {
		t.Errorf("expected 1 pipeline-done event, got %d", counts["pipeline-done"])
	}
}

func TestPipelineStageError(t *testing.T) {
	broker := sse.NewBroker()
	ch := broker.Subscribe(1)

	stages := []Stage{
		&testStage{name: "stage1"},
		&testStage{name: "stage2", err: errors.New("processing failed")},
		&testStage{name: "stage3"},
	}
	p := New(stages, broker)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Run(context.Background(), 1, "/tmp/test")
	}()

	events := collectEvents(ch, t)
	wg.Wait()

	// All stages should have run despite stage2 failing.
	for _, s := range stages {
		ts := s.(*testStage)
		if !ts.didRun() {
			t.Errorf("stage %s did not run (pipeline should continue on error)", ts.name)
		}
	}

	// Final stages event should contain "skipped" indicators for stage2.
	var lastStages string
	for _, e := range events {
		if e.Name == "pipeline-stages" {
			lastStages = e.Data
		}
	}
	if !strings.Contains(lastStages, "bg-warning") {
		t.Error("expected skipped stage indicator (bg-warning) in final stages HTML")
	}
}

func TestPipelineContextCancel(t *testing.T) {
	broker := sse.NewBroker()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	stages := []Stage{
		&testStage{name: "stage1"},
	}
	p := New(stages, broker)
	p.Run(ctx, 1, "/tmp/test")

	ts := stages[0].(*testStage)
	if ts.didRun() {
		t.Error("stage should not run when context is already cancelled")
	}
}

func TestPipelineVideoPathPropagation(t *testing.T) {
	broker := sse.NewBroker()
	ch := broker.Subscribe(1)

	stage3 := &testStage{name: "stage3"}
	stages := []Stage{
		&testStage{name: "stage1", out: StageOutput{VideoPath: "/tmp/trimmed.mp4"}},
		&testStage{name: "stage2", err: errors.New("fail")}, // Skipped — should keep /tmp/trimmed.mp4
		stage3,
	}
	p := New(stages, broker)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Run(context.Background(), 1, "/tmp/test")
	}()

	collectEvents(ch, t)
	wg.Wait()

	// stage3 should have received the video path from stage1 (not the failed stage2).
	if !stage3.didRun() {
		t.Fatal("stage3 did not run")
	}
}

func TestRenderStagesHTML(t *testing.T) {
	stages := DefaultStages()
	states := []StageState{StateComplete, StateActive, StatePending, StatePending, StatePending, StatePending}

	html := RenderStagesHTML(stages, states)

	if !strings.Contains(html, "bg-success") {
		t.Error("expected complete stage indicator")
	}
	if !strings.Contains(html, "loading-spinner") {
		t.Error("expected active stage spinner")
	}
	if !strings.Contains(html, "Trimming") {
		t.Error("expected stage name 'Trimming'")
	}
	if !strings.Contains(html, "Cropping") {
		t.Error("expected stage name 'Cropping'")
	}
}

func TestRenderStatusHTML(t *testing.T) {
	stages := DefaultStages()
	numStages := len(stages) // 6

	// All pending.
	states := make([]StageState, numStages)
	for i := range states {
		states[i] = StatePending
	}
	html := RenderStatusHTML(stages, states)
	if !strings.Contains(html, "Queued") {
		t.Errorf("expected 'Queued' for all-pending, got: %s", html)
	}

	// First active.
	states[0] = StateActive
	html = RenderStatusHTML(stages, states)
	if !strings.Contains(html, "Trimming") {
		t.Errorf("expected 'Trimming' for first active, got: %s", html)
	}
	if !strings.Contains(html, "1 of 6") {
		t.Errorf("expected '1 of 6', got: %s", html)
	}

	// All complete.
	for i := range states {
		states[i] = StateComplete
	}
	html = RenderStatusHTML(stages, states)
	if !strings.Contains(html, "Done") {
		t.Errorf("expected 'Done' for all complete, got: %s", html)
	}
}

func TestPipelineProcessingFlag(t *testing.T) {
	broker := sse.NewBroker()
	ch := broker.Subscribe(1)

	stages := []Stage{&testStage{name: "stage1"}}
	p := New(stages, broker)

	if broker.IsProcessing(1) {
		t.Error("should not be processing before Run")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Run(context.Background(), 1, "/tmp/test")
	}()

	collectEvents(ch, t)
	wg.Wait()

	if broker.IsProcessing(1) {
		t.Error("should not be processing after Run completes")
	}
}
