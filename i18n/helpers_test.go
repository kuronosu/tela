package i18n

import (
	"sync"
	"testing"
	"testing/fstest"
)

// isolate gives a test an empty catalog set, no system locale and a
// captured dev log; the process-wide state is restored afterwards. Tests
// that use it must not run in parallel.
func isolate(t *testing.T) *[]string {
	t.Helper()
	oldOS, oldEnv, oldOut := osLocale, getenv, devOut
	osLocale = func() string { return "" }
	getenv = func(string) string { return "" }
	var mu sync.Mutex
	var log []string
	devOut = func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }
	devSeen = sync.Map{}
	reset := func() {
		stMu.Lock()
		st.Store(build(&state{cats: map[string]*catalog{}}))
		stMu.Unlock()
	}
	reset()
	t.Cleanup(func() {
		osLocale, getenv, devOut = oldOS, oldEnv, oldOut
		devSeen = sync.Map{}
		reset()
	})
	return &log
}

// catalogs builds a file system of catalog files: name -> JSON.
func catalogs(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys["locales/"+name] = &fstest.MapFile{Data: []byte(data)}
	}
	return fsys
}

func mustLoad(t *testing.T, files map[string]string) {
	t.Helper()
	if err := Load(catalogs(files), "locales/*.json"); err != nil {
		t.Fatal(err)
	}
}

// msg parses and evaluates a message for a locale.
func msg(t *testing.T, loc, src string, args ...any) string {
	t.Helper()
	nodes, err := parseMessage(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return evaluate(For(loc).d, "k", nodes, args)
}
