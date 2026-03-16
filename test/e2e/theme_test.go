package e2e

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	_ "github.com/mattn/go-sqlite3"
	"press-out/internal/handler"
	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

// projectRoot returns the absolute path to the project root by walking up
// from the current working directory until go.mod is found.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod)")
		}
		dir = parent
	}
}

// startServer creates a real HTTP server with templates, static files, and DB,
// returning the base URL.
func startServer(t *testing.T) string {
	t.Helper()

	root := projectRoot(t)

	// Change to project root so static file serving works (web/static is relative).
	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to project root: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Verify output.css exists.
	cssPath := filepath.Join(root, "web", "static", "output.css")
	if _, err := os.Stat(cssPath); os.IsNotExist(err) {
		t.Fatal("web/static/output.css not found — run 'make tailwind-build' first")
	}

	// Set up temp DB.
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

	// Parse base layout + partials as shared foundation.
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

	// Clone base for each page so {{define "content"}} blocks don't conflict.
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

	// Listen on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: mux}
	go server.Serve(ln)
	t.Cleanup(func() { server.Close() })

	return fmt.Sprintf("http://%s", ln.Addr().String())
}

// newBrowserCtx creates a headless Chrome context via chromedp.
func newBrowserCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	t.Cleanup(func() {
		cancel()
		allocCancel()
	})

	return ctx, cancel
}

// colorToRGB uses a canvas to convert any CSS color (including oklch) to rgb().
const colorToRGBJS = `
(function(sel, prop) {
	var el = document.querySelector(sel);
	if (!el) return '';
	var raw = window.getComputedStyle(el)[prop];
	var canvas = document.createElement('canvas');
	canvas.width = 1; canvas.height = 1;
	var ctx = canvas.getContext('2d');
	ctx.fillStyle = raw;
	ctx.fillRect(0, 0, 1, 1);
	var d = ctx.getImageData(0, 0, 1, 1).data;
	return 'rgb(' + d[0] + ', ' + d[1] + ', ' + d[2] + ')';
})
`

// cssVarToRGB resolves a DaisyUI CSS variable to an rgb() string via canvas.
const cssVarToRGBJS = `
(function(varName) {
	var raw = getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
	if (!raw) return '';
	var canvas = document.createElement('canvas');
	canvas.width = 1; canvas.height = 1;
	var ctx = canvas.getContext('2d');
	ctx.fillStyle = raw;
	ctx.fillRect(0, 0, 1, 1);
	var d = ctx.getImageData(0, 0, 1, 1).data;
	return 'rgb(' + d[0] + ', ' + d[1] + ', ' + d[2] + ')';
})
`

func TestOutputCSSLoads(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var linkHref string

	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue(`link[href="/static/output.css"]`, "href", &linkHref, nil),
	)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}

	if linkHref != "/static/output.css" {
		t.Errorf("expected link href=/static/output.css, got %q", linkHref)
	}

	// Verify output.css returns 200.
	resp, err := http.Get(baseURL + "/static/output.css")
	if err != nil {
		t.Fatalf("fetch output.css: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("output.css returned status %d, want 200", resp.StatusCode)
	}
}

