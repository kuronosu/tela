package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// filmPicture is a frame filled with one color, with a box of another.
func filmPicture(w, h int, bg color.RGBA, box image.Rectangle, fg color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			if image.Pt(x, y).In(box) {
				c = fg
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestCheckFilm(t *testing.T) {
	for _, tc := range []struct {
		span, every time.Duration
		wantEvery   time.Duration
		wantFrames  int
		wantErr     string
	}{
		{600 * time.Millisecond, 0, 50 * time.Millisecond, 13, ""},
		{600 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond, 7, ""},
		{450 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond, 6, ""}, // the last frame at the span
		{time.Second, time.Millisecond, 0, 0, "1001 frames; the most is 60: use an interval of at least 17ms"},
		{0, 0, 0, 0, "span of a film must be positive"},
		{time.Minute, time.Second, 0, 0, "too long"},
		{time.Second, 500 * time.Microsecond, 0, 0, "at least 1ms"},
	} {
		every, n, err := checkFilm(tc.span, tc.every)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkFilm(%v, %v) = %v, want %q", tc.span, tc.every, err, tc.wantErr)
			}
			continue
		}
		if err != nil || every != tc.wantEvery || n != tc.wantFrames {
			t.Errorf("checkFilm(%v, %v) = %v, %d, %v; want %v, %d", tc.span, tc.every, every, n, err, tc.wantEvery, tc.wantFrames)
		}
	}
}

func TestScaleBox(t *testing.T) {
	// A flat picture stays flat, and the average of a checkerboard is gray.
	flat := filmPicture(40, 30, color.RGBA{10, 20, 30, 255}, image.Rectangle{}, color.RGBA{})
	out := scaleBox(flat, 7, 5)
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 10 || out.Pix[i+1] != 20 || out.Pix[i+2] != 30 || out.Pix[i+3] != 255 {
			t.Fatalf("pixel %d is %v", i/4, out.Pix[i:i+4])
		}
	}
	check := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			check.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	got := scaleBox(check, 1, 1).RGBAAt(0, 0)
	if got.R < 127 || got.R > 128 {
		t.Errorf("average of a checkerboard is %v", got)
	}
}

func TestContactSheet(t *testing.T) {
	bg := color.RGBA{255, 255, 255, 255}
	var frames []filmFrame
	for i := 0; i < 13; i++ {
		frames = append(frames, filmFrame{At: time.Duration(i*50) * time.Millisecond, Img: filmPicture(960, 640, bg, image.Rect(10*i, 100, 10*i+40, 140), color.RGBA{255, 0, 0, 255})})
	}
	sheet := contactSheet(frames)
	b := sheet.Bounds()
	if b.Dx() > sheetMaxSide || b.Dy() > sheetMaxSide {
		t.Errorf("the sheet is %v: over %d", b, sheetMaxSide)
	}
	if b.Dx() < 800 {
		t.Errorf("the sheet is only %v wide: the frames are scaled down too much", b)
	}
	// The sheet shows every frame: its red boxes, one per cell.
	reds := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		in := false
		for x := b.Min.X; x < b.Max.X; x++ {
			c := sheet.RGBAAt(x, y)
			isRed := c.R > 200 && c.G < 80 && c.B < 80
			if isRed && !in {
				reds++
			}
			in = isRed
		}
	}
	if reds < 13 {
		t.Errorf("only %d red runs on the sheet, want a box for each of the 13 frames", reds)
	}
	// More frames than the sheet holds are sampled, the first and last kept.
	var many []filmFrame
	for i := 0; i < 60; i++ {
		many = append(many, filmFrame{At: time.Duration(i) * time.Millisecond, Img: frames[0].Img})
	}
	s := sampleFrames(many, filmSheetFrames)
	if len(s) != filmSheetFrames || s[0].At != 0 || s[len(s)-1].At != 59*time.Millisecond {
		t.Errorf("sampled %d frames from %v to %v", len(s), s[0].At, s[len(s)-1].At)
	}
	for i := 1; i < len(s); i++ {
		if s[i].At <= s[i-1].At {
			t.Errorf("the sample is not in order at %d", i)
		}
	}
	if sheet := contactSheet(many); sheet.Bounds().Dx() > sheetMaxSide || sheet.Bounds().Dy() > sheetMaxSide {
		t.Errorf("the sheet of 60 frames is %v", sheet.Bounds())
	}
}

