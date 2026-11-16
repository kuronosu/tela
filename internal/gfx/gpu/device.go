package gpu

import (
	"errors"
	"image"
	"time"

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

// ErrDeviceLost is returned by a device and its targets once the device is
// gone (a driver that was reset, a GPU that was removed): nothing more can
// be drawn with it. The caller closes it and renders with the CPU
// rasterizer, or opens a device again.
var ErrDeviceLost = errors.New("gpu: the device is lost")

// ErrSoftwareDevice is returned by Open, with Config.HardwareOnly, when
// the device chosen is a software renderer (a Vulkan device of the type
// CPU: lavapipe, llvmpipe, SwiftShader), which draws a page of glass far
// slower than the CPU rasterizer does (ADR-288). No device was created.
var ErrSoftwareDevice = errors.New("gpu: the device is a software renderer")

// Config configures Open.
type Config struct {
	// Device selects the physical device by index or by a part of its
	// name. Empty means the environment variable TELA_GPU_DEVICE, and
	// without it the device of DRM, else the first discrete device, else
	// the first integrated one (ADR-281, ADR-288).
	Device string
	// DRM, with HasDRM, is the device number of a DRM node of the device
	// the display server composites with (a window's dmabuf feedback):
	// the device that has that node, primary or render, is chosen, so
	// that its images are shown without a copy. It is ignored when Device
	// or TELA_GPU_DEVICE names a device, and when no device has the node.
	DRM    DRMNode
	HasDRM bool
	// HardwareOnly makes Open fail with ErrSoftwareDevice, before it
	// creates a device, when the one chosen is a software renderer: any
	// hardware device, integrated ones included, is used.
	HardwareOnly bool
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
	// atlases, the images and the offscreen targets) and the buffers it
	// grew for uploads. The frames that follow must come from an Encoder
	// that started over.
	Trim()
	// Rest applies a frame without passes that an Encoder made for
	// windows that rest (Encoder.Rest): the image textures and the
	// offscreen targets the frame gives up are destroyed, with the
	// device's own buffers of uploads and readbacks and the surface of its
	// shifts; the atlases and what the frame still lists stay. It returns
	// ErrResourcesLost, having released everything as Trim does, when the
	// device does not hold the resources of the frame's Encoder.
	Rest(f *Frame) error
	// Close destroys the device and the targets it still has.
	Close()
}

// A Profiler is a Device that can measure where the time of its frames
// goes, for the development of the renderer: a caller asks a Device for it
// with a type assertion. While profiling, every pass of a frame is a
// submission of its own and the instances of each class are drawn again,
// alone, into a surface apart, so frames take many times longer; their
// pixels are the same.
type Profiler interface {
	// SetProfiling turns the measurement on or off.
	SetProfiling(on bool)
	// TakeProfile returns what the frames drawn since the last call cost,
	// by class, and starts over.
	TakeProfile() []ProfileRow
}

// ProfileRow is what the draws of one class cost a device.
type ProfileRow struct {
	// Class names what was drawn: a kind of instance of the quad pipeline
	// (a fill, a shadow, coverage from the atlas, a layer...), a step of
	// the filter pipeline, or in parentheses a total: "(passes)" is every
	// pass as the frame draws it, "(submit)" what the submissions of the
	// measurement cost with nothing to draw.
	Class string
	// Runs counts the draws (or the passes), Pixels the pixels they cover
	// inside their scissors, and Texels the texels the steps of a filter
	// read (its pixels times its taps).
	Runs           int
	Pixels, Texels int64
	// Time is the time the device took, with the cost of an empty
	// submission taken away.
	Time time.Duration
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
	// ReadPixels copies the pixels of the rectangles into dst, at the same
	// coordinates. Only the pixels inside both the target and dst are
	// copied. The rectangles are read in one submission: reading the damage
	// of a frame with one call costs one wait, not one per rectangle.
	ReadPixels(dst *image.RGBA, rects ...image.Rectangle) error
	// Shift moves pixels inside the target, as a scroll does: every pixel
	// (x, y) of r takes the value that the pixel (x-dx, y-dy) had before
	// the call. The pixels of r whose source lies outside the target are
	// left as they are. Source and destination may overlap.
	Shift(r image.Rectangle, dx, dy int) error
	// Close destroys the target.
	Close()
}

// A Renderer renders display lists into targets: an Encoder, the buffers
// it reuses from frame to frame and the textures its frames keep on the
// device (the atlases, the images, the offscreen targets of layers and
// filters). A device mirrors the textures of one Renderer at a time, so the
// targets of a device share one Renderer; another Renderer on the same
// device works, and uploads again whenever the two alternate. The zero
// value is ready to use. It is not safe for concurrent use.
type Renderer struct {
	enc Encoder
	dev Device // of the last target
}

// Result describes one Render.
type Result struct {
	// Unsupported names what the list holds that this device cannot draw
	// (see Frame.Unsupported): something larger than its textures. When it
	// is not empty the target does not show the list's picture: the caller
	// draws with the CPU rasterizer instead.
	Unsupported []string
	// Instances and Draws count what the frame drew, and Passes the passes
	// it took: one, and more for each layer, filter and backdrop filter,
	// or when the frame needed more pictures than an atlas holds. Targets
	// is the number of offscreen targets the device holds after the frame.
	Instances, Draws, Passes, Targets int
	// Uploads counts the pictures the frame added to the atlases and the
	// image textures it created, and UploadBytes their bytes: both are 0
	// for a frame that draws what earlier frames drew.
	Uploads, UploadBytes int
	// Culled counts the commands of the list that were not drawn because
	// an opaque command painted after them covers what this render shows
	// of them, and Backdrops what the render did with the backdrop filters
	// of Options.Backdrops.
	Culled    int
	Backdrops BackdropStats
	// Encode is the time the Encoder took to turn the list into the frame
	// (on the calling goroutine), and Draw the time the device took to
	// execute it, the wait for the GPU included.
	Encode, Draw time.Duration
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
	t0 := time.Now()
	f := r.enc.Encode(dl, t.Size(), opts)
	t1 := time.Now()
	err := t.Draw(f)
	t2 := time.Now()
	encode, draw := t1.Sub(t0), t2.Sub(t1)
	if errors.Is(err, ErrResourcesLost) {
		r.enc.Trim()
		f = r.enc.Encode(dl, t.Size(), opts)
		t3 := time.Now()
		err = t.Draw(f)
		encode, draw = encode+t3.Sub(t2), draw+time.Since(t3)
	}
	if err != nil {
		// What the frame left in the cache's targets is not known.
		opts.Backdrops.Reset()
	}
	res := Result{Instances: len(f.Instances) / InstanceSize, Passes: len(f.Passes), Targets: len(f.Targets), Uploads: len(f.Images), UploadBytes: len(f.Data),
		Culled: r.enc.culled, Backdrops: r.enc.glass, Encode: encode, Draw: draw}
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

// Trim releases everything the Renderer keeps, on the device too: the
// next Render uploads what it draws with.
func (r *Renderer) Trim() {
	r.enc.Trim()
	if r.dev != nil {
		r.dev.Trim()
	}
}

// Rest releases what the Renderer keeps only to draw the next frame
// faster, on the device too, for windows that rest (Encoder.Rest): the
// offscreen targets, the buffers, and the textures of the images that none
// of lists, what the windows show, draws. The atlases, the images on
// screen and the cached backdrops stay, so the frame after a rest uploads
// nothing for what was drawn before it.
func (r *Renderer) Rest(lists ...*gfx.DisplayList) {
	if r.dev == nil {
		r.enc.Trim()
		return
	}
	f := r.enc.Rest(lists...)
	if f == nil {
		r.dev.Trim()
		return
	}
	if err := r.dev.Rest(f); err != nil {
		// The device holds nothing of the Encoder's any more.
		r.enc.Trim()
	}
}

// Memory describes what a Renderer holds on its device, in bytes.
type Memory struct {
	// Atlases is the pages of the coverage and color atlases, Images the
	// image textures, Targets the offscreen targets of the pool (layers,
	// the steps of filters), and Backdrops the filtered backdrops that
	// BackdropCaches keep in targets of their own.
	Atlases, Images, Targets, Backdrops int64
}

// Total returns the sum of m's parts.
func (m Memory) Total() int64 { return m.Atlases + m.Images + m.Targets + m.Backdrops }

// Memory returns what the Renderer holds on its device. The targets
// themselves (a window's frame) and the buffers of a swapchain are their
// owners' to count.
func (r *Renderer) Memory() Memory { return r.enc.Memory() }
