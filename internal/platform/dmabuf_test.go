package platform

import (
	"image"
	"slices"
	"strings"
	"testing"
)

func TestDmabufFeedbackLookups(t *testing.T) {
	fb := DmabufFeedback{Formats: []DmabufFormat{
		{DRMFormatXRGB8888, 0x0300000000606014},
		{DRMFormatARGB8888, 0x0300000000606014},
		{DRMFormatXRGB8888, DRMModifierLinear},
		{DRMFormatXBGR8888, DRMModifierInvalid},
	}}
	if !fb.Accepts(DRMFormatXRGB8888, DRMModifierLinear) || fb.Accepts(DRMFormatARGB8888, DRMModifierLinear) || fb.Accepts(DRMFormatABGR8888, DRMModifierInvalid) {
		t.Errorf("Accepts is wrong for %v", fb.Formats)
	}
	if got := fb.Modifiers(DRMFormatXRGB8888); !slices.Equal(got, []uint64{0x0300000000606014, DRMModifierLinear}) {
		t.Errorf("Modifiers(XRGB8888) = %#x, want the tiled one, then linear", got)
	}
	if got := fb.Modifiers(DRMFormatABGR8888); got != nil {
		t.Errorf("Modifiers of a format that is not listed = %#x", got)
	}
	// The fourcc codes spell their names, least significant byte first.
	for code, want := range map[uint32]string{
		DRMFormatARGB8888: "AR24", DRMFormatXRGB8888: "XR24", DRMFormatABGR8888: "AB24", DRMFormatXBGR8888: "XB24",
	} {
		if got := (DmabufFormat{Fourcc: code}).String(); got != want+":0x0" {
			t.Errorf("format %#x prints %q, want %q", code, got, want+":0x0")
		}
	}
}

func TestCheckDmabuf(t *testing.T) {
	good := func() DmabufDescriptor {
		return DmabufDescriptor{
			Size: image.Pt(640, 480), Fourcc: DRMFormatXRGB8888, Modifier: DRMModifierLinear,
			Planes: []DmabufPlane{{FD: 3, Stride: 2560}},
		}
	}
	CheckDmabuf(good()) // does not panic
	four := good()
	four.Planes = append(four.Planes, four.Planes[0], four.Planes[0], four.Planes[0])
	CheckDmabuf(four)

	for what, tc := range map[string]struct {
		change func(*DmabufDescriptor)
		says   string
	}{
		"an empty size":      {func(d *DmabufDescriptor) { d.Size.X = 0 }, "size"},
		"a negative size":    {func(d *DmabufDescriptor) { d.Size.Y = -1 }, "size"},
		"no format":          {func(d *DmabufDescriptor) { d.Fourcc = 0 }, "Fourcc"},
		"no planes":          {func(d *DmabufDescriptor) { d.Planes = nil }, "0 planes"},
		"five planes":        {func(d *DmabufDescriptor) { d.Planes = make([]DmabufPlane, 5) }, "5 planes"},
		"a closed plane":     {func(d *DmabufDescriptor) { d.Planes[0].FD = -1 }, "file descriptor -1"},
		"a strideless plane": {func(d *DmabufDescriptor) { d.Planes[0].Stride = 0 }, "stride"},
	} {
		d := good()
		tc.change(&d)
		func() {
			defer func() {
				msg, _ := recover().(string)
				if !strings.Contains(msg, tc.says) {
					t.Errorf("%s: panic %q, want one that says %q", what, msg, tc.says)
				}
			}()
			CheckDmabuf(d)
		}()
	}
}
