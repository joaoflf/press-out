package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"press-out/internal/ffmpeg"
	"press-out/internal/pose"
)

// Python spike expected trim boundaries (density-bridged, validated).
var pyExpected = map[string][2]float64{
	"sample-snatch-1":        {4.93, 12.03},
	"sample-snatch-2":        {14.28, 20.50},
	"sample-cj-walk-away":    {4.25, 15.05},
	"sample-clean-walk-away": {4.62, 12.08},
}

type rigPair struct {
	name      string
	videoPath string
	kpPath    string
}

type rigResult struct {
	name      string
	confident bool

	// Trim boundaries (raw = before padding, padded = after padding + clamping).
	goStart    float64
	goEnd      float64
	goPadStart float64
	goPadEnd   float64
	pyStart    float64 // Python values are already padded.
	pyEnd      float64

	// Crop parameters.
	cropW     int
	cropH     int
	originY   int
	cropX     int
	isDynamic bool
	segments  []CropSegment
	walkPct   float64

	// Output paths relative to output root.
	trimmedRel string
	croppedRel string
}

func discoverRigPairs(t *testing.T, dir string) []rigPair {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read test dir %s: %v", dir, err)
	}

	var pairs []rigPair
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".mp4")
		kpPath := filepath.Join(dir, name+".json")
		if _, err := os.Stat(kpPath); err == nil {
			pairs = append(pairs, rigPair{
				name:      name,
				videoPath: filepath.Join(dir, e.Name()),
				kpPath:    kpPath,
			})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })
	return pairs
}

