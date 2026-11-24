//go:build telagpu && windows

package gpu

import "github.com/kuronosu/tela/internal/vk"

// Imported buffers are Linux's dmabufs (import.go): on Windows the device
// is not an Importer, no Imported of it exists and no frame names one.

type importCache struct{}

func (c *importCache) forget() {}

// vkImported is never created here.
type vkImported struct {
	kind imageKind
	view vk.ImageView
	set  vk.DescriptorSet
}

func (m *vkImported) open() bool     { return false }
func (m *vkImported) pixels() []byte { return nil }
func (m *vkImported) close()         {}
func (m *vkImported) destroy()       {}

func (d *vkDevice) checkImports(*Frame) error                          { return nil }
func (d *vkDevice) takeImports(vk.CommandBuffer, *Frame)               {}
func (d *vkDevice) giveImports(vk.CommandBuffer, *Frame)               {}
func (d *vkDevice) copyImport(vk.CommandBuffer, *vkImported, vk.Image) {}
