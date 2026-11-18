package update

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
)

func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }

func TestSemverOrder(t *testing.T) {
	// Ascending, from the Semantic Versioning specification's examples.
	order := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "v1.0.1", "1.2.0", "1.10.0", "2.0.0"}
	for i := range order {
		for j := range order {
			c, err := CompareVersions(order[i], order[j])
			if err != nil {
				t.Fatal(err)
			}
			if want := cmpInt(i, j); c != want {
				t.Errorf("CompareVersions(%s, %s) = %d, want %d", order[i], order[j], c, want)
			}
		}
	}
	if c, _ := CompareVersions("1.0.0+build.5", "1.0.0"); c != 0 {
		t.Errorf("build metadata changes precedence: %d", c)
	}
	for _, bad := range []string{"", "1", "1.2", "1.2.3.4", "01.2.3", "1.2.3-", "1.2.3-01", "1.2.3-a..b", "a.b.c", "1.2.3+", "-1.2.3"} {
		if _, err := parseSemver(bad); err == nil {
			t.Errorf("parseSemver(%q) accepted", bad)
		}
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestCheckChoosesTheNewestOfItsChannel(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	m := manifest(
		release("1.1.0", f.artifact("a-1.1.0", "appimage", []byte("v1.1"))),
		release("1.2.0", f.artifact("a-1.2.0", "appimage", []byte("v1.2"))),
		release("1.3.0", f.artifact("deb-1.3.0", "deb", []byte("deb")), Artifact{OS: "plan9", Arch: goarch(), Format: "appimage", URL: "x", Size: 1, SHA256: strings.Repeat("0", 64)}),
	)
	m.Channels["beta"] = []Release{release("1.4.0-beta.1", f.artifact("a-1.4.0-beta.1", "appimage", []byte("beta")))}
	f.publish(m)

	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	// 1.3.0 has no AppImage for this OS.
	if u.Version != "1.2.0" || u.Notes != "notes of 1.2.0" || u.From != "1.0.0" || u.Policy != ActivateNextStart {
		t.Errorf("update %+v, want 1.2.0 from 1.0.0, next-start", u.Release)
	}
	if want := f.srv.URL + "/files/a-1.2.0"; u.url != want {
		t.Errorf("artifact URL %s, want %s (relative to the manifest)", u.url, want)
	}

	cfg := f.config()
	cfg.Channel = "beta"
	cfg.Policy = ActivateByApp
	u, err = Check(context.Background(), cfg)
	if err != nil || u.Version != "1.4.0-beta.1" || u.Policy != ActivateByApp {
		t.Errorf("beta: %v, %v; want 1.4.0-beta.1 with the app's policy", u, err)
	}
	cfg.Channel = "nightly"
	if _, err := Check(context.Background(), cfg); !errors.Is(err, ErrUpToDate) {
		t.Errorf("a channel the manifest lacks: %v, want ErrUpToDate", err)
	}

	f.setVersion("1.2.0")
	if _, err := Check(context.Background(), f.config()); !errors.Is(err, ErrUpToDate) {
		t.Errorf("at 1.2.0: %v, want ErrUpToDate", err)
	}
}

func TestCheckMinVersionAndActivation(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	r2 := release("2.0.0", f.artifact("a-2", "appimage", []byte("2")))
	r2.MinVersion = "1.5.0"
	r15 := release("1.5.0", f.artifact("a-15", "appimage", []byte("15")))
	r15.Activation = ActivateNow
	f.publish(manifest(r2, r15))
	u, err := Check(context.Background(), f.config())
	if err != nil || u.Version != "1.5.0" || u.Policy != ActivateNow {
		t.Fatalf("from 1.0.0: %v, %v; want the stepping stone 1.5.0, activation now", u, err)
	}
	f.setVersion("1.5.0")
	if u, err := Check(context.Background(), f.config()); err != nil || u.Version != "2.0.0" {
		t.Fatalf("from 1.5.0: %v, %v; want 2.0.0", u, err)
	}
}

func TestCheckSignatureAndFreshness(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	a := f.artifact("a", "appimage", []byte("x"))
	good, _ := manifest(release("1.1.0", a)).Encode()
	_, other, _ := ed25519.GenerateKey(nil)
	ctx := context.Background()

	check := func(name string, want error) {
		t.Helper()
		_, err := Check(ctx, f.config())
		if !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
	}

	f.publishBytes(good, f.key)
	f.put("/manifest.json", append(slices.Clone(good), ' ')) // changed after signing
	check("a manifest changed after signing", ErrSignature)

	f.publishBytes(good, other)
	check("signed by an unknown key", ErrSignature)

	f.publishBytes(good, other, f.key) // key rotation: both sign
	check("signed by an unknown and the trusted key", nil)

	f.publishBytes(good, f.key)
	f.put("/manifest.json.sig", []byte(`{"signatures":[]}`))
	check("no signature", ErrSignature)
	f.mu.Lock()
	delete(f.docs, "/manifest.json.sig")
	f.mu.Unlock()
	check("no signature file", ErrSignature)

	cfg := f.config()
	cfg.PublicKeys = nil
	f.publishBytes(good, f.key)
	if _, err := Check(ctx, cfg); !errors.Is(err, ErrSignature) {
		t.Errorf("no public keys: %v", err)
	}
	cfg.PublicKeys = []PublicKey{f.pub, "ed25519:not-base64"}
	if _, err := Check(ctx, cfg); !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), "not-base64") {
		t.Errorf("a malformed public key: %v, want ErrSignature naming it", err)
	}

	m := manifest(release("1.1.0", a))
	m.Expires = t0
	f.publish(m)
	check("expired", ErrExpired)

	m = manifest(release("1.1.0", a))
	m.Published = t0.Add(-10 * time.Minute)
	f.publish(m)
	check("newer", nil)
	m.Published = t0.Add(-20 * time.Minute)
	f.publish(m)
	check("older than the newest seen", ErrOlderManifest)
	m.Published = t0.Add(-10 * time.Minute)
	f.publish(m)
	check("the same again", nil)

	m.AppID = "com.example.other"
	f.publish(m)
	if _, err := Check(ctx, f.config()); err == nil || !strings.Contains(err.Error(), "com.example.other") {
		t.Errorf("another app's manifest: %v", err)
	}
	m.AppID, m.Schema = testAppID, 2
	f.publish(m)
	if _, err := Check(ctx, f.config()); err == nil || !strings.Contains(err.Error(), "schema 2") {
		t.Errorf("schema 2: %v", err)
	}
}

