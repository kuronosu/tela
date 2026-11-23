package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
	"time"
)

// Film: the frames of a motion (a transition, a spring, a Presence exit)
// as a contact sheet and, optionally, an animated PNG. The frames are
// taken by telatest's App.Film on the app's own clock, by tela snapshot
// --film (what the mount, a click, a hover or a key starts) and by the film
// tool of tela mcp (what an action starts; over --attach, the real window's
// frames in real time); this file turns them into pictures.

// Limits of a film.
const (
	// filmMaxFrames is the most frames one film captures (telatest allows
	// 100): each is a full screenshot, and the pictures stay readable.
	filmMaxFrames = 60
	// filmSheetFrames is the most frames the contact sheet shows; a longer
	// film is sampled evenly (the first and last always), and the animated
	// PNG keeps every frame.
	filmSheetFrames = 24
	// filmDefaultEvery is the interval between frames when none is given.
	filmDefaultEvery = 50 * time.Millisecond
	// filmMaxSpan bounds the span of the app's clock a film runs.
	filmMaxSpan = 30 * time.Second
)

// filmFrame is a frame of a film.
type filmFrame struct {
	At  time.Duration
	Img *image.RGBA
}

// checkFilm validates the span and the interval of a film and returns the
// interval to use (the default when every is 0) and the number of frames.
func checkFilm(span, every time.Duration) (time.Duration, int, error) {
	if every == 0 {
		every = filmDefaultEvery
	}
	switch {
	case span <= 0:
		return 0, 0, errors.New("the span of a film must be positive, such as 600ms")
	case span > filmMaxSpan:
		return 0, 0, fmt.Errorf("a film of %v is too long: the most is %v of the app's clock", span, filmMaxSpan)
	case every < time.Millisecond:
		return 0, 0, fmt.Errorf("the interval of a film must be at least 1ms, got %v", every)
	}
	n := int((span+every-1)/every) + 1
	if n > filmMaxFrames {
		least := (span + time.Duration(filmMaxFrames-2)) / time.Duration(filmMaxFrames-1)
		return 0, 0, fmt.Errorf("a film of %v every %v is %d frames; the most is %d: use an interval of at least %v, or a shorter span", span, every, n, filmMaxFrames, least.Round(time.Millisecond))
	}
	return every, n, nil
}

// filmLabel is the caption of a frame.
func filmLabel(at time.Duration) string {
	return fmt.Sprintf("%d ms", at.Milliseconds())
}

// toRGBA converts any image to an *image.RGBA with origin (0, 0).
func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// decodeFilmFrame decodes the PNG of a frame.
func decodeFilmFrame(at time.Duration, data []byte) (filmFrame, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return filmFrame{}, fmt.Errorf("the frame at %v is not a PNG: %v", at, err)
	}
	return filmFrame{At: at, Img: toRGBA(img)}, nil
}

// ---------------------------------------------------------------------
// The contact sheet.

// Metrics of the sheet, in pixels.
const (
	sheetMaxSide = 1600 // the sheet is at most this wide and this tall
	sheetGap     = 8
	sheetLabelH  = 22 // a caption: 7 rows of the font at scale 2 and padding
	sheetScale   = 2  // the font's scale
)

var (
	sheetBG     = color.RGBA{0xee, 0xee, 0xee, 0xff}
	sheetBorder = color.RGBA{0xb4, 0xb4, 0xb4, 0xff}
	sheetInk    = color.RGBA{0x22, 0x22, 0x22, 0xff}
)

// sampleFrames picks at most max frames, evenly spaced, the first and last
// among them.
func sampleFrames(frames []filmFrame, max int) []filmFrame {
	if len(frames) <= max {
		return frames
	}
	out := make([]filmFrame, 0, max)
	for i := 0; i < max; i++ {
		out = append(out, frames[i*(len(frames)-1)/(max-1)])
	}
	return out
}

