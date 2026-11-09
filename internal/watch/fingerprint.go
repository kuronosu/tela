package watch

import (
	"encoding/binary"
	"hash/fnv"
	"io/fs"
	"sort"
)

// Fingerprint walks the trees at roots as the poller does, with the
// SkipDir and Filter of opts (Filter is called with dir false), and
// returns a hash of the path, size, modification time and mode of every
// file that passes the filter. Two fingerprints differ when a file that
// matters was added, removed or saved in between; like the poller, a save
// that keeps the size and the modification time is missed. It is how
// tela mcp decides, before each call, whether the sources changed since
// its build (ADR-174), without a watcher running in the background.
// Errors (an unreadable directory) leave that part of the tree out.
func Fingerprint(opts Options, roots ...string) uint64 {
	skip := func(path string) bool { return opts.SkipDir != nil && opts.SkipDir(path) }
	type entry struct {
		path string
		st   stamp
	}
	var files []entry
	for _, r := range roots {
		walkTree(r, skip, func(error) {}, nil, func(path string, d fs.DirEntry) {
			if opts.Filter != nil && !opts.Filter(path, false) {
				return
			}
			if fi, err := d.Info(); err == nil {
				files = append(files, entry{path, stamp{fi.Size(), fi.ModTime(), fi.Mode()}})
			}
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := fnv.New64a()
	var b [8]byte
	num := func(v uint64) {
		binary.LittleEndian.PutUint64(b[:], v)
		h.Write(b[:])
	}
	for i, f := range files {
		if i > 0 && files[i-1].path == f.path {
			continue // overlapping roots
		}
		h.Write([]byte(f.path))
		h.Write([]byte{0})
		num(uint64(f.st.size))
		num(uint64(f.st.mod.UnixNano()))
		num(uint64(f.st.mode))
	}
	return h.Sum64()
}
