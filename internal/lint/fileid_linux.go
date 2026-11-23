package lint

import (
	"os"
	"syscall"
)

// changeTime returns the status change time of a file (st_ctim), or 0.
func changeTime(fi os.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ctim.Nano()
	}
	return 0
}