func TestCheckEndOfSupport(t *testing.T) {
	f := newFixture(t, "0.9.0")
	appImage(t, "v1")
	m := manifest(release("2.0.0", f.artifact("a", "appimage", []byte("x"))))
	m.EndOfSupport = &EndOfSupport{Below: "1.0.0", Message: "Download Notes 2.", URL: "https://example.com/notes"}
	f.publish(m)
	_, err := Check(context.Background(), f.config())
	var eos *EndOfSupportError
	if !errors.Is(err, ErrEndOfSupport) || !errors.As(err, &eos) || eos.Message != "Download Notes 2." || eos.URL != "https://example.com/notes" || eos.Version != "0.9.0" {
		t.Fatalf("Check = %v, want an EndOfSupportError with the manifest's message and URL", err)
	}
	f.setVersion("1.0.0")
	if u, err := Check(context.Background(), f.config()); err != nil || u.Version != "2.0.0" {
		t.Errorf("at 1.0.0: %v, %v", u, err)
	}
}

func TestRolloutBuckets(t *testing.T) {
	// The bucket is fixed by its definition: SHA-256 of the ID, a zero
	// byte and the version, its first 8 bytes scaled to [0, 1).
	if got := rolloutBucket("00112233445566778899aabbccddeeff", "1.2.0"); fmt.Sprintf("%.6f", got) != "0.250737" {
		t.Errorf("bucket = %.6f, want 0.250737 (independently computed)", got)
	}
	in := 0
	for i := range 20000 {
		id := fmt.Sprintf("%032x", i)
		b := rolloutBucket(id, "1.2.0")
		r25, r50 := Release{Rollout: share(0.25)}, Release{Rollout: share(0.5)}
		if r25.inRollout(b) {
			in++
			if !r50.inRollout(b) {
				t.Fatalf("install %s in the 25%% rollout but not in the 50%% one", id)
			}
		}
		if (&Release{Rollout: share(0)}).inRollout(b) || !(&Release{Rollout: share(1)}).inRollout(b) || !(&Release{}).inRollout(b) {
			t.Fatalf("install %s: rollout 0 includes it or 1 or none excludes it", id)
		}
	}
	if in < 4700 || in > 5300 {
		t.Errorf("%d of 20000 installs in a 25%% rollout", in)
	}
	if rolloutBucket("a", "1.2.0") == rolloutBucket("a", "1.3.0") {
		t.Errorf("the version does not reshuffle the buckets")
	}
}

