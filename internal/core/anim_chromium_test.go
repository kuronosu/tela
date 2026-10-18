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

// Transitions and keyframe animations are compared with Chromium's. The
// page testdata/chromium/anim/anim.html has one cell per case, written in
// the CSS Tailwind v4 compiles its classes to. A script puts every cell in
// its "on" state, which starts the transitions and the animate-*
// animations at the same instant, pauses all the animations of the
// document and sets their current time (Web Animations API:
// document.getAnimations(), pause(), currentTime) to the t of the URL.
// Chromium's screenshots at four times are committed
// (t100.png ... t750.png); the test renders the same cells in Tela's
// headless host with a controlled clock at the same times and compares
// them. Regenerate the page and the screenshots with
//
//	TELA_CHROMIUM=1 go test -run TestChromiumAnimations ./internal/core/
//
// which needs chromium in PATH. Set TELA_CHROMIUM_DUMP to a directory to
// get Tela's renders there.

// animCell is one case of the page.
type animCell struct {
	name string
	// base are the Tela classes of the box in both states; off and on are
	// added in each state.
	base, off, on string
	// css is the page's CSS for the box in both states: the rules for
	// .c-<name> and .c-<name>.on.
	css string
	// bound is the largest difference from Chromium accepted.
	bound chromiumBound
}

const (
	oklchBlue500    = "oklch(62.3% .214 259.815)"
	oklchBlue600    = "oklch(54.6% .245 262.881)"
	oklchRed500     = "oklch(63.7% .237 25.331)"
	oklchGray200    = "oklch(92.8% .006 264.531)"
	oklchGray300    = "oklch(87.2% .01 258.338)"
	twEaseInOut     = "cubic-bezier(.4, 0, .2, 1)"
	twEaseOut       = "cubic-bezier(0, 0, .2, 1)"
	animCellSize    = 120
	animGridColumns = 4
)

