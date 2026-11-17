package squashfs

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

// Reader reads an image. It is not safe for concurrent use.
type Reader struct {
	r    io.ReaderAt
	size uint64 // the end of the file system (bytes_used)

	inodes, dirs, blockSize uint64
	root                    uint64
	mtime                   time.Time
	ids                     []uint32
	frags                   []fragment
	meta                    map[uint64]metaBlock
}

type fragment struct {
	start uint64
	size  uint32
}

type metaBlock struct {
	data []byte
	next uint64 // where the next block starts
}

// File is an entry of the image.
type File struct {
	// Path is the slash-separated path; "." is the root.
	Path string
	// Mode is the type and permission bits.
	Mode fs.FileMode
	// Size is a regular file's size.
	Size int64
	// ModTime is the inode's time.
	ModTime time.Time
	// Target is a symbolic link's target.
	Target string
	// UID and GID are the owner and group.
	UID, GID uint32

	ref uint64
	in  inode
}

// inode is what the reader keeps of an inode.
type inode struct {
	typ    uint16
	number uint32
	// Regular files: data.
	start  uint64
	size   uint64
	frag   uint32
	fragAt uint32
	blocks []uint32
	// Folders: the listing.
	dirBlock uint32
	dirOff   uint16
	dirSize  uint64
}

// maxEntries and maxDepth bound what Walk reads of a hostile image.
const (
	maxEntries = 1 << 22
	maxDepth   = 256
)

// NewReader reads the superblock and the tables of the image at the start
// of r, which is size bytes long.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	var sb [superSize]byte
	if _, err := r.ReadAt(sb[:], 0); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	le := binary.LittleEndian
	if le.Uint32(sb[0:]) != magic {
		return nil, fmt.Errorf("%w: no squashfs magic", ErrFormat)
	}
	if major, minor := le.Uint16(sb[28:]), le.Uint16(sb[30:]); major != 4 || minor != 0 {
		return nil, fmt.Errorf("%w: version %d.%d", ErrFormat, major, minor)
	}
	if c := le.Uint16(sb[20:]); c != zlibID {
		return nil, fmt.Errorf("%w: compressor %d is not zlib", ErrFormat, c)
	}
	rd := &Reader{
		r:         r,
		size:      le.Uint64(sb[40:]),
		blockSize: uint64(le.Uint32(sb[12:])),
		root:      le.Uint64(sb[32:]),
		inodes:    le.Uint64(sb[64:]),
		dirs:      le.Uint64(sb[72:]),
		mtime:     time.Unix(int64(le.Uint32(sb[8:])), 0).UTC(),
		meta:      map[uint64]metaBlock{},
	}
	bs := rd.blockSize
	if bs < 4096 || bs > 1<<20 || bs&(bs-1) != 0 || 1<<le.Uint16(sb[22:]) != bs {
		return nil, fmt.Errorf("%w: block size %d", ErrFormat, bs)
	}
	if size >= 0 && rd.size > uint64(size) || rd.size < superSize {
		return nil, fmt.Errorf("%w: the image is %d bytes but says it uses %d", ErrFormat, size, rd.size)
	}
	if !(superSize <= rd.inodes && rd.inodes < rd.dirs && rd.dirs <= rd.size) {
		return nil, fmt.Errorf("%w: table offsets", ErrFormat)
	}
	var err error
	nIDs := uint64(le.Uint16(sb[26:]))
	if rd.ids, err = readTable(rd, le.Uint64(sb[48:]), nIDs, 4, func(b []byte) uint32 { return le.Uint32(b) }); err != nil {
		return nil, fmt.Errorf("%w: the ID table: %v", ErrFormat, err)
	}
	if nFrags := uint64(le.Uint32(sb[16:])); nFrags > 0 {
		rd.frags, err = readTable(rd, le.Uint64(sb[80:]), nFrags, fragEntrySz, func(b []byte) fragment {
			return fragment{le.Uint64(b), le.Uint32(b[8:])}
		})
		if err != nil {
			return nil, fmt.Errorf("%w: the fragment table: %v", ErrFormat, err)
		}
	}
	return rd, nil
}

