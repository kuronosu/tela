package lint

import (
	"os"
	"time"
)

// fileID tells a version of a file on disk from another without reading
// it: its size, modification time and, where the system keeps one, the
// time of its last change of any kind. The change time cannot be set by a
// program, so a copy that keeps the modification time (cp -p, rsync -t)
// and the size is still a new version; Windows has none, and there such a
// copy is not seen (gopls trusts the modification time the same way).
type fileID struct {
	size   int64
	mod    int64
	change int64
}

// stampSlack is the coarsest modification time a file system is expected
// to keep (FAT's two seconds; HFS+ and some network file systems keep one).
// A file modified less than this before it is looked at may be written
// again with the same size and time, so what is known of it then is not
// trusted the next time.
const stampSlack = 2 * time.Second

// statFile returns the identity of a regular file, and whether it can be
// trusted later (the file was modified more than stampSlack ago).
func statFile(path string) (id fileID, trusted, ok bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return fileID{}, false, false
	}
	id = fileID{size: fi.Size(), mod: fi.ModTime().UnixNano(), change: changeTime(fi)}
	// A later write gets a modification time at least stampSlack after
	// this one, whatever the granularity: the change time need not be
	// old as well.
	return id, time.Since(fi.ModTime()) > stampSlack, true
}