var animCells = []animCell{
	{
		name: "oklch", base: "size-16 rounded-xl transition-colors duration-400 ease-in-out", off: "bg-blue-500", on: "bg-red-500",
		css: ".c-oklch { width: 4rem; height: 4rem; border-radius: .75rem; background-color: " + oklchBlue500 +
			"; transition: color .4s " + twEaseInOut + ", background-color .4s " + twEaseInOut + ", border-color .4s " + twEaseInOut + "; }\n" +
			".c-oklch.on { background-color: " + oklchRed500 + "; }",
		bound: chromiumBound{max: 40, mean: 0.1, why: "anti-aliased corners"},
	},
	{
		name: "hex", base: "size-16 rounded-xl transition-colors duration-400 ease-in-out", off: "bg-[#fde047]", on: "bg-[#1e3a8a]",
		css: ".c-hex { width: 4rem; height: 4rem; border-radius: .75rem; background-color: #fde047; transition: background-color .4s " + twEaseInOut + "; }\n" +
			".c-hex.on { background-color: #1e3a8a; }",
		bound: chromiumBound{max: 40, mean: 0.1, why: "anti-aliased corners"},
	},
	{
		name: "white-black", base: "size-16 rounded-xl border border-gray-300 transition-colors duration-400 ease-in-out", off: "bg-white", on: "bg-black",
		css: ".c-white-black { width: 4rem; height: 4rem; border-radius: .75rem; border: 1px solid " + oklchGray300 +
			"; background-color: #fff; transition: background-color .4s " + twEaseInOut + "; }\n" +
			".c-white-black.on { background-color: #000; }",
		bound: chromiumBound{max: 60, mean: 0.3, why: "the 1px border meets the black fill at the corners"},
	},
	{
		name: "opacity", base: "size-16 rounded-xl bg-indigo-600 transition-opacity duration-400 ease-out", off: "opacity-100", on: "opacity-20",
		css: ".c-opacity { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(51.1% .262 276.966); opacity: 1; transition: opacity .4s " + twEaseOut + "; }\n" +
			".c-opacity.on { opacity: .2; }",
		bound: chromiumBound{max: 20, mean: 0.4, why: "8-bit rounding of the composited layer"},
	},
	{
		name: "transform", base: "size-16 rounded-xl bg-emerald-500 transition-transform duration-400 ease-in-out", off: "", on: "translate-x-3 -translate-y-2 scale-125 rotate-45",
		css: ".c-transform { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(69.6% .17 162.48); " +
			"transition: transform .4s " + twEaseInOut + ", translate .4s " + twEaseInOut + ", scale .4s " + twEaseInOut + ", rotate .4s " + twEaseInOut + "; }\n" +
			".c-transform.on { translate: 12px -8px; scale: 1.25; rotate: 45deg; }",
		bound: chromiumBound{max: 80, mean: 0.6, why: "anti-aliased edges of a rotated box"},
	},
	{
		name: "shadow", base: "size-16 rounded-xl border border-gray-200 bg-white transition-shadow duration-400 ease-in-out", off: "shadow-sm", on: "shadow-xl ring-2 ring-blue-500",
		css: ".c-shadow { width: 4rem; height: 4rem; border-radius: .75rem; border: 1px solid " + oklchGray200 + "; background-color: #fff; " +
			"box-shadow: 0 0 #0000, 0 0 #0000, 0 0 #0000, 0 0 #0000, 0 1px 3px 0 rgb(0 0 0 / .1), 0 1px 2px -1px rgb(0 0 0 / .1); " +
			"transition: box-shadow .4s " + twEaseInOut + "; }\n" +
			".c-shadow.on { box-shadow: 0 0 #0000, 0 0 #0000, 0 0 0 0 #fff, 0 0 0 2px " + oklchBlue500 +
			", 0 20px 25px -5px rgb(0 0 0 / .1), 0 8px 10px -6px rgb(0 0 0 / .1); }",
		bound: chromiumBound{max: 70, mean: 0.5, why: "Skia's shadow blur and the ring's anti-aliasing"},
	},
	{
		name: "width", base: "h-16 rounded-md bg-violet-500 transition-[width] duration-600 ease-in-out", off: "w-10", on: "w-24",
		css: ".c-width { width: 2.5rem; height: 4rem; border-radius: .375rem; background-color: oklch(60.6% .25 292.717); transition: width .6s " + twEaseInOut + "; }\n" +
			".c-width.on { width: 6rem; }",
		bound: chromiumBound{max: 40, mean: 0.1, why: "anti-aliased corners"},
	},
	{
		name: "blur", base: "size-16 rounded-xl bg-rose-500 transition-[filter] duration-400 ease-in-out", off: "", on: "blur-md",
		css: ".c-blur { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(64.5% .246 16.439); transition: filter .4s " + twEaseInOut + "; }\n" +
			".c-blur.on { filter: blur(12px); }",
		bound: chromiumBound{max: 10, mean: 0.25, why: "Skia's Gaussian blur against our three box blurs"},
	},
	{
		name: "spin", base: "size-12 rounded-full border-4 border-gray-300 border-t-blue-600", off: "", on: "animate-spin",
		css: ".c-spin { width: 3rem; height: 3rem; border-radius: 9999px; border: 4px solid " + oklchGray300 + "; border-top-color: " + oklchBlue600 + "; }\n" +
			".c-spin.on { animation: spin 1s linear infinite; }",
		bound: chromiumBound{max: 80, mean: 0.4, why: "anti-aliased arcs of the rotated ring"},
	},
	{
		name: "ping", base: "size-12 rounded-full bg-sky-400 opacity-75", off: "", on: "animate-ping",
		css: ".c-ping { width: 3rem; height: 3rem; border-radius: 9999px; background-color: oklch(74.6% .16 232.661); opacity: .75; }\n" +
			".c-ping.on { animation: ping 1s cubic-bezier(0, 0, .2, 1) infinite; }",
		bound: chromiumBound{max: 40, mean: 0.3, why: "anti-aliased circle edge under scale"},
	},
	{
		name: "pulse", base: "size-16 rounded-xl bg-slate-400", off: "", on: "animate-pulse",
		css: ".c-pulse { width: 4rem; height: 4rem; border-radius: .75rem; background-color: oklch(70.4% .04 256.788); }\n" +
			".c-pulse.on { animation: pulse 2s cubic-bezier(.4, 0, .6, 1) infinite; }",
		bound: chromiumBound{max: 25, mean: 0.4, why: "8-bit rounding of the composited layer"},
	},
	{
		name: "bounce", base: "size-12 rounded-full bg-amber-400", off: "", on: "animate-bounce",
		css: ".c-bounce { width: 3rem; height: 3rem; border-radius: 9999px; background-color: oklch(82.8% .189 84.429); }\n" +
			".c-bounce.on { animation: bounce 1s infinite; }",
		bound: chromiumBound{max: 80, mean: 0.3, why: "anti-aliased circle edge"},
	},
}

// animOracleTimes are the times, since the start of the animations, the
// page and the test are compared at.
var animOracleTimes = []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond}

func animOracleSize() (w, h int) {
	rows := (len(animCells) + animGridColumns - 1) / animGridColumns
	return animGridColumns * animCellSize, rows * animCellSize
}

