//go:build amd64 || arm64

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"sync"
	"unsafe"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/texture/vulkan"
)

//go:generate glslangValidator --quiet -V -o shaders/scene.vert.spv shaders/scene.vert
//go:generate glslangValidator --quiet -V -o shaders/scene.frag.spv shaders/scene.frag

var (
	//go:embed shaders/scene.vert.spv
	vertSPV []byte
	//go:embed shaders/scene.frag.spv
	fragSPV []byte
)

// What the tests replace: how Vulkan is opened, and who hears that a painter
// closed.
var (
	openInstance = vulkan.Open
	onClose      = func() {}
)

// gpu is the painter that draws with Vulkan, into dmabufs that it imported
// into the texture once each, and submits them with sync files: the pixels
// are never the CPU's.
//
// Everything but the Release of a frame runs on the loop's goroutine (and
// then on the one of close), so nothing of Vulkan needs a lock.
type gpu struct {
	tex  *tela.Texture
	inst *vulkan.Instance // nil if it is not the painter's
	r    *vulkan.Renderer
	// modifier is the layout of its buffers.
	modifier uint64
	// w and h are the size of its frames. idle are the buffers that are
	// free, held those Tela has.
	w, h int
	idle []*gpuBuffer
	held int

	mu       sync.Mutex
	released []*gpuBuffer  // what Tela let go of since the loop looked
	wake     chan struct{} // it let go of one
}

// gpuBuffers is how many buffers of a size there are: with fences, Tela may
// hold four frames.
const gpuBuffers = 5

// gpuBuffer is a buffer, and its import.
type gpuBuffer struct {
	target *vulkan.Target
	buf    *tela.TextureBuffer
}

// newGPU opens the painter of the GPU that dev is, or, with other, of another
// one.
func newGPU(tex *tela.Texture, dev tela.TextureDevice, other bool) (painter, error) {
	inst, err := openInstance()
	if err != nil {
		return nil, err
	}
	r, err := newRenderer(inst, dev, other)
	if err != nil {
		inst.Close()
		return nil, err
	}
	return newPainter(tex, inst, r), nil
}

func newPainter(tex *tela.Texture, inst *vulkan.Instance, r *vulkan.Renderer) *gpu {
	return &gpu{tex: tex, inst: inst, r: r, wake: make(chan struct{}, 1)}
}

// newRenderer makes a renderer on the GPU that dev is, or, with other, on the
// first other one that works.
func newRenderer(inst *vulkan.Instance, dev tela.TextureDevice, other bool) (*vulkan.Renderer, error) {
	devices, err := inst.Devices()
	if err != nil {
		return nil, err
	}
	formats, readBy := dev.Formats, vulkan.Own
	if other {
		formats, readBy = dev.ForeignFormats, vulkan.Shared
	}
	if !dev.GPU {
		readBy = vulkan.Mapped
	}
	var why error
	for _, d := range devices {
		if isDevice(d, dev) == other {
			continue
		}
		var r *vulkan.Renderer
		if r, err = openRenderer(inst, d, formats, readBy); err == nil {
			return r, nil
		}
		why = errors.Join(why, err)
	}
	switch {
	case why != nil:
		return nil, why
	case other:
		return nil, errors.New("no other GPU has Vulkan")
	}
	return nil, fmt.Errorf("no Vulkan device is %s", dev.Name)
}

func openRenderer(inst *vulkan.Instance, d vulkan.Device, formats []tela.TextureFormat, readBy vulkan.Memory) (*vulkan.Renderer, error) {
	take := make([]vulkan.Format, len(formats))
	for i, f := range formats {
		take[i] = vulkan.Format(f)
	}
	return inst.NewRenderer(d, take, readBy, vertSPV, fragSPV, int(unsafe.Sizeof(uniforms{})))
}

// isDevice reports whether d is the GPU that draws Tela's windows: by its
// UUID, else by its render node.
func isDevice(d vulkan.Device, dev tela.TextureDevice) bool {
	switch {
	case !dev.GPU:
		return false
	case dev.UUID != [16]byte{}:
		return d.UUID == dev.UUID
	}
	return dev.RenderMajor != 0 && d.Major == int64(dev.RenderMajor) && d.Minor == int64(dev.RenderMinor)
}

