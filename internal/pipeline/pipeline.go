package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"press-out/internal/sse"
	"press-out/internal/storage"
)

// Pipeline orchestrates sequential execution of processing stages.
type Pipeline struct {
	Stages []Stage
	Broker *sse.Broker
}

// New creates a Pipeline with the given stages and SSE broker.
func New(stages []Stage, broker *sse.Broker) *Pipeline {
	return &Pipeline{
		Stages: stages,
		Broker: broker,
	}
}

// Run executes all stages sequentially for a lift, emitting SSE events.
// Call as a goroutine; uses the provided context for cancellation.
func (p *Pipeline) Run(ctx context.Context, liftID int64, dataDir string) {
	p.Broker.StartProcessing(liftID)
	defer p.Broker.StopProcessing(liftID)

	videoPath := storage.LiftFile(dataDir, liftID, storage.FileOriginal)
	total := len(p.Stages)

	slog.Info("pipeline started", "lift_id", liftID, "stages", total)

	states := make([]StageState, total)
	for i := range states {
		states[i] = StatePending
	}
	p.emitEvents(liftID, states)

	for i, stage := range p.Stages {
		if ctx.Err() != nil {
			slog.Info("pipeline cancelled", "lift_id", liftID, "stage", stage.Name())
			return
		}

		states[i] = StateActive
		p.emitEvents(liftID, states)

		start := time.Now()
		output, err := stage.Run(ctx, StageInput{
			LiftID:    liftID,
			DataDir:   dataDir,
			VideoPath: videoPath,
		})
		durationMs := time.Since(start).Milliseconds()

		if err != nil {
			slog.Error("pipeline stage failed",
				"lift_id", liftID,
				"stage", stage.Name(),
				"error", err,
				"duration_ms", durationMs,
			)
			states[i] = StateSkipped
		} else {
			slog.Info("pipeline stage complete",
				"lift_id", liftID,
				"stage", stage.Name(),
				"duration_ms", durationMs,
			)
			states[i] = StateComplete
			if output.VideoPath != "" {
				videoPath = output.VideoPath
			}
		}

		p.emitEvents(liftID, states)
	}

	p.Broker.Publish(liftID, sse.Event{Name: "pipeline-done", Data: ""})
	slog.Info("pipeline complete", "lift_id", liftID)
}

func (p *Pipeline) emitEvents(liftID int64, states []StageState) {
	p.Broker.Publish(liftID, sse.Event{
		Name: "pipeline-stages",
		Data: RenderStagesHTML(p.Stages, states),
	})
	p.Broker.Publish(liftID, sse.Event{
		Name: "pipeline-status",
		Data: RenderStatusHTML(p.Stages, states),
	})
}

// RenderStagesHTML builds the full pipeline checklist HTML for the detail page.
func RenderStagesHTML(stages []Stage, states []StageState) string {
	var b strings.Builder
	b.WriteString(`<div class="flex flex-col gap-3">`)
	for i, stage := range stages {
		state := states[i]
		b.WriteString(`<div class="flex items-center gap-3">`)

		switch state {
		case StateComplete:
			b.WriteString(`<div class="w-6 h-6 rounded-full bg-success flex items-center justify-center flex-shrink-0">`)
			b.WriteString(`<svg class="w-3.5 h-3.5 text-success-content" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">`)
			b.WriteString(`<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg></div>`)
			b.WriteString(fmt.Sprintf(`<span class="text-sm">%s</span>`, stage.Name()))
		case StateActive:
			b.WriteString(`<span class="loading loading-spinner loading-sm text-primary flex-shrink-0"></span>`)
			b.WriteString(fmt.Sprintf(`<span class="text-sm font-medium text-primary">%s</span>`, stage.Name()))
		case StateSkipped:
			b.WriteString(`<div class="w-6 h-6 rounded-full bg-warning flex items-center justify-center flex-shrink-0">`)
			b.WriteString(`<svg class="w-3.5 h-3.5 text-warning-content" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">`)
			b.WriteString(`<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01"/></svg></div>`)
			b.WriteString(fmt.Sprintf(`<span class="text-sm text-base-content/50">%s</span>`, stage.Name()))
		default: // pending
			b.WriteString(`<div class="w-6 h-6 rounded-full border-2 border-base-content/20 flex-shrink-0"></div>`)
			b.WriteString(fmt.Sprintf(`<span class="text-sm text-base-content/40">%s</span>`, stage.Name()))
		}

		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// RenderStatusHTML builds the compact pipeline status for the list page.
func RenderStatusHTML(stages []Stage, states []StageState) string {
	activeName := ""
	completedCount := 0
	total := len(stages)

	for i, state := range states {
		switch state {
		case StateActive:
			activeName = stages[i].Name()
		case StateComplete, StateSkipped:
			completedCount++
		}
	}

	if completedCount == total {
		return `<span class="badge badge-success badge-sm gap-1">` +
			`<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">` +
			`<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>Done</span>`
	}

	if activeName == "" {
		return `<span class="badge badge-ghost badge-sm">Queued</span>`
	}

	return fmt.Sprintf(
		`<div class="flex items-center gap-1.5">`+
			`<span class="loading loading-spinner loading-xs text-primary"></span>`+
			`<span class="text-xs text-base-content/70">%s</span>`+
			`<span class="text-xs text-base-content/40">%d of %d</span></div>`,
		activeName, completedCount+1, total,
	)
}