func TestContactSheetCaptionsDiffer(t *testing.T) {
	// The caption is drawn: two sheets of the same picture at other times
	// are not the same sheet.
	img := filmPicture(100, 60, color.RGBA{255, 255, 255, 255}, image.Rectangle{}, color.RGBA{})
	a := contactSheet([]filmFrame{{At: 0, Img: img}})
	b := contactSheet([]filmFrame{{At: 150 * time.Millisecond, Img: img}})
	if bytes.Equal(a.Pix, b.Pix) {
		t.Error("the captions of 0 ms and 150 ms are the same")
	}
}

// TestContactSheetCaptionsStayInTheirCell checks that a caption wider than
// its cell (a long time over a narrow frame) is shrunk or clipped, not drawn
// over the cell beside it.
func TestContactSheetCaptionsStayInTheirCell(t *testing.T) {
	img := filmPicture(30, 20, color.RGBA{255, 255, 255, 255}, image.Rectangle{}, color.RGBA{})
	frames := []filmFrame{{At: 20000 * time.Millisecond, Img: img}, {At: 29950 * time.Millisecond, Img: img}, {At: 30000 * time.Millisecond, Img: img}}
	sheet := contactSheet(frames)
	cols, scale := sheetLayout(len(frames), 30, 20)
	cw := max(1, int(30*scale))
	if cols < 2 {
		t.Fatalf("%d columns: the test needs neighbors", cols)
	}
	for c := 0; c < cols-1; c++ {
		// The gap between the cell c and the next one holds no caption ink.
		x0 := sheetGap + c*(cw+2+sheetGap) + cw + 2
		for x := x0; x < x0+sheetGap; x++ {
			for y := sheetGap; y < sheetGap+sheetLabelH; y++ {
				if sheet.RGBAAt(x, y) == sheetInk {
					t.Fatalf("the caption of cell %d runs past its cell: ink at (%d, %d)", c, x, y)
				}
			}
		}
	}
}

// TestCheckAPNGTarget checks that the film tool writes a new path or
// replaces an animated PNG it wrote, and refuses any other file: a golden
// or an asset of the module.
func TestCheckAPNGTarget(t *testing.T) {
	dir := t.TempDir()
	white := color.RGBA{255, 255, 255, 255}
	plain, err := encodePNG(filmPicture(8, 8, white, image.Rectangle{}, color.RGBA{}))
	if err != nil {
		t.Fatal(err)
	}
	film, err := encodeAPNG([]filmFrame{{At: 0, Img: filmPicture(8, 8, white, image.Rectangle{}, color.RGBA{})}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"golden.png": plain, "mine.png": film, "garbage.png": []byte("not a png"), "empty.png": nil}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, ok := range map[string]bool{"new.png": true, "mine.png": true, "golden.png": false, "garbage.png": false, "empty.png": false} {
		err := checkAPNGTarget(filepath.Join(dir, name))
		if ok && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), "was not written by the film tool")) {
			t.Errorf("%s: error %v, want a refusal", name, err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "mine.png"), filepath.Join(dir, "link.png")); err == nil {
		if err := checkAPNGTarget(filepath.Join(dir, "link.png")); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("a symlink: %v", err)
		}
	}
}

// apngFile is a parsed animated PNG.
type apngFile struct {
	w, h   int
	plays  int
	frames []apngFrame
}

type apngFrame struct {
	box     image.Rectangle
	delayMs int
	img     *image.NRGBA // the frame's own picture (the box)
	canvas  *image.NRGBA // the animation's canvas after the frame
}