// sheetLayout picks the number of columns that makes the frames largest
// within sheetMaxSide, and the scale they are drawn at (at most 1); of
// equal ones, the sheet closest to a square.
func sheetLayout(n, fw, fh int) (cols int, scale float64) {
	best, bestSkew := -1.0, 0.0
	for c := 1; c <= n; c++ {
		rows := (n + c - 1) / c
		sw := float64(sheetMaxSide-sheetGap*(c+1)-2*c) / float64(c*fw)
		sh := float64(sheetMaxSide-sheetGap*(rows+1)-rows*(sheetLabelH+2)) / float64(rows*fh)
		s := math.Min(1, math.Min(sw, sh))
		w := float64(c) * (float64(fw)*s + 2 + sheetGap)
		h := float64(rows) * (float64(fh)*s + 2 + sheetGap + sheetLabelH)
		skew := math.Abs(math.Log(w / h))
		if s > best+1e-9 || s > best-1e-9 && skew < bestSkew {
			best, bestSkew, cols = s, skew, c
		}
	}
	return cols, math.Max(best, 0.05)
}

// contactSheet lays the frames out in a grid, each scaled down and
// captioned with its time on the app's clock, in a picture of at most
// sheetMaxSide pixels a side. More than filmSheetFrames frames are
// sampled.
func contactSheet(frames []filmFrame) *image.RGBA {
	frames = sampleFrames(frames, filmSheetFrames)
	if len(frames) == 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	fb := frames[0].Img.Bounds()
	cols, scale := sheetLayout(len(frames), fb.Dx(), fb.Dy())
	rows := (len(frames) + cols - 1) / cols
	cw, ch := max(1, int(float64(fb.Dx())*scale)), max(1, int(float64(fb.Dy())*scale))
	w := sheetGap + cols*(cw+2+sheetGap)
	h := sheetGap + rows*(sheetLabelH+ch+2+sheetGap)
	sheet := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(sheetBG), image.Point{}, draw.Src)
	for i, f := range frames {
		x := sheetGap + (i%cols)*(cw+2+sheetGap)
		y := sheetGap + (i/cols)*(sheetLabelH+ch+2+sheetGap)
		drawLabel(sheet, x+2, y+(sheetLabelH-7*sheetScale)/2, cw, filmLabel(f.At), sheetInk)
		cell := image.Rect(x, y+sheetLabelH, x+cw+2, y+sheetLabelH+ch+2)
		draw.Draw(sheet, cell, image.NewUniform(sheetBorder), image.Point{}, draw.Src)
		scaled := scaleBox(f.Img, cw, ch)
		draw.Draw(sheet, image.Rect(cell.Min.X+1, cell.Min.Y+1, cell.Max.X-1, cell.Max.Y-1), scaled, image.Point{}, draw.Src)
	}
	return sheet
}

