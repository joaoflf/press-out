package e2e

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"

	_ "github.com/mattn/go-sqlite3"
	"press-out/internal/handler"
	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

// testEnv holds references needed by lift detail e2e tests.
type testEnv struct {
	BaseURL string
	Queries *sqlc.Queries
	DataDir string
}

// startTestEnv is like startServer but also exposes Queries and DataDir.
func startTestEnv(t *testing.T) testEnv {
	t.Helper()

	root := projectRoot(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to project root: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	cssPath := filepath.Join(root, "web", "static", "output.css")
	if _, err := os.Stat(cssPath); os.IsNotExist(err) {
		t.Fatal("web/static/output.css not found — run 'make tailwind-build' first")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.RunMigrations(db, filepath.Join(root, "sql", "schema")); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	queries := sqlc.New(db)

	base, err := template.ParseGlob(filepath.Join(root, "web", "templates", "layouts", "*.html"))
	if err != nil {
		t.Fatalf("parse layouts: %v", err)
	}
	if partials, _ := filepath.Glob(filepath.Join(root, "web", "templates", "partials", "*.html")); len(partials) > 0 {
		base, err = base.ParseGlob(filepath.Join(root, "web", "templates", "partials", "*.html"))
		if err != nil {
			t.Fatalf("parse partials: %v", err)
		}
	}

	pages, err := filepath.Glob(filepath.Join(root, "web", "templates", "pages", "*.html"))
	if err != nil {
		t.Fatalf("glob pages: %v", err)
	}
	tmplMap := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		name := filepath.Base(page)
		clone, err := base.Clone()
		if err != nil {
			t.Fatalf("clone base for %s: %v", name, err)
		}
		tmplMap[name], err = clone.ParseFiles(page)
		if err != nil {
			t.Fatalf("parse page %s: %v", name, err)
		}
	}

	srv := &handler.Server{
		Queries:   queries,
		Templates: tmplMap,
		DataDir:   tmpDir,
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: mux}
	go server.Serve(ln)
	t.Cleanup(func() { server.Close() })

	return testEnv{
		BaseURL: fmt.Sprintf("http://%s", ln.Addr().String()),
		Queries: queries,
		DataDir: tmpDir,
	}
}

// createTestLift inserts a lift and creates its video file.
func createTestLift(t *testing.T, env testEnv, liftType, createdAt string) int64 {
	t.Helper()
	lift, err := env.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  liftType,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("CreateLift: %v", err)
	}
	if err := storage.CreateLiftDir(env.DataDir, lift.ID); err != nil {
		t.Fatalf("CreateLiftDir: %v", err)
	}
	videoPath := storage.LiftFile(env.DataDir, lift.ID, storage.FileOriginal)
	if err := os.WriteFile(videoPath, []byte("fake video data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return lift.ID
}

func TestLiftDetail_PageLoads(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var theme string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue("html", "data-theme", &theme, nil),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if theme != "press-out" {
		t.Errorf("data-theme=%q, want %q", theme, "press-out")
	}
}

func TestLiftDetail_LiftTypeAndDate(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "clean_and_jerk", "2026-02-14T12:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var titleText, dateText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Text("h1", &titleText),
		chromedp.Text(".text-sm.text-base-content\\/60", &dateText),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if titleText != "Clean & Jerk" {
		t.Errorf("title=%q, want %q", titleText, "Clean & Jerk")
	}
	if dateText != "Feb 14, 2026" {
		t.Errorf("date=%q, want %q", dateText, "Feb 14, 2026")
	}
}

func TestLiftDetail_VideoElement(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var videoExists bool
	var videoSrc string
	var hasPlaysinline, hasPreload bool
	var preloadVal string

	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('video') !== null`, &videoExists),
		chromedp.AttributeValue("video", "src", &videoSrc, nil),
		chromedp.Evaluate(`document.querySelector('video').hasAttribute('playsinline')`, &hasPlaysinline),
		chromedp.AttributeValue("video", "preload", &preloadVal, &hasPreload),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !videoExists {
		t.Fatal("video element not found")
	}
	expectedSrc := fmt.Sprintf("/data/lifts/%d/original.mp4", liftID)
	if videoSrc != expectedSrc {
		t.Errorf("video src=%q, want %q", videoSrc, expectedSrc)
	}
	if !hasPlaysinline {
		t.Error("video missing playsinline attribute")
	}
	if preloadVal != "metadata" {
		t.Errorf("video preload=%q, want %q", preloadVal, "metadata")
	}
}

func TestLiftDetail_VideoFullWidth(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var isFullWidth bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var video = document.querySelector('video');
				if (!video) return false;
				var container = video.parentElement;
				var parentWidth = container.parentElement.clientWidth;
				return container.offsetWidth >= parentWidth - 1;
			})()
		`, &isFullWidth),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !isFullWidth {
		t.Error("video container is not full-width edge-to-edge")
	}
}

func TestLiftDetail_BackButton(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var backExists bool
	var backHref string
	var touchHeight float64

	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('a[aria-label="Back to lift list"]') !== null`, &backExists),
		chromedp.AttributeValue(`a[aria-label="Back to lift list"]`, "href", &backHref, nil),
		chromedp.Evaluate(`
			(function() {
				var el = document.querySelector('a[aria-label="Back to lift list"]');
				if (!el) return 0;
				var rect = el.getBoundingClientRect();
				return Math.min(rect.width, rect.height);
			})()
		`, &touchHeight),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !backExists {
		t.Fatal("back button not found")
	}
	if backHref != "/" {
		t.Errorf("back button href=%q, want %q", backHref, "/")
	}
	if touchHeight < 44 {
		t.Errorf("back button touch target=%vpx, want >= 44px", touchHeight)
	}
}

func TestLiftDetail_PlaceholderSections(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

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

func TestLiftDetail_VideoHeightConstrained(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	// Set mobile viewport (375x812 typical iPhone)
	var videoHeight, viewportHeight float64
	var contentVisible bool
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var video = document.querySelector('video');
				if (!video) return 0;
				var rect = video.getBoundingClientRect();
				return rect.height;
			})()
		`, &videoHeight),
		chromedp.Evaluate(`window.innerHeight`, &viewportHeight),
		// Check that content below the video is visible in the viewport
		chromedp.Evaluate(`
			(function() {
				var back = document.querySelector('a[aria-label="Back to lift list"]');
				if (!back) return false;
				var rect = back.getBoundingClientRect();
				return rect.top < window.innerHeight;
			})()
		`, &contentVisible),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	maxAllowed := viewportHeight * 0.55 // 50vh + small tolerance
	if videoHeight > maxAllowed {
		t.Errorf("video height=%.0fpx exceeds 50vh (%.0fpx max for viewport %.0fpx)", videoHeight, maxAllowed, viewportHeight)
	}
	if !contentVisible {
		t.Error("back button / content below video is not visible in viewport")
	}
}