func TestDataThemeAttribute(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var theme string
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
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

func TestBackgroundColor(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var bgColor string
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(colorToRGBJS+`('body', 'backgroundColor')`, &bgColor),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	// #FAFAF8 = rgb(250, 250, 248)
	expected := "rgb(250, 250, 248)"
	if bgColor != expected {
		t.Errorf("body background-color=%q, want %q (#FAFAF8)", bgColor, expected)
	}
}

func TestTextColor(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var textColor string
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(colorToRGBJS+`('body', 'color')`, &textColor),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	// #2D2D2D = rgb(45, 45, 45)
	expected := "rgb(45, 45, 45)"
	if textColor != expected {
		t.Errorf("body color=%q, want %q (#2D2D2D)", textColor, expected)
	}
}

func TestUploadButton(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var btnExists bool
	var btnBgColor string
	var btnColor string
	var btnHeight string
	var isFullWidth bool

	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),

		// Check button exists.
		chromedp.Evaluate(`document.querySelector('.btn.btn-primary') !== null`, &btnExists),

		// Get computed styles — use canvas conversion for colors.
		chromedp.Evaluate(colorToRGBJS+`('.btn.btn-primary', 'backgroundColor')`, &btnBgColor),
		chromedp.Evaluate(colorToRGBJS+`('.btn.btn-primary', 'color')`, &btnColor),
		chromedp.Evaluate(`window.getComputedStyle(document.querySelector('.btn.btn-primary')).height`, &btnHeight),

		// Check w-full: button should fill parent content area.
		chromedp.Evaluate(`
			(function() {
				var el = document.querySelector('.btn.btn-primary');
				var parentStyle = window.getComputedStyle(el.parentElement);
				var parentContent = el.parentElement.clientWidth
					- parseFloat(parentStyle.paddingLeft)
					- parseFloat(parentStyle.paddingRight);
				return el.offsetWidth >= parentContent - 1;
			})()
		`, &isFullWidth),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	if !btnExists {
		t.Fatal("upload button (.btn.btn-primary) not found")
	}

	// Primary color #8BA888 = rgb(139, 168, 136)
	expectedBg := "rgb(139, 168, 136)"
	if btnBgColor != expectedBg {
		t.Errorf("button bg=%q, want %q (#8BA888 sage)", btnBgColor, expectedBg)
	}

	// Text should be white.
	if btnColor != "rgb(255, 255, 255)" {
		t.Errorf("button text color=%q, want rgb(255, 255, 255)", btnColor)
	}

	// h-12 = 3rem = 48px
	if btnHeight != "48px" {
		t.Errorf("button height=%q, want 48px (h-12)", btnHeight)
	}

	if !isFullWidth {
		t.Error("upload button is not full-width (w-full)")
	}
}

func TestSystemFontStack(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var fontFamily string
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`window.getComputedStyle(document.body).fontFamily`, &fontFamily),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	lower := strings.ToLower(fontFamily)
	systemFonts := []string{"-apple-system", "blinkmacsystemfont", "segoe ui", "system-ui", "sans-serif"}
	for _, font := range systemFonts {
		if !strings.Contains(lower, font) {
			t.Errorf("font-family %q does not contain %q", fontFamily, font)
		}
	}
}

func TestMobileViewportMeta(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	var hasViewport bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('meta[name="viewport"]') !== null`, &hasViewport),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasViewport {
		t.Error("mobile viewport meta tag not found")
	}
}

func TestNoRedWarningColors(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	// The press-out theme intentionally omits warning/error colors.
	// DaisyUI fills defaults, but they should NOT be used in the UI.
	// Verify no visible elements on the page use red or warning colors.
	var redElements int
	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var count = 0;
				var all = document.querySelectorAll('*');
				for (var i = 0; i < all.length; i++) {
					var style = window.getComputedStyle(all[i]);
					var bg = style.backgroundColor;
					var fg = style.color;
					// Check for red-ish colors: oklch or rgb with high red, low green/blue.
					var colors = [bg, fg];
					for (var j = 0; j < colors.length; j++) {
						var c = colors[j];
						if (!c || c === 'rgba(0, 0, 0, 0)' || c === 'transparent') continue;
						// Convert via canvas to get RGB.
						var canvas = document.createElement('canvas');
						canvas.width = 1; canvas.height = 1;
						var ctx = canvas.getContext('2d');
						ctx.fillStyle = c;
						ctx.fillRect(0, 0, 1, 1);
						var d = ctx.getImageData(0, 0, 1, 1).data;
						// Red-ish: R > 180, G < 100, B < 100
						if (d[0] > 180 && d[1] < 100 && d[2] < 100) count++;
					}
				}
				return count;
			})()
		`, &redElements),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}

	if redElements > 0 {
		t.Errorf("found %d elements with red/warning colors on the page", redElements)
	}
}