// bytes returns the push constants.
func (u *uniforms) bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(u)), unsafe.Sizeof(*u))
}

func (g *gpu) about() string {
	how := "drawn in place"
	if g.r.Copies {
		how = "copied into linear memory"
	}
	f, layout := g.r.Fourcc, "linear"
	if g.modifier != vulkan.Linear {
		layout = fmt.Sprintf("modifier 0x%x", g.modifier)
	}
	return fmt.Sprintf("%s, %c%c%c%c, %s, %s", g.r.Device.Name, f&0xff, f>>8&0xff, f>>16&0xff, f>>24, layout, how)
}

func (g *gpu) limit() int { return min(g.r.Device.MaxSize, frameLimit) }

func (g *gpu) paint(ctx context.Context, u *uniforms) error {
	b, err := g.buffer(ctx, int(u.Size[0]), int(u.Size[1]))
	if err != nil {
		return err
	}
	fd, err := b.target.Draw(u.bytes())
	if err != nil {
		g.idle = append(g.idle, b)
		return err
	}
	g.held++
	return g.tex.Submit(b.buf, tela.TextureFrame{
		Fence:   tela.SyncFile(fd),
		Release: func() { g.release(b) },
	})
}

// release takes back a buffer that Tela does not read any more. It runs on a
// goroutine of Tela's.
func (g *gpu) release(b *gpuBuffer) {
	g.mu.Lock()
	g.released = append(g.released, b)
	g.mu.Unlock()
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

// collect makes the buffers Tela let go of idle.
func (g *gpu) collect() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.idle = append(g.idle, g.released...)
	g.held -= len(g.released)
	clear(g.released)
	g.released = g.released[:0]
}

// returned tells that Tela let go of a buffer, for trim.
func (g *gpu) returned() <-chan struct{} { return g.wake }

// trim frees the idle buffers that are not of the size of the frames.
func (g *gpu) trim() {
	g.collect()
	g.idle = slices.DeleteFunc(g.idle, func(b *gpuBuffer) bool {
		old := b.target.Width != g.w || b.target.Height != g.h
		if old {
			b.close()
		}
		return old
	})
}

// buffer returns a buffer of w by h pixels that Tela does not read.
func (g *gpu) buffer(ctx context.Context, w, h int) (*gpuBuffer, error) {
	g.w, g.h = w, h
	for {
		g.trim()
		if n := len(g.idle); n > 0 {
			b := g.idle[n-1]
			g.idle = g.idle[:n-1]
			return b, nil
		}
		// What Tela holds of another size makes room soon: the count is of
		// all of them.
		if g.held < gpuBuffers {
			return g.newBuffer(w, h)
		}
		select {
		case <-g.wake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// newBuffer allocates a buffer and imports it into the texture.
func (g *gpu) newBuffer(w, h int) (*gpuBuffer, error) {
	t, err := g.r.NewTarget(w, h)
	if err != nil {
		return nil, err
	}
	d := tela.Dmabuf{Width: w, Height: h, Fourcc: g.r.Fourcc, Modifier: t.Modifier}
	for _, p := range t.Planes {
		d.Planes = append(d.Planes, tela.DmabufPlane{FD: t.FD, Offset: p.Offset, Stride: p.Stride})
	}
	buf, err := g.tex.Import(d)
	if err != nil {
		t.Close()
		return nil, err
	}
	g.modifier = t.Modifier
	return &gpuBuffer{target: t, buf: buf}, nil
}

func (b *gpuBuffer) close() {
	b.buf.Close()
	b.target.Close()
}

func (g *gpu) close() {
	for {
		g.collect()
		for _, b := range g.idle {
			b.close()
		}
		g.idle = nil
		if g.held == 0 {
			break
		}
		<-g.wake
	}
	g.r.Close()
	if g.inst != nil {
		g.inst.Close()
	}
	onClose()
}