func TestCheckRollout(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	r := release("1.1.0", f.artifact("a", "appimage", []byte("x")))
	f.publish(manifest(r))
	if _, err := Check(context.Background(), f.config()); err != nil {
		t.Fatal(err)
	}
	stateMu.Lock()
	us, _, err := loadUserState()
	stateMu.Unlock()
	if err != nil || len(us.InstallID) != 32 {
		t.Fatalf("install ID %q, %v", us.InstallID, err)
	}
	b := rolloutBucket(us.InstallID, "1.1.0")
	r.Rollout = share(b) // just above the bucket: excluded
	f.publish(manifest(r))
	if _, err := Check(context.Background(), f.config()); !errors.Is(err, ErrUpToDate) {
		t.Errorf("rollout at the install's bucket: %v, want ErrUpToDate", err)
	}
	r.Rollout = share(b + 1e-9)
	f.publish(manifest(r))
	if _, err := Check(context.Background(), f.config()); err != nil {
		t.Errorf("rollout above the install's bucket: %v", err)
	}
}

// A manifest from a later Tela: unknown fields everywhere and a patch of
// an unknown kind. The client ignores them and downloads the full file.
func TestUnknownFieldsAndPatchKinds(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	a := f.artifact("full", "appimage", []byte("the full file"))
	doc := fmt.Sprintf(`{
  "schema": 1, "appID": %q, "published": %q, "expires": %q, "mirrors": ["https://mirror.example"],
  "channels": {"stable": [{
    "version": "1.1.0", "date": %q, "signedBy": "release-bot", "rollout": 1,
    "artifacts": [{
      "os": %q, "arch": %q, "format": "appimage", "url": %q, "size": %d, "sha256": %q, "compression": "none",
      "patches": [
        {"from": "1.0.0", "fromSha256": "%s", "kind": "future-delta-v9", "url": "files/patch", "size": 3, "sha256": "%s", "window": 64}
      ]
    }]
  }]}
}`, testAppID, t0.Add(-time.Hour).Format(time.RFC3339), t0.Add(time.Hour).Format(time.RFC3339), t0.Format(time.RFC3339),
		a.OS, a.Arch, a.URL, a.Size, a.SHA256, strings.Repeat("1", 64), strings.Repeat("2", 64))
	f.put("/files/patch", []byte("not a patch Tela knows"))
	f.publishBytes([]byte(doc), f.key)

	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Artifact.Patches) != 1 || u.Artifact.Patches[0].Kind != "future-delta-v9" {
		t.Errorf("patches %+v", u.Artifact.Patches)
	}
	if err := u.Download(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.requests() {
		if strings.Contains(r, "/files/patch") {
			t.Errorf("the client fetched the patch: %v", f.requests())
		}
	}
	if !slices.Contains(f.requests(), "GET /files/full") {
		t.Errorf("the full artifact was not fetched: %v", f.requests())
	}
}

