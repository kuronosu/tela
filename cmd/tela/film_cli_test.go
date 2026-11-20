package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSnapshotFilm films the mount of testdata/effectanim: an effect starts
// a 300 ms widening of a box and a spinner turns. The sheet captions the
// frames, and the animated PNG holds every one, the widening growing from
// frame to frame and resting in the last.
func TestSnapshotFilm(t *testing.T) {
	skipShort(t)
	dir := t.TempDir()
	sheetPath, apngPath := filepath.Join(dir, "sheet.png"), filepath.Join(dir, "motion.png")
	var out, errOut bytes.Buffer
	args := []string{"snapshot", "--film", "400ms", "--every", "100ms", "--size", "300x120", "--apng", apngPath, "-o", sheetPath, "./testdata/effectanim"}
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatalf("tela %v: exit %d\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "(5 frames, 0 to 400 ms every 100 ms)") || !strings.Contains(out.String(), "animated PNG, 5 frames") {
		t.Errorf("stdout: %s", out.String())
	}
	f, err := os.Open(sheetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet, err := png.Decode(f)
	if err != nil {
		t.Fatalf("the contact sheet is not a PNG: %v", err)
	}
	if b := sheet.Bounds(); b.Dx() > sheetMaxSide || b.Dy() > sheetMaxSide || b.Dx() < 300 {
		t.Errorf("the contact sheet is %v", b)
	}
	data, err := os.ReadFile(apngPath)
	if err != nil {
		t.Fatal(err)
	}
	a, err := parseAPNG(data)
	if err != nil {
		t.Fatalf("the animated PNG breaks a rule of the format: %v", err)
	}
	if len(a.frames) != 5 || a.w != 300 || a.h != 120 {
		t.Fatalf("%d frames of %dx%d, want 5 of 300x120", len(a.frames), a.w, a.h)
	}
	// The box (black, at y 16..48, from x 16) widens: count its black row.
	width := func(c *image.NRGBA) int {
		n := 0
		for x := 0; x < a.w; x++ {
			if p := c.NRGBAAt(x, 32); p.R == 0 && p.G == 0 && p.B == 0 {
				n++
			}
		}
		return n
	}
	var widths []int
	for _, fr := range a.frames {
		widths = append(widths, width(fr.canvas))
	}
	for i := 1; i < 4; i++ {
		if widths[i] <= widths[i-1] {
			t.Errorf("the box does not widen between frames %d and %d: %v", i-1, i, widths)
		}
	}
	if widths[4] != widths[3] || widths[0] >= widths[4] {
		t.Errorf("the 300 ms widening should rest at frame 3 (300 ms) and start narrower: %v", widths)
	}
	// Flags that need --film.
	for _, bad := range [][]string{{"--every", "50ms"}, {"--apng", "x.png"}} {
		var o, e bytes.Buffer
		if code := run(append([]string{"snapshot"}, append(bad, "./testdata/effectanim")...), &o, &e); code != 2 || !strings.Contains(e.String(), "needs --film") {
			t.Errorf("%v: exit %d, %s", bad, code, e.String())
		}
	}
	// A sheet and an animation are two files.
	same := []string{"snapshot", "--film", "100ms", "--apng", sheetPath, "-o", sheetPath, "./testdata/effectanim"}
	if code := run(same, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "are the same file") {
		t.Errorf("-o and --apng the same: exit %d, %s", code, errOut.String())
	}
}

// TestSnapshotFilmKeepsSubMillisecondValues checks that the interval the
// instance films with is the one checkFilm validated, not that value
// truncated to whole milliseconds: 9ms every 1500us is 7 frames, where an
// interval of 1ms would make 10.
func TestSnapshotFilmKeepsSubMillisecondValues(t *testing.T) {
	skipShort(t)
	sheetPath := filepath.Join(t.TempDir(), "sheet.png")
	var out, errOut bytes.Buffer
	args := []string{"snapshot", "--film", "9ms", "--every", "1500us", "--size", "300x120", "-o", sheetPath, "./testdata/effectanim"}
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatalf("tela %v: exit %d\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "(7 frames") {
		t.Errorf("stdout: %s", out.String())
	}
}