// readTable reads a table of n entries of size bytes stored in metadata
// blocks whose positions are listed at start.
func readTable[T any](rd *Reader, start, n, size uint64, conv func([]byte) T) ([]T, error) {
	if n == 0 || n > 1<<24 {
		return nil, fmt.Errorf("%d entries", n)
	}
	blocks := (n*size + metaSize - 1) / metaSize
	if start > rd.size || blocks*8 > rd.size-start {
		return nil, fmt.Errorf("index outside the image")
	}
	index := make([]byte, blocks*8)
	if _, err := rd.r.ReadAt(index, int64(start)); err != nil {
		return nil, err
	}
	var raw []byte
	for i := range blocks {
		b, err := rd.block(binary.LittleEndian.Uint64(index[i*8:]))
		if err != nil {
			return nil, err
		}
		raw = append(raw, b.data...)
	}
	if uint64(len(raw)) < n*size {
		return nil, fmt.Errorf("short table")
	}
	out := make([]T, n)
	for i := range out {
		out[i] = conv(raw[uint64(i)*size:])
	}
	return out, nil
}

// block reads the metadata block at the absolute position pos.
func (rd *Reader) block(pos uint64) (metaBlock, error) {
	if b, ok := rd.meta[pos]; ok {
		return b, nil
	}
	if pos+2 > rd.size || pos < superSize {
		return metaBlock{}, fmt.Errorf("%w: metadata block at %d is outside the image", ErrFormat, pos)
	}
	var h [2]byte
	if _, err := rd.r.ReadAt(h[:], int64(pos)); err != nil {
		return metaBlock{}, err
	}
	hdr := binary.LittleEndian.Uint16(h[:])
	n := uint64(hdr &^ metaRaw)
	if n == 0 || n > metaSize || pos+2+n > rd.size {
		return metaBlock{}, fmt.Errorf("%w: metadata block at %d has size %d", ErrFormat, pos, n)
	}
	buf := make([]byte, n)
	if _, err := rd.r.ReadAt(buf, int64(pos+2)); err != nil {
		return metaBlock{}, err
	}
	if hdr&metaRaw == 0 {
		var err error
		if buf, err = inflate(buf, metaSize); err != nil {
			return metaBlock{}, fmt.Errorf("%w: metadata block at %d: %v", ErrFormat, pos, err)
		}
	}
	b := metaBlock{data: buf, next: pos + 2 + n}
	rd.meta[pos] = b
	return b, nil
}

