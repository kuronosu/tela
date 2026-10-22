package term

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal pair and returns its terminal side.
func openPTY(t *testing.T) *os.File {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminals: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("unlockpt: %v", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Skipf("ptsname: %v", err)
	}
	s, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("opening the terminal side: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCbreakAndRestore(t *testing.T) {
	s := openPTY(t)
	if !IsTerminal(s.Fd()) {
		t.Fatal("a pty is not a terminal")
	}
	before, err := unix.IoctlGetTermios(int(s.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Cbreak(s.Fd())
	if err != nil {
		t.Fatal(err)
	}
	now, _ := unix.IoctlGetTermios(int(s.Fd()), unix.TCGETS)
	if now.Lflag&(unix.ICANON|unix.ECHO) != 0 {
		t.Errorf("ICANON or ECHO still set: %#x", now.Lflag)
	}
	if now.Lflag&unix.ISIG == 0 || now.Oflag&unix.OPOST != before.Oflag&unix.OPOST {
		t.Errorf("ISIG or output processing changed: lflag %#x oflag %#x", now.Lflag, now.Oflag)
	}
	if err := st.Restore(); err != nil {
		t.Fatal(err)
	}
	st.Restore() // idempotent
	after, _ := unix.IoctlGetTermios(int(s.Fd()), unix.TCGETS)
	if *after != *before {
		t.Errorf("not restored: %+v, want %+v", after, before)
	}
}

func TestNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f.Fd()) {
		t.Error("a file is a terminal")
	}
	if _, err := Cbreak(f.Fd()); err == nil {
		t.Error("Cbreak on a file succeeded")
	}
}
