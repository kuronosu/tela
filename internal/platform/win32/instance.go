package win32

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure part of single instance (ADR-209): the names derived from the
// app ID and the payload of the WM_COPYDATA message that carries a Launch
// from the second process to the first. instance_windows.go does the OS
// calls.

// maxIDName bounds the readable part of a derived name: a kernel object
// name is at most MAX_PATH characters.
const maxIDName = 120

// sanitizedAppID makes an app ID usable in the name of a kernel object
// and of a window: ASCII letters, digits, '.', '-' and '_' stay, anything
// else becomes '_'. An ID that changed (or is long) gets a hash of the
// original appended, so that two IDs never share a name by mapping to the
// same text.
func sanitizedAppID(id string) string {
	var b strings.Builder
	changed := false
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
			changed = true
		}
	}
	s := b.String()
	if len(s) > maxIDName {
		s, changed = s[:maxIDName], true
	}
	if changed {
		sum := sha256.Sum256([]byte(id))
		s += "-" + hex.EncodeToString(sum[:8])
	}
	return s
}

// instanceMutexName is the name of the mutex that decides which process
// is the first: in the session's namespace, so every user and every
// session has its own instance.
func instanceMutexName(appID string) string { return `Local\tela-` + sanitizedAppID(appID) }

// instanceWindowName is the name (title) of the first process's
// message-only window, which the second finds with FindWindowExW and sends
// the launch to.
func instanceWindowName(appID string) string { return "tela-instance-" + sanitizedAppID(appID) }

// copyDataLaunch is the dwData of the WM_COPYDATA message that carries a
// Launch ("TLA" and the format version in the low byte): a window that
// receives another dwData ignores the message.
const copyDataLaunch = 0x544C4101

// launchVersion is the first byte of the payload; a payload of another
// version is refused.
const launchVersion = 1

// maxLaunchPayload bounds what the first process accepts: the command line
// of a process is at most 32 KB, so a payload larger than this is not from
// a Tela app.
const maxLaunchPayload = 1 << 20

// copyDataStruct is COPYDATASTRUCT.
type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

// encodeLaunch encodes l for WM_COPYDATA: the version byte, then the
// working directory, the token and the number of arguments, each string
// as its length in bytes (uvarint) and its UTF-8 bytes, then the
// arguments.
func encodeLaunch(l platform.Launch) []byte {
	b := []byte{launchVersion}
	str := func(s string) {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	str(l.Dir)
	str(l.Token)
	b = binary.AppendUvarint(b, uint64(len(l.Args)))
	for _, a := range l.Args {
		str(a)
	}
	return b
}

var errBadLaunch = errors.New("win32: malformed launch message")

// decodeLaunch is the inverse of encodeLaunch. It copies what it keeps and
// never panics on malformed input.
func decodeLaunch(b []byte) (platform.Launch, error) {
	var l platform.Launch
	if len(b) == 0 || len(b) > maxLaunchPayload {
		return l, errBadLaunch
	}
	if b[0] != launchVersion {
		return l, fmt.Errorf("win32: launch message of version %d; this build understands %d", b[0], launchVersion)
	}
	b = b[1:]
	uvarint := func() (uint64, bool) {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			return 0, false
		}
		b = b[n:]
		return v, true
	}
	str := func() (string, bool) {
		n, ok := uvarint()
		if !ok || n > uint64(len(b)) {
			return "", false
		}
		s := string(b[:n])
		b = b[n:]
		return s, true
	}
	var ok bool
	if l.Dir, ok = str(); !ok {
		return platform.Launch{}, errBadLaunch
	}
	if l.Token, ok = str(); !ok {
		return platform.Launch{}, errBadLaunch
	}
	count, ok := uvarint()
	if !ok || count > uint64(len(b)) { // every argument takes at least a byte
		return platform.Launch{}, errBadLaunch
	}
	for range count {
		a, ok := str()
		if !ok {
			return platform.Launch{}, errBadLaunch
		}
		l.Args = append(l.Args, a)
	}
	if len(b) != 0 {
		return platform.Launch{}, errBadLaunch
	}
	return l, nil
}