// parseAPNG checks the chunk sequence of an animated PNG against the rules
// browsers apply and returns its frames, composited on the canvas (every
// frame here uses blend SOURCE and dispose NONE).
func parseAPNG(data []byte) (*apngFile, error) {
	chunks, err := splitPNG(data)
	if err != nil {
		return nil, err
	}
	if len(chunks) < 4 || chunks[0].Type != "IHDR" || chunks[len(chunks)-1].Type != "IEND" {
		return nil, errors.New("a PNG starts with IHDR and ends with IEND")
	}
	ihdr := chunks[0].Data
	if len(ihdr) != 13 {
		return nil, errors.New("IHDR is not 13 bytes")
	}
	f := &apngFile{w: int(binary.BigEndian.Uint32(ihdr)), h: int(binary.BigEndian.Uint32(ihdr[4:]))}
	if chunks[1].Type != "acTL" {
		return nil, fmt.Errorf("the chunk after IHDR is %s, want acTL (it must precede the first IDAT)", chunks[1].Type)
	}
	numFrames := int(binary.BigEndian.Uint32(chunks[1].Data))
	f.plays = int(binary.BigEndian.Uint32(chunks[1].Data[4:]))
	seq := uint32(0)
	var idats [][]byte
	var pending []byte // the fcTL of the frame being read
	canvas := image.NewNRGBA(image.Rect(0, 0, f.w, f.h))
	flush := func() error {
		if pending == nil {
			return nil
		}
		fc := pending
		x0, y0 := int(binary.BigEndian.Uint32(fc[12:])), int(binary.BigEndian.Uint32(fc[16:]))
		w, h := int(binary.BigEndian.Uint32(fc[4:])), int(binary.BigEndian.Uint32(fc[8:]))
		if x0+w > f.w || y0+h > f.h || w == 0 || h == 0 {
			return fmt.Errorf("frame box %dx%d at (%d,%d) does not fit in %dx%d", w, h, x0, y0, f.w, f.h)
		}
		if fc[24] != 0 || fc[25] != 0 {
			return errors.New("dispose and blend must be NONE and SOURCE")
		}
		if den := binary.BigEndian.Uint16(fc[22:]); den != 1000 {
			return fmt.Errorf("delay denominator %d, want 1000", den)
		}
		// The frame is a PNG of its own: this IHDR with the frame's size.
		hdr := append([]byte(nil), ihdr...)
		binary.BigEndian.PutUint32(hdr, uint32(w))
		binary.BigEndian.PutUint32(hdr[4:], uint32(h))
		var buf []byte
		buf = append(buf, pngSignature...)
		buf = appendChunk(buf, "IHDR", hdr)
		for _, d := range idats {
			buf = appendChunk(buf, "IDAT", d)
		}
		buf = appendChunk(buf, "IEND")
		img, err := png.Decode(bytes.NewReader(buf))
		if err != nil {
			return fmt.Errorf("frame %d does not decode: %v", len(f.frames), err)
		}
		nr := image.NewNRGBA(img.Bounds())
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				nr.Set(x, y, img.At(x, y))
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				canvas.Set(x0+x, y0+y, nr.At(x, y))
			}
		}
		snap := image.NewNRGBA(canvas.Bounds())
		copy(snap.Pix, canvas.Pix)
		f.frames = append(f.frames, apngFrame{box: image.Rect(x0, y0, x0+w, y0+h), delayMs: int(binary.BigEndian.Uint16(fc[20:])), img: nr, canvas: snap})
		pending, idats = nil, nil
		return nil
	}
	sawIDAT := false
	for _, c := range chunks[2 : len(chunks)-1] {
		switch c.Type {
		case "fcTL":
			if err := flush(); err != nil {
				return nil, err
			}
			if got := binary.BigEndian.Uint32(c.Data); got != seq {
				return nil, fmt.Errorf("fcTL sequence %d, want %d", got, seq)
			}
			seq++
			pending = c.Data
		case "IDAT":
			if pending == nil || len(f.frames) != 0 {
				return nil, errors.New("IDAT outside the first frame: the first frame's fcTL must precede its IDAT")
			}
			sawIDAT = true
			idats = append(idats, c.Data)
		case "fdAT":
			if !sawIDAT || pending == nil {
				return nil, errors.New("fdAT before the first frame")
			}
			if got := binary.BigEndian.Uint32(c.Data); got != seq {
				return nil, fmt.Errorf("fdAT sequence %d, want %d", got, seq)
			}
			seq++
			idats = append(idats, c.Data[4:])
		case "tEXt":
			if string(c.Data) != apngSoftware {
				return nil, fmt.Errorf("unexpected tEXt chunk %q", c.Data)
			}
		default:
			return nil, fmt.Errorf("unexpected %s chunk", c.Type)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(f.frames) != numFrames {
		return nil, fmt.Errorf("acTL says %d frames, the file has %d", numFrames, len(f.frames))
	}
	return f, nil
}

func TestAPNGSequenceAndFrames(t *testing.T) {
	white, red, blue := color.RGBA{255, 255, 255, 255}, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	var frames []filmFrame
	for i, box := range []image.Rectangle{image.Rect(0, 0, 20, 20), image.Rect(10, 10, 30, 30), image.Rect(10, 10, 30, 30), image.Rect(40, 20, 60, 50)} {
		fg := red
		if i == 3 {
			fg = blue
		}
		frames = append(frames, filmFrame{At: time.Duration(i*50) * time.Millisecond, Img: filmPicture(64, 48, white, box, fg)})
	}
	data, err := encodeAPNG(frames)
	if err != nil {
		t.Fatal(err)
	}
	apng, err := parseAPNG(data)
	if err != nil {
		t.Fatalf("the animated PNG breaks a rule of the format: %v", err)
	}
	if apng.w != 64 || apng.h != 48 || apng.plays != 0 || len(apng.frames) != 4 {
		t.Errorf("%dx%d, %d plays, %d frames", apng.w, apng.h, apng.plays, len(apng.frames))
	}
	// Every frame composited on the canvas is the original picture.
	for i, f := range apng.frames {
		want := frames[i].Img
		for y := 0; y < 48; y++ {
			for x := 0; x < 64; x++ {
				w := want.RGBAAt(x, y)
				g := f.canvas.NRGBAAt(x, y)
				if g.R != w.R || g.G != w.G || g.B != w.B || g.A != w.A {
					t.Fatalf("frame %d at (%d,%d) is %v, want %v", i, x, y, g, w)
				}
			}
		}
	}
	// Delays are the time to the next frame, the last one's a second; a
	// frame that changed nothing is one pixel, and the others are cut to
	// what changed.
	for i, want := range []int{50, 50, 50, 1000} {
		if apng.frames[i].delayMs != want {
			t.Errorf("frame %d lasts %d ms, want %d", i, apng.frames[i].delayMs, want)
		}
	}
	if b := apng.frames[0].box; b != image.Rect(0, 0, 64, 48) {
		t.Errorf("the first frame is %v, want the whole image", b)
	}
	if b := apng.frames[2].box; b.Dx() != 1 || b.Dy() != 1 {
		t.Errorf("an unchanged frame is %v, want one pixel", b)
	}
	if b := apng.frames[1].box; b.Dx() >= 64 || b.Dy() >= 48 {
		t.Errorf("a small change is a %v frame: the box of what changed was expected", b)
	}
	// The first frame alone is a valid PNG for a program that does not
	// animate: the file decodes with image/png.
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the file does not decode as a PNG: %v", err)
	}
	if got := img.At(5, 5); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("the default image at (5,5) is %v, want the first frame's red", got)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Errorf("the default image is %v", b)
	}
}