func TestDownloadVerifies(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	data := []byte(strings.Repeat("0123456789", 10000))
	a := f.artifact("a", "appimage", data)
	f.publish(manifest(release("1.1.0", a)))
	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}

	// Other bytes of the same length.
	f.put("/files/a", []byte(strings.Repeat("x", len(data))))
	if err := u.Download(context.Background(), nil); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("other bytes: %v, want ErrHashMismatch", err)
	}
	// Longer.
	f.put("/files/a", append(slices.Clone(data), 'x'))
	if err := u.Download(context.Background(), nil); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("a longer file: %v, want ErrHashMismatch", err)
	}
	if err := u.Apply(); !errors.Is(err, errNotDownloaded) {
		t.Errorf("Apply after failed downloads: %v", err)
	}
	dir := filepath.Dir(cachePath(t, a))
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("failed downloads left %v", ents)
	}

	// The right bytes, with progress.
	f.put("/files/a", data)
	var calls [][2]int64
	if err := u.Download(context.Background(), func(done, total int64) { calls = append(calls, [2]int64{done, total}) }); err != nil {
		t.Fatal(err)
	}
	if len(calls) < 2 || calls[0] != [2]int64{0, int64(len(data))} || calls[len(calls)-1] != [2]int64{int64(len(data)), int64(len(data))} {
		t.Errorf("progress %v", calls)
	}
	for i := 1; i < len(calls); i++ {
		if calls[i][0] < calls[i-1][0] {
			t.Errorf("progress went back: %v", calls)
		}
	}
	// A second download uses the file.
	n := len(f.requests())
	if err := u.Download(context.Background(), nil); err != nil || len(f.requests()) != n {
		t.Errorf("second download: %v, %d requests", err, len(f.requests())-n)
	}
	// A file that changed in the cache is caught before use.
	os.WriteFile(u.file, []byte(strings.Repeat("y", len(data))), 0o600)
	if err := u.Apply(); !errors.Is(err, ErrHashMismatch) {
		t.Errorf("Apply of a changed download: %v", err)
	}
}

func cachePath(t *testing.T, a Artifact) string {
	t.Helper()
	dir, err := desktop.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "update", a.SHA256)
}