func animOracleHTML() string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Tela animation oracle</title>
<style>
*, ::before, ::after { box-sizing: border-box; border: 0 solid; margin: 0; padding: 0; }
html, body { background: #fff; }
.grid { display: grid; grid-template-columns: repeat(4, 120px); }
.cell { width: 120px; height: 120px; display: flex; align-items: center; justify-content: center; }
@keyframes spin { to { transform: rotate(360deg); } }
@keyframes ping { 75%, 100% { transform: scale(2); opacity: 0; } }
@keyframes pulse { 50% { opacity: .5; } }
@keyframes bounce { 0%, 100% { transform: translateY(-25%); animation-timing-function: cubic-bezier(.8, 0, 1, 1); } 50% { transform: none; animation-timing-function: cubic-bezier(0, 0, .2, 1); } }
`)
	for _, c := range animCells {
		sb.WriteString(c.css + "\n")
	}
	sb.WriteString("</style></head><body>\n<div class=\"grid\">\n")
	for _, c := range animCells {
		fmt.Fprintf(&sb, "<div class=\"cell\"><div class=\"box c-%s\"></div></div>\n", c.name)
	}
	sb.WriteString(`</div>
<script>
// Put every box in its "on" state (starting the transitions and the
// animations), pause every animation and set its current time to ?t= (ms).
const t = +new URLSearchParams(location.search).get("t");
const boxes = document.querySelectorAll(".box");
document.body.offsetWidth; // the before-change style
boxes.forEach((b) => b.classList.add("on"));
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

func regenerateAnimOracle(t *testing.T, dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	htmlPath, err := filepath.Abs(filepath.Join(dir, "anim.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(animOracleHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := animOracleSize()
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

// animOracleRoot is the page in Tela: the same cells, in the state set.
func animOracleRoot(set *Setter[bool]) Node {
	return Component(func() Node {
		on, s := UseState(false)
		*set = s
		cells := []Child{Class("flex w-[480px] flex-wrap")}
		for _, c := range animCells {
			cls := c.base + " " + c.off
			if on {
				cls = c.base + " " + c.on
			}
			cells = append(cells, div(Class("flex size-[120px] shrink-0 items-center justify-center"), div(Key(c.name), Class(cls))))
		}
		return div(cells...)
	})
}

// renderAnimOracle shows the page in a headless window, switches the cells
// on, and returns the window's pixels at the given time after the frame in
// which the animations start.
func renderAnimOracle(t *testing.T, at time.Duration) *image.RGBA {
	t.Helper()
	w, h := animOracleSize()
	b := headless.New(headless.Options{Size: platform.Size{W: float32(w), H: float32(h)}})
	var set Setter[bool]
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts, RootClass: "bg-white"}, Root: animOracleRoot(&set)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopTimer)
	win := host.Window().(*headless.Window)
	b.Drain(host)
	for range 4 { // the first frames, until the window is quiet
		b.Advance(b.FrameInterval())
		b.Drain(host)
	}
	set.Set(true)
	b.Drain(host)
	// The animations start in the frame that restyles the cells: the time
	// of the first transition.
	var start time.Duration
	for range 4 {
		if ba := keyed(host.Tree(), "oklch").box.anim; ba != nil && len(ba.trans) > 0 {
			start = ba.trans[0].Start
			break
		}
		b.Advance(b.FrameInterval())
		b.Drain(host)
	}
	if start == 0 {
		t.Fatal("the transitions did not start")
	}
	b.Advance(start + at - b.Now())
	b.Drain(host)
	if got := host.Tree().anim.tk.Now(); got != start+at {
		t.Fatalf("the frame ran at %v, want %v (start %v + %v)", got, start+at, start, at)
	}
	return win.Frame()
}

func TestChromiumAnimations(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "anim")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateAnimOracle(t, dir)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "anim.html")); err != nil || string(h) != animOracleHTML() {
		t.Fatal("testdata/chromium/anim/anim.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	report.WriteString("case          time   max   mean  >16\n")
	for _, at := range animOracleTimes {
		want := readRGBA(t, filepath.Join(dir, fmt.Sprintf("t%d.png", at.Milliseconds())))
		got := renderAnimOracle(t, at)
		if got.Rect != want.Rect {
			t.Fatalf("t=%v: render %v, Chromium %v", at, got.Rect, want.Rect)
		}
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("anim_t%d_tela.png", at.Milliseconds())), got)
		}
		for i, c := range animCells {
			col, row := i%animGridColumns, i/animGridColumns
			d := diffRegion(got, want, image.Rect(col*animCellSize, row*animCellSize, (col+1)*animCellSize, (row+1)*animCellSize))
			fmt.Fprintf(&report, "%-13s %4dms %4d %6.3f %4d\n", c.name, at.Milliseconds(), d.max, d.mean, d.over)
			b := c.bound
			if b.max == 0 {
				b = animDefaultBound
			}
			if d.max > b.max || d.mean > b.mean {
				t.Errorf("%s at %v: max %d mean %.3f, want at most %d and %.2f", c.name, at, d.max, d.mean, b.max, b.mean)
			}
		}
	}
	t.Log("difference from Chromium per case (channel levels):\n" + report.String())
}

// animDefaultBound applies to the cases without a bound of their own.
var animDefaultBound = chromiumBound{max: 80, mean: 0.5}