func TestAPNGWithTransparency(t *testing.T) {
	// Some frames transparent, one box opaque: every frame's data has the
	// color type of the header.
	clear := color.RGBA{}
	a := filmPicture(32, 32, clear, image.Rect(0, 0, 32, 32), color.RGBA{9, 9, 9, 255}) // opaque
	b := filmPicture(32, 32, clear, image.Rect(0, 0, 8, 8), color.RGBA{9, 9, 9, 255})   // transparent
	c := filmPicture(32, 32, clear, image.Rect(0, 0, 32, 32), color.RGBA{9, 9, 9, 255}) // opaque again
	data, err := encodeAPNG([]filmFrame{{0, a}, {40 * time.Millisecond, b}, {80 * time.Millisecond, c}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAPNG(data); err != nil {
		t.Fatalf("mixed transparency breaks the format: %v", err)
	}
	chunks, _ := splitPNG(data)
	if ct := chunks[0].Data[9]; ct != 6 {
		t.Errorf("color type %d, want 6 (RGBA)", ct)
	}
}

func TestAPNGRejectsFramesOfOtherSizes(t *testing.T) {
	_, err := encodeAPNG([]filmFrame{{0, image.NewRGBA(image.Rect(0, 0, 10, 10))}, {time.Millisecond, image.NewRGBA(image.Rect(0, 0, 12, 10))}})
	if err == nil || !strings.Contains(err.Error(), "one size") {
		t.Errorf("error = %v", err)
	}
	if _, err := encodeAPNG(nil); err == nil {
		t.Error("no frames: no error")
	}
}

func TestParseSnapshotFilmFlags(t *testing.T) {
	o, err := parseSnapshotArgs([]string{"--film", "600ms", "./x"})
	if err != nil || o.Film != 600*time.Millisecond || o.Every != 50*time.Millisecond || o.Out != "film.png" || o.APNG != "" {
		t.Errorf("defaults: %+v, %v", o, err)
	}
	o, err = parseSnapshotArgs([]string{"--film=1.2s", "--every", "100ms", "--apng", "m.png", "-o", "sheet.png", "./x"})
	if err != nil || o.Film != 1200*time.Millisecond || o.Every != 100*time.Millisecond || o.Out != "sheet.png" || o.APNG != "m.png" {
		t.Errorf("flags: %+v, %v", o, err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--every", "50ms", "./x"}, "--every needs --film"},
		{[]string{"--apng", "a.png", "./x"}, "--apng needs --film"},
		{[]string{"--film", "600", "./x"}, "use a duration with a unit"},
		{[]string{"--film", "1s", "--every", "1ms", "./x"}, "use an interval of at least"},
		{[]string{"--film", "0s", "./x"}, "must be positive"},
		{[]string{"--film", "1m", "./x"}, "too long"},
		{[]string{"-o", "", "--film", "1s", "./x"}, "empty -o"},
	} {
		if _, err := parseSnapshotArgs(tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: error = %v, want %q", tc.args, err, tc.want)
		}
	}
	// A still keeps its default output.
	if o, err := parseSnapshotArgs([]string{"./x"}); err != nil || o.Out != "snapshot.png" || o.Film != 0 {
		t.Errorf("still: %+v, %v", o, err)
	}
}

func TestFilmAction(t *testing.T) {
	parse := func(s string) (string, string, error) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		tool, args, err := filmAction(m)
		return tool, string(args), err
	}
	for _, tc := range []struct{ in, tool, args string }{
		{`{"click": "e5"}`, "click", `{"ref":"e5"}`},
		{`{"hover": "e3"}`, "hover", `{"ref":"e3"}`},
		{`{"press": "Escape"}`, "press", `{"key":"Escape"}`},
		{`{"fill": {"ref": "e7", "text": "ana"}}`, "fill", `{"ref": "e7", "text": "ana"}`},
		{`{"click": {"role": "button", "name": "Save"}}`, "click", `{"role": "button", "name": "Save"}`},
	} {
		tool, args, err := parse(tc.in)
		if err != nil || tool != tc.tool || args != tc.args {
			t.Errorf("%s: %q %q %v, want %q %q", tc.in, tool, args, err, tc.tool, tc.args)
		}
	}
	for in, want := range map[string]string{
		`{}`:                             "has 0 keys",
		`{"click": "e1", "hover": "e2"}`: "has 2 keys",
		`{"resize": {"width": 3}}`:       "a film starts with one of click, drag, fill, hover, press, scroll",
		`{"fill": "text"}`:               "takes its arguments as an object",
	} {
		if _, _, err := parse(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want %q", in, err, want)
		}
	}
}
