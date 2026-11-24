//go:build telagpu && (amd64 || arm64)

package main

import (
	"fmt"
	"image"
	"os"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/texture/vulkan"
	telagpu "github.com/kuronosu/tela/internal/gfx/gpu"
)

// TestTelasRendererReadsTheFrames imports the frames of every GPU into Tela's
// renderer on every GPU (the one of the frames and another one), in the
// formats that renderer says it takes, and compares the pixels it reads with
// those of the software renderer: nothing of them went through the CPU.
func TestTelasRendererReadsTheFrames(t *testing.T) {
	eachDevice(t, func(t *testing.T, inst *vulkan.Instance, d vulkan.Device) {
		for i := range 4 {
			dev, err := telagpu.Open(telagpu.Config{Device: fmt.Sprint(i), HardwareOnly: true, Validate: os.Getenv("TELA_VK_VALIDATE") == "1",
				Log: func(m string) { t.Errorf("Tela's renderer: %s", m) }})
			if err != nil {
				break // the devices end
			}
			t.Run("on "+dev.Name(), func(t *testing.T) {
				defer dev.Close()
				imp, ok := dev.(telagpu.Importer)
				if !ok {
					t.Skip("the device imports nothing")
				}
				own, foreign := imp.ImportFormats()
				id := imp.DeviceID()
				td := tela.TextureDevice{GPU: true, Name: id.Name, UUID: id.UUID}
				for _, f := range own {
					td.Formats = append(td.Formats, tela.TextureFormat{Fourcc: f.Fourcc, Modifier: f.Modifier})
				}
				for _, f := range foreign {
					td.ForeignFormats = append(td.ForeignFormats, tela.TextureFormat{Fourcc: f.Fourcc, Modifier: f.Modifier})
				}
				formats, readBy := td.Formats, vulkan.Own
				if !isDevice(d, td) {
					formats, readBy = td.ForeignFormats, vulkan.Shared
				}
				r := renderer(t, inst, d, formats, readBy)
				defer r.Close()
				target, err := r.NewTarget(640, 360)
				if err != nil {
					t.Fatal(err)
				}
				defer target.Close()
				desc := telagpu.DmabufDescriptor{Width: 640, Height: 360, Fourcc: r.Fourcc, Modifier: target.Modifier}
				for _, p := range target.Planes {
					desc.Planes = append(desc.Planes, telagpu.DmabufPlane{FD: target.FD, Offset: p.Offset, Stride: p.Stride})
				}
				m, err := imp.Import(desc)
				if err != nil {
					t.Fatal(err)
				}
				defer m.Close()
				t.Logf("modifier %#x, planes %v", target.Modifier, target.Planes)
				for _, at := range []time.Duration{4 * time.Second, 20 * time.Second} {
					u := sceneAt(at, 640, 360)
					fd, err := target.Draw(u.bytes())
					if err != nil {
						t.Fatal(err)
					}
					if err := tela.SyncFile(fd).Wait(t.Context()); err != nil {
						t.Fatal(err)
					}
					want := image.NewRGBA(image.Rect(0, 0, 640, 360))
					drawScene(want, &u)
					compare(t, &image.RGBA{Pix: m.Pixels(), Stride: 4 * 640, Rect: want.Rect}, want)
				}
			})
		}
	})
}