// inflate decompresses zlib data of at most limit bytes.
func inflate(b []byte, limit int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(io.LimitReader(zr, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(out) > limit {
		return nil, fmt.Errorf("more than %d bytes", limit)
	}
	return out, nil
}

// cursor reads a metadata table as a stream from a position.
type cursor struct {
	rd    *Reader
	block uint64 // absolute position of the current block
	data  []byte // what is left of it
	next  uint64
}

func (rd *Reader) cursor(table uint64, block uint64, off uint16) (*cursor, error) {
	b, err := rd.block(table + block)
	if err != nil {
		return nil, err
	}
	if int(off) > len(b.data) {
		return nil, fmt.Errorf("%w: offset %d past its metadata block", ErrFormat, off)
	}
	return &cursor{rd: rd, block: table + block, data: b.data[off:], next: b.next}, nil
}

func (c *cursor) read(n int) ([]byte, error) {
	if len(c.data) >= n {
		b := c.data[:n]
		c.data = c.data[n:]
		return b, nil
	}
	out := make([]byte, 0, n)
	for len(out) < n {
		if len(c.data) == 0 {
			b, err := c.rd.block(c.next)
			if err != nil {
				return nil, err
			}
			c.block, c.data, c.next = c.next, b.data, b.next
		}
		k := min(n-len(out), len(c.data))
		out = append(out, c.data[:k]...)
		c.data = c.data[k:]
	}
	return out, nil
}

// inode reads the inode at ref.
func (rd *Reader) inode(ref uint64) (File, error) {
	c, err := rd.cursor(rd.inodes, ref>>16, uint16(ref))
	if err != nil {
		return File{}, err
	}
	le := binary.LittleEndian
	h, err := c.read(16)
	if err != nil {
		return File{}, err
	}
	f := File{ref: ref, ModTime: time.Unix(int64(le.Uint32(h[8:])), 0).UTC()}
	f.in.typ = le.Uint16(h)
	f.in.number = le.Uint32(h[12:])
	uid, gid := int(le.Uint16(h[4:])), int(le.Uint16(h[6:]))
	if uid >= len(rd.ids) || gid >= len(rd.ids) {
		return File{}, fmt.Errorf("%w: inode %d has an owner outside the ID table", ErrFormat, f.in.number)
	}
	f.UID, f.GID = rd.ids[uid], rd.ids[gid]
	perm := le.Uint16(h[2:])
	f.Mode = fs.FileMode(perm & 0o777)
	if perm&0o4000 != 0 {
		f.Mode |= fs.ModeSetuid
	}
	if perm&0o2000 != 0 {
		f.Mode |= fs.ModeSetgid
	}
	if perm&0o1000 != 0 {
		f.Mode |= fs.ModeSticky
	}
	switch f.in.typ {
	case typeDir:
		b, err := c.read(16)
		if err != nil {
			return File{}, err
		}
		f.Mode |= fs.ModeDir
		f.in.dirBlock = le.Uint32(b)
		f.in.dirSize = uint64(le.Uint16(b[8:]))
		f.in.dirOff = le.Uint16(b[10:])
	case typeLDir:
		b, err := c.read(24)
		if err != nil {
			return File{}, err
		}
		f.Mode |= fs.ModeDir
		f.in.dirSize = uint64(le.Uint32(b[4:]))
		f.in.dirBlock = le.Uint32(b[8:])
		f.in.dirOff = le.Uint16(b[18:])
	case typeFile, typeLFile:
		if f.in.typ == typeFile {
			b, err := c.read(16)
			if err != nil {
				return File{}, err
			}
			f.in.start = uint64(le.Uint32(b))
			f.in.frag = le.Uint32(b[4:])
			f.in.fragAt = le.Uint32(b[8:])
			f.in.size = uint64(le.Uint32(b[12:]))
		} else {
			b, err := c.read(40)
			if err != nil {
				return File{}, err
			}
			f.in.start = le.Uint64(b)
			f.in.size = le.Uint64(b[8:])
			f.in.frag = le.Uint32(b[28:])
			f.in.fragAt = le.Uint32(b[32:])
		}
		if f.in.size > 1<<46 {
			return File{}, fmt.Errorf("%w: inode %d claims %d bytes", ErrFormat, f.in.number, f.in.size)
		}
		n := f.in.size / rd.blockSize
		if f.in.frag == noFragment && f.in.size%rd.blockSize != 0 {
			n++
		}
		if f.in.frag != noFragment && int(f.in.frag) >= len(rd.frags) {
			return File{}, fmt.Errorf("%w: inode %d has fragment %d of %d", ErrFormat, f.in.number, f.in.frag, len(rd.frags))
		}
		// Read the list in pieces, so a bogus size fails at the end of
		// the table rather than allocating it.
		for left := n; left > 0; {
			k := min(left, 1024)
			b, err := c.read(int(k) * 4)
			if err != nil {
				return File{}, err
			}
			for i := range k {
				f.in.blocks = append(f.in.blocks, le.Uint32(b[i*4:]))
			}
			left -= k
		}
		f.Size = int64(f.in.size)
	case typeSymlink, typeLSymlink:
		b, err := c.read(8)
		if err != nil {
			return File{}, err
		}
		n := le.Uint32(b[4:])
		if n == 0 || n > 4096 {
			return File{}, fmt.Errorf("%w: inode %d has a link target of %d bytes", ErrFormat, f.in.number, n)
		}
		t, err := c.read(int(n))
		if err != nil {
			return File{}, err
		}
		f.Mode |= fs.ModeSymlink
		f.Target = string(t)
	default:
		return File{}, fmt.Errorf("%w: inode %d has type %d (devices, FIFOs and sockets are not read)", ErrFormat, f.in.number, f.in.typ)
	}
	return f, nil
}

// Root returns the root folder.
func (rd *Reader) Root() (File, error) {
	f, err := rd.inode(rd.root)
	if err != nil {
		return File{}, err
	}
	if !f.Mode.IsDir() {
		return File{}, fmt.Errorf("%w: the root is not a folder", ErrFormat)
	}
	f.Path = "."
	return f, nil
}

// ReadDir returns the entries of the folder d, in the image's order.
func (rd *Reader) ReadDir(d File) ([]File, error) {
	if !d.Mode.IsDir() {
		return nil, fmt.Errorf("squashfs: %s is not a folder", d.Path)
	}
	if d.in.dirSize < 3 {
		return nil, fmt.Errorf("%w: folder %s has size %d", ErrFormat, d.Path, d.in.dirSize)
	}
	left := d.in.dirSize - 3
	if left == 0 {
		return nil, nil
	}
	c, err := rd.cursor(rd.dirs, uint64(d.in.dirBlock), d.in.dirOff)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	var out []File
	for left > 0 {
		if left < 12 {
			return nil, fmt.Errorf("%w: folder %s: a truncated listing", ErrFormat, d.Path)
		}
		h, err := c.read(12)
		if err != nil {
			return nil, err
		}
		left -= 12
		count := uint64(le.Uint32(h)) + 1
		block := uint64(le.Uint32(h[4:]))
		base := le.Uint32(h[8:])
		if count > dirCountMax {
			return nil, fmt.Errorf("%w: folder %s: a run of %d entries", ErrFormat, d.Path, count)
		}
		for range count {
			if left < 8 {
				return nil, fmt.Errorf("%w: folder %s: a truncated listing", ErrFormat, d.Path)
			}
			e, err := c.read(8)
			if err != nil {
				return nil, err
			}
			nameLen := uint64(le.Uint16(e[6:])) + 1
			if nameLen > nameMax || left < 8+nameLen {
				return nil, fmt.Errorf("%w: folder %s: a name of %d bytes", ErrFormat, d.Path, nameLen)
			}
			left -= 8 + nameLen
			nb, err := c.read(int(nameLen))
			if err != nil {
				return nil, err
			}
			name := string(nb)
			if name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
				return nil, fmt.Errorf("%w: folder %s has an entry named %q", ErrFormat, d.Path, name)
			}
			f, err := rd.inode(block<<16 | uint64(le.Uint16(e)))
			if err != nil {
				return nil, err
			}
			if f.in.number != uint32(int64(base)+int64(int16(le.Uint16(e[2:])))) {
				return nil, fmt.Errorf("%w: %s/%s: the listing and the inode disagree on its number", ErrFormat, d.Path, name)
			}
			if d.Path == "." {
				f.Path = name
			} else {
				f.Path = d.Path + "/" + name
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// Walk calls fn for every entry of the image but the root, each folder
// before its entries, in the image's order (sorted by name). An error of
// fn stops the walk and is returned.
func (rd *Reader) Walk(fn func(File) error) error {
	root, err := rd.Root()
	if err != nil {
		return err
	}
	seen := map[uint64]bool{root.ref: true}
	count := 0
	var walk func(d File, depth int) error
	walk = func(d File, depth int) error {
		if depth > maxDepth {
			return fmt.Errorf("%w: folders nested deeper than %d", ErrFormat, maxDepth)
		}
		ents, err := rd.ReadDir(d)
		if err != nil {
			return err
		}
		for _, f := range ents {
			if count++; count > maxEntries {
				return fmt.Errorf("%w: more than %d entries", ErrFormat, maxEntries)
			}
			if err := fn(f); err != nil {
				return err
			}
			if f.Mode.IsDir() {
				if seen[f.ref] {
					return fmt.Errorf("%w: %s is listed twice", ErrFormat, f.Path)
				}
				seen[f.ref] = true
				if err := walk(f, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root, 0)
}

// WriteTo writes the content of the regular file f to w.
func (rd *Reader) WriteTo(w io.Writer, f File) (int64, error) {
	if !f.Mode.IsRegular() {
		return 0, fmt.Errorf("squashfs: %s is not a regular file", f.Path)
	}
	bs := rd.blockSize
	pos := f.in.start
	var done uint64
	var raw []byte
	for _, s := range f.in.blocks {
		want := min(bs, f.in.size-done)
		var block []byte
		if s == 0 { // sparse
			block = make([]byte, want)
		} else {
			n := uint64(s &^ blockRaw)
			if n > bs || pos > rd.size || n > rd.size-pos {
				return int64(done), fmt.Errorf("%w: %s: a data block outside the image", ErrFormat, f.Path)
			}
			if cap(raw) < int(n) {
				raw = make([]byte, n)
			}
			raw = raw[:n]
			if _, err := rd.r.ReadAt(raw, int64(pos)); err != nil {
				return int64(done), err
			}
			pos += n
			block = raw
			if s&blockRaw == 0 {
				var err error
				if block, err = inflate(raw, int(bs)); err != nil {
					return int64(done), fmt.Errorf("%w: %s: %v", ErrFormat, f.Path, err)
				}
			}
		}
		if uint64(len(block)) != want {
			return int64(done), fmt.Errorf("%w: %s: a data block of %d bytes, want %d", ErrFormat, f.Path, len(block), want)
		}
		if _, err := w.Write(block); err != nil {
			return int64(done), err
		}
		done += want
	}
	if done < f.in.size {
		if f.in.frag == noFragment {
			return int64(done), fmt.Errorf("%w: %s: missing data", ErrFormat, f.Path)
		}
		fr := rd.frags[f.in.frag]
		n := uint64(fr.size &^ blockRaw)
		if n == 0 || n > bs || fr.start > rd.size || n > rd.size-fr.start {
			return int64(done), fmt.Errorf("%w: %s: a fragment outside the image", ErrFormat, f.Path)
		}
		block := make([]byte, n)
		if _, err := rd.r.ReadAt(block, int64(fr.start)); err != nil {
			return int64(done), err
		}
		if fr.size&blockRaw == 0 {
			var err error
			if block, err = inflate(block, int(bs)); err != nil {
				return int64(done), fmt.Errorf("%w: %s: %v", ErrFormat, f.Path, err)
			}
		}
		tail := f.in.size - done
		if uint64(f.fragOffset()) > uint64(len(block)) || tail > uint64(len(block))-uint64(f.fragOffset()) {
			return int64(done), fmt.Errorf("%w: %s: the tail is outside its fragment", ErrFormat, f.Path)
		}
		if _, err := w.Write(block[f.fragOffset() : uint64(f.fragOffset())+tail]); err != nil {
			return int64(done), err
		}
		done += tail
	}
	return int64(done), nil
}

func (f File) fragOffset() uint32 { return f.in.fragAt }

// ModTime returns the image's time.
func (rd *Reader) ModTime() time.Time { return rd.mtime }

// Size returns the bytes the file system uses, without the padding.
func (rd *Reader) Size() int64 { return int64(rd.size) }
