package win32

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The data of the Windows desktop surfaces (ADR-298), on every OS.

// memRun is a runRegistry in memory.
type memRun struct {
	strs   map[string]string
	bins   map[string][]byte
	writes int
}

func (m *memRun) k(key, name string) string { return strings.ToLower(key + `\` + name) }

func (m *memRun) String(key, name string) (string, bool, error) {
	v, ok := m.strs[m.k(key, name)]
	return v, ok, nil
}

func (m *memRun) SetString(key, name, value string) error {
	if m.strs == nil {
		m.strs = map[string]string{}
	}
	m.strs[m.k(key, name)] = value
	m.writes++
	return nil
}

func (m *memRun) Binary(key, name string) ([]byte, bool, error) {
	v, ok := m.bins[m.k(key, name)]
	return v, ok, nil
}

func (m *memRun) DeleteValue(key, name string) error {
	k := m.k(key, name)
	if _, ok := m.strs[k]; ok {
		delete(m.strs, k)
		m.writes++
	}
	if _, ok := m.bins[k]; ok {
		delete(m.bins, k)
		m.writes++
	}
	return nil
}

var runApp = platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: `C:\Program Files\Notes\notes.exe`, Args: []string{"--profile", `work "a"`}}

func TestRunValue(t *testing.T) {
	want := `"C:\Program Files\Notes\notes.exe" --profile "work \"a\"" -tela-at-login`
	if got := runCommand(runApp); got != want {
		t.Errorf("runCommand = %s, want %s", got, want)
	}
	// What Windows splits the value into is the app's arguments, then the
	// flag, which platform strips.
	args := splitCommandLine(runCommand(runApp))
	if !slices.Equal(args, []string{runApp.Exec, "--profile", `work "a"`, platform.AtLoginFlag}) {
		t.Errorf("the Run value splits into %q", args)
	}
	if stripped, at := platform.StripAtLoginFlag(args); !at || !slices.Equal(stripped, args[:3]) {
		t.Errorf("StripAtLoginFlag = %q, %v", stripped, at)
	}
}

func TestSetRunAtLogin(t *testing.T) {
	reg := &memRun{}
	if on, _ := runsAtLogin(reg, runApp); on {
		t.Error("runs at login before anything was written")
	}
	if changed, err := setRunAtLogin(reg, runApp, true); !changed || err != nil {
		t.Fatalf("on = %v, %v", changed, err)
	}
	if v, _, _ := reg.String(runKey, runApp.ID); v != runCommand(runApp) {
		t.Errorf("Run value %q", v)
	}
	if on, _ := runsAtLogin(reg, runApp); !on {
		t.Error("not on after on")
	}
	writes := reg.writes
	if changed, _ := setRunAtLogin(reg, runApp, true); changed || reg.writes != writes {
		t.Error("on again wrote")
	}
	// The user turns it off in Task Manager: off for StartsAtLogin; the
	// app's own switch turns it on again.
	reg.bins = map[string][]byte{reg.k(startupApprovedKey, runApp.ID): {3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}}
	if on, _ := runsAtLogin(reg, runApp); on {
		t.Error("on although Task Manager turned it off")
	}
	if changed, _ := setRunAtLogin(reg, runApp, true); !changed {
		t.Error("on did not clear Task Manager's switch")
	}
	if on, _ := runsAtLogin(reg, runApp); !on {
		t.Error("still off")
	}
	reg.bins[reg.k(startupApprovedKey, runApp.ID)] = []byte{2, 0, 0, 0}
	if on, _ := runsAtLogin(reg, runApp); !on {
		t.Error("an enabled StartupApproved value counts as off")
	}
	if changed, _ := setRunAtLogin(reg, runApp, false); !changed {
		t.Error("off did not remove the value")
	}
	if _, ok, _ := reg.String(runKey, runApp.ID); ok {
		t.Error("the Run value is still there")
	}
	if changed, _ := setRunAtLogin(reg, runApp, false); changed {
		t.Error("off twice changed something")
	}
}

func TestQuoteArgs(t *testing.T) {
	for _, args := range [][]string{
		{"notes:new"},
		{"", "a b", `a\b`, `a\\"b`, `trailing\`, `trailing space\ `, `"`, "tab\there"},
		{`C:\Users\Ana\My Notes\`, "--x=1"},
	} {
		line := quoteArgs(args)
		if got := splitCommandLine("x.exe " + line)[1:]; !slices.Equal(got, args) {
			t.Errorf("quoteArgs(%q) = %s, splits into %q", args, line, got)
		}
	}
}

// splitCommandLine splits a command line as CommandLineToArgvW does for
// the arguments after the program (the program itself: up to the next
// space, or quoted without escapes), the rules quoteArgs targets.
func splitCommandLine(s string) []string {
	var out []string
	i := 0
	// The program name.
	var prog strings.Builder
	if strings.HasPrefix(s, `"`) {
		j := strings.IndexByte(s[1:], '"')
		prog.WriteString(s[1 : 1+j])
		i = j + 2
	} else {
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			prog.WriteByte(s[i])
			i++
		}
	}
	out = append(out, prog.String())
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return out
		}
		var b strings.Builder
		quoted := false
		for i < len(s) && (quoted || (s[i] != ' ' && s[i] != '\t')) {
			switch s[i] {
			case '\\':
				n := 0
				for i < len(s) && s[i] == '\\' {
					n++
					i++
				}
				if i < len(s) && s[i] == '"' {
					b.WriteString(strings.Repeat(`\`, n/2))
					if n%2 == 1 {
						b.WriteByte('"')
						i++
					}
				} else {
					b.WriteString(strings.Repeat(`\`, n))
				}
				continue
			case '"':
				if quoted && i+1 < len(s) && s[i+1] == '"' {
					b.WriteByte('"')
					i += 2
					continue
				}
				quoted = !quoted
			default:
				b.WriteByte(s[i])
			}
			i++
		}
		out = append(out, b.String())
	}
}

func TestICOFile(t *testing.T) {
	small := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	big := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	data, err := icoFile([]image.Image{small, big})
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if le.Uint16(data[0:]) != 0 || le.Uint16(data[2:]) != 1 || le.Uint16(data[4:]) != 2 {
		t.Fatalf("ICONDIR % x", data[:6])
	}
	for i, side := range []byte{16, 0} {
		e := data[6+16*i:]
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		if e[0] != side || e[1] != side || le.Uint16(e[4:]) != 1 || le.Uint16(e[6:]) != 32 {
			t.Errorf("entry %d: % x", i, e[:16])
		}
		img, err := png.Decode(bytes.NewReader(data[off : off+size]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if w := img.Bounds().Dx(); w != []int{16, 256}[i] {
			t.Errorf("entry %d is %d px wide", i, w)
		}
	}
	if _, err := icoFile(nil); err == nil {
		t.Error("an icon of no images")
	}
}
