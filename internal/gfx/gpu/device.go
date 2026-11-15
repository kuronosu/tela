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

// ErrResourcesLost is returned by Target.Draw for a frame that draws with
// textures the device does not hold as the frame's Encoder left them: a
// frame of the Encoder was not drawn, another Encoder drew in between, or
// the device was trimmed. Nothing is drawn. The Encoder starts over
// (Encoder.Trim) and encodes the frame again; Renderer does that.
var ErrResourcesLost = errors.New("gpu: the device does not hold the resources of the frame's encoder")

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
	// Trim releases the textures the device keeps for its frames (the
	// atlases and the images) and the buffers it grew for uploads. The
	// frames that follow must come from an Encoder that started over.
	Trim()
	// Close destroys the device and the targets it still has.
	Close()
}

// A Target is a surface a Device draws into: premultiplied RGBA8 holding
// sRGB-encoded values, blended as they are (ADR-007).
type Target interface {
	// Size returns the size in device pixels.
	Size() image.Point
	// Device returns the device of the target.
	Device() Device
	// Draw executes a frame encoded for this size and returns when the
	// pixels are drawn. It returns ErrResourcesLost, having drawn nothing,
	// when the device does not hold what the frame draws with.
	Draw(f *Frame) error
	// ReadPixels copies the pixels of r into dst, at the same
	// coordinates. Only the pixels of r inside both the target and dst
	// are copied.
	ReadPixels(dst *image.RGBA, r image.Rectangle) error
	// Close destroys the target.
	Close()
}

// A Renderer renders display lists into targets: an Encoder, the buffers
// it reuses from frame to frame and the textures its frames keep on the
// device. A device mirrors the textures of one Renderer at a time, so the
// targets of a device share one Renderer; another Renderer on the same
// device works, and uploads again whenever the two alternate. The zero
// value is ready to use. It is not safe for concurrent use.
type Renderer struct {
	enc Encoder
	dev Device // of the last target
}

// Result describes one Render.
type Result struct {
	// Unsupported names what the list holds that was not drawn (see
	// Frame.Unsupported). When it is not empty the target does not show
	// the list's picture: the caller draws with the CPU rasterizer
	// instead.
	Unsupported []string
	// Instances and Draws count what the frame drew, and Passes the passes
	// it took: one, unless the frame needed more pictures than an atlas
	// holds.
	Instances, Draws, Passes int
	// Uploads counts the pictures the frame added to the atlases and the
	// image textures it created, and UploadBytes their bytes: both are 0
	// for a frame that draws what earlier frames drew.
	Uploads, UploadBytes int
}

// Render draws dl into t, over the pixels t holds (after clearing the
// region when opts.Clear is set), like raster.Render into an image. A nil
// list draws nothing.
func (r *Renderer) Render(t Target, dl *gfx.DisplayList, opts Options) (Result, error) {
	if dev := t.Device(); dev != r.dev {
		// The textures are on another device: start over on this one.
		if r.dev != nil {
			r.enc.Trim()
		}
		r.dev = dev
	}
	if r.enc.Limits.MaxTextureSize == 0 && r.dev != nil {
		r.enc.Limits.MaxTextureSize = r.dev.MaxTargetSize()
	}
	f := r.enc.Encode(dl, t.Size(), opts)
	err := t.Draw(f)
	if errors.Is(err, ErrResourcesLost) {
		r.enc.Trim()
		f = r.enc.Encode(dl, t.Size(), opts)
		err = t.Draw(f)
	}
	res := Result{Instances: len(f.Instances) / InstanceSize, Passes: len(f.Passes), Uploads: len(f.Images), UploadBytes: len(f.Data)}
	for i := range f.Passes {
		res.Draws += len(f.Passes[i].Batches)
		res.Uploads += len(f.Passes[i].Uploads)
	}
	for i := range f.Images {
		res.UploadBytes += len(f.Images[i].Pix)
	}
	if len(f.Unsupported) > 0 {
		res.Unsupported = append([]string(nil), f.Unsupported...)
	}
	return res, err
}

// EndFrame ends a frame: the Renders since the previous EndFrame. What no
// frame drew with for a while is given up (Encoder.EndFrame), and the
// device releases it when the next frame is drawn. The owner of a Renderer
// that draws frames calls it once per frame.
func (r *Renderer) EndFrame() { r.enc.EndFrame() }

// Trim releases everything the Renderer keeps, on the device too, for a
// window that rests: the next Render uploads what it draws with.
func (r *Renderer) Trim() {
	r.enc.Trim()
	if r.dev != nil {
		r.dev.Trim()
	}
}
