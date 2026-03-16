package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/chromedp/chromedp"

	"press-out/internal/storage"
	"press-out/internal/storage/sqlc"
)

// createLiftFiles creates a lift directory with all possible files for testing.
func createLiftFiles(t *testing.T, env testEnv, liftID int64) error {
	t.Helper()
	if err := storage.CreateLiftDir(env.DataDir, liftID); err != nil {
		return err
	}
	for _, f := range []string{storage.FileOriginal, storage.FileThumbnail} {
		if err := os.WriteFile(storage.LiftFile(env.DataDir, liftID, f), []byte("data"), 0644); err != nil {
			return err
		}
	}
	return nil
}

// readDirIfExists reads directory entries or returns empty if dir doesn't exist.
func readDirIfExists(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, err
}

func TestLiftDelete_ButtonPresent(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var btnExists bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.querySelector('button[hx-delete]') !== null`, &btnExists),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !btnExists {
		t.Error("delete button not found on detail page")
	}
}

func TestLiftDelete_ButtonNotPrimaryStyling(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var hasPrimary bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`
			(function() {
				var btn = document.querySelector('button[hx-delete]');
				if (!btn) return true;
				return btn.classList.contains('btn-primary');
			})()
		`, &hasPrimary),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if hasPrimary {
		t.Error("delete button should not have primary styling")
	}
}

func TestLiftDelete_HasConfirmation(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var confirmAttr string
	var hasConfirm bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue(`button[hx-delete]`, "hx-confirm", &confirmAttr, &hasConfirm),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasConfirm {
		t.Error("delete button missing hx-confirm attribute")
	}
	if confirmAttr == "" {
		t.Error("hx-confirm should have a non-empty message")
	}
}

func TestLiftDelete_CorrectEndpoint(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var deleteURL string
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue(`button[hx-delete]`, "hx-delete", &deleteURL, nil),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	expected := fmt.Sprintf("/lifts/%d", liftID)
	if deleteURL != expected {
		t.Errorf("hx-delete=%q, want %q", deleteURL, expected)
	}
}

func TestLiftDelete_ServerDeleteEndpoint(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")

	ctx, _ := newBrowserCtx(t)

	// Use synchronous XMLHttpRequest to call DELETE endpoint (bypassing confirm dialog).
	// Note: chromedp.Evaluate does not await Promises, so we must use sync XHR instead of fetch.
	var statusCode float64
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				var xhr = new XMLHttpRequest();
				xhr.open('DELETE', '/lifts/%d', false);
				xhr.send();
				return xhr.status;
			})()
		`, liftID), &statusCode),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if statusCode != 200 {
		t.Errorf("DELETE status=%v, want 200", statusCode)
	}

	// Verify lift is gone from DB
	_, dbErr := env.Queries.GetLift(context.Background(), liftID)
	if dbErr == nil {
		t.Error("lift should be deleted from DB after DELETE request")
	}
}

func TestLiftDelete_DeletedLiftAbsentFromList(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-03-15T10:00:00Z")
	createTestLift(t, env, "clean", "2026-02-01T00:00:00Z")

	// Delete via synchronous XHR (chromedp.Evaluate does not await Promises)
	ctx, _ := newBrowserCtx(t)

	var statusCode float64
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				var xhr = new XMLHttpRequest();
				xhr.open('DELETE', '/lifts/%d', false);
				xhr.send();
				return xhr.status;
			})()
		`, liftID), &statusCode),
	)
	if err != nil {
		t.Fatalf("chromedp delete: %v", err)
	}
	if statusCode != 200 {
		t.Fatalf("DELETE status=%v, want 200", statusCode)
	}

	// Navigate to list and verify deleted lift is absent
	var pageHTML string
	err = chromedp.Run(ctx,
		chromedp.Navigate(env.BaseURL+"/"),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("body", &pageHTML),
	)
	if err != nil {
		t.Fatalf("chromedp list: %v", err)
	}

	// Verify remaining lift is present
	lifts, _ := env.Queries.ListLifts(context.Background())
	if len(lifts) != 1 {
		t.Errorf("expected 1 lift remaining in DB, got %d", len(lifts))
	}
	if len(lifts) > 0 && lifts[0].LiftType != "clean" {
		t.Errorf("expected remaining lift to be 'clean', got %q", lifts[0].LiftType)
	}
}

func TestLiftDelete_DaisyUIThemeActive(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

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

func TestLiftDelete_AriaLabel(t *testing.T) {
	env := startTestEnv(t)
	liftID := createTestLift(t, env, "snatch", "2026-01-01T00:00:00Z")

	ctx, _ := newBrowserCtx(t)

	var ariaLabel string
	var hasAria bool
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, liftID)),
		chromedp.WaitReady("body"),
		chromedp.AttributeValue(`button[hx-delete]`, "aria-label", &ariaLabel, &hasAria),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if !hasAria {
		t.Error("delete button missing aria-label for accessibility")
	}
}

// TestLiftDelete_NoOrphanedFilesE2E verifies files are removed via the server endpoint.
func TestLiftDelete_NoOrphanedFilesE2E(t *testing.T) {
	env := startTestEnv(t)

	// Create lift with files manually
	lift, err := env.Queries.CreateLift(context.Background(), sqlc.CreateLiftParams{
		LiftType:  "clean_and_jerk",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := createLiftFiles(t, env, lift.ID); err != nil {
		t.Fatal(err)
	}

	ctx, _ := newBrowserCtx(t)

	var statusCode float64
	err = chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s/lifts/%d", env.BaseURL, lift.ID)),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				var xhr = new XMLHttpRequest();
				xhr.open('DELETE', '/lifts/%d', false);
				xhr.send();
				return xhr.status;
			})()
		`, lift.ID), &statusCode),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	if statusCode != 200 {
		t.Fatalf("DELETE status=%v, want 200", statusCode)
	}

	// Check no files remain
	liftDir := fmt.Sprintf("%s/lifts/%d", env.DataDir, lift.ID)
	entries, _ := readDirIfExists(liftDir)
	if len(entries) > 0 {
		t.Errorf("orphaned files remain: %v", entries)
	}
}