// TestRig runs the Go trim+crop pipeline against all test videos and generates
// an HTML report for visual comparison.
//
// Run:
//
//	go test -run TestRig -v ./internal/pipeline/stages/ -count=1
//
// Open report automatically:
//
//	OPEN_REPORT=1 go test -run TestRig -v ./internal/pipeline/stages/ -count=1
func TestRig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rig in short mode")
	}
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	root := filepath.Join("..", "..", "..")
	testDir := filepath.Join(root, "testdata", "videos")
	outputRoot := filepath.Join(root, "spikes", "go-rig", "output")

	pairs := discoverRigPairs(t, testDir)
	if len(pairs) == 0 {
		t.Skip("no test video pairs found in testdata/videos/")
	}

	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	var results []rigResult

	for _, p := range pairs {
		outDir := filepath.Join(outputRoot, p.name)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Fatalf("create %s output dir: %v", p.name, err)
		}

		t.Logf("=== %s ===", p.name)

		// --- Trim detection ---
		goStart, goEnd, confident, err := detectLiftDensityBridged(p.kpPath)
		if err != nil {
			t.Errorf("%s: trim detection error: %v", p.name, err)
			continue
		}

		py := pyExpected[p.name]
		r := rigResult{
			name:      p.name,
			confident: confident,
			goStart:   goStart,
			goEnd:     goEnd,
			pyStart:   py[0],
			pyEnd:     py[1],
		}

		if !confident {
			t.Logf("  trim: NOT CONFIDENT  py=%.2f-%.2fs", py[0], py[1])
			results = append(results, r)
			continue
		}

		// --- Trim video ---
		trimStart := goStart - trimPaddingSec
		if trimStart < 0 {
			trimStart = 0
		}
		trimEnd := goEnd + trimPaddingSec
		videoDur, _ := ffmpeg.GetDuration(ctx, p.videoPath)
		if videoDur > 0 && trimEnd > videoDur {
			trimEnd = videoDur
		}
		r.goPadStart = trimStart
		r.goPadEnd = trimEnd

		t.Logf("  trim raw:    go=%.2f-%.2fs", goStart, goEnd)
		t.Logf("  trim padded: go=%.2f-%.2fs  py=%.2f-%.2fs  Δstart=%+.2f  Δend=%+.2f",
			trimStart, trimEnd, py[0], py[1],
			trimStart-py[0], trimEnd-py[1])

		trimmedPath := filepath.Join(outDir, "trimmed.mp4")
		if err := ffmpeg.TrimVideo(ctx, p.videoPath, trimmedPath, trimStart, trimEnd-trimStart); err != nil {
			t.Errorf("%s: ffmpeg trim: %v", p.name, err)
			continue
		}
		r.trimmedRel = filepath.Join(p.name, "trimmed.mp4")

		// --- Parse keypoints + filter to trim range ---
		kpData, err := os.ReadFile(p.kpPath)
		if err != nil {
			t.Errorf("%s: read keypoints: %v", p.name, err)
			continue
		}
		var result pose.Result
		if err := json.Unmarshal(kpData, &result); err != nil {
			t.Errorf("%s: parse keypoints: %v", p.name, err)
			continue
		}

		trimStartMs := int64(trimStart * 1000)
		trimEndMs := int64(trimEnd * 1000)
		var frames []pose.Frame
		for _, f := range result.Frames {
			if f.TimeOffsetMs >= trimStartMs && f.TimeOffsetMs <= trimEndMs {
				frames = append(frames, f)
			}
		}
		if len(frames) == 0 {
			t.Errorf("%s: no frames in trim range", p.name)
			continue
		}

		sourceW := result.SourceWidth
		sourceH := result.SourceHeight

		// --- Crop computation ---
		cropW, cropH, originY := computeExtentCropRegion(frames, sourceW, sourceH)
		segments := computeHybridSegments(frames, sourceW, cropW, trimStartMs)

		r.cropW = cropW
		r.cropH = cropH
		r.originY = originY
		r.segments = segments

		// Determine static X.
		cropX := 0
		if len(segments) > 0 {
			cropX = segments[0].StartX
		} else {
			rawCX := make([]float64, len(frames))
			for i, f := range frames {
				rawCX[i] = ((f.BoundingBox.Left + f.BoundingBox.Right) / 2) * float64(sourceW)
			}
			cropX = int(math.Round(median(rawCX))) - cropW/2
			if cropX < 0 {
				cropX = 0
			}
			if cropX+cropW > sourceW {
				cropX = sourceW - cropW
			}
		}
		r.cropX = cropX

		// Check if dynamic crop needed.
		isDynamic := false
		if len(segments) > 1 {
			firstX := segments[0].StartX
			for _, seg := range segments {
				if seg.StartX != firstX || seg.EndX != firstX {
					isDynamic = true
					break
				}
			}
		}
		r.isDynamic = isDynamic
		r.walkPct = computeWalkingPercent(frames, sourceW)

		t.Logf("  crop: %dx%d at (%d,%d)  dynamic=%v  segs=%d  walk=%.0f%%",
			cropW, cropH, cropX, originY, isDynamic, len(segments), r.walkPct*100)

		for i, seg := range segments {
			segType := "lock"
			if seg.StartX != seg.EndX {
				segType = "pan"
			}
			t.Logf("    seg[%d]: %.2f-%.2fs  x=%d→%d  (%s)", i,
				seg.StartSec, seg.EndSec, seg.StartX, seg.EndX, segType)
		}

		// --- Crop video ---
		croppedPath := filepath.Join(outDir, "cropped.mp4")
		if isDynamic {
			xExpr := buildCropXExpr(segments)
			if err := ffmpeg.CropVideoExpr(ctx, trimmedPath, croppedPath, xExpr, originY, cropW, cropH); err != nil {
				t.Errorf("%s: ffmpeg crop: %v", p.name, err)
				continue
			}
		} else {
			if err := ffmpeg.CropVideo(ctx, trimmedPath, croppedPath, cropX, originY, cropW, cropH); err != nil {
				t.Errorf("%s: ffmpeg crop: %v", p.name, err)
				continue
			}
		}
		r.croppedRel = filepath.Join(p.name, "cropped.mp4")

		results = append(results, r)
	}

	// Generate HTML report.
	reportPath := filepath.Join(outputRoot, "report.html")
	writeRigReport(t, results, reportPath, outputRoot)
	t.Logf("\nReport: %s", reportPath)

	if os.Getenv("OPEN_REPORT") == "1" {
		exec.Command("open", reportPath).Run()
	}
}

