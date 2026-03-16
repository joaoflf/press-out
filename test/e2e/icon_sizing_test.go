package e2e

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestIconSizing_ListPagePlaceholderIcon(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	// The list page with no lifts won't have icons, so we need a lift without a thumbnail.
	// Use startTestEnv to create a lift (no thumbnail file = placeholder icon shown).
	env := startTestEnv(t)
	createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	var iconWidth, iconHeight float64
	err := chromedp.Run(ctx,
		chromedp.Navigate(env.BaseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('.w-5.h-5');
				if (!svg) return 0;
				return svg.getBoundingClientRect().width;
			})()
		`, &iconWidth),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('.w-5.h-5');
				if (!svg) return 0;
				return svg.getBoundingClientRect().height;
			})()
		`, &iconHeight),
	)
	_ = baseURL // startServer used for shared helpers
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// w-5 h-5 = 1.25rem = 20px
	if iconWidth < 18 || iconWidth > 22 {
		t.Errorf("placeholder icon width=%.1fpx, want ~20px (w-5)", iconWidth)
	}
	if iconHeight < 18 || iconHeight > 22 {
		t.Errorf("placeholder icon height=%.1fpx, want ~20px (h-5)", iconHeight)
	}
}

func TestIconSizing_DetailPageBackArrow(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var iconWidth, iconHeight float64
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('a[aria-label="Back to lift list"] svg');
				if (!svg) return 0;
				return svg.getBoundingClientRect().width;
			})()
		`, &iconWidth),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('a[aria-label="Back to lift list"] svg');
				if (!svg) return 0;
				return svg.getBoundingClientRect().height;
			})()
		`, &iconHeight),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// w-6 h-6 = 1.5rem = 24px
	if iconWidth < 22 || iconWidth > 26 {
		t.Errorf("back arrow icon width=%.1fpx, want ~24px (w-6)", iconWidth)
	}
	if iconHeight < 22 || iconHeight > 26 {
		t.Errorf("back arrow icon height=%.1fpx, want ~24px (h-6)", iconHeight)
	}
}

func TestIconSizing_DeleteButtonIcon(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "clean", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var iconWidth, iconHeight float64
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('button[aria-label="Delete lift"] svg');
				if (!svg) return 0;
				return svg.getBoundingClientRect().width;
			})()
		`, &iconWidth),
		chromedp.Evaluate(`
			(function() {
				var svg = document.querySelector('button[aria-label="Delete lift"] svg');
				if (!svg) return 0;
				return svg.getBoundingClientRect().height;
			})()
		`, &iconHeight),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	// w-4 h-4 = 1rem = 16px
	if iconWidth < 14 || iconWidth > 18 {
		t.Errorf("delete icon width=%.1fpx, want ~16px (w-4)", iconWidth)
	}
	if iconHeight < 14 || iconHeight > 18 {
		t.Errorf("delete icon height=%.1fpx, want ~16px (h-4)", iconHeight)
	}
}

func TestIconSizing_TailwindUtilityClassesPresent(t *testing.T) {
	baseURL := startServer(t)

	// Verify the critical sizing utility classes exist in the CSS.
	resp, err := fetch(baseURL + "/static/output.css")
	if err != nil {
		t.Fatalf("fetch output.css: %v", err)
	}

	requiredClasses := []struct {
		name    string
		pattern string
	}{
		{"w-4", ".w-4"},
		{"h-4", ".h-4"},
		{"w-5", ".w-5"},
		{"h-5", ".h-5"},
		{"w-6", ".w-6"},
		{"h-6", ".h-6"},
		{"w-11", ".w-11"},
		{"h-11", ".h-11"},
		{"w-12", ".w-12"},
		{"h-12", ".h-12"},
		{"flex", ".flex"},
		{"items-center", ".items-center"},
		{"justify-center", ".justify-center"},
		{"gap-3", ".gap-3"},
		{"flex-shrink-0", ".flex-shrink-0"},
	}

	for _, c := range requiredClasses {
		if !strings.Contains(resp, c.pattern) {
			t.Errorf("output.css missing class %s (pattern: %q)", c.name, c.pattern)
		}
	}
}

// fetch retrieves the body of the given URL as a string.
func fetch(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
