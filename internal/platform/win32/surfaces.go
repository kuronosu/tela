package win32

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces of platform contract v16 (ADR-298): the parts that
// are plain data, tested on every OS. surfaces_windows.go makes the calls.
//
// Start at login is the value <app ID> of the user's Run key, the command
// Explorer runs at logon: the quoted executable, its arguments and
// platform.AtLoginFlag (what Electron's setLoginItemSettings writes, with
// a marker of Tela's). Task Manager's Startup apps page keeps the user's
// switch beside it, in Explorer\StartupApproved\Run: a REG_BINARY of 12
// bytes whose first byte is 2 (or 6) when the entry runs and 3 (or 7) when
// the user turned it off, then a FILETIME. StartsAtLogin reads it, and
// SetStartAtLogin(true) removes it, since turning the app's own switch on
// is the user's choice too (Windows then runs the value as for a new
// one).

const (
	runKey             = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupApprovedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// runRegistry is the part of HKEY_CURRENT_USER that start at login uses
// (hkcuRegistry; a map in tests).
type runRegistry interface {
	String(key, name string) (value string, ok bool, err error)
	SetString(key, name, value string) error
	// Binary returns a REG_BINARY value; ok is false when it is missing.
	Binary(key, name string) (value []byte, ok bool, err error)
	// DeleteValue removes a value; a missing one is not an error.
	DeleteValue(key, name string) error
}

// runCommand returns the Run value of app: the quoted executable, its
// arguments and AtLoginFlag.
func runCommand(app platform.HandlerApp) string {
	args := append(append([]string(nil), app.Args...), platform.AtLoginFlag)
	return `"` + app.Exec + `" ` + quoteArgs(args)
}

// startupDisabled reports whether a StartupApproved value says the user
// turned the entry off: an odd first byte.
func startupDisabled(v []byte) bool { return len(v) > 0 && v[0]&1 == 1 }

// setRunAtLogin writes or removes app's Run value and reports whether it
// changed anything.
func setRunAtLogin(reg runRegistry, app platform.HandlerApp, on bool) (bool, error) {
	cur, ok, err := reg.String(runKey, app.ID)
	if err != nil {
		return false, err
	}
	if !on {
		if !ok {
			return false, nil
		}
		return true, reg.DeleteValue(runKey, app.ID)
	}
	changed := false
	if want := runCommand(app); !ok || cur != want {
		if err := reg.SetString(runKey, app.ID, want); err != nil {
			return false, err
		}
		changed = true
	}
	if v, ok, err := reg.Binary(startupApprovedKey, app.ID); err != nil {
		return changed, err
	} else if ok && startupDisabled(v) {
		if err := reg.DeleteValue(startupApprovedKey, app.ID); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// runsAtLogin reports whether Explorer runs app's Run value at logon.
func runsAtLogin(reg runRegistry, app platform.HandlerApp) (bool, error) {
	_, ok, err := reg.String(runKey, app.ID)
	if err != nil || !ok {
		return false, err
	}
	v, ok, err := reg.Binary(startupApprovedKey, app.ID)
	if err != nil {
		return false, err
	}
	return !(ok && startupDisabled(v)), nil
}

// quoteArgs joins args into a command line that CommandLineToArgvW (and
// Go's os.Args) splits back into them: an argument with a space, a tab, a
// quote or nothing in it is quoted, with the backslashes before a quote
// doubled and the quote escaped.
func quoteArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = quoteArg(a)
	}
	return strings.Join(parts, " ")
}

func quoteArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\n\v\"") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(a); i++ {
		c := a[i]
		switch c {
		case '\\':
			slashes++
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, 2*slashes+1))
		default:
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteByte(c)
	}
	b.WriteString(strings.Repeat(`\`, 2*slashes))
	b.WriteByte('"')
	return b.String()
}

// icoFile returns an .ico file holding each image as a PNG (Windows Vista
// and later read PNG entries), for a jump list task's IShellLinkW icon,
// which must be a file.
func icoFile(images []image.Image) ([]byte, error) {
	var pngs [][]byte
	var sizes []image.Point
	for _, img := range images {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return nil, err
		}
		pngs = append(pngs, b.Bytes())
		sizes = append(sizes, img.Bounds().Size())
	}
	if len(pngs) == 0 {
		return nil, fmt.Errorf("win32: an icon without images")
	}
	var out bytes.Buffer
	le := binary.LittleEndian
	out.Write(le.AppendUint16(le.AppendUint16(le.AppendUint16(nil, 0), 1), uint16(len(pngs))))
	offset := 6 + 16*len(pngs)
	for i, p := range pngs {
		side := func(n int) byte {
			if n >= 256 {
				return 0 // 256 is written 0
			}
			return byte(n)
		}
		e := []byte{side(sizes[i].X), side(sizes[i].Y), 0, 0}
		e = le.AppendUint16(e, 1)  // planes
		e = le.AppendUint16(e, 32) // bits per pixel
		e = le.AppendUint32(e, uint32(len(p)))
		e = le.AppendUint32(e, uint32(offset))
		out.Write(e)
		offset += len(p)
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes(), nil
}