func writeRigReport(t *testing.T, results []rigResult, reportPath, outputRoot string) {
	t.Helper()

	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>Go Trim+Crop Rig</title>
<style>
  body { font-family: -apple-system, system-ui, sans-serif; margin: 20px; background: #1a1a1a; color: #eee; }
  h1 { border-bottom: 2px solid #444; padding-bottom: 10px; }
  h2 { margin-top: 40px; color: #88ccff; }
  h3 { color: #ccc; margin: 0 0 8px 0; }
  table { border-collapse: collapse; width: 100%; margin: 16px 0; }
  th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #444; font-size: 14px; }
  th { color: #88ccff; }
  .good { color: #4ade80; }
  .warn { color: #fbbf24; }
  .bad { color: #f87171; }
  .muted { color: #888; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 16px; margin: 16px 0; }
  .card { background: #2a2a2a; border-radius: 8px; padding: 12px; }
  .card video { width: 100%; border-radius: 4px; background: #000; }
  .card .meta { font-size: 12px; color: #888; margin-top: 8px; line-height: 1.6; }
  .seg-table { font-size: 12px; margin-top: 8px; }
  .seg-table td, .seg-table th { padding: 4px 8px; }
  .tag { display: inline-block; padding: 2px 6px; border-radius: 3px; font-size: 11px; font-weight: 600; }
  .tag-lock { background: #2d5a2d; color: #4ade80; }
  .tag-pan { background: #4a3d8f; color: #c4b5fd; }
  .tag-static { background: #444; color: #ccc; }
  .tag-dynamic { background: #4a3d8f; color: #c4b5fd; }
  .controls { margin: 16px 0; }
  .controls button {
    background: #444; color: #eee; border: none; padding: 8px 16px;
    border-radius: 4px; cursor: pointer; margin-right: 8px; font-size: 14px;
  }
  .controls button:hover { background: #555; }
  nav { margin: 16px 0; }
  nav a { color: #88ccff; margin-right: 16px; text-decoration: none; }
  nav a:hover { text-decoration: underline; }
</style>
</head><body>
<h1>Go Trim+Crop Rig</h1>
<div class="controls">
  <button onclick="playAll()">Play All</button>
  <button onclick="pauseAll()">Pause All</button>
  <button onclick="restartAll()">Restart All</button>
</div>
`)

	// Navigation.
	b.WriteString("<nav>")
	for _, r := range results {
		fmt.Fprintf(&b, `<a href="#%s">%s</a>`, r.name, r.name)
	}
	b.WriteString("</nav>\n")

	// Summary table.
	b.WriteString(`<table>
<tr>
  <th>Video</th>
  <th>Go Raw</th>
  <th>Go Padded</th>
  <th>Py Padded</th>
  <th>&Delta;Start</th>
  <th>&Delta;End</th>
  <th>Crop</th>
  <th>Mode</th>
  <th>Walk%</th>
</tr>
`)
	for _, r := range results {
		if !r.confident {
			fmt.Fprintf(&b, `<tr><td><b>%s</b></td><td class="bad">NOT CONFIDENT</td>`+
				`<td>-</td><td>%.1f-%.1fs</td><td>-</td><td>-</td><td>-</td><td>-</td><td>-</td></tr>`+"\n",
				r.name, r.pyStart, r.pyEnd)
			continue
		}

		// Compare padded Go vs padded Python (apples-to-apples).
		deltaStart := r.goPadStart - r.pyStart
		deltaEnd := r.goPadEnd - r.pyEnd
		startClass := deltaClass(deltaStart, 0.5, 1.5)
		endClass := deltaClass(deltaEnd, 0.5, 1.5)

		modeTag := `<span class="tag tag-static">static</span>`
		if r.isDynamic {
			modeTag = `<span class="tag tag-dynamic">dynamic</span>`
		}

		fmt.Fprintf(&b, `<tr>
  <td><b>%s</b></td>
  <td>%.2f-%.2fs (%.1fs)</td>
  <td class="muted">%.2f-%.2fs (%.1fs)</td>
  <td>%.2f-%.2fs (%.1fs)</td>
  <td class="%s">%+.2fs</td>
  <td class="%s">%+.2fs</td>
  <td>%d&times;%d</td>
  <td>%s</td>
  <td>%.0f%%</td>
</tr>
`,
			r.name,
			r.goStart, r.goEnd, r.goEnd-r.goStart,
			r.goPadStart, r.goPadEnd, r.goPadEnd-r.goPadStart,
			r.pyStart, r.pyEnd, r.pyEnd-r.pyStart,
			startClass, deltaStart,
			endClass, deltaEnd,
			r.cropW, r.cropH,
			modeTag,
			r.walkPct*100,
		)
	}
	b.WriteString("</table>\n")

	// Per-video sections.
	for _, r := range results {
		fmt.Fprintf(&b, `<h2 id="%s">%s</h2>`+"\n", r.name, r.name)

		if !r.confident {
			b.WriteString(`<p class="bad">Trim detection was not confident. No output produced.</p>` + "\n")
			continue
		}

		// Video players.
		b.WriteString(`<div class="grid">` + "\n")

		// Trimmed video card.
		if r.trimmedRel != "" {
			fmt.Fprintf(&b, `<div class="card">
  <h3>Trimmed</h3>
  <video class="rig-video" src="%s" controls loop muted playsinline></video>
  <div class="meta">
    Go raw: %.2fs &ndash; %.2fs (%.1fs)<br>
    Go pad: %.2fs &ndash; %.2fs (%.1fs)<br>
    Py pad: %.2fs &ndash; %.2fs (%.1fs)<br>
    &Delta;start: %+.2fs &middot; &Delta;end: %+.2fs
  </div>
</div>
`,
				r.trimmedRel,
				r.goStart, r.goEnd, r.goEnd-r.goStart,
				r.goPadStart, r.goPadEnd, r.goPadEnd-r.goPadStart,
				r.pyStart, r.pyEnd, r.pyEnd-r.pyStart,
				r.goPadStart-r.pyStart, r.goPadEnd-r.pyEnd,
			)
		}

		// Cropped video card.
		if r.croppedRel != "" {
			modeLabel := "static"
			if r.isDynamic {
				modeLabel = "dynamic"
			}
			fmt.Fprintf(&b, `<div class="card">
  <h3>Cropped (9:16 hybrid)</h3>
  <video class="rig-video" src="%s" controls loop muted playsinline></video>
  <div class="meta">
    %d&times;%d at (%d, %d) &middot; %s &middot; %.0f%% walking
  </div>
</div>
`,
				r.croppedRel,
				r.cropW, r.cropH, r.cropX, r.originY,
				modeLabel, r.walkPct*100,
			)
		}
		b.WriteString("</div>\n")

		// Segment breakdown.
		if len(r.segments) > 0 {
			b.WriteString(`<details><summary>Segments (` + fmt.Sprintf("%d", len(r.segments)) + `)</summary>`)
			b.WriteString(`<table class="seg-table">`)
			b.WriteString(`<tr><th>#</th><th>Time</th><th>X Start</th><th>X End</th><th>Type</th></tr>` + "\n")
			for i, seg := range r.segments {
				segType := "lock"
				tagClass := "tag-lock"
				if seg.StartX != seg.EndX {
					segType = "pan"
					tagClass = "tag-pan"
				}
				fmt.Fprintf(&b, `<tr><td>%d</td><td>%.2f-%.2fs</td><td>%d</td><td>%d</td>`+
					`<td><span class="tag %s">%s</span></td></tr>`+"\n",
					i, seg.StartSec, seg.EndSec, seg.StartX, seg.EndX, tagClass, segType)
			}
			b.WriteString("</table></details>\n")
		}
	}

	// JavaScript.
	b.WriteString(`
<script>
function playAll() { document.querySelectorAll('.rig-video').forEach(v => v.play()); }
function pauseAll() { document.querySelectorAll('.rig-video').forEach(v => v.pause()); }
function restartAll() {
  document.querySelectorAll('.rig-video').forEach(v => { v.currentTime = 0; v.play(); });
}
</script>
</body></html>`)

	if err := os.WriteFile(reportPath, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

func deltaClass(delta, warnThresh, badThresh float64) string {
	abs := math.Abs(delta)
	if abs <= warnThresh {
		return "good"
	}
	if abs <= badThresh {
		return "warn"
	}
	return "bad"
}
