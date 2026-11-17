package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/bits"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one file of the image.
type Entry struct {
	// Path is the slash-separated path in the image, without a leading
	// slash ("usr/bin/notes"). "." is the root folder, whose mode an entry
	// may set (0755 by default).
	Path string
	// Mode is the type (fs.ModeDir, fs.ModeSymlink, or neither for a
	// regular file) and the permission bits, setuid, setgid and sticky
	// included. Other type bits are an error.
	Mode fs.FileMode
	// Data is a regular file's content.
	Data []byte
	// Target is a symbolic link's target.
	Target string
}

// Options are the image's settings.
type Options struct {
	// ModTime is the time of every inode and of the image; it must lie
	// between 1970 and 2106 (a 32-bit count of seconds).
	ModTime time.Time
	// BlockSize is the data block size: a power of two from 4 KiB to
	// 1 MiB; 0 is 128 KiB, mksquashfs's default.
	BlockSize int
}

// Level is the zlib compression level of the blocks: 9, mksquashfs's
// default for gzip.
const Level = zlib.BestCompression

// node is a file of the tree being written.
type node struct {
	name     string
	mode     fs.FileMode
	data     []byte
	target   string
	explicit bool
	children []*node
	byName   map[string]*node

	number uint32 // inode number
	ref    uint64 // inode reference: metadata block << 16 | offset
	// The data blocks of a regular file: where the first starts and the
	// on-disk size word of each.
	start uint64
	sizes []uint32
}

func (n *node) isDir() bool { return n.mode&fs.ModeDir != 0 }

// Write writes the image of entries to w, padded to a multiple of 4 KiB as
// mksquashfs does (the padding is not part of the file system's size).
func Write(w io.Writer, entries []Entry, opt Options) error {
	img, err := Build(entries, opt)
	if err != nil {
		return err
	}
	_, err = w.Write(img)
	return err
}

// Build returns the image Write writes.
func Build(entries []Entry, opt Options) ([]byte, error) {
	bs := opt.BlockSize
	if bs == 0 {
		bs = defaultBlock
	}
	if bs < 4096 || bs > 1<<20 || bs&(bs-1) != 0 {
		return nil, fmt.Errorf("squashfs: block size %d is not a power of two from 4 KiB to 1 MiB", bs)
	}
	sec := opt.ModTime.Unix()
	if sec < 0 || sec > math.MaxUint32 {
		return nil, fmt.Errorf("squashfs: time %v is outside 1970 to 2106", opt.ModTime)
	}
	root, err := tree(entries)
	if err != nil {
		return nil, err
	}
	w := &writer{bs: bs, mtime: uint32(sec)}
	return w.write(root)
}

// tree builds the folder tree of entries.
func tree(entries []Entry) (*node, error) {
	root := &node{mode: fs.ModeDir | 0o755, byName: map[string]*node{}}
	for _, e := range entries {
		if e.Path == "." {
			if !e.Mode.IsDir() {
				return nil, fmt.Errorf("squashfs: the root %q must be a folder", e.Path)
			}
			root.mode, root.explicit = fs.ModeDir|e.Mode.Perm()|e.Mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky), true
			continue
		}
		if err := checkPath(e.Path); err != nil {
			return nil, err
		}
		if e.Mode&fs.ModeType&^(fs.ModeDir|fs.ModeSymlink) != 0 || e.Mode&fs.ModeDir != 0 && e.Mode&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("squashfs: %s: mode %v is not a regular file, a folder or a symbolic link", e.Path, e.Mode)
		}
		parts := strings.Split(e.Path, "/")
		dir := root
		for _, p := range parts[:len(parts)-1] {
			c := dir.byName[p]
			if c == nil {
				c = &node{name: p, mode: fs.ModeDir | 0o755, byName: map[string]*node{}}
				dir.byName[p] = c
				dir.children = append(dir.children, c)
			} else if !c.isDir() {
				return nil, fmt.Errorf("squashfs: %s: %s is not a folder", e.Path, p)
			}
			dir = c
		}
		name := parts[len(parts)-1]
		perm := e.Mode.Perm() | e.Mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky)
		if c := dir.byName[name]; c != nil {
			if c.explicit || !c.isDir() || !e.Mode.IsDir() {
				return nil, fmt.Errorf("squashfs: %s is given twice", e.Path)
			}
			c.mode, c.explicit = fs.ModeDir|perm, true
			continue
		}
		c := &node{name: name, mode: e.Mode&(fs.ModeDir|fs.ModeSymlink) | perm, explicit: true}
		switch {
		case c.isDir():
			c.byName = map[string]*node{}
		case c.mode&fs.ModeSymlink != 0:
			if e.Target == "" || len(e.Target) > 4096 || strings.IndexByte(e.Target, 0) >= 0 {
				return nil, fmt.Errorf("squashfs: %s: the link target %q is empty, too long or has a NUL", e.Path, e.Target)
			}
			c.target = e.Target
		default:
			c.data = e.Data
		}
		dir.byName[name] = c
		dir.children = append(dir.children, c)
	}
	sortTree(root)
	return root, nil
}

