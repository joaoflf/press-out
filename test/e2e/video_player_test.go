package e2e

import (
	"fmt"
	"os"
	"testing"

	"github.com/chromedp/chromedp"

	"press-out/internal/storage"
)

// createTestLiftWithSkeleton creates a test lift and writes both a fake
// original.mp4 and skeleton.mp4 to the lift directory.
func createTestLiftWithSkeleton(t *testing.T, env testEnv, liftType, createdAt string) int64 {
	t.Helper()
	liftID := createTestLift(t, env, liftType, createdAt)
	skeletonPath := storage.LiftFile(env.DataDir, liftID, storage.FileSkeleton)
	if err := os.WriteFile(skeletonPath, []byte("fake skeleton data"), 0644); err != nil {
		t.Fatalf("WriteFile skeleton: %v", err)
	}
	return liftID
}

func TestVideoPlayer_SpeedStripVisible(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var btn025, btn05, btn1 bool
	var text025, text05, text1 string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('.speed-btn[data-speed="0.25"]') !== null`, &btn025),
		chromedp.Evaluate(`document.querySelector('.speed-btn[data-speed="0.5"]') !== null`, &btn05),
		chromedp.Evaluate(`document.querySelector('.speed-btn[data-speed="1"]') !== null`, &btn1),
		chromedp.Text(`.speed-btn[data-speed="0.25"]`, &text025),
		chromedp.Text(`.speed-btn[data-speed="0.5"]`, &text05),
		chromedp.Text(`.speed-btn[data-speed="1"]`, &text1),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !btn025 {
		t.Error("0.25x speed button not found")
	}
	if !btn05 {
		t.Error("0.5x speed button not found")
	}
	if !btn1 {
		t.Error("1x speed button not found")
	}
	if text025 != "0.25x" {
		t.Errorf("0.25x button text=%q, want %q", text025, "0.25x")
	}
	if text05 != "0.5x" {
		t.Errorf("0.5x button text=%q, want %q", text05, "0.5x")
	}
	if text1 != "1x" {
		t.Errorf("1x button text=%q, want %q", text1, "1x")
	}
}

func TestVideoPlayer_ModeBadgeWithSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLiftWithSkeleton(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var badgeExists bool
	var badgeText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('mode-badge') !== null`, &badgeExists),
		chromedp.Text("#mode-badge", &badgeText),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !badgeExists {
		t.Fatal("mode badge not found when skeleton.mp4 exists")
	}
	if badgeText != "Skeleton" {
		t.Errorf("mode badge text=%q, want %q", badgeText, "Skeleton")
	}
}

func TestVideoPlayer_NoBadgeWithoutSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var badgeExists bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('mode-badge') !== null`, &badgeExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if badgeExists {
		t.Error("mode badge should NOT be displayed when only clean video exists")
	}
}

func TestVideoPlayer_GradientBackdrop(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasGradient bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var el = document.querySelector('.bg-gradient-to-t');
				if (!el) return false;
				var bg = window.getComputedStyle(el).backgroundImage;
				return bg.indexOf('gradient') >= 0;
			})()
		`, &hasGradient),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasGradient {
		t.Error("speed strip gradient backdrop not found")
	}
}

func TestVideoPlayer_AutoplayWithSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLiftWithSkeleton(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasAutoplay, hasMuted, hasLoop bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('lift-video').hasAttribute('autoplay')`, &hasAutoplay),
		chromedp.Evaluate(`document.getElementById('lift-video').hasAttribute('muted')`, &hasMuted),
		chromedp.Evaluate(`document.getElementById('lift-video').hasAttribute('loop')`, &hasLoop),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasAutoplay {
		t.Error("video missing autoplay attribute when skeleton exists")
	}
	if !hasMuted {
		t.Error("video missing muted attribute when skeleton exists")
	}
	if !hasLoop {
		t.Error("video missing loop attribute when skeleton exists")
	}
}

func TestVideoPlayer_NoAutoplayWithoutSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasAutoplay bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('lift-video').hasAttribute('autoplay')`, &hasAutoplay),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if hasAutoplay {
		t.Error("video should NOT have autoplay when skeleton does not exist")
	}
}

func TestVideoPlayer_ToggleOverlayWithSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLiftWithSkeleton(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var overlayExists bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('video-toggle-overlay') !== null`, &overlayExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !overlayExists {
		t.Error("toggle overlay not found when skeleton.mp4 exists")
	}
}

func TestVideoPlayer_NoToggleOverlayWithoutSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var overlayExists bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.getElementById('video-toggle-overlay') !== null`, &overlayExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if overlayExists {
		t.Error("toggle overlay should NOT be present when only clean video exists")
	}
}

func TestVideoPlayer_SkeletonVideoSrc(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLiftWithSkeleton(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var videoSrc string
	var skelSrc, cleanSrc string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue("#lift-video", "src", &videoSrc, nil),
		chromedp.Evaluate(`document.getElementById('lift-video').dataset.skeletonSrc`, &skelSrc),
		chromedp.Evaluate(`document.getElementById('lift-video').dataset.cleanSrc`, &cleanSrc),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	expectedSkel := fmt.Sprintf("/data/lifts/%d/skeleton.mp4", liftID)
	expectedClean := fmt.Sprintf("/data/lifts/%d/original.mp4", liftID)

	if videoSrc != expectedSkel {
		t.Errorf("video src=%q, want skeleton %q", videoSrc, expectedSkel)
	}
	if skelSrc != expectedSkel {
		t.Errorf("data-skeleton-src=%q, want %q", skelSrc, expectedSkel)
	}
	if cleanSrc != expectedClean {
		t.Errorf("data-clean-src=%q, want %q", cleanSrc, expectedClean)
	}
}

func TestVideoPlayer_SpeedStripWithoutSkeleton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var speedBtnCount int
	var badgeExists, overlayExists bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelectorAll('.speed-btn').length`, &speedBtnCount),
		chromedp.Evaluate(`document.getElementById('mode-badge') !== null`, &badgeExists),
		chromedp.Evaluate(`document.getElementById('video-toggle-overlay') !== null`, &overlayExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if speedBtnCount != 3 {
		t.Errorf("speed button count=%d, want 3", speedBtnCount)
	}
	if badgeExists {
		t.Error("mode badge should NOT exist without skeleton")
	}
	if overlayExists {
		t.Error("toggle overlay should NOT exist without skeleton")
	}
}
