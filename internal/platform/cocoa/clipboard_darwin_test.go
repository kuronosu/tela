//go:build darwin

package cocoa

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/tiff"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The clipboard tests replace the general pasteboard's content, so they
// run on CI (GitHub sets CI) or with TELA_LIVE, and skip on a developer's
// Mac otherwise.
func clipboardAllowed() bool { return os.Getenv("CI") != "" || os.Getenv("TELA_LIVE") != "" }

func requireClipboard(t *testing.T) {
	t.Helper()
	if !clipboardAllowed() {
		t.Skip("replaces the clipboard's content: set TELA_LIVE=1 (CI runs it)")
	}
}

// runScript runs b with one window and calls script on the UI thread once
// the window presented its first frame. script calls quit when it is done;
// the run fails after 30 s.
func runScript(t *testing.T, b *Backend, script func(quit func()), onEvent func(e platform.Event)) {
	t.Helper()
	var timedOut atomic.Bool
	onMain(func() {
		if _, err := b.NewWindow(platform.WindowOptions{Title: "tela " + t.Name(), Size: platform.Size{W: 160, H: 100}}); err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		timer := time.AfterFunc(30*time.Second, func() { timedOut.Store(true); b.Quit() })
		defer timer.Stop()
		started := false
		_ = b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
			if onEvent != nil {
				onEvent(e)
			}
			if _, ok := e.(platform.FrameEvent); ok && !started {
				started = true
				presentSolid(t, w, color.RGBA{0x30, 0x60, 0x30, 0xff})
				script(b.Quit)
			}
		}))
	})
	if timedOut.Load() {
		t.Fatalf("the script did not finish within 30 s")
	}
}

// readBack reads the clipboard's types and then each of mimes, in order,
// and calls done with what came back.
type readBack struct {
	types []string
	data  map[string][]byte
	errs  []string
}

func readAll(cb platform.Clipboard, mimes []string, done func(readBack)) {
	r := readBack{data: map[string][]byte{}}
	var next func(i int)
	next = func(i int) {
		if i == len(mimes) {
			done(r)
			return
		}
		m := mimes[i]
		if err := cb.Read(m, func(b []byte, err error) {
			if err != nil {
				r.errs = append(r.errs, fmt.Sprintf("Read(%s): %v", m, err))
			}
			r.data[m] = b
			next(i + 1)
		}); err != nil {
			r.errs = append(r.errs, fmt.Sprintf("Read(%s) at once: %v", m, err))
			next(i + 1)
		}
	}
	if err := cb.ReadTypes(func(types []string, err error) {
		if err != nil {
			r.errs = append(r.errs, fmt.Sprintf("ReadTypes: %v", err))
		}
		r.types = types
		next(0)
	}); err != nil {
		r.errs = append(r.errs, fmt.Sprintf("ReadTypes at once: %v", err))
		next(0)
	}
}

// rawPasteboard describes the pasteboard's items and their types, as other
// apps see them.
func rawPasteboard() string {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	pb := generalPasteboard()
	var parts []string
	for i, it := range items(pb) {
		parts = append(parts, fmt.Sprintf("item %d %q", i, stringsOf(darwin.Send(it, pbSel.types))))
	}
	return fmt.Sprintf("changeCount %d, %s", darwin.SendInt(pb, pbSel.changeCount), strings.Join(parts, "; "))
}

func changeCount() int {
	return darwin.SendInt(generalPasteboard(), pbSel.changeCount)
}

// externalWrite puts items on the pasteboard as another app would, with
// NSPasteboard directly: each item maps types to strings (text) or bytes.
func externalWrite(t *testing.T, items ...map[string]any) {
	t.Helper()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	pb := generalPasteboard()
	darwin.Send(pb, pbSel.clearContents)
	array := darwin.SendClass(darwin.GetClass("NSMutableArray"), pbSel.array)
	for _, m := range items {
		item := darwin.Send(darwin.SendClass(darwin.GetClass("NSPasteboardItem"), sel.alloc), sel.init)
		for uti, v := range m {
			typ := darwin.NSString(uti)
			switch v := v.(type) {
			case string:
				s := darwin.NSStringBytes([]byte(v))
				darwin.SendBool(item, pbSel.setStringFor, s, typ)
				darwin.Release(s)
			case []byte:
				d := darwin.NSData(v)
				darwin.SendBool(item, pbSel.setDataForType, d, typ)
				darwin.Release(d)
			}
			darwin.Release(typ)
		}
		darwin.Send(array, pbSel.addObject, item)
		darwin.Release(item)
	}
	if !darwin.SendBool(pb, pbSel.writeObjects, array) {
		t.Errorf("external writeObjects: failed")
	}
}