func TestLiftDetail_ReprocessButtonVisible(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var btnExists bool
	var btnText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('button[aria-label="Re-process lift"]') !== null`, &btnExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !btnExists {
		t.Fatal("re-process button not found when pipeline is not running")
	}

	err = chromedp.Run(ctx,
		chromedp.Text(`button[aria-label="Re-process lift"]`, &btnText),
	)
	if err != nil {
		t.Fatalf("chromedp text: %v", err)
	}
	if btnText != "Re-process" {
		t.Errorf("button text=%q, want %q", btnText, "Re-process")
	}
}

func TestLiftDetail_CSSAndScriptsLoad(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var cssLink, htmxScript, appScript bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('link[href="/static/output.css"]') !== null`, &cssLink),
		chromedp.Evaluate(`document.querySelector('script[src*="htmx"]') !== null`, &htmxScript),
		chromedp.Evaluate(`document.querySelector('script[src="/static/app.js"]') !== null`, &appScript),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !cssLink {
		t.Error("output.css link not found")
	}
	if !htmxScript {
		t.Error("htmx script not found")
	}
	if !appScript {
		t.Error("app.js script not found")
	}

	// Verify output.css returns 200.
	resp, err := http.Get(env.BaseURL + "/static/output.css")
	if err != nil {
		t.Fatalf("fetch output.css: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("output.css status=%d, want 200", resp.StatusCode)
	}
}
