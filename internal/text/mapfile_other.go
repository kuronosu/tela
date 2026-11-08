//go:build !unix && !windows

package text

import (
	"errors"
	"os"
)

// mapFile is not available here (js, wasip1, plan9): readFontFile reads
// the file instead.
func mapFile(*os.File, int) ([]byte, error) {
	return nil, errors.New("text: memory-mapped files are not supported on this platform")
}

func unmapFile([]byte) error { return nil }