// checkPath reports whether p is a clean relative path of valid names.
func checkPath(p string) error {
	if p == "" || path.Clean(p) != p || strings.HasPrefix(p, "/") || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("squashfs: %q is not a clean relative path", p)
	}
	for _, part := range strings.Split(p, "/") {
		if len(part) > nameMax || strings.IndexByte(part, 0) >= 0 {
			return fmt.Errorf("squashfs: %q has a name longer than %d bytes or a NUL", p, nameMax)
		}
	}
	return nil
}

// sortTree orders every folder's entries by name, byte by byte, as the
// format requires.
func sortTree(n *node) {
	sort.Slice(n.children, func(i, j int) bool { return n.children[i].name < n.children[j].name })
	for _, c := range n.children {
		if c.isDir() {
			sortTree(c)
		}
	}
}

// writer is the state of one image.
type writer struct {
	bs     int
	mtime  uint32
	count  uint32 // inodes numbered so far
	order  []*node
	inodes metaWriter
	dirs   metaWriter
}

// number gives every inode its number in the order they are written: the
// entries of a folder, each folder after its own entries, so the root is
// last (as mksquashfs numbers them).
func (w *writer) number(d *node) {
	for _, c := range d.children {
		if c.isDir() {
			w.number(c)
		} else {
			w.count++
			c.number = w.count
			w.order = append(w.order, c)
		}
	}
	w.count++
	d.number = w.count
	w.order = append(w.order, d)
}

func (w *writer) write(root *node) ([]byte, error) {
	w.number(root)
	img := make([]byte, superSize, superSize+1<<20)
	img = w.writeData(img)

	if err := w.writeInodes(root, root.number+1); err != nil {
		return nil, err
	}
	w.inodes.flush()
	w.dirs.flush()
	inodeStart := uint64(len(img))
	img = append(img, w.inodes.out...)
	dirStart := uint64(len(img))
	img = append(img, w.dirs.out...)
	// No fragments: the fragment table is empty and starts where the
	// next table does, as unsquashfs checks.
	fragStart := uint64(len(img))
	var ids metaWriter
	ids.write(make([]byte, 4)) // one ID: 0, the root user and group
	ids.flush()
	idBlock := uint64(len(img))
	img = append(img, ids.out...)
	idStart := uint64(len(img))
	img = binary.LittleEndian.AppendUint64(img, idBlock)
	used := uint64(len(img))

	sb := img[:superSize]
	le := binary.LittleEndian
	le.PutUint32(sb[0:], magic)
	le.PutUint32(sb[4:], w.count)
	le.PutUint32(sb[8:], w.mtime)
	le.PutUint32(sb[12:], uint32(w.bs))
	le.PutUint32(sb[16:], 0) // fragments
	le.PutUint16(sb[20:], zlibID)
	le.PutUint16(sb[22:], uint16(bits.TrailingZeros(uint(w.bs))))
	le.PutUint16(sb[24:], flagNoFrag|flagNoXattr)
	le.PutUint16(sb[26:], 1) // IDs
	le.PutUint16(sb[28:], 4)
	le.PutUint16(sb[30:], 0)
	le.PutUint64(sb[32:], root.ref)
	le.PutUint64(sb[40:], used)
	le.PutUint64(sb[48:], idStart)
	le.PutUint64(sb[56:], invalid) // extended attributes
	le.PutUint64(sb[64:], inodeStart)
	le.PutUint64(sb[72:], dirStart)
	le.PutUint64(sb[80:], fragStart)
	le.PutUint64(sb[88:], invalid) // export table

	if r := len(img) % padTo; r != 0 {
		img = append(img, make([]byte, padTo-r)...)
	}
	return img, nil
}

