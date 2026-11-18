package appbundle

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"testing"
)

// thinMachO is the smallest file debug/macho reads as a 64-bit Mach-O of
// the CPU type cpu: a header without load commands, then body.
func thinMachO(cpu macho.Cpu, sub uint32, body string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, macho.Magic64)
	for _, v := range []uint32{uint32(cpu), sub, uint32(macho.TypeExec), 0, 0, 0, 0} {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	return append(b, body...)
}

func TestUniversal(t *testing.T) {
	arm := thinMachO(macho.CpuArm64, 0, "arm64 program")
	amd := thinMachO(macho.CpuAmd64, 3, "amd64 program, longer")
	fat, err := Universal(arm, amd)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Universal(amd, arm)
	if !bytes.Equal(fat, again) {
		t.Errorf("the order of the inputs changed the file")
	}
	f, err := macho.NewFatFile(bytes.NewReader(fat))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Arches) != 2 || f.Arches[0].Cpu != macho.CpuAmd64 || f.Arches[1].Cpu != macho.CpuArm64 {
		t.Fatalf("arches %+v", f.Arches)
	}
	for i, want := range [][]byte{amd, arm} {
		a := f.Arches[i].FatArchHeader
		if a.Offset%(1<<14) != 0 || a.Align != 14 || a.SubCpu != binary.LittleEndian.Uint32(want[8:]) {
			t.Errorf("slice %d: %+v", i, a)
		}
		if got := fat[a.Offset : a.Offset+a.Size]; !bytes.Equal(got, want) {
			t.Errorf("slice %d is not the thin file byte for byte", i)
		}
	}
	for _, bad := range [][][]byte{{arm}, {arm, arm}, {arm, []byte("#!/bin/sh\n" + string(make([]byte, 40)))}} {
		if _, err := Universal(bad...); err == nil {
			t.Errorf("%d inputs accepted", len(bad))
		}
	}
}