// externalRead reads a type as another app would: stringForType: for
// text types, dataForType: otherwise (nil when missing).
func externalRead(uti string, text bool) []byte {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	typ := darwin.NSString(uti)
	defer darwin.Release(typ)
	pb := generalPasteboard()
	if text {
		return darwin.StringBytes(darwin.Send(pb, pbSel.stringForType, typ))
	}
	return darwin.Bytes(darwin.Send(pb, pbSel.dataForType, typ))
}

func testPNG(t *testing.T) ([]byte, *image.NRGBA) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 13)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), img
}

// TestClipboardRoundTrips writes Data through the backend and reads every
// type back through it (the pasteboard server in between: reads never
// come from memory), logging the pasteboard types other apps see.
func TestClipboardRoundTrips(t *testing.T) {
	requireClipboard(t)
	b := newBackend(t)
	pngBytes, _ := testPNG(t)
	dir := t.TempDir()
	fileA, fileB := filepath.Join(dir, "a b.txt"), filepath.Join(dir, "ü.txt")
	uriA, _ := platform.FileURI(fileA)
	uriB, _ := platform.FileURI(fileB)
	cases := []struct {
		name string
		data platform.Data
	}{
		{"text", platform.TextData("tela clipboard ✓\nline two\ttab")},
		{"html and text", platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<b>tela ✓</b>")}, {Type: platform.MIMEText, Bytes: []byte("tela ✓")}}},
		{"png", platform.Data{{Type: platform.MIMEPNG, Bytes: pngBytes}}},
		{"custom binary", platform.Data{{Type: "application/x-tela-test", Bytes: []byte{0, 1, 2, 0xff, 0xfe, 0}}, {Type: platform.MIMEText, Bytes: []byte("fallback")}}},
		{"declared type", platform.Data{{Type: "application/json", Bytes: []byte(`{"tela":1}`)}}},
		{"custom with parameters", platform.Data{{Type: "application/x-tela-item;version=2", Bytes: []byte("v2")}}},
		{"files and a URL", platform.Data{{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{uriA, uriB, "https://telaui.dev/"})}}},
	}
	runScript(t, b, func(quit func()) {
		cb := b.Clipboard()
		var next func(i int)
		next = func(i int) {
			if i == len(cases) {
				quit()
				return
			}
			c := cases[i]
			before := changeCount()
			if err := cb.Write(c.data); err != nil {
				t.Errorf("%s: Write: %v", c.name, err)
				next(i + 1)
				return
			}
			after := changeCount()
			if after <= before {
				t.Errorf("%s: changeCount %d -> %d after Write, want it to grow", c.name, before, after)
			}
			raw := rawPasteboard()
			readAll(cb, c.data.Types(), func(r readBack) {
				t.Logf("%s: types %q; pasteboard: %s", c.name, r.types, raw)
				for _, e := range r.errs {
					t.Errorf("%s: %s", c.name, e)
				}
				for _, e := range c.data {
					if e.Type != platform.MIMEText && e.Type != platform.MIMEHTML && e.Type != platform.MIMEPNG && e.Type != platform.MIMEURIList {
						t.Logf("%s: %s is pasteboard type %s", c.name, e.Type, utiForMIME(e.Type))
					}
					if !slices.Contains(r.types, e.Type) {
						t.Errorf("%s: ReadTypes = %q, missing %s", c.name, r.types, e.Type)
					}
					got := r.data[e.Type]
					if e.Type == platform.MIMEURIList {
						if g, w := platform.ParseURIList(got), platform.ParseURIList(e.Bytes); !slices.Equal(g, w) {
							t.Errorf("%s: URIs read back %q, want %q", c.name, g, w)
						}
						continue
					}
					if !bytes.Equal(got, e.Bytes) {
						t.Errorf("%s: %s read back %q, want %q", c.name, e.Type, got, e.Bytes)
					}
				}
				next(i + 1)
			})
		}
		next(0)
	}, nil)
}