func TestDownloadResumes(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	data := []byte(strings.Repeat("abcdefghij", 5000))
	a := f.artifact("a", "appimage", data)
	f.publish(manifest(release("1.1.0", a)))
	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	part := cachePath(t, a) + ".part"
	os.MkdirAll(filepath.Dir(part), 0o700)
	if err := os.WriteFile(part, data[:20000], 0o600); err != nil {
		t.Fatal(err)
	}
	var first int64 = -1
	if err := u.Download(context.Background(), func(done, _ int64) {
		if first < 0 {
			first = done
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.requests(), "GET /files/a bytes=20000-") {
		t.Errorf("no range request: %v", f.requests())
	}
	if first != 20000 {
		t.Errorf("progress started at %d, want 20000", first)
	}
	if got := readFile(t, u.file); got != string(data) {
		t.Errorf("resumed download differs")
	}
}

// A part that holds every byte (the process ended between the last write
// and the rename) is checked and renamed, not fetched again; a full part
// of other bytes is fetched again.
func TestDownloadCompletePart(t *testing.T) {
	f := newFixture(t, "1.0.0")
	appImage(t, "v1")
	data := []byte(strings.Repeat("abcdefghij", 5000))
	a := f.artifact("a", "appimage", data)
	f.publish(manifest(release("1.1.0", a)))
	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	part := cachePath(t, a) + ".part"
	os.MkdirAll(filepath.Dir(part), 0o700)
	if err := os.WriteFile(part, data, 0o600); err != nil {
		t.Fatal(err)
	}
	n := len(f.requests())
	var calls [][2]int64
	if err := u.Download(context.Background(), func(done, total int64) { calls = append(calls, [2]int64{done, total}) }); err != nil {
		t.Fatal(err)
	}
	if got := f.requests()[n:]; len(got) != 0 {
		t.Errorf("a complete part was fetched again: %v", got)
	}
	if u.file != cachePath(t, a) || readFile(t, u.file) != string(data) {
		t.Errorf("the part was not renamed into place: %q", u.file)
	}
	if _, err := os.Stat(part); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the part is still there: %v", err)
	}
	if len(calls) != 1 || calls[0] != [2]int64{int64(len(data)), int64(len(data))} {
		t.Errorf("progress %v", calls)
	}

	// A full part of other bytes.
	os.Remove(u.file)
	if err := os.WriteFile(part, []byte(strings.Repeat("y", len(data))), 0o600); err != nil {
		t.Fatal(err)
	}
	n = len(f.requests())
	if err := u.Download(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := f.requests()[n:]; !slices.Equal(got, []string{"GET /files/a"}) {
		t.Errorf("a wrong full part: requests %v, want one full GET", got)
	}
	if readFile(t, u.file) != string(data) {
		t.Errorf("download differs")
	}
}

func TestDetectInstall(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"var/lib/dpkg/info/notes.list": "/.\n/usr\n/usr/bin\n/usr/bin/notes\n",
		"var/lib/dpkg/info/other.list": "/usr/bin/other\n",
	})
	zipDir := tempDir(t)
	writeFiles(t, zipDir, map[string]string{MarkerName: marker("1.0.0", "notes.exe", MarkerName), "notes.exe": "v1"})

	// An AppImage whose APPDIR is reached through a link: detection
	// resolves the program's path, and APPDIR's with it.
	linked := filepath.Join(tempDir(t), "mount")
	writeFiles(t, root, map[string]string{"mount/usr/bin/notes": "v1"})
	linkErr := os.Symlink(filepath.Join(root, "mount"), linked)

	type testCase struct {
		name   string
		e      sysEnv
		format string // "" for ErrNotInstalled
		path   string
		mgr    string
	}
	var linkedCase []testCase
	if linkErr == nil { // Windows makes links only with a privilege
		linkedCase = append(linkedCase, testCase{name: "AppImage through a link", e: env("linux", filepath.Join(linked, "usr", "bin", "notes"), map[string]string{"APPIMAGE": "/home/u/Notes.AppImage", "APPDIR": linked}), format: "appimage", path: "/home/u/Notes.AppImage"})
	}
	for _, tc := range append(linkedCase, []testCase{
		{name: "flatpak", e: env("linux", "/app/bin/notes", map[string]string{"FLATPAK_ID": testAppID}), format: "flatpak", mgr: "flatpak"},
		{name: "AppImage", e: env("linux", "/tmp/.mount_x/usr/bin/notes", map[string]string{"APPIMAGE": "/home/u/Notes.AppImage", "APPDIR": "/tmp/.mount_x"}), format: "appimage", path: "/home/u/Notes.AppImage"},
		{name: "a program started from an AppImage", e: env("linux", "/usr/bin/xterm", map[string]string{"APPIMAGE": "/home/u/Notes.AppImage", "APPDIR": "/tmp/.mount_x"})},
		{name: "bundle", e: env("darwin", "/Applications/Notes.app/Contents/MacOS/notes", nil), format: "app", path: "/Applications/Notes.app"},
		{name: "not a bundle", e: env("darwin", "/usr/local/bin/notes", nil)},
		{name: "zip", e: env("windows", filepath.Join(zipDir, "notes.exe"), nil), format: "zip", path: zipDir},
		{name: "deb", e: env("linux", "/usr/bin/notes", nil), format: "deb", mgr: "apt"},
		{name: "not a deb", e: env("linux", "/usr/bin/unlisted", nil)},
		{name: "msix", e: func() sysEnv {
			e := env("windows", `C:\Program Files\WindowsApps\Notes\notes.exe`, nil)
			e.msix = func() bool { return true }
			return e
		}(), format: "msix", mgr: "msix"},
		{name: "go run", e: env("linux", "/home/u/.cache/go-build/exe/notes", nil)},
	}...) {
		tc.e.root = root
		useEnv(t, tc.e)
		in, err := findInstall("")
		var me *ManagedError
		switch {
		case tc.format == "":
			if !errors.Is(err, ErrNotInstalled) {
				t.Errorf("%s: %v, %v; want ErrNotInstalled", tc.name, in, err)
			}
		case tc.mgr != "":
			if !errors.As(err, &me) || !errors.Is(err, ErrManaged) || me.Format != tc.format || me.Manager != tc.mgr {
				t.Errorf("%s: %v, want a ManagedError %s by %s", tc.name, err, tc.format, tc.mgr)
			}
		case err != nil || in.format != tc.format || filepath.Clean(in.path) != filepath.Clean(filepath.FromSlash(tc.path)):
			t.Errorf("%s: %+v, %v; want %s at %s", tc.name, in, err, tc.format, tc.path)
		}
	}
}

// Check answers managed installs without a request.
func TestCheckManaged(t *testing.T) {
	f := newFixture(t, "1.0.0")
	useEnv(t, env("linux", "/app/bin/notes", map[string]string{"FLATPAK_ID": testAppID}))
	_, err := Check(context.Background(), f.config())
	if !errors.Is(err, ErrManaged) || len(f.requests()) != 0 {
		t.Errorf("flatpak: %v after %d requests", err, len(f.requests()))
	}
	appImage(t, "v1")
	f.setVersion("")
	if _, err := Check(context.Background(), f.config()); !errors.Is(err, ErrNoVersion) {
		t.Errorf("no version: %v", err)
	}
}
