//go:build telagpu

package core

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// keepDmabuf returns d with a descriptor of its own, for closeDmabuf; ok is
// false for what cannot be mapped (mapDmabuf), and when no descriptor is left:
// it is then shown by the device alone.
func keepDmabuf(d Dmabuf) (kept Dmabuf, ok bool) {
	if d.checkMapped() != nil {
		return Dmabuf{}, false
	}
	p := d.Planes[0]
	fd, err := unix.FcntlInt(uintptr(p.FD), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return Dmabuf{}, false
	}
	p.FD = fd
	d.Planes = []DmabufPlane{p}
	return d, true
}

func closeDmabuf(d Dmabuf) { unix.Close(d.Planes[0].FD) }

// renderNode returns the path of the DRM render node with a device number,
// "" when there is none.
func renderNode(major, minor uint32) string {
	nodes, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil {
		return ""
	}
	for _, name := range nodes {
		var st unix.Stat_t
		if unix.Stat(name, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFCHR && st.Rdev == unix.Mkdev(major, minor) {
			return name
		}
	}
	return ""
}
