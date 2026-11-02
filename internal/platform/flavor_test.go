package platform

import (
	"runtime"
	"testing"
)

func TestFlavor(t *testing.T) {
	for goos, want := range map[string]Flavor{
		"darwin": FlavorMac, "windows": FlavorWindows, "linux": FlavorLinux,
		"freebsd": FlavorLinux, "openbsd": FlavorLinux, "": FlavorLinux,
	} {
		if got := FlavorOf(goos); got != want {
			t.Errorf("FlavorOf(%q) = %v, want %v", goos, got, want)
		}
	}
	if HostFlavor() != FlavorOf(runtime.GOOS) {
		t.Errorf("HostFlavor() = %v on %s", HostFlavor(), runtime.GOOS)
	}
	if FlavorHost.Resolve() != HostFlavor() || Flavor(99).Resolve() != HostFlavor() {
		t.Error("FlavorHost and unknown values must resolve to the host's flavor")
	}
	for _, f := range []Flavor{FlavorLinux, FlavorWindows, FlavorMac} {
		if f.Resolve() != f {
			t.Errorf("%v.Resolve() = %v", f, f.Resolve())
		}
		mac := f == FlavorMac
		if f.CmdOrCtrl(true, false) != !mac || f.CmdOrCtrl(false, true) != mac || f.CmdOrCtrl(false, false) {
			t.Errorf("%v.CmdOrCtrl is wrong", f)
		}
	}
	if FlavorHost.CmdOrCtrl(false, true) != (HostFlavor() == FlavorMac) {
		t.Error("FlavorHost.CmdOrCtrl does not follow the host")
	}
	for f, s := range map[Flavor]string{FlavorHost: "host", FlavorLinux: "linux", FlavorWindows: "windows", FlavorMac: "mac", 99: "?"} {
		if f.String() != s {
			t.Errorf("%d.String() = %q, want %q", f, f.String(), s)
		}
	}
}

// ParseAccelerator's CmdOrCtrl follows the host's flavor, as before.
func TestParseAcceleratorHostFlavor(t *testing.T) {
	want := Accelerator{ModCtrl, "s"}
	if HostFlavor() == FlavorMac {
		want = Accelerator{ModSuper, "s"}
	}
	if got, err := ParseAccelerator("CmdOrCtrl+S"); err != nil || got != want {
		t.Errorf("ParseAccelerator(CmdOrCtrl+S) = %+v, %v; want %+v", got, err, want)
	}
}
