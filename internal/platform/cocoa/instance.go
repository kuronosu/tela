package cocoa

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure part of single instance (ADR-224): the files derived from the
// app ID and the messages on the instance socket. instance_unix.go holds
// the lock and the socket (it builds on Linux too, for its tests), and
// instance_darwin.go is the Desktop method.
//
// The protocol, on a stream connection to the first process's socket:
//
//	first  -> second: hello {pid}         the first process's PID
//	second -> first:  launch {Launch}     dir, token, args
//	first  -> second: ack {}              the launch was taken
//
// Each message is a frame: its length (4 bytes, big endian, at most
// maxInstanceFrame), then the protocol version, the kind and the body.
// Strings are their length in bytes (uvarint) and their UTF-8 bytes. The
// hello comes first so that the second process can yield the activation
// to the first before the first acts on the launch.

// maxSunPath is the longest socket path: macOS's sun_path holds 104 bytes
// with the terminating NUL (Linux's 108; the shorter one is used
// everywhere).
const maxSunPath = 103

// maxReadableID bounds the readable part of a derived file name.
const maxReadableID = 64

// instanceName returns the base name of the lock file and the socket of
// appID, without extension: "tela-" and the ID when it is made of ASCII
// letters, digits, '.', '-' and '_' and is short, else "tela-" and a hash
// of the ID (so that two IDs never share files). hashed forces the hash.
func instanceName(appID string, hashed bool) string {
	plain := len(appID) <= maxReadableID && strings.IndexFunc(appID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_')
	}) < 0
	if plain && !hashed {
		return "tela-" + appID
	}
	sum := sha256.Sum256([]byte(appID))
	return "tela-" + hex.EncodeToString(sum[:12])
}

// instancePaths returns the lock file and the socket of appID in dir (the
// per-user temporary folder, os.TempDir on macOS): dir/<name>.lock and
// dir/<name>.sock, with the readable name when the socket path fits in
// sun_path, else the hashed one. A dir too long even for the hashed name
// is an error.
func instancePaths(dir, appID string) (lock, sock string, err error) {
	if dir == "" {
		return "", "", errors.New("no temporary folder")
	}
	for _, hashed := range []bool{false, true} {
		name := instanceName(appID, hashed)
		sock = filepath.Join(dir, name+".sock")
		if len(sock) <= maxSunPath {
			return filepath.Join(dir, name+".lock"), sock, nil
		}
	}
	return "", "", fmt.Errorf("the temporary folder %q is too long for a socket path (%d bytes at most)", dir, maxSunPath)
}

// Protocol version and message kinds.
const (
	instanceVersion = 1

	msgHello  = 1
	msgLaunch = 2
	msgAck    = 3
)

// maxInstanceFrame bounds a message: a command line is at most ARG_MAX
// (1 MiB on macOS), so a larger frame is not from a Tela app.
const maxInstanceFrame = 1 << 20

var errBadInstanceMessage = errors.New("cocoa: malformed single-instance message")

// appendFrame appends the frame of a message of kind with body to dst.
func appendFrame(dst []byte, kind byte, body []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(2+len(body)))
	dst = append(dst, instanceVersion, kind)
	return append(dst, body...)
}

// readFrame reads one frame and returns its kind and body. It refuses a
// frame above maxInstanceFrame without reading it, and one of another
// protocol version.
func readFrame(r io.Reader) (kind byte, body []byte, err error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < 2 || n > maxInstanceFrame {
		return 0, nil, fmt.Errorf("%w: a frame of %d bytes", errBadInstanceMessage, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	if buf[0] != instanceVersion {
		return 0, nil, fmt.Errorf("cocoa: single-instance protocol version %d; this build speaks %d", buf[0], instanceVersion)
	}
	return buf[1], buf[2:], nil
}

// encodeHello is the body of a hello: the first process's PID.
func encodeHello(pid int) []byte { return binary.AppendUvarint(nil, uint64(pid)) }

func decodeHello(b []byte) (int, error) {
	v, n := binary.Uvarint(b)
	if n <= 0 || n != len(b) || v == 0 || v > 1<<31-1 {
		return 0, errBadInstanceMessage
	}
	return int(v), nil
}

// encodeLaunch is the body of a launch: the working directory, the token
// and the number of arguments, then the arguments.
func encodeLaunch(l platform.Launch) []byte {
	var b []byte
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

// decodeLaunch is the inverse of encodeLaunch. It copies what it keeps and
// never panics on malformed input.
func decodeLaunch(b []byte) (platform.Launch, error) {
	var l platform.Launch
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
		return platform.Launch{}, errBadInstanceMessage
	}
	if l.Token, ok = str(); !ok {
		return platform.Launch{}, errBadInstanceMessage
	}
	count, ok := uvarint()
	if !ok || count > uint64(len(b)) { // every argument takes at least a byte
		return platform.Launch{}, errBadInstanceMessage
	}
	for range count {
		a, ok := str()
		if !ok {
			return platform.Launch{}, errBadInstanceMessage
		}
		l.Args = append(l.Args, a)
	}
	if len(b) != 0 {
		return platform.Launch{}, errBadInstanceMessage
	}
	return l, nil
}
