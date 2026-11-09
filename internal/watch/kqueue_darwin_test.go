//go:build darwin

package watch

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Past the open file limit, kqueue polls a file instead of holding a
// descriptor of it (ADR-262): a write in place, which only the file's own
// descriptor would report (its directory does not change), is still seen;
// a file that takes its name gets a descriptor again once one is free.
func TestKqueueDescriptorLimit(t *testing.T) {
	limited := func(p string) bool { return strings.HasSuffix(p, "limited.go") }
	kqFileLimit.Store(&limited)
	t.Cleanup(func() { kqFileLimit.Store(nil) })
	h := newHarness(t, false, Options{})
	k := h.w.impl.(*kqueue)

	h.write("limited.go", "package a")
	h.expect("a file past the limit created", []string{"limited.go"})
	k.mu.Lock()
	_, polled := k.polled[h.path("limited.go")]
	_, watched := k.files[h.path("limited.go")]
	k.mu.Unlock()
	if !polled || watched {
		t.Fatalf("polled %v, watched %v; want a polled file", polled, watched)
	}
	h.absent("settle", 150*time.Millisecond)

	// In place: same inode, a new size.
	f, err := os.OpenFile(h.path("limited.go"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n// more\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	h.expect("a write in place of a polled file", []string{"limited.go"})

	// Room again: a file renamed over it takes a descriptor.
	kqFileLimit.Store(nil)
	h.write("next.tmp", "package b")
	rename(t, h.path("next.tmp"), h.path("limited.go"))
	h.expect("a file renamed over the polled one", []string{"limited.go"})
	k.mu.Lock()
	_, polled = k.polled[h.path("limited.go")]
	_, watched = k.files[h.path("limited.go")]
	k.mu.Unlock()
	if polled || !watched {
		t.Errorf("polled %v, watched %v after the rename; want a descriptor", polled, watched)
	}

	// Closing stops the poll with the rest.
	if err := h.w.Close(); err != nil {
		t.Fatal(err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.polled) != 0 || k.pollStop != nil {
		t.Errorf("after Close: %d polled files, poll running %v", len(k.polled), k.pollStop != nil)
	}
}
