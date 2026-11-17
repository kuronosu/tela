// Package squashfs writes and reads squashfs 4.0 images, the file system
// of an AppImage's payload (ADR-295).
//
// The writer makes the subset an AppImage needs: regular files,
// directories and symbolic links with their permission bits, one owner
// (root), zlib compression (the format's "gzip"), little-endian, no
// fragments, no extended attributes, no export table. The bytes depend
// only on the entries and the options: entries are sorted, every time is
// Options.ModTime, and the blocks are compressed by the standard library's
// zlib at a fixed level (a new Go release may compress differently).
//
// The reader reads that subset and also fragments, extended inodes and
// sparse blocks, so it reads what mksquashfs writes without
// -no-fragments; it does not read extended attributes or other
// compressors. It checks every offset against the image, so a damaged or
// hostile image is an error, not a crash.
//
// Written from the layout of the Linux kernel's fs/squashfs/squashfs_fs.h
// and the squashfs-tools documentation; no code was copied.
package squashfs

import "errors"

// Layout constants of squashfs 4.0.
const (
	magic        = 0x73717368 // "hsqs"
	superSize    = 96
	metaSize     = 8192 // uncompressed size of a metadata block
	metaRaw      = 1 << 15
	blockRaw     = 1 << 24
	invalid      = ^uint64(0)
	noFragment   = ^uint32(0)
	zlibID       = 1
	dirCountMax  = 256
	nameMax      = 256
	padTo        = 4096
	flagNoFrag   = 1 << 4
	flagNoXattr  = 1 << 9
	fragEntrySz  = 16
	defaultBlock = 128 << 10

	// Inode types.
	typeDir      = 1
	typeFile     = 2
	typeSymlink  = 3
	typeLDir     = 8
	typeLFile    = 9
	typeLSymlink = 10
)

// ErrFormat is wrapped by the errors of a damaged image or one that uses
// what the reader does not read.
var ErrFormat = errors.New("squashfs: not a readable squashfs 4.0 image")
