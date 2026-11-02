package cocoa

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestInstancePaths(t *testing.T) {
	dir := "/var/folders/zz/zyxvpxvq6csfxvn_n0000000000000/T"
	lock, sock, err := instancePaths(dir+"/", "com.example.Notes")
	if err != nil || lock != filepath.Join(dir, "tela-com.example.Notes.lock") || sock != filepath.Join(dir, "tela-com.example.Notes.sock") {
		t.Errorf("instancePaths(macOS temp, com.example.Notes) = %q, %q, %v", lock, sock, err)
	}
	// IDs that are not plain ASCII names are hashed: two IDs that differ
	// only there never share files.
	_, a, _ := instancePaths(dir, "My Notes")
	_, b, _ := instancePaths(dir, "My-Notes")
	_, c, _ := instancePaths(dir, "Notas é")
	if a == b || !strings.HasPrefix(filepath.Base(a), "tela-") || strings.Contains(a, " ") || strings.Contains(c, "é") {
		t.Errorf("hashed names: %q, %q, %q", a, b, c)
	}
	if b != filepath.Join(dir, "tela-My-Notes.sock") {
		t.Errorf("a plain ID kept its name: %q", b)
	}
	// A long ID, or a long folder, gets the hashed name so the socket path
	// fits in sun_path.
	long := strings.Repeat("x", 90)
	lock, sock, err = instancePaths(dir, "com.example."+long)
	if err != nil || len(sock) > maxSunPath || strings.Contains(sock, long) || filepath.Dir(lock) != filepath.Clean(dir) ||
		strings.TrimSuffix(lock, ".lock") != strings.TrimSuffix(sock, ".sock") {
		t.Errorf("instancePaths with a long ID = %q (%d bytes), %q, %v", sock, len(sock), lock, err)
	}
	longDir := "/" + strings.Repeat("d", 60)
	if _, sock, err = instancePaths(longDir, "com.example.Notes.Application"); err != nil || len(sock) > maxSunPath {
		t.Errorf("instancePaths with a long folder = %q (%d bytes), %v", sock, len(sock), err)
	}
	if _, _, err := instancePaths("/"+strings.Repeat("d", 100), "a"); err == nil {
		t.Errorf("instancePaths with a folder too long for any socket = nil error")
	} else {
		t.Logf("too long: %v", err)
	}
	if _, _, err := instancePaths("", "a"); err == nil {
		t.Errorf("instancePaths without a folder = nil error")
	}
}

func TestLaunchEncoding(t *testing.T) {
	for _, l := range []platform.Launch{
		{},
		{Dir: "/Users/me", Args: []string{"notes:open?id=3", "/tmp/a b.txt", ""}},
		{Dir: "/", Token: "tok", Args: []string{strings.Repeat("é", 300)}},
	} {
		got, err := decodeLaunch(encodeLaunch(l))
		if err != nil || !reflect.DeepEqual(got, l) {
			t.Errorf("decodeLaunch(encodeLaunch(%+v)) = %+v, %v", l, got, err)
		}
	}
	enc := encodeLaunch(platform.Launch{Dir: "/d", Token: "t", Args: []string{"one", "two"}})
	for i := range len(enc) {
		if _, err := decodeLaunch(enc[:i]); err == nil {
			t.Errorf("decodeLaunch of the first %d of %d bytes = nil error", i, len(enc))
		}
	}
	if _, err := decodeLaunch(append(enc, 0)); err == nil {
		t.Errorf("decodeLaunch with a trailing byte = nil error")
	}
	if _, err := decodeLaunch([]byte{0, 0, 0xff, 0xff, 0xff, 0xff, 0x0f}); err == nil {
		t.Errorf("decodeLaunch claiming 4 billion arguments = nil error")
	}
}

func TestInstanceFrames(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(appendFrame(nil, msgHello, encodeHello(4242)))
	buf.Write(appendFrame(nil, msgAck, nil))
	kind, body, err := readFrame(&buf)
	if err != nil || kind != msgHello {
		t.Fatalf("readFrame = %d, %v", kind, err)
	}
	if pid, err := decodeHello(body); err != nil || pid != 4242 {
		t.Errorf("decodeHello = %d, %v", pid, err)
	}
	if kind, body, err = readFrame(&buf); err != nil || kind != msgAck || len(body) != 0 {
		t.Errorf("second frame = %d %q %v", kind, body, err)
	}
	for _, b := range [][]byte{
		{0, 0, 0, 1, instanceVersion},         // shorter than a header
		{0x7f, 0, 0, 0, instanceVersion, 1},   // too large: refused before reading
		{0, 0, 0, 2, instanceVersion + 1, 1},  // another version
		{0, 0, 0, 5, instanceVersion, msgAck}, // truncated
	} {
		if _, _, err := readFrame(bytes.NewReader(b)); err == nil {
			t.Errorf("readFrame(% x) = nil error", b)
		} else if errors.Is(err, errBadInstanceMessage) || strings.Contains(err.Error(), "version") {
			t.Logf("readFrame(% x): %v", b, err)
		}
	}
	for _, b := range [][]byte{nil, {0}, {0x80}, append(encodeHello(1), 0)} {
		if _, err := decodeHello(b); err == nil {
			t.Errorf("decodeHello(% x) = nil error", b)
		}
	}
}
