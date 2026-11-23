//go:build !linux && !darwin

package lint

import "os"

// changeTime is 0: the system keeps no change time that os.FileInfo
// exposes (Windows).
func changeTime(os.FileInfo) int64 { return 0 }