// scaleBox scales src to dw x dh by averaging the source pixels each
// destination pixel covers (a box filter, which keeps thin lines visible
// when a frame is scaled down). It is deterministic.
func scaleBox(src *image.RGBA, dw, dh int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if dw == sw && dh == sh {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xs, ys := float64(sw)/float64(dw), float64(sh)/float64(dh)
	for dy := 0; dy < dh; dy++ {
		y0, y1 := float64(dy)*ys, float64(dy+1)*ys
		for dx := 0; dx < dw; dx++ {
			x0, x1 := float64(dx)*xs, float64(dx+1)*xs
			var r, g, b, a, wsum float64
			for sy := int(y0); sy < sh && float64(sy) < y1; sy++ {
				wy := math.Min(y1, float64(sy+1)) - math.Max(y0, float64(sy))
				row := src.Pix[sy*src.Stride:]
				for sx := int(x0); sx < sw && float64(sx) < x1; sx++ {
					wt := wy * (math.Min(x1, float64(sx+1)) - math.Max(x0, float64(sx)))
					p := row[sx*4 : sx*4+4]
					r += wt * float64(p[0])
					g += wt * float64(p[1])
					b += wt * float64(p[2])
					a += wt * float64(p[3])
					wsum += wt
				}
			}
			if wsum == 0 {
				continue
			}
			o := dst.Pix[dy*dst.Stride+dx*4:]
			o[0], o[1], o[2], o[3] = uint8(r/wsum+0.5), uint8(g/wsum+0.5), uint8(b/wsum+0.5), uint8(a/wsum+0.5)
		}
	}
	return dst
}

// The font of the captions: 5x7 pixels, for what a caption says ("150 ms").
var labelGlyphs = map[rune][7]string{
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3': {".###.", "#...#", "....#", "..##.", "....#", "#...#", ".###."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5': {"#####", "#....", "####.", "....#", "....#", "#...#", ".###."},
	'6': {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."},
	'm': {".....", ".....", "##.#.", "#.#.#", "#.#.#", "#.#.#", "#.#.#"},
	's': {".....", ".....", ".####", "#....", ".###.", "....#", "####."},
	'.': {".....", ".....", ".....", ".....", ".....", ".##..", ".##.."},
	'-': {".....", ".....", ".....", "#####", ".....", ".....", "....."},
}

// drawLabel draws text at (x, y) in the font, within the columns x to
// x+width: at sheetScale when it fits, else at scale 1, and clipped to the
// cell if it still does not, so a caption never runs into the next cell.
func drawLabel(dst *image.RGBA, x, y, width int, text string, c color.RGBA) {
	scale := sheetScale
	if n := len([]rune(text)); n*6*scale-scale > width {
		scale = 1
		y += 7 * (sheetScale - 1) / 2 // stay centered in the caption row
	}
	clip := image.Rect(x, dst.Bounds().Min.Y, x+width, dst.Bounds().Max.Y).Intersect(dst.Bounds())
	for _, r := range text {
		g, ok := labelGlyphs[r]
		if ok {
			for row, line := range g {
				for col, ch := range line {
					if ch == '#' {
						rect := image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale)
						draw.Draw(dst, rect.Intersect(clip), image.NewUniform(c), image.Point{}, draw.Src)
					}
				}
			}
		}
		x += 6 * scale
	}
}

// encodePNG encodes an image as a PNG.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------
// Animated PNG.
//
// An APNG is a PNG with an acTL chunk before its first IDAT, the first
// frame described by an fcTL chunk before the IDAT, and each further frame
// an fcTL followed by fdAT chunks (IDAT data behind a sequence number). The
// sequence numbers of the fcTL and fdAT chunks run from 0 without a gap,
// frames must fit in the image and use the image's color type and bit
// depth (https://wiki.mozilla.org/APNG_Specification). The data of each
// frame is what image/png compresses; this file only wraps it.
//
// Every frame after the first is cut to the box that changed from the
// previous one (blend_op SOURCE, dispose_op NONE: it replaces its box), so
// a small motion in a big window stays small.

// apngSoftware is the tEXt chunk of an animated PNG this tool writes: the
// film tool replaces a file only while it carries it.
const apngSoftware = "Software\x00tela film"

const (
	apngLastHold = time.Second // how long the last frame stays on screen
	pngSignature = "\x89PNG\r\n\x1a\n"
)

// pngChunk is a chunk of a PNG file.
type pngChunk struct {
	Type string
	Data []byte
}

// splitPNG returns the chunks of a PNG file.
func splitPNG(data []byte) ([]pngChunk, error) {
	if !bytes.HasPrefix(data, []byte(pngSignature)) {
		return nil, errors.New("not a PNG: bad signature")
	}
	var out []pngChunk
	for p := len(pngSignature); p < len(data); {
		if p+12 > len(data) {
			return nil, errors.New("truncated chunk")
		}
		n := int(binary.BigEndian.Uint32(data[p:]))
		if p+12+n > len(data) {
			return nil, errors.New("truncated chunk")
		}
		typ, body := string(data[p+4:p+8]), data[p+8:p+8+n]
		if crc32.ChecksumIEEE(data[p+4:p+8+n]) != binary.BigEndian.Uint32(data[p+8+n:]) {
			return nil, fmt.Errorf("bad CRC in the %s chunk", typ)
		}
		out = append(out, pngChunk{typ, body})
		p += 12 + n
	}
	return out, nil
}

// appendChunk writes a chunk.
func appendChunk(dst []byte, typ string, data ...[]byte) []byte {
	n := 0
	for _, d := range data {
		n += len(d)
	}
	dst = binary.BigEndian.AppendUint32(dst, uint32(n))
	start := len(dst)
	dst = append(dst, typ...)
	for _, d := range data {
		dst = append(dst, d...)
	}
	return binary.BigEndian.AppendUint32(dst, crc32.ChecksumIEEE(dst[start:]))
}

// opaque reports whether every pixel of img is opaque.
func opaque(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}

// changedBox returns the smallest rectangle that holds every pixel where
// the two images (of one size) differ; empty when they do not.
func changedBox(a, b *image.NRGBA) image.Rectangle {
	r := image.Rectangle{Min: image.Pt(math.MaxInt, math.MaxInt), Max: image.Pt(-1, -1)}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	for y := 0; y < h; y++ {
		ra, rb := a.Pix[y*a.Stride:y*a.Stride+w*4], b.Pix[y*b.Stride:y*b.Stride+w*4]
		if bytes.Equal(ra, rb) {
			continue
		}
		r.Min.Y, r.Max.Y = min(r.Min.Y, y), y+1
		for x := 0; x < w; x++ {
			if !bytes.Equal(ra[x*4:x*4+4], rb[x*4:x*4+4]) {
				r.Min.X, r.Max.X = min(r.Min.X, x), max(r.Max.X, x+1)
			}
		}
	}
	if r.Max.X < 0 {
		return image.Rectangle{}
	}
	return r
}

// encodeAPNG writes the frames as an animated PNG that loops forever, each
// frame lasting until the next one's time on the app's clock and the last
// for a second. Every frame must have the size of the first.
func encodeAPNG(frames []filmFrame) ([]byte, error) {
	if len(frames) == 0 {
		return nil, errors.New("no frames")
	}
	size := frames[0].Img.Bounds().Size()
	imgs := make([]*image.NRGBA, len(frames))
	allOpaque := true
	for i, f := range frames {
		if f.Img.Bounds().Size() != size {
			return nil, fmt.Errorf("frame %d is %v, the first is %v: the frames of an animated PNG have one size", i, f.Img.Bounds().Size(), size)
		}
		n := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
		draw.Draw(n, n.Bounds(), f.Img, f.Img.Bounds().Min, draw.Src)
		imgs[i] = n
		allOpaque = allOpaque && opaque(n)
	}
	var out []byte
	out = append(out, pngSignature...)
	seq := uint32(0)
	fcTL := func(r image.Rectangle, i int) []byte {
		delay := apngLastHold
		if i+1 < len(frames) {
			delay = frames[i+1].At - frames[i].At
		}
		ms := uint16(min(max(delay.Milliseconds(), 1), math.MaxUint16))
		b := make([]byte, 26)
		binary.BigEndian.PutUint32(b[0:], seq)
		binary.BigEndian.PutUint32(b[4:], uint32(r.Dx()))
		binary.BigEndian.PutUint32(b[8:], uint32(r.Dy()))
		binary.BigEndian.PutUint32(b[12:], uint32(r.Min.X))
		binary.BigEndian.PutUint32(b[16:], uint32(r.Min.Y))
		binary.BigEndian.PutUint16(b[20:], ms)
		binary.BigEndian.PutUint16(b[22:], 1000)
		b[24], b[25] = 0, 0 // dispose NONE, blend SOURCE
		seq++
		return b
	}
	// A frame's PNG, with the color type of the whole animation: when some
	// frames are transparent and a box is not, one corner pixel gets an
	// alpha of 254 so that the box is encoded with an alpha channel too.
	encodeBox := func(img *image.NRGBA, r image.Rectangle) ([]pngChunk, error) {
		sub := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(sub, sub.Bounds(), img, r.Min, draw.Src)
		if !allOpaque && opaque(sub) {
			sub.Pix[3] = 0xfe
		}
		data, err := encodePNG(sub)
		if err != nil {
			return nil, err
		}
		return splitPNG(data)
	}
	full := image.Rect(0, 0, size.X, size.Y)
	for i, img := range imgs {
		box := full
		if i > 0 {
			box = changedBox(imgs[i-1], img)
			if box.Empty() {
				box = image.Rect(0, 0, 1, 1) // nothing changed: a frame of one pixel, to keep the time
			}
		}
		chunks, err := encodeBox(img, box)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			// IHDR, then acTL (frames, plays: 0 is forever), then the rest.
			out = appendChunk(out, "IHDR", chunks[0].Data)
			ac := make([]byte, 8)
			binary.BigEndian.PutUint32(ac, uint32(len(frames)))
			out = appendChunk(out, "acTL", ac)
			out = appendChunk(out, "tEXt", []byte(apngSoftware))
		}
		out = appendChunk(out, "fcTL", fcTL(box, i))
		for _, c := range chunks {
			if c.Type != "IDAT" {
				continue
			}
			if i == 0 {
				out = appendChunk(out, "IDAT", c.Data)
				continue
			}
			sq := binary.BigEndian.AppendUint32(nil, seq)
			seq++
			out = appendChunk(out, "fdAT", sq, c.Data)
		}
	}
	return appendChunk(out, "IEND"), nil
}

