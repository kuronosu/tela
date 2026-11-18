package update

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
)

const testAppID = "com.example.notes"

func TestMain(m *testing.M) {
	if mode := os.Getenv(childEnv); mode != "" {
		runChild(mode) // run_test.go
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "tela-update-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv(desktop.DirEnv, dir)
	desktop.SetAppID(testAppID)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// t0 is the tests' "now".
var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// fixture is one test's world: a server, a key, the clock, the running
// version and a clean user state.
type fixture struct {
	t    *testing.T
	srv  *httptest.Server
	key  ed25519.PrivateKey
	pub  PublicKey
	mu   sync.Mutex
	docs map[string][]byte
	log  []string // "GET /path range"
}

func newFixture(t *testing.T, version string) *fixture {
	t.Helper()
	f := &fixture{t: t, docs: map[string][]byte{}}
	_, f.key, _ = ed25519.GenerateKey(nil)
	f.pub = NewPublicKey(f.key.Public().(ed25519.PublicKey))
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.log = append(f.log, strings.TrimSpace("GET "+r.URL.Path+" "+r.Header.Get("Range")))
		data, ok := f.docs[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(f.srv.Close)
	setVar(t, &now, func() time.Time { return t0 })
	setVar(t, &currentVersion, func() string { return version })
	// A clean user state and cache.
	for _, d := range []func() (string, error){desktop.StateDir, desktop.CacheDir} {
		dir, err := d()
		if err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(dir)
	}
	return f
}

func setVar[T any](t *testing.T, p *T, v T) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func (f *fixture) setVersion(v string) {
	currentVersion = func() string { return v }
}

func (f *fixture) put(path string, data []byte) {
	f.mu.Lock()
	f.docs[path] = data
	f.mu.Unlock()
}

func (f *fixture) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

// publish signs m with the fixture's key and serves it at /manifest.json.
func (f *fixture) publish(m *Manifest) {
	f.t.Helper()
	data, err := m.Encode()
	if err != nil {
		f.t.Fatal(err)
	}
	f.publishBytes(data, f.key)
}

func (f *fixture) publishBytes(data []byte, keys ...ed25519.PrivateKey) {
	f.t.Helper()
	sig, err := Sign(data, keys...)
	if err != nil {
		f.t.Fatal(err)
	}
	f.put("/manifest.json", data)
	f.put("/manifest.json.sig", sig)
}

func (f *fixture) config() Config {
	return Config{URL: f.srv.URL + "/manifest.json", PublicKeys: []PublicKey{f.pub}}
}

// artifact serves data at /files/name and returns its Artifact.
func (f *fixture) artifact(name, format string, data []byte) Artifact {
	f.put("/files/"+name, data)
	sum := sha256.Sum256(data)
	return Artifact{OS: goos(), Arch: goarch(), Format: format, URL: "files/" + name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
}

// manifest returns a manifest of the test app with releases on stable.
func manifest(rs ...Release) *Manifest {
	return &Manifest{Schema: 1, AppID: testAppID, Published: t0.Add(-time.Hour), Expires: t0.Add(30 * 24 * time.Hour), Channels: map[string][]Release{"stable": rs}}
}

func release(v string, as ...Artifact) Release {
	return Release{Version: v, Date: t0.Add(-2 * time.Hour), Notes: "notes of " + v, Artifacts: as}
}

func share(x float64) *float64 { return &x }

// tempDir returns a new temporary folder by its canonical path. Detection
// resolves the running program's path (filepath.EvalSymlinks), so the
// install's paths are canonical: on macOS the temporary folder is reached
// through a link (/var -> /private/var), and on a Windows runner TMP may
// hold an 8.3 short name (C:\Users\RUNNER~1), which EvalSymlinks expands.
// Tests that compare paths with the install's, or match them in a hook,
// make their folders here.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// useEnv makes detection see e.
func useEnv(t *testing.T, e sysEnv) {
	setVar(t, &detectEnv, func() sysEnv { return e })
}

func env(goos, exe string, vars map[string]string) sysEnv {
	return sysEnv{
		goos:       goos,
		getenv:     func(k string) string { return vars[k] },
		executable: func() (string, error) { return exe, nil },
		msix:       func() bool { return false },
	}
}

// appImage makes an AppImage install holding content and makes
// detection see it.
func appImage(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "Notes.AppImage")
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	useEnv(t, env("linux", "/tmp/.mount_Notes/usr/bin/notes", map[string]string{"APPIMAGE": p, "APPDIR": "/tmp/.mount_Notes"}))
	return p
}

// zipOfFiles returns a zip of files (name → content; a name ending in
// "/" is a folder, content "->target" a symbolic link), sorted by name.
func zipOfFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		c := files[n]
		h := &zip.FileHeader{Name: n, Method: zip.Deflate}
		switch {
		case strings.HasSuffix(n, "/"):
			h.SetMode(fs.ModeDir | 0o755)
		case strings.HasPrefix(c, "->"):
			h.SetMode(fs.ModeSymlink | 0o777)
			c = c[2:]
		case strings.Contains(n, "MacOS/") || strings.HasSuffix(n, ".exe"):
			h.SetMode(0o755)
		default:
			h.SetMode(0o644)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(c))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for n, c := range files {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func marker(version string, files ...string) string {
	q := make([]string, len(files))
	for i, f := range files {
		q[i] = fmt.Sprintf("%q", f)
	}
	return fmt.Sprintf(`{"schema":1,"format":"zip","appID":%q,"version":%q,"exe":"notes.exe","files":[%s]}`, testAppID, version, strings.Join(q, ","))
}
