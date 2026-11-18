package core

import (
	"slices"
	"testing"
)

// saveRunHooks empties the hooks of Run's start and end for the test and
// puts the process's back when it ends.
func saveRunHooks(t *testing.T) {
	runStart.mu.Lock()
	first, fs := runStart.first, runStart.fs
	runStart.first, runStart.fs = nil, nil
	runStart.mu.Unlock()
	runEnd.mu.Lock()
	efs, last := runEnd.fs, runEnd.last
	runEnd.fs, runEnd.last = nil, nil
	runEnd.mu.Unlock()
	t.Cleanup(func() {
		runStart.mu.Lock()
		runStart.first, runStart.fs = first, fs
		runStart.mu.Unlock()
		runEnd.mu.Lock()
		runEnd.fs, runEnd.last = efs, last
		runEnd.mu.Unlock()
	})
}

// OnRunStartFirst's functions run before OnRunStart's whatever the order
// of registration, and a false ends the start; OnRunEndLast's run after
// OnRunEnd's, also those registered later.
func TestRunHookOrder(t *testing.T) {
	saveRunHooks(t)
	var got []string
	start := func(name string, ok bool) func(RunStart) bool {
		return func(RunStart) bool { got = append(got, name); return ok }
	}
	end := func(name string) func() { return func() { got = append(got, name) } }

	OnRunStart(start("desktop claim", true))
	OnRunStartFirst(start("update rollback", true))
	OnRunStart(start("desktop logs", true))
	OnRunStartFirst(start("first 2", true))
	if !runStartHooks(RunStart{}) {
		t.Fatal("the start ended")
	}
	if want := []string{"update rollback", "first 2", "desktop claim", "desktop logs"}; !slices.Equal(got, want) {
		t.Errorf("start order %q, want %q", got, want)
	}

	got = nil
	OnRunEnd(end("desktop flush"))
	OnRunEndLast(end("update restart"))
	OnRunEnd(end("desktop crash capture"))
	runEndHooks()
	if want := []string{"desktop flush", "desktop crash capture", "update restart"}; !slices.Equal(got, want) {
		t.Errorf("end order %q, want %q", got, want)
	}

	got = nil
	OnRunStartFirst(start("stop", false))
	if runStartHooks(RunStart{}) {
		t.Error("a false did not end the start")
	}
	if want := []string{"update rollback", "first 2", "stop"}; !slices.Equal(got, want) {
		t.Errorf("start with a false ran %q, want %q", got, want)
	}
}
