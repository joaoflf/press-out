#!/usr/bin/env python3
"""Validation report generator — creates an HTML page for human review of crop/trim strategies.

Scans output directories from crop_rig.py and trim_rig.py, generates a self-contained
HTML page with all screenshots, strategy parameters, and side-by-side comparisons.

Usage:
    uv run spikes/crop-rig/validate.py [output_dirs...] [--open]

    # Auto-discover all outputs
    uv run spikes/crop-rig/validate.py

    # Specific directories
    uv run spikes/crop-rig/validate.py spikes/crop-rig/output/sample-snatch-1

    # Auto-open in browser
    uv run spikes/crop-rig/validate.py --open
"""

import argparse
import base64
import glob
import json
import os
import sys
import subprocess


OUTPUT_ROOT = os.path.join("spikes", "crop-rig", "output")


def find_output_dirs(root: str) -> list[str]:
    """Find all output directories that contain a summary.json."""
    dirs = []
    for dirpath, _, filenames in os.walk(root):
        if "summary.json" in filenames:
            dirs.append(dirpath)
    dirs.sort()
    return dirs


def load_summary(output_dir: str) -> dict:
    with open(os.path.join(output_dir, "summary.json")) as f:
        return json.load(f)


def img_to_data_uri(path: str) -> str:
    """Convert an image file to a base64 data URI for embedding in HTML."""
    ext = os.path.splitext(path)[1].lower()
    mime = {"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg"}.get(ext.lstrip("."), "image/png")
    with open(path, "rb") as f:
        data = base64.b64encode(f.read()).decode()
    return f"data:{mime};base64,{data}"


def is_trim_section(summary: dict) -> bool:
    """Check if this summary contains trim results (vs crop results)."""
    strategies = summary.get("strategies", {})
    for params in strategies.values():
        if "start_sec" in params or "reason" in params:
            return True
    return False


def render_cross_video_table(summaries: list[tuple[str, dict]]) -> str:
    """Render a cross-video comparison table showing all strategies x all videos."""
    # Collect all strategy names across all summaries
    all_strategies: list[str] = []
    for _, summary in summaries:
        for name in summary.get("strategies", {}):
            if name not in all_strategies:
                all_strategies.append(name)

    html = []
    html.append('<section class="section" id="cross-video">')
    html.append('<h2>Cross-Video Strategy Comparison</h2>')
    html.append('<table class="summary-table">')
    html.append('<tr><th>Video</th><th>Duration</th>')
    for s in all_strategies:
        html.append(f'<th>{s}</th>')
    html.append('</tr>')

    for dir_name, summary in summaries:
        video_label = summary.get("video_label", dir_name)
        html.append(f'<tr><td><a href="#{dir_name}">{video_label}</a></td>')
        html.append(f'<td>{summary.get("duration_sec", "?")}s</td>')
        strategies = summary.get("strategies", {})
        for s in all_strategies:
            params = strategies.get(s)
            if params is None:
                html.append('<td style="color:#555">—</td>')
            elif params.get("confident"):
                start = params["start_sec"]
                end = params["end_sec"]
                dur = params["duration_sec"]
                html.append(f'<td class="pass">{start:.1f}–{end:.1f}s ({dur:.0f}s)</td>')
            else:
                reason = params.get("reason", "failed")
                # Truncate long reasons
                if len(reason) > 30:
                    reason = reason[:27] + "..."
                html.append(f'<td class="fail" title="{params.get("reason", "")}">{reason}</td>')
        html.append('</tr>')

    html.append('</table>')
    html.append('</section>')
    return "\n".join(html)


def render_section(output_dir: str, summary: dict) -> str:
    """Render one output directory as an HTML section."""
    dir_name = os.path.basename(output_dir)
    video_label = summary.get("video_label", dir_name)
    is_trim = is_trim_section(summary)

    html = []
    html.append(f'<section class="section" id="{dir_name}">')
    html.append(f'<h2>{video_label}</h2>')
    html.append(f'<p style="color:#888; margin-bottom:1rem;">'
                f'{summary.get("fps", "?")} fps, {summary.get("duration_sec", "?")}s total</p>')

    # Strategy summary table
    html.append('<table class="summary-table">')
    html.append('<tr><th>Strategy</th>')
    if is_trim:
        html.append('<th>Start</th><th>End</th><th>Duration</th><th>Confident</th>')
    else:
        html.append('<th>X</th><th>Y</th><th>W</th><th>H</th>')
    html.append('</tr>')

    strategies = summary.get("strategies", {})
    for name, params in strategies.items():
        html.append(f'<tr><td><strong>{name}</strong></td>')
        if is_trim:
            if params.get("confident"):
                html.append(f'<td>{params["start_sec"]:.2f}s</td>')
                html.append(f'<td>{params["end_sec"]:.2f}s</td>')
                html.append(f'<td>{params["duration_sec"]:.1f}s</td>')
                html.append('<td class="pass">YES</td>')
            else:
                html.append(f'<td colspan="3">{params.get("reason", "—")}</td>')
                html.append('<td class="fail">NO</td>')
        else:
            html.append(f'<td>{params.get("x", "?")}</td>')
            html.append(f'<td>{params.get("y", "?")}</td>')
            html.append(f'<td>{params.get("w", "?")}</td>')
            html.append(f'<td>{params.get("h", "?")}</td>')
        html.append('</tr>')
    html.append('</table>')

    # Images
    image_files = sorted(glob.glob(os.path.join(output_dir, "*.png")))
    for img_path in image_files:
        filename = os.path.basename(img_path)
        data_uri = img_to_data_uri(img_path)
        html.append(f'<div class="image-block">')
        html.append(f'<h3>{filename}</h3>')
        html.append(f'<img src="{data_uri}" alt="{filename}" loading="lazy">')
        html.append(f'</div>')

    html.append('</section>')
    return "\n".join(html)