// writeData appends the data blocks of every regular file, in inode
// order, compressing the blocks in parallel.
func (w *writer) writeData(img []byte) []byte {
	type job struct {
		data []byte
		out  []byte
	}
	var jobs []*job
	var files []*node
	for _, n := range w.order {
		if n.isDir() || n.mode&fs.ModeSymlink != 0 {
			continue
		}
		files = append(files, n)
		for off := 0; off < len(n.data); off += w.bs {
			jobs = append(jobs, &job{data: n.data[off:min(off+w.bs, len(n.data))]})
		}
	}
	work := make(chan *job)
	var wg sync.WaitGroup
	for range max(1, min(runtime.GOMAXPROCS(0), len(jobs))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range work {
				j.out = compress(j.data)
			}
		}()
	}
	for _, j := range jobs {
		work <- j
	}
	close(work)
	wg.Wait()
	i := 0
	for _, n := range files {
		n.start = uint64(len(img))
		for off := 0; off < len(n.data); off += w.bs {
			j := jobs[i]
			i++
			if len(j.out) < len(j.data) {
				img = append(img, j.out...)
				n.sizes = append(n.sizes, uint32(len(j.out)))
			} else {
				img = append(img, j.data...)
				n.sizes = append(n.sizes, uint32(len(j.data))|blockRaw)
			}
		}
	}
	return img
}

// compress returns b compressed with zlib.
func compress(b []byte) []byte {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, Level)
	zw.Write(b)
	zw.Close()
	return buf.Bytes()
}

// writeInodes writes the inodes of folder d's tree and d's listing, in
// the order of number; parent is the number of d's parent.
func (w *writer) writeInodes(d *node, parent uint32) error {
	for _, c := range d.children {
		if c.isDir() {
			if err := w.writeInodes(c, d.number); err != nil {
				return err
			}
			continue
		}
		c.ref = w.inodes.ref()
		if c.mode&fs.ModeSymlink != 0 {
			b := w.header(typeSymlink, c)
			b = le32(b, 1)
			b = le32(b, uint32(len(c.target)))
			b = append(b, c.target...)
			w.inodes.write(b)
			continue
		}
		size := uint64(len(c.data))
		if c.start <= math.MaxUint32 && size <= math.MaxUint32 {
			b := w.header(typeFile, c)
			b = le32(b, uint32(c.start))
			b = le32(b, noFragment)
			b = le32(b, 0)
			b = le32(b, uint32(size))
			for _, s := range c.sizes {
				b = le32(b, s)
			}
			w.inodes.write(b)
			continue
		}
		b := w.header(typeLFile, c)
		b = binary.LittleEndian.AppendUint64(b, c.start)
		b = binary.LittleEndian.AppendUint64(b, size)
		b = binary.LittleEndian.AppendUint64(b, 0) // sparse bytes
		b = le32(b, 1)                             // links
		b = le32(b, noFragment)
		b = le32(b, 0)
		b = le32(b, ^uint32(0)) // no extended attributes
		for _, s := range c.sizes {
			b = le32(b, s)
		}
		w.inodes.write(b)
	}

	// The listing: runs of at most 256 entries whose inodes start in the
	// same metadata block and whose numbers are within an int16 of the
	// run's first.
	lblock, loff := w.dirs.pos()
	var listing []byte
	subdirs := uint32(0)
	for i := 0; i < len(d.children); {
		first := d.children[i]
		j := i + 1
		for j < len(d.children) && j-i < dirCountMax {
			c := d.children[j]
			delta := int64(c.number) - int64(first.number)
			if c.ref>>16 != first.ref>>16 || delta < math.MinInt16 || delta > math.MaxInt16 {
				break
			}
			j++
		}
		listing = le32(listing, uint32(j-i-1))
		listing = le32(listing, uint32(first.ref>>16))
		listing = le32(listing, first.number)
		for _, c := range d.children[i:j] {
			listing = le16(listing, uint16(c.ref&0xffff))
			listing = le16(listing, uint16(int16(int64(c.number)-int64(first.number))))
			listing = le16(listing, basicType(c))
			listing = le16(listing, uint16(len(c.name)-1))
			listing = append(listing, c.name...)
		}
		i = j
	}
	for _, c := range d.children {
		if c.isDir() {
			subdirs++
		}
	}
	w.dirs.write(listing)
	size := uint64(len(listing)) + 3 // "." and "..", as the format counts them
	if size > math.MaxUint32 {
		return fmt.Errorf("squashfs: the folder %q lists too much", d.name)
	}

	d.ref = w.inodes.ref()
	if size <= math.MaxUint16 {
		b := w.header(typeDir, d)
		b = le32(b, lblock)
		b = le32(b, 2+subdirs)
		b = le16(b, uint16(size))
		b = le16(b, loff)
		b = le32(b, parent)
		w.inodes.write(b)
		return nil
	}
	b := w.header(typeLDir, d)
	b = le32(b, 2+subdirs)
	b = le32(b, uint32(size))
	b = le32(b, lblock)
	b = le32(b, parent)
	b = le16(b, 0) // no index
	b = le16(b, loff)
	b = le32(b, ^uint32(0)) // no extended attributes
	w.inodes.write(b)
	return nil
}

