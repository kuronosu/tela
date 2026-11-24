//go:build !linux || (!amd64 && !arm64)

package main

import (
	"errors"

	"github.com/kuronosu/tela"
)

// newGPU returns an error: the scene's Vulkan renderer shares its buffers as
// dmabufs, which are Linux's, and was checked on 64-bit processors.
func newGPU(*tela.Texture, tela.TextureDevice, bool) (painter, error) {
	return nil, errors.New("the scene's GPU renderer runs on Linux")
}
