package telatest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"runtime/debug"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

// The functions below are the public headless entry point: they mount a
// tree without a window and a test, and need no *testing.T. `tela
// snapshot` and `tela inspect` are built on them, so the CLI works on any
// module that requires github.com/kuronosu/tela (amends ADR-047). They
// accept the options [Size], [Scale], [Dark], [ColorScheme], [ReducedMotion],
// [ClassicScrollbars], [InstantScroll], [RootClass], [NoDev] (they run in
// dev mode by default, reporting dev errors and warnings on standard error)
// and [Debug] (overlays are drawn into the image); the others concern
// mounted apps and are ignored.
//
// Each call builds a new tree, runs frames at the given size until the
// tree has no more work (a few at most, for what the first one measures,
// such as tela.UseBounds) and unmounts it. They use the embedded fonts only, so the results are the
// same on every machine. A panic in the tree (a hook misuse, say) is
// returned as an error.

// headlessSetup resolves the options of the headless entry points.
func headlessSetup(opts []Option) (cfg config, size gfx.Size, err error) {
	cfg = newConfig(opts)
	if err = cfg.validate(); err != nil {
		return cfg, size, err
	}
	size = gfx.Size{W: cfg.width, H: cfg.height}
	if size.W == 0 && size.H == 0 {
		size = gfx.Size{W: platform.DefaultWindowSize.W, H: platform.DefaultWindowSize.H}
	}
	if phys := platform.PhysicalSize(platform.Size{W: size.W, H: size.H}, cfg.scale); phys.X <= 0 || phys.Y <= 0 {
		return cfg, size, fmt.Errorf("empty image: %gx%g at scale %g; use telatest.Size and telatest.Scale to give the window a size", size.W, size.H, cfg.scale)
	}
	return cfg, size, nil
}

// recovered turns a panic into an error.
func recovered(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("the UI panicked: %v\n%s", r, debug.Stack())
	}
}

// Render mounts root, lays it out and paints it at the configured size and
// scale (default 800x600 at scale 1) and returns the image, in physical
// pixels over the root's background (white by default). Effects run as in
// a window and then clean up; state passive effects set is rendered only
// when the tree needs further frames.
func Render(root tela.Node, opts ...Option) (img *image.RGBA, err error) {
	cfg, size, err := headlessSetup(opts)
	if err != nil {
		return nil, err
	}
	defer recovered(&err)
	_, img = core.Render(root, core.RenderOptions{
		Config: cfg.coreConfig(),
		Width:  size.W, Height: size.H,
		Scale: float32(cfg.scale),
	})
	return img, nil
}

// AriaSnapshot mounts root and returns its semantics tree as a Playwright
// aria snapshot, without a trailing newline: the YAML `tela inspect --aria`
// prints.
func AriaSnapshot(root tela.Node, opts ...Option) (yaml string, err error) {
	cfg, size, err := headlessSetup(opts)
	if err != nil {
		return "", err
	}
	defer recovered(&err)
	t := framed(root, cfg, size)
	defer t.Unmount()
	return semantics.Snapshot(t.Semantics()), nil
}

// LayoutJSON mounts root and returns its layout tree as indented JSON, one
// object per box with its border box, edges, text and computed style
// (paragraphs also with the fragments of their inline elements; atomic
// inlines with inlineBox), ending with a newline: the output of `tela
// inspect --layout`. rect is the box as layout placed it; a box that is
// painted elsewhere, because of a transform or scrolled content, also has
// bounds, the same shape: the bounding box of its transformed border box in
// window coordinates, as tela.ElementRef.Bounds returns it.
func LayoutJSON(root tela.Node, opts ...Option) (js []byte, err error) {
	cfg, size, err := headlessSetup(opts)
	if err != nil {
		return nil, err
	}
	defer recovered(&err)
	t := framed(root, cfg, size)
	defer t.Unmount()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(t.Inspect()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// framed mounts root in a new tree of the given size and runs a frame.
func framed(root tela.Node, cfg config, size gfx.Size) *core.Tree {
	cc := cfg.coreConfig()
	cc.IDPrefix = "r-"
	t := core.NewTree(cc)
	t.SetViewport(size)
	t.SetScale(float32(cfg.scale))
	t.Mount(root)
	t.Frame()
	for i := 1; i < core.MaxRenderFrames && t.Dirty(); i++ {
		t.Frame()
	}
	return t
}