def generate_html(output_dirs: list[str]) -> str:
    """Generate the full HTML validation report."""
    # Load all summaries
    loaded: list[tuple[str, str, dict]] = []  # (dir_name, output_dir, summary)
    for output_dir in output_dirs:
        summary = load_summary(output_dir)
        dir_name = os.path.basename(output_dir)
        loaded.append((dir_name, output_dir, summary))

    # Build nav
    nav_items = ['<a href="#cross-video">Overview</a>']
    for dir_name, _, summary in loaded:
        video_label = summary.get("video_label", dir_name)
        nav_items.append(f'<a href="#{dir_name}">{video_label}</a>')
    nav_html = " | ".join(nav_items)

    # Cross-video table
    cross_table = render_cross_video_table([(dn, s) for dn, _, s in loaded])

    # Per-video sections
    sections = []
    for _, output_dir, summary in loaded:
        sections.append(render_section(output_dir, summary))

    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Press-Out — Strategy Validation</title>
<style>
  * {{ margin: 0; padding: 0; box-sizing: border-box; }}
  body {{
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    background: #1a1a2e; color: #e0e0e0; padding: 1rem;
  }}
  h1 {{ text-align: center; margin: 1rem 0; color: #fff; }}
  nav {{
    text-align: center; margin-bottom: 2rem; padding: 0.5rem;
    background: #16213e; border-radius: 8px;
  }}
  nav a {{ color: #4fc3f7; margin: 0 0.5rem; text-decoration: none; }}
  nav a:hover {{ text-decoration: underline; }}
  .section {{
    background: #16213e; border-radius: 8px; padding: 1.5rem;
    margin-bottom: 2rem;
  }}
  .section h2 {{
    color: #4fc3f7; margin-bottom: 1rem; padding-bottom: 0.5rem;
    border-bottom: 1px solid #2a2a4a;
  }}
  .summary-table {{
    width: 100%; border-collapse: collapse; margin-bottom: 1.5rem;
  }}
  .summary-table th, .summary-table td {{
    padding: 0.5rem 1rem; text-align: left; border-bottom: 1px solid #2a2a4a;
  }}
  .summary-table th {{ color: #90caf9; }}
  .pass {{ color: #66bb6a; font-weight: bold; }}
  .fail {{ color: #ef5350; font-weight: bold; }}
  .image-block {{ margin-bottom: 1.5rem; }}
  .image-block h3 {{ color: #aaa; font-size: 0.9rem; margin-bottom: 0.5rem; }}
  .image-block img {{
    max-width: 100%; border-radius: 4px; border: 1px solid #2a2a4a;
  }}
</style>
</head>
<body>
<h1>Trim Strategy Validation Report</h1>
<nav>{nav_html}</nav>
{cross_table}
{"".join(sections)}
<footer style="text-align:center; color:#666; margin-top:2rem; padding:1rem;">
  Generated by spikes/crop-rig/validate.py
</footer>
</body>
</html>"""


def main():
    parser = argparse.ArgumentParser(description="Generate validation HTML report")
    parser.add_argument("dirs", nargs="*", help="Output directories to include (auto-discovers if none)")
    parser.add_argument("--output", default=os.path.join(OUTPUT_ROOT, "validation.html"),
                        help="Output HTML path")
    parser.add_argument("--open", action="store_true", help="Open in browser after generating")
    args = parser.parse_args()

    if args.dirs:
        output_dirs = args.dirs
    else:
        output_dirs = find_output_dirs(OUTPUT_ROOT)

    if not output_dirs:
        print("No output directories found. Run crop_rig.py or trim_rig.py first.", file=sys.stderr)
        sys.exit(1)

    print(f"Found {len(output_dirs)} output directories:", file=sys.stderr)
    for d in output_dirs:
        print(f"  {d}", file=sys.stderr)

    html = generate_html(output_dirs)

    os.makedirs(os.path.dirname(args.output), exist_ok=True)
    with open(args.output, "w") as f:
        f.write(html)

    print(f"\nReport: {args.output}", file=sys.stderr)

    if args.open:
        subprocess.run(["open", args.output])


if __name__ == "__main__":
    main()