// TestClipboardOwnership: another app's write replaces ours at once (no
// stale data from memory), the backend's writes are what other apps read,
// TIFF-only images and Finder's file reference URLs are converted, and
// empty Data clears the pasteboard.
func TestClipboardOwnership(t *testing.T) {
	requireClipboard(t)
	b := newBackend(t)
	_, img := testPNG(t)
	var tiffBytes bytes.Buffer
	if err := tiff.Encode(&tiffBytes, img, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "referenced.txt")
	if err := os.WriteFile(file, []byte("tela"), 0o644); err != nil {
		t.Fatal(err)
	}
	runScript(t, b, func(quit func()) {
		cb := b.Clipboard()
		steps := []func(next func()){
			// Ours, then another app's: the read sees theirs.
			func(next func()) {
				if err := cb.WriteText("ours"); err != nil {
					t.Errorf("WriteText: %v", err)
				}
				ours := changeCount()
				externalWrite(t, map[string]any{utiUTF8Text: "theirs ✓"})
				if theirs := changeCount(); theirs <= ours {
					t.Errorf("changeCount %d after our write, %d after theirs: want it to grow", ours, theirs)
				}
				readAll(cb, nil, func(r readBack) {
					cb.ReadText(func(s string, err error) {
						t.Logf("after another app's write: ReadText %q, types %q; pasteboard: %s", s, r.types, rawPasteboard())
						if err != nil || s != "theirs ✓" {
							t.Errorf("ReadText = %q, %v; want the other app's text", s, err)
						}
						if !slices.Equal(r.types, []string{platform.MIMEText}) {
							t.Errorf("ReadTypes = %q, want [%s]", r.types, platform.MIMEText)
						}
						next()
					})
				})
			},
			// What other apps read from our write.
			func(next func()) {
				if err := cb.Write(platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<i>tela</i>")}, {Type: platform.MIMEText, Bytes: []byte("tela ✓")}}); err != nil {
					t.Errorf("Write: %v", err)
				}
				html, text := externalRead(utiHTML, true), externalRead(utiUTF8Text, true)
				t.Logf("other apps read public.html %q, public.utf8-plain-text %q; pasteboard: %s", html, text, rawPasteboard())
				if string(html) != "<i>tela</i>" || string(text) != "tela ✓" {
					t.Errorf("other apps read HTML %q and text %q", html, text)
				}
				next()
			},
			// A TIFF-only image reads as PNG.
			func(next func()) {
				externalWrite(t, map[string]any{utiTIFF: tiffBytes.Bytes()})
				readAll(cb, []string{platform.MIMEPNG}, func(r readBack) {
					t.Logf("TIFF only: types %q, %d PNG bytes; pasteboard: %s", r.types, len(r.data[platform.MIMEPNG]), rawPasteboard())
					for _, e := range r.errs {
						t.Errorf("TIFF: %s", e)
					}
					if !slices.Contains(r.types, platform.MIMEPNG) {
						t.Errorf("ReadTypes = %q, want %s for a TIFF", r.types, platform.MIMEPNG)
					}
					if got, err := png.Decode(bytes.NewReader(r.data[platform.MIMEPNG])); err != nil {
						t.Errorf("the TIFF read as PNG does not decode: %v", err)
					} else if g, w := color.NRGBAModel.Convert(got.At(1, 1)), img.At(1, 1); g != w {
						t.Errorf("TIFF pixel (1,1) read as %v, want %v", g, w)
					}
					next()
				})
			},
			// Finder writes file reference URLs; they read as paths.
			func(next func()) {
				ref := fileReferenceURL(file)
				t.Logf("file reference URL of %s: %q", file, ref)
				if ref == "" {
					t.Logf("no file reference URL: skipped")
					next()
					return
				}
				externalWrite(t, map[string]any{utiFileURL: ref})
				readAll(cb, []string{platform.MIMEURIList}, func(r readBack) {
					uris := platform.ParseURIList(r.data[platform.MIMEURIList])
					t.Logf("read back %q", uris)
					want, _ := filepath.EvalSymlinks(file)
					if len(uris) != 1 {
						t.Errorf("URIs = %q, want one", uris)
					} else if p, err := platform.FilePath(uris[0]); err != nil {
						t.Errorf("FilePath(%q): %v", uris[0], err)
					} else if got, _ := filepath.EvalSymlinks(strings.TrimSuffix(p, "/")); got != want {
						t.Errorf("the reference URL resolved to %q, want %q", got, want)
					}
					next()
				})
			},
			// Empty data clears.
			func(next func()) {
				if err := cb.Write(nil); err != nil {
					t.Errorf("Write(nil): %v", err)
				}
				readAll(cb, []string{platform.MIMEText}, func(r readBack) {
					t.Logf("after Write(nil): types %q; pasteboard: %s", r.types, rawPasteboard())
					if r.types != nil || r.data[platform.MIMEText] != nil {
						t.Errorf("after Write(nil): types %q, text %q; want nothing", r.types, r.data[platform.MIMEText])
					}
					next()
				})
			},
		}
		var run func(i int)
		run = func(i int) {
			if i == len(steps) {
				quit()
				return
			}
			steps[i](func() { run(i + 1) })
		}
		run(0)
	}, nil)
}

// fileReferenceURL returns the file reference URL of path
// (file:///.file/id=…), as Finder puts it on the pasteboard.
func fileReferenceURL(path string) string {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	p := darwin.NSString(path)
	defer darwin.Release(p)
	u := darwin.SendClass(darwin.GetClass("NSURL"), darwin.Sel("fileURLWithPath:"), p)
	if u == 0 {
		return ""
	}
	ref := darwin.Send(u, darwin.Sel("fileReferenceURL"))
	if ref == 0 {
		return ""
	}
	return darwin.GoString(darwin.Send(ref, pbSel.absoluteString))
}
