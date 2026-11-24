//go:build !linux

package core

import (
	"context"
	"fmt"
	"runtime"
)

// Dmabufs and sync files exist on Linux only.

func mappedFormats() []TextureFormat { return nil }

func mapDmabuf(d Dmabuf) (mappedTexture, error) {
	return nil, fmt.Errorf("tela: Texture.Import: %s: dmabufs exist on Linux, and this is %s; submit pixels with SubmitImage: %w", d.describe(), runtime.GOOS, ErrTextureUnsupported)
}

// syncFile is the Fence of SyncFile.
type syncFile struct{}

func newSyncFile(int) Fence { return syncFile{} }

// Wait implements Fence.
func (syncFile) Wait(context.Context) error {
	return fmt.Errorf("tela: SyncFile: sync files exist on Linux, and this is %s: %w", runtime.GOOS, ErrTextureUnsupported)
}
