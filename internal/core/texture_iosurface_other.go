//go:build !darwin

package core

import (
	"fmt"
	"runtime"
)

// IOSurfaces and Metal exist on macOS only.

func metalRegistryID() uint64 { return 0 }

// mapIOSurface returns the copy of an IOSurface, and its size. Tests replace
// it.
var mapIOSurface = func(IOSurface) (mappedTexture, int, int, error) {
	return nil, 0, 0, fmt.Errorf("tela: Texture.Import: an IOSurface: they exist on macOS, and this is %s; submit pixels with SubmitImage: %w", runtime.GOOS, ErrTextureUnsupported)
}