func TestDaisyUIClassesInCSS(t *testing.T) {
	baseURL := startServer(t)

	// Fetch output.css and verify it contains key DaisyUI component styles.
	resp, err := http.Get(baseURL + "/static/output.css")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	css := string(data)

	// Check for DaisyUI component selectors (minified CSS uses .btn, .modal, etc.)
	requiredPatterns := []struct {
		name    string
		pattern string
	}{
		{"btn component", ".btn"},
		{"modal component", ".modal"},
		{"join component", ".join"},
	}
	for _, p := range requiredPatterns {
		if !strings.Contains(css, p.pattern) {
			t.Errorf("output.css missing %s (pattern: %q)", p.name, p.pattern)
		}
	}

	// DaisyUI v4 stores theme colors as oklch CSS variables, not hex values.
	// Verify the custom theme variables are defined.
	themeVars := []struct {
		name   string
		varDef string
	}{
		{"base-100 (--b1)", "--b1:"},
		{"base-content (--bc)", "--bc:"},
		{"primary (--p)", "--p:"},
		{"secondary (--s)", "--s:"},
		{"neutral (--n)", "--n:"},
		{"info (--in)", "--in:"},
		{"success (--su)", "--su:"},
	}
	for _, v := range themeVars {
		if !strings.Contains(css, v.varDef) {
			t.Errorf("output.css missing theme variable %s", v.name)
		}
	}
}

func TestThemeColorsCombined(t *testing.T) {
	baseURL := startServer(t)
	ctx, _ := newBrowserCtx(t)

	err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.WaitReady("body"),
	)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}

	// DaisyUI v4 defines theme colors as CSS custom properties on [data-theme].
	// We read them via getComputedStyle and convert oklch→rgb via canvas.
	type colorCheck struct {
		name     string
		cssVar   string
		expected string
	}

	checks := []colorCheck{
		{"base-100 (background)", "--b1", "rgb(250, 250, 248)"},   // #FAFAF8
		{"base-content (text)", "--bc", "rgb(45, 45, 45)"},        // #2D2D2D
		{"primary (sage)", "--p", "rgb(139, 168, 136)"},           // #8BA888
		{"secondary (stone)", "--s", "rgb(196, 191, 174)"},        // #C4BFAE
		{"neutral", "--n", "rgb(237, 237, 234)"},                  // #EDEDEA
		{"info (coaching)", "--in", "rgb(155, 176, 186)"},         // #9BB0BA
		{"success", "--su", "rgb(125, 166, 125)"},                 // #7DA67D
	}

	for _, check := range checks {
		var got string
		js := fmt.Sprintf(`
			(function() {
				var raw = getComputedStyle(document.documentElement).getPropertyValue('%s').trim();
				if (!raw) return 'NOT_DEFINED';
				// DaisyUI v4 stores oklch components, need to reconstruct.
				// Try setting as-is on a canvas context.
				var canvas = document.createElement('canvas');
				canvas.width = 1; canvas.height = 1;
				var ctx = canvas.getContext('2d');
				// Try oklch() wrapping if it looks like raw numbers.
				if (raw.match(/^[\d.\s%%\/]+$/) || raw.indexOf('oklch') >= 0) {
					ctx.fillStyle = raw.indexOf('oklch') >= 0 ? raw : 'oklch(' + raw + ')';
				} else {
					ctx.fillStyle = raw;
				}
				ctx.fillRect(0, 0, 1, 1);
				var d = ctx.getImageData(0, 0, 1, 1).data;
				return 'rgb(' + d[0] + ', ' + d[1] + ', ' + d[2] + ')';
			})()
		`, check.cssVar)

		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &got)); err != nil {
			t.Errorf("%s: evaluate failed: %v", check.name, err)
			continue
		}
		if got == "NOT_DEFINED" {
			t.Errorf("%s: CSS variable %s not defined in theme", check.name, check.cssVar)
			continue
		}
		if got != check.expected {
			t.Errorf("%s: got %q, want %q", check.name, got, check.expected)
		}
	}
}
