package pkgfmt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// debFormat writes <package>-<version>-linux-<arch>.deb: an ar archive of
// debian-binary, control.tar.gz and data.tar.gz (dpkg's format 2.0), with
//
//	/usr/bin/<package>
//	/usr/share/applications/<ID>.desktop
//	/usr/share/metainfo/<ID>.metainfo.xml
//	/usr/share/mime/packages/<ID>.xml   (with file types)
//	/usr/share/icons/hicolor/...
//
// and postinst and postrm scripts that refresh the MIME and desktop
// databases when their tools are installed (Debian's triggers do it too).
// The control file has no Depends: the program is static. apt or dpkg
// manage the install, so package update reports ErrManaged for it.
type debFormat struct{}

func (debFormat) Name() string { return "deb" }
func (debFormat) OS() string   { return "linux" }

// debArches are Debian's names of the Go architectures.
var debArches = map[string]string{
	"amd64": "amd64", "arm64": "arm64", "386": "i386", "arm": "armhf",
	"riscv64": "riscv64", "ppc64le": "ppc64el", "s390x": "s390x",
	"loong64": "loong64", "mips64le": "mips64el",
}

func (debFormat) Check(app App, t Target) error {
	if _, ok := debArches[t.Arch]; !ok {
		return fmt.Errorf("linux/%s has no Debian architecture", t.Arch)
	}
	return nil
}

// DebianVersion turns a semantic version into a Debian upstream version
// that sorts the same way: a pre-release after '~' (1.2.3-rc.1 is
// 1.2.3~rc.1, before 1.2.3), build metadata after '+', other hyphens as
// periods, and anything Debian does not allow as a period.
func DebianVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	core, meta, hasMeta := strings.Cut(v, "+")
	core, pre, hasPre := strings.Cut(core, "-")
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '.' || r == '+' || r == '~' {
				return r
			}
			return '.'
		}, s)
	}
	out := clean(core)
	if hasPre {
		out += "~" + clean(pre)
	}
	if hasMeta {
		out += "+" + clean(meta)
	}
	if out == "" || out[0] < '0' || out[0] > '9' {
		out = "0~" + out
	}
	return out
}

// debSections maps the main categories to Debian's sections.
var debSections = map[string]string{
	"AudioVideo": "sound", "Audio": "sound", "Video": "video", "Development": "devel",
	"Education": "education", "Game": "games", "Graphics": "graphics", "Network": "net",
	"Office": "text", "Science": "science", "Settings": "admin", "System": "admin", "Utility": "utils",
}

const debScript = `#!/bin/sh
# Written by tela package: refresh the desktop's databases when their tools
# are installed.
set -e
if command -v update-mime-database >/dev/null 2>&1; then
	update-mime-database /usr/share/mime || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q /usr/share/applications || true
fi
exit 0
`

func (f debFormat) Write(job Job) ([]Output, error) {
	app := job.App
	prog, err := readProgram(job)
	if err != nil {
		return nil, err
	}
	files, err := usrFiles(app, prog, job.Time)
	if err != nil {
		return nil, err
	}
	data, err := tarGz(files, job.Time)
	if err != nil {
		return nil, err
	}
	var kib int64
	var sums strings.Builder
	for _, fl := range files {
		kib += (int64(len(fl.data)) + 1023) / 1024
		fmt.Fprintf(&sums, "%x  %s\n", md5.Sum(fl.data), fl.path)
	}
	maint := strings.TrimSpace(app.Publisher)
	if maint == "" {
		maint = "unknown"
		fmt.Fprintf(job.Log, "tela package: the deb's Maintainer is %q because App.Publisher is empty; set Publisher (a name, or \"Name <email>\")\n", maint)
	}
	var ctl strings.Builder
	fmt.Fprintf(&ctl, "Package: %s\n", app.Package)
	fmt.Fprintf(&ctl, "Version: %s\n", DebianVersion(app.Version))
	fmt.Fprintf(&ctl, "Architecture: %s\n", debArches[job.Target.Arch])
	fmt.Fprintf(&ctl, "Maintainer: %s\n", oneLine(maint))
	fmt.Fprintf(&ctl, "Installed-Size: %d\n", kib)
	fmt.Fprintf(&ctl, "Section: %s\n", debSections[app.category()])
	ctl.WriteString("Priority: optional\n")
	fmt.Fprintf(&ctl, "Description: %s\n", oneLine(app.summary()))
	control, err := tarGz([]file{
		{path: "control", mode: 0o644, data: []byte(ctl.String())},
		{path: "md5sums", mode: 0o644, data: []byte(sums.String())},
		{path: "postinst", mode: 0o755, data: []byte(debScript)},
		{path: "postrm", mode: 0o755, data: []byte(debScript)},
	}, job.Time)
	if err != nil {
		return nil, err
	}
	deb := arArchive(job.Time, []arMember{
		{"debian-binary", []byte("2.0\n")},
		{"control.tar.gz", control},
		{"data.tar.gz", data},
	})
	p, err := writeOut(job.OutDir, FileName(app, job.Target, ".deb"), deb, 0o644)
	if err != nil {
		return nil, err
	}
	out, err := sign(job, p)
	return []Output{out}, err
}

// oneLine keeps a control field on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// tarGz returns a gzip-compressed tar of files under "./", with every
// folder they need, sorted, owned by root and dated t.
func tarGz(files []file, t time.Time) ([]byte, error) {
	dirs := map[string]bool{}
	for _, f := range files {
		for d := path.Dir(f.path); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	all := append([]file(nil), files...)
	for d := range dirs {
		all = append(all, file{path: d, mode: fs.ModeDir | 0o755})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].path < all[j].path })
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(zw)
	hdr := func(name string, typ byte, mode fs.FileMode, size int64, link string) error {
		return tw.WriteHeader(&tar.Header{
			Typeflag: typ, Name: name, Linkname: link, Size: size, Mode: int64(mode.Perm()),
			Uname: "root", Gname: "root", ModTime: t, Format: tar.FormatGNU,
		})
	}
	if err := hdr("./", tar.TypeDir, 0o755, 0, ""); err != nil {
		return nil, err
	}
	for _, f := range all {
		var err error
		switch {
		case f.mode.IsDir():
			err = hdr("./"+f.path+"/", tar.TypeDir, f.mode, 0, "")
		case f.mode&fs.ModeSymlink != 0:
			err = hdr("./"+f.path, tar.TypeSymlink, 0o777, 0, f.target)
		default:
			if err = hdr("./"+f.path, tar.TypeReg, f.mode, int64(len(f.data)), ""); err == nil {
				_, err = tw.Write(f.data)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type arMember struct {
	name string
	data []byte
}

// arArchive returns a Unix ar archive of members, as dpkg-deb writes one:
// names without a trailing slash, owner 0, mode 100644, time t.
func arArchive(t time.Time, members []arMember) []byte {
	var b bytes.Buffer
	b.WriteString("!<arch>\n")
	for _, m := range members {
		fmt.Fprintf(&b, "%-16s%-12s%-6s%-6s%-8s%-10s`\n", m.name, strconv.FormatInt(t.Unix(), 10), "0", "0", "100644", strconv.Itoa(len(m.data)))
		b.Write(m.data)
		if len(m.data)%2 == 1 {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}
