//go:build darwin

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/plist"
)

// bundleProbeSource is a program that asks AppKit's Foundation who its main
// bundle is, with purego like the cocoa backend (an out-of-repository module
// cannot import internal/darwin). The app's ID comes through a constant, so
// tela run reads it from the source.
const bundleProbeSource = `package main

import (
	"fmt"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/kuronosu/tela"
)

const appID = "dev.example." + "probe"

var App = tela.App{Title: "Probe", AppID: appID}

func main() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		fmt.Println("error:", err)
		return
	}
	bundle := objc.ID(objc.GetClass("NSBundle")).Send(objc.RegisterName("mainBundle"))
	id := objc.Send[string](objc.Send[objc.ID](bundle, objc.RegisterName("bundleIdentifier")), objc.RegisterName("UTF8String"))
	path := objc.Send[string](objc.Send[objc.ID](bundle, objc.RegisterName("bundlePath")), objc.RegisterName("UTF8String"))
	fmt.Printf("bundleIdentifier=%s\nbundlePath=%s\n", id, path)
}
`

// TestRunBundleIdentityDarwin is the macOS end-to-end check of tela run
// (ADR-227): the program it runs sees +[NSBundle mainBundle] as the bundle
// tela run wrote in the cache folder, with the app's ID read from the
// source. CI runs it with -run TestRunBundleIdentityDarwin on macos-latest.
func TestRunBundleIdentityDarwin(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{"probe/main.go": bundleProbeSource})
	t.Chdir(dir)
	cache := t.TempDir()
	defer func(f func() (string, error)) { userCacheDir = f }(userCacheDir)
	userCacheDir = func() (string, error) { return cache, nil }

	var stdout, stderr bytes.Buffer
	if code := run([]string{"run", "-v", "./probe"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela run: status %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	t.Logf("stdout: %s", stdout.String())
	bundle := filepath.Join(cache, "tela", "run", "dev.example.probe", "Probe.app")
	if !strings.Contains(stdout.String(), "bundleIdentifier=dev.example.probe\n") {
		t.Errorf("stdout lacks the bundle identifier dev.example.probe:\n%s", stdout.String())
	}
	// t.TempDir may be under /var, a link to /private/var, and AppKit
	// reports the path as the process was started (the runner gave
	// /var/...), so both sides are compared resolved.
	wantPath, err := filepath.EvalSymlinks(bundle)
	if err != nil {
		t.Fatalf("the bundle is not at %s: %v\nstderr: %s", bundle, err, stderr.String())
	}
	_, after, ok := strings.Cut(stdout.String(), "bundlePath=")
	gotPath, _, _ := strings.Cut(after, "\n")
	if resolved, err := filepath.EvalSymlinks(gotPath); !ok || err != nil || resolved != wantPath {
		t.Errorf("the program's bundle path is %q (resolved %q, %v), want %s", gotPath, resolved, err, wantPath)
	}

	raw, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := plist.UnmarshalDict(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info["CFBundleIdentifier"] != "dev.example.probe" || info["CFBundleExecutable"] != "Probe" {
		t.Errorf("Info.plist: %v", info)
	}

	// The same bundle the next time: nothing is rewritten.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"run", "-v", "./probe"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second tela run: status %d\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "created false, Info.plist written false, executable written false") {
		t.Errorf("the second run rewrote the bundle:\n%s", stderr.String())
	}
}
