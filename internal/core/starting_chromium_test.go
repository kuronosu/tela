package core

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Entry transitions from the starting: variant are compared with
// Chromium's @starting-style. The page testdata/chromium/starting/
// starting.html has one empty cell per case; a script inserts every box at
// once (the boxes' first style change event: the transitions start from
// their @starting-style), pauses every animation of the document and
// sets its current time to the t of the URL (Web Animations API), as the
// transition oracle of anim_chromium_test.go does. Chromium's
// screenshots at four times are committed (t100.png ... t750.png); the
// test mounts the same boxes in Tela's headless host with a controlled
// clock and compares them at the same times. Regenerate with
//
//	TELA_CHROMIUM=1 go test -run TestChromiumStarting ./internal/core/
//
// which needs chromium in PATH; TELA_CHROMIUM_DUMP=dir writes Tela's
// renders there.

const (
	twEaseOutCSS   = "cubic-bezier(0, 0, .2, 1)"
	twEaseInOutCSS = "cubic-bezier(.4, 0, .2, 1)"
)

// startingCells are the cases: base is the Tela class string, css the
// page's rules for .c-<name> including its @starting-style block.
var startingCells = []animCell{
	{
		name: "fade", base: "size-16 rounded-xl bg-indigo-600 transition-opacity duration-400 ease-out starting:opacity-0",
		css: ".c-fade { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(51.1% .262 276.966); transition: opacity .4s " + twEaseOutCSS + "; }\n" +
			"@starting-style { .c-fade { opacity: 0; } }",
		bound: chromiumBound{max: 40, mean: 0.4, why: "anti-aliased corners, 8-bit rounding of the composited layer"},
	},
	{
		name: "pop", base: "size-16 rounded-xl bg-emerald-500 transition duration-400 ease-out opacity-100 scale-100 starting:opacity-0 starting:scale-75",
		css: ".c-pop { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(69.6% .17 162.48); opacity: 1; scale: 1; " +
			"transition: opacity .4s " + twEaseOutCSS + ", scale .4s " + twEaseOutCSS + "; }\n" +
			"@starting-style { .c-pop { opacity: 0; scale: .75; } }",
		bound: chromiumBound{max: 70, mean: 0.6, why: "anti-aliased edges of a scaled, translucent box"},
	},
	{
		name: "slide", base: "size-16 rounded-xl bg-rose-500 transition-transform duration-400 ease-in-out starting:translate-y-8 starting:rotate-12",
		css: ".c-slide { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(64.5% .246 16.439); " +
			"transition: transform .4s " + twEaseInOutCSS + ", translate .4s " + twEaseInOutCSS + ", scale .4s " + twEaseInOutCSS + ", rotate .4s " + twEaseInOutCSS + "; }\n" +
			"@starting-style { .c-slide { translate: 0 2rem; rotate: 12deg; } }",
		bound: chromiumBound{max: 70, mean: 0.3, why: "anti-aliased edges of a rotated box"},
	},
	{
		name: "color", base: "size-16 rounded-xl bg-blue-500 transition-colors duration-400 ease-in-out starting:bg-amber-300",
		css: ".c-color { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(62.3% .214 259.815); " +
			"transition: color .4s " + twEaseInOutCSS + ", background-color .4s " + twEaseInOutCSS + ", border-color .4s " + twEaseInOutCSS + "; }\n" +
			"@starting-style { .c-color { background-color: oklch(87.9% .169 91.605); } }",
		bound: chromiumBound{max: 40, mean: 0.1, why: "anti-aliased corners"},
	},
	{
		name: "width", base: "h-16 w-24 rounded-md bg-violet-500 transition-[width] duration-600 ease-in-out starting:w-4",
		css: ".c-width { width: 6rem; height: 4rem; border-radius: .375rem; background-color: oklch(60.6% .25 292.717); transition: width .6s " + twEaseInOutCSS + "; }\n" +
			"@starting-style { .c-width { width: 1rem; } }",
		bound: chromiumBound{max: 40, mean: 0.1, why: "anti-aliased corners"},
	},
	{
		name: "delay", base: "size-16 rounded-full bg-sky-500 transition-opacity duration-300 delay-150 ease-linear starting:opacity-0",
		css: ".c-delay { width: 4rem; height: 4rem; border-radius: 9999px; background-color: oklch(68.5% .169 237.323); transition: opacity .3s linear .15s; }\n" +
			"@starting-style { .c-delay { opacity: 0; } }",
		bound: chromiumBound{max: 80, mean: 0.3, why: "anti-aliased circle edge, 8-bit layer rounding"},
	},
	{
		name: "blur", base: "size-16 rounded-xl bg-fuchsia-500 transition-[filter] duration-400 ease-in-out starting:blur-lg",
		css: ".c-blur { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(66.7% .295 322.15); transition: filter .4s " + twEaseInOutCSS + "; }\n" +
			"@starting-style { .c-blur { filter: blur(16px); } }",
		bound: chromiumBound{max: 40, mean: 0.4, why: "Skia's Gaussian blur against our three box blurs, anti-aliased corners"},
	},
	{
		name: "after", base: "size-16 rounded-xl bg-slate-200 opacity-100 starting:opacity-0",
		css: ".c-after { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(92.9% .013 255.508); opacity: 1; }\n" +
			"@starting-style { .c-after { opacity: 0; } }",
		bound: chromiumBound{max: 20, mean: 0.1, why: "no transition: the starting style never shows"},
	},
}

