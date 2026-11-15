package gpu

import (
	"errors"
	"image"

	"github.com/kuronosu/tela/internal/gfx"
)

// ErrUnavailable is returned by Open when the program has no GPU device
// backend: it was built without the tag telagpu, or for a system whose
// backend does not exist yet (macOS).
var ErrUnavailable = errors.New("gpu: no GPU backend in this build (it needs the build tag telagpu, on Linux or Windows)")

// Config configures Open.
type Config struct {
	// Device selects the physical device by index or by a part of its
	// name. Empty means the environment variable TELA_GPU_DEVICE, and
	// without it the first discrete device (ADR-281).
	Device string
	// Validate enables the API's validation layer (Vulkan:
	// VK_LAYER_KHRONOS_validation); Open fails when it is not installed.
	Validate bool
	// Log, when not nil, receives the messages of the driver and of the
	// validation layer. It may be called from any goroutine.
	Log func(msg string)
}

// A Device executes frames with one graphics API on one physical device.
// It is not safe for concurrent use, and neither are its targets.
type Device interface {
	// Name describes the physical device.
	Name() string
	// MaxTargetSize is the largest width and height of a target.
	MaxTargetSize() int
	// NewTarget creates a surface of the given size in device pixels, at
	// least 1x1, transparent. It keeps its pixels between frames, as the
	// destination image of the CPU rasterizer does.
	NewTarget(width, height int) (Target, error)
	// Close destroys the device and the targets it still has.
	Close()
}

// A Target is a surface a Device draws into: premultiplied RGBA8 holding
// sRGB-encoded values, blended as they are (ADR-007).
type Target interface {
	// Size returns the size in device pixels.
	Size() image.Point
	// Draw executes a frame encoded for this size and returns when the
	// pixels are drawn.
	Draw(f *Frame) error
	// ReadPixels copies the pixels of r into dst, at the same
	// coordinates. Only the pixels of r inside both the target and dst
	// are copied.
	ReadPixels(dst *image.RGBA, r image.Rectangle) error
	// Close destroys the target.
	Close()
}

// A Renderer renders display lists into targets: an Encoder and the
// buffers it reuses from frame to frame. The zero value is ready to use.
// It is not safe for concurrent use.
type Renderer struct {
	enc Encoder
}

// Result describes one Render.
type Result struct {
	// Unsupported names what the list holds that was not drawn (see
	// Frame.Unsupported). When it is not empty the target does not show
	// the list's picture: the caller draws with the CPU rasterizer
	// instead.
	Unsupported []string
	// Instances and Draws count what the frame drew.
	Instances, Draws int
}

// Render draws dl into t, over the pixels t holds (after clearing the
// region when opts.Clear is set), like raster.Render into an image. A nil
// list draws nothing.
func (r *Renderer) Render(t Target, dl *gfx.DisplayList, opts Options) (Result, error) {
	f := r.enc.Encode(dl, t.Size(), opts)
	res := Result{Instances: len(f.Instances) / InstanceSize}
	for i := range f.Passes {
		res.Draws += len(f.Passes[i].Batches)
	}
	if len(f.Unsupported) > 0 {
		res.Unsupported = append([]string(nil), f.Unsupported...)
	}
	return res, t.Draw(f)
}
