package e2e

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestCardLayout_ContentPadding(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var contentPadding string
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var el = document.querySelector('.p-3.flex.flex-col');
				if (!el) return '';
				return getComputedStyle(el).padding;
			})()
		`, &contentPadding),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if contentPadding != "12px" {
		t.Errorf("content padding=%q, want %q (p-3 = 12px)", contentPadding, "12px")
	}
}

func TestCardLayout_PlaceholderSectionsExist(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasCoaching, hasTimeline, hasMetrics bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('coaching-section') !== null`, &hasCoaching),
		chromedp.Evaluate(`document.getElementById('phase-timeline-section') !== null`, &hasTimeline),
		chromedp.Evaluate(`document.getElementById('metrics-section') !== null`, &hasMetrics),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasCoaching {
		t.Error("missing #coaching-section placeholder")
	}
	if !hasTimeline {
		t.Error("missing #phase-timeline-section placeholder")
	}
	if !hasMetrics {
		t.Error("missing #metrics-section placeholder")
	}
}

func TestCardLayout_BackButtonAboveContent(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var backAbove bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var back = document.querySelector('a[aria-label="Back to lift list"]');
				var content = document.getElementById('coaching-section');
				if (!back || !content) return false;
				return back.getBoundingClientRect().top < content.getBoundingClientRect().top;
			})()
		`, &backAbove),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !backAbove {
		t.Error("back button should be above content sections")
	}
}

func TestCardLayout_ProcessingTitle(t *testing.T) {
	env := startTestEnvWithBroker(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	env.Broker.StartProcessing(liftID)
	t.Cleanup(func() { env.Broker.StopProcessing(liftID) })

	ctx, _ := newBrowserCtx(t)

	var titleText, dateText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Text("h1", &titleText),
		chromedp.Evaluate(`
			(function() {
				var h1 = document.querySelector('h1');
				if (!h1) return '';
				var dateSub = h1.parentElement.querySelector('p');
				return dateSub ? dateSub.textContent.trim() : '';
			})()
		`, &dateText),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if titleText != "Processing Snatch" {
		t.Errorf("title=%q, want %q", titleText, "Processing Snatch")
	}
	if dateText != "Mar 20, 2026" {
		t.Errorf("date=%q, want %q", dateText, "Mar 20, 2026")
	}
}

func TestCardLayout_PipelineStagesPendingIcons(t *testing.T) {
	env := startTestEnvWithBroker(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	env.Broker.StartProcessing(liftID)
	t.Cleanup(func() { env.Broker.StopProcessing(liftID) })

	ctx, _ := newBrowserCtx(t)

	var stageCount int
	var firstStageHasPulse bool
	var pendingBgColor string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		// Count stage rows in the SSE swap target.
		chromedp.Evaluate(`
			(function() {
				var swap = document.querySelector('[sse-swap]');
				if (!swap) return 0;
				return swap.querySelectorAll('.flex.items-center.gap-3').length;
			})()
		`, &stageCount),
		// First stage should have animate-pulse (active).
		chromedp.Evaluate(`
			(function() {
				var swap = document.querySelector('[sse-swap]');
				if (!swap) return false;
				return swap.querySelector('.animate-pulse') !== null;
			})()
		`, &firstStageHasPulse),
		// Pending stages should have neutral bg.
		chromedp.Evaluate(`
			(function() {
				var swap = document.querySelector('[sse-swap]');
				if (!swap) return '';
				var icons = swap.querySelectorAll('.w-7.h-7.rounded-full');
				if (icons.length < 2) return '';
				return getComputedStyle(icons[1]).backgroundColor;
			})()
		`, &pendingBgColor),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if stageCount != 6 {
		t.Errorf("stage count=%d, want 6", stageCount)
	}
	if !firstStageHasPulse {
		t.Error("first stage should have animate-pulse class (active state)")
	}
	// #EDEDEA = rgb(237, 237, 234)
	if pendingBgColor != "rgb(237, 237, 234)" {
		t.Errorf("pending stage bg=%q, want %q", pendingBgColor, "rgb(237, 237, 234)")
	}
}

func TestCardLayout_NoHorizontalOverflow(t *testing.T) {
	env := startTestEnvWithBroker(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	env.Broker.StartProcessing(liftID)
	t.Cleanup(func() { env.Broker.StopProcessing(liftID) })

	ctx, _ := newBrowserCtx(t)

	var hasOverflow bool
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.body.scrollWidth > document.body.clientWidth`, &hasOverflow),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if hasOverflow {
		t.Error("page has horizontal overflow at 375px viewport")
	}
}

func TestCardLayout_ProcessingHasSSEConnection(t *testing.T) {
	env := startTestEnvWithBroker(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	env.Broker.StartProcessing(liftID)
	t.Cleanup(func() { env.Broker.StopProcessing(liftID) })

	ctx, _ := newBrowserCtx(t)

	var hasSSE bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('[sse-connect]') !== null`, &hasSSE),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasSSE {
		t.Error("processing lift should have SSE connection")
	}
}

func TestCardLayout_NonProcessingNoSSE(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-20T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasSSE bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('[sse-connect]') !== null`, &hasSSE),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if hasSSE {
		t.Error("non-processing lift should not have SSE connection")
	}
}