// parseFilmDuration reads a duration for --film and --every.
// filmAct is the action a film of tela snapshot starts with (--click,
// --hover, --press), as the generated test reads it from envAct.
type filmAct struct {
	Op  string  `json:"op"` // click, hover or press
	Loc *mcpLoc `json:"loc,omitempty"`
	Key string  `json:"key,omitempty"`
}

// env is the value of envAct: "" for no action.
func (a *filmAct) env() string {
	if a == nil {
		return ""
	}
	b, _ := json.Marshal(a)
	return string(b)
}

// describe names the action for the output, "after clicking role=button[name="Open"]".
func (a *filmAct) describe(arg string) string {
	switch a.Op {
	case "press":
		return "after pressing " + a.Key
	case "hover":
		return "after hovering " + arg
	}
	return "after clicking " + arg
}

// parseFilmLocator reads the element of --click and --hover, in the
// syntax of Playwright's selectors: role=button[name="Open"] (the name
// optional), text=Open (contained, any case) or text="Open" (exact), and
// testid=open, the h.TestID.
func parseFilmLocator(flagName, v string) (*mcpLoc, error) {
	help := fmt.Errorf(`invalid --%s %q: name the element as role=button[name="Open"], text=Open (text="Open" for the exact text) or testid=open`, flagName, v)
	kind, rest, ok := strings.Cut(strings.TrimSpace(v), "=")
	if !ok || rest == "" {
		return nil, help
	}
	switch kind {
	case "role":
		role, attr, hasAttr := strings.Cut(rest, "[")
		loc := &mcpLoc{Role: strings.TrimSpace(role)}
		if loc.Role == "" {
			return nil, help
		}
		if hasAttr {
			attr, ok = strings.CutSuffix(strings.TrimSpace(attr), "]")
			name, ok2 := strings.CutPrefix(strings.TrimSpace(attr), "name=")
			if !ok || !ok2 {
				return nil, help
			}
			if name = strings.TrimSpace(name); strings.HasPrefix(name, `"`) {
				u, err := strconv.Unquote(name)
				if err != nil {
					return nil, help
				}
				name = u
			}
			loc.Name = name
		}
		return loc, nil
	case "text":
		if strings.HasPrefix(rest, `"`) {
			u, err := strconv.Unquote(rest)
			if err != nil {
				return nil, help
			}
			return &mcpLoc{Text: u, Exact: true}, nil
		}
		return &mcpLoc{Text: rest}, nil
	case "testid":
		return &mcpLoc{TestID: rest}, nil
	}
	return nil, help
}

func parseFilmDuration(flagName, v string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid --%s %q: use a duration with a unit, such as 600ms or 1.5s", flagName, v)
	}
	return d, nil
}