// header returns the common inode header of n.
func (w *writer) header(typ uint16, n *node) []byte {
	b := make([]byte, 0, 64)
	b = le16(b, typ)
	b = le16(b, modeBits(n.mode))
	b = le16(b, 0) // owner: ID 0
	b = le16(b, 0) // group: ID 0
	b = le32(b, w.mtime)
	b = le32(b, n.number)
	return b
}

// modeBits returns the 12 permission bits of the inode's mode field.
func modeBits(m fs.FileMode) uint16 {
	v := uint16(m.Perm())
	if m&fs.ModeSetuid != 0 {
		v |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		v |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		v |= 0o1000
	}
	return v
}

// basicType is the type a listing gives an entry: always the basic one.
func basicType(n *node) uint16 {
	switch {
	case n.isDir():
		return typeDir
	case n.mode&fs.ModeSymlink != 0:
		return typeSymlink
	}
	return typeFile
}

func le16(b []byte, v uint16) []byte { return binary.LittleEndian.AppendUint16(b, v) }
func le32(b []byte, v uint32) []byte { return binary.LittleEndian.AppendUint32(b, v) }

// metaWriter writes a metadata table: 8 KiB blocks, each compressed
// unless that makes it larger, after a 16-bit header with its size.
type metaWriter struct {
	out []byte // the written blocks
	cur []byte // the block being filled
}

// pos returns where the next byte goes: the offset of its block in the
// table and its offset in the block.
func (m *metaWriter) pos() (uint32, uint16) { return uint32(len(m.out)), uint16(len(m.cur)) }

// ref returns pos as an inode reference.
func (m *metaWriter) ref() uint64 {
	b, o := m.pos()
	return uint64(b)<<16 | uint64(o)
}

func (m *metaWriter) write(b []byte) {
	for len(b) > 0 {
		n := min(len(b), metaSize-len(m.cur))
		m.cur = append(m.cur, b[:n]...)
		b = b[n:]
		if len(m.cur) == metaSize {
			m.flush()
		}
	}
}

func (m *metaWriter) flush() {
	if len(m.cur) == 0 {
		return
	}
	if c := compress(m.cur); len(c) < len(m.cur) {
		m.out = le16(m.out, uint16(len(c)))
		m.out = append(m.out, c...)
	} else {
		m.out = le16(m.out, uint16(len(m.cur))|metaRaw)
		m.out = append(m.out, m.cur...)
	}
	m.cur = m.cur[:0]
}
