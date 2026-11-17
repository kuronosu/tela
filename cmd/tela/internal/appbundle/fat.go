package appbundle

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// The universal ("fat") Mach-O format, as <mach-o/fat.h> defines it and
// lipo writes it: a big-endian header, FAT_MAGIC and the number of slices,
// then one fat_arch per slice (cputype, cpusubtype, offset, size, align),
// then each thin Mach-O file at its offset, a multiple of 2^align. The
// offsets inside a slice (segments, the code signature's LC_CODE_SIGNATURE
// dataoff) are relative to the slice's start, so a slice is the thin file
// byte for byte, and the Go linker's ad-hoc signature of the arm64 slice
// (which hashes the slice's pages from its first byte) stays valid. The
// kernel and dyld pick the slice of the machine; Rosetta runs the x86_64
// one when asked to.
const (
	fatMagic  = 0xcafebabe
	machMagic = 0xfeedfacf // MH_MAGIC_64, little-endian in the file

	// fatAlign is the alignment of every slice: 2^14, the 16 KiB page of
	// Apple silicon (lipo's alignment for arm64), which is also a multiple
	// of the 4 KiB page x86_64 needs.
	fatAlign = 14
)

// Universal returns the universal Mach-O file holding the thin 64-bit
// little-endian Mach-O files thin (an amd64 and an arm64 executable, say),
// each unchanged. The slices are ordered as lipo orders them, x86_64 before
// arm64 (by CPU type), whatever the order of thin, so the bytes depend only
// on the inputs.
func Universal(thin ...[]byte) ([]byte, error) {
	if len(thin) < 2 {
		return nil, errors.New("appbundle: a universal executable needs two thin ones or more")
	}
	type slice struct {
		cpu, sub uint32
		data     []byte
	}
	var ss []slice
	for i, b := range thin {
		if len(b) < 32 || binary.LittleEndian.Uint32(b) != machMagic {
			return nil, fmt.Errorf("appbundle: executable %d is not a thin 64-bit Mach-O file", i+1)
		}
		s := slice{cpu: binary.LittleEndian.Uint32(b[4:]), sub: binary.LittleEndian.Uint32(b[8:]), data: b}
		for _, o := range ss {
			if o.cpu == s.cpu {
				return nil, fmt.Errorf("appbundle: two executables for the same CPU type %#x", s.cpu)
			}
		}
		ss = append(ss, s)
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].cpu < ss[j].cpu })

	const align = 1 << fatAlign
	up := func(n int) int { return (n + align - 1) &^ (align - 1) }
	offsets := make([]int, len(ss))
	end := 8 + 20*len(ss)
	for i, s := range ss {
		offsets[i] = up(end)
		end = offsets[i] + len(s.data)
		if end > 1<<32-1 {
			return nil, errors.New("appbundle: the universal executable would be larger than 4 GiB")
		}
	}
	var out bytes.Buffer
	out.Grow(end)
	be := binary.BigEndian
	out.Write(be.AppendUint32(nil, fatMagic))
	out.Write(be.AppendUint32(nil, uint32(len(ss))))
	for i, s := range ss {
		for _, v := range []uint32{s.cpu, s.sub, uint32(offsets[i]), uint32(len(s.data)), fatAlign} {
			out.Write(be.AppendUint32(nil, v))
		}
	}
	for i, s := range ss {
		out.Write(make([]byte, offsets[i]-out.Len()))
		out.Write(s.data)
	}
	return out.Bytes(), nil
}