func startingOracleSize() (w, h int) {
	rows := (len(startingCells) + animGridColumns - 1) / animGridColumns
	return animGridColumns * animCellSize, rows * animCellSize
}

func startingOracleHTML() string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Tela starting-style oracle</title>
<style>
*, ::before, ::after { box-sizing: border-box; border: 0 solid; margin: 0; padding: 0; }
html, body { background: #fff; }
.grid { display: grid; grid-template-columns: repeat(4, 120px); }
.cell { width: 120px; height: 120px; display: flex; align-items: center; justify-content: center; }
`)
	for _, c := range startingCells {
		sb.WriteString(c.css + "\n")
	}
	sb.WriteString("</style></head><body>\n<div class=\"grid\">\n")
	for _, c := range startingCells {
		fmt.Fprintf(&sb, "<div class=\"cell\" data-name=\"%s\"></div>\n", c.name)
	}
	sb.WriteString(`</div>
<script>
// Insert every box (its first style change event starts its transitions
// from its @starting-style), pause every animation and set its current
// time to ?t= (ms).
const t = +new URLSearchParams(location.search).get("t");
document.body.offsetWidth;
for (const c of document.querySelectorAll(".cell")) {
  const b = document.createElement("div");
  b.className = "box c-" + c.dataset.name;
  c.appendChild(b);
}
document.body.offsetWidth; // the style change event: transitions are created
for (const a of document.getAnimations()) {
  a.pause();
  a.currentTime = t;
}
</script>
</body></html>
`)
	return sb.String()
}

func regenerateStartingOracle(t *testing.T, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "starting.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(startingOracleHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := startingOracleSize()
	for _, at := range animOracleTimes {
		shot, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("t%d.png", at.Milliseconds())))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--hide-scrollbars",
			"--force-color-profile=srgb", "--force-device-scale-factor=1",
			fmt.Sprintf("--window-size=%d,%d", w, h), "--screenshot="+shot,
			fmt.Sprintf("file://%s?t=%d", htmlPath, at.Milliseconds()))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("chromium: %v\n%s", err, out)
		}
	}
}

// renderStartingOracle mounts the boxes in a headless window and returns
// its pixels at the given time after the frame that mounted them.
func renderStartingOracle(t *testing.T, at time.Duration) *image.RGBA {
	t.Helper()
	w, h := startingOracleSize()
	b := headless.New(headless.Options{Size: platform.Size{W: float32(w), H: float32(h)}})
	var set Setter[bool]
	root := Component(func() Node {
		on, s := UseState(false)
		set = s
		cells := []Child{Class("flex w-[480px] flex-wrap")}
		for _, c := range startingCells {
			cells = append(cells, div(Class("flex size-[120px] shrink-0 items-center justify-center"),
				If(on, div(Key(c.name), Class(c.base)))))
		}
		return div(cells...)
	})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts, RootClass: "bg-white"}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopTimer)
	win := host.Window().(*headless.Window)
	b.Drain(host)
	for range 4 {
		b.Advance(b.FrameInterval())
		b.Drain(host)
	}
	set.Set(true)
	b.Drain(host)
	var start time.Duration
	for range 4 {
		if el := keyed(host.Tree(), "fade"); el != nil && el.box.anim != nil && len(el.box.anim.trans) > 0 {
			start = el.box.anim.trans[0].Start
			break
		}
		b.Advance(b.FrameInterval())
		b.Drain(host)
	}
	if start == 0 {
		t.Fatal("the entry transitions did not start")
	}
	b.Advance(start + at - b.Now())
	b.Drain(host)
	if got := host.Tree().anim.tk.Now(); got != start+at {
		t.Fatalf("the frame ran at %v, want %v", got, start+at)
	}
	return win.Frame()
}

func TestChromiumStarting(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "starting")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateStartingOracle(t, dir)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "starting.html")); err != nil || string(h) != startingOracleHTML() {
		t.Fatal("testdata/chromium/starting/starting.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	report.WriteString("case          time   max   mean  >16\n")
	for _, at := range animOracleTimes {
		want := readRGBA(t, filepath.Join(dir, fmt.Sprintf("t%d.png", at.Milliseconds())))
		got := renderStartingOracle(t, at)
		if got.Rect != want.Rect {
			t.Fatalf("t=%v: render %v, Chromium %v", at, got.Rect, want.Rect)
		}
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("starting_t%d_tela.png", at.Milliseconds())), got)
		}
		for i, c := range startingCells {
			col, row := i%animGridColumns, i/animGridColumns
			d := diffRegion(got, want, image.Rect(col*animCellSize, row*animCellSize, (col+1)*animCellSize, (row+1)*animCellSize))
			fmt.Fprintf(&report, "%-13s %4dms %4d %6.3f %4d\n", c.name, at.Milliseconds(), d.max, d.mean, d.over)
			if d.max > c.bound.max || d.mean > c.bound.mean {
				t.Errorf("%s at %v: max %d mean %.3f, want at most %d and %.2f", c.name, at, d.max, d.mean, c.bound.max, c.bound.mean)
			}
		}
	}
	t.Log("difference from Chromium per case (channel levels):\n" + report.String())
}
