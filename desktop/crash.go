package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/appver"
	"github.com/kuronosu/tela/internal/core"
)

// A Crash is the report of a run of the app that ended in a crash, given
// to the function of [OnCrash] on the next start.
type Crash struct {
	// Time is when the crash happened: the time of a panic, the time the
	// runtime last wrote the file of a fatal error.
	Time time.Time
	// Version is the app's version of the run that crashed
	// (tela build -version, or the module's version), "" when none was
	// stamped.
	Version string
	// Report is the text of the report: a header (time, version, OS and
	// architecture, Go version, app ID, kind of crash), then the panic
	// value and the stack of every goroutine involved.
	Report string
	// Path is the file the report was read from, LogDir()/crash-<time>.txt.
	// When the function returns, the file is renamed to Path+".handled":
	// it is delivered once, and the newest ten handled reports are kept.
	Path string
}

// OnCrash makes f receive the crash reports of the earlier runs of the
// app, so it can offer them to the user or send them where it wants: Tela
// sends nothing anywhere. Call it at the start of main, before tela.Run.
//
// A report exists when a run ended because of
//
//   - a panic on the UI thread (a handler, a build, an effect, a function
//     given to tela.Post) or in a goroutine started by tela.UseGo,
//   - a fatal error of the Go runtime (concurrent map writes, out of
//     memory, a stack overflow) or a panic of any other goroutine, which
//     the runtime reports with [runtime/debug.SetCrashOutput].
//
// The reports are written in [LogDir] as crash-<UTC time>.txt, and only
// by a release build (dev mode off: go build stamps it, or TELA_DEV=0)
// or by a program without a usable standard error (a Windows GUI program,
// ADR-127). The program still crashes as before: Tela records a panic and
// panics again with the same value. A report of the UI thread or of
// UseGo has the stack at the panic; a fatal error has the traces the
// runtime writes (its stack and every goroutine's), but not the line that
// names the error ("fatal error: concurrent map writes"), which only the
// standard error gets: the runtime does not copy it. Tela takes over
// [runtime/debug.SetCrashOutput] for them.
//
// On the next start, once the main window made its first commit, f runs
// on the UI thread once for each report not handled yet, oldest first;
// when f returns (or panics) the report is marked handled (see
// [Crash.Path]), never earlier. Nothing is delivered, or deleted, while
// no function is set. A second call replaces f; nil removes it. Called
// after the first commit, it delivers at once, on the UI thread.
//
// Reports are not written or delivered under telatest, which does not
// call tela.Run.
func OnCrash(f func(Crash)) {
	crashes.mu.Lock()
	crashes.cb = f
	ready := crashes.ready
	crashes.mu.Unlock()
	if f != nil && ready {
		tela.Post(deliverCrashes)
	}
}

// crashes is the process's crash reporting.
var crashes struct {
	hooks sync.Once

	mu sync.Mutex
	cb func(Crash)
	// ready: the main window made its first commit.
	ready bool
	// dir is LogDir of this run, "" while no report is written; app is the
	// ID that names it; fatal is the file the runtime writes to.
	dir, app, fatal string
}

// runStartLogs runs when tela.Run starts, after the single instance was
// claimed (ADR-297; instance.go registers it after runStart): it sends
// Tela's own messages to the log file, and sets up the crash reports. A
// second instance ends at the claim, so it never makes the runtime's
// file, which only the end of Run (endCapture) removes.
func runStartLogs(rs core.RunStart) bool {
	core.SetLogTee(logTela)
	crashes.hooks.Do(func() {
		core.OnFirstCommit(onFirstCommit)
		core.OnPanic(panicReport)
		core.OnRunEnd(endCapture)
	})
	if rs.Dev && hasStderr() {
		return true // dev mode with a terminal: the panic is on the screen
	}
	running := func() (string, bool) { return rs.AppID, rs.AppID != "" }
	dir, err := appDirFrom(dirLogs, running)
	if err != nil {
		return true
	}
	id, _ := resolveAppIDFrom(running)
	startCapture(dir, id)
	return true
}

// startCapture makes the runtime write its fatal errors to a file of dir.
// The file begins with the header of a report, so that the next start
// tells it from an empty one.
func startCapture(dir, app string) {
	now := time.Now()
	path := filepath.Join(dir, fmt.Sprintf("fatal-%d-%s.tmp", os.Getpid(), stamp(now)))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	crashes.mu.Lock()
	crashes.app = app
	crashes.mu.Unlock()
	_, werr := f.WriteString(reportHeader("started", now, "fatal error of the runtime or panic of a goroutine Tela does not run") + "\n")
	// SetCrashOutput duplicates the file descriptor: f closes here.
	if werr != nil || debug.SetCrashOutput(f, debug.CrashOptions{}) != nil {
		f.Close()
		_ = os.Remove(path)
		return
	}
	f.Close()
	crashes.mu.Lock()
	crashes.dir, crashes.fatal = dir, path
	crashes.mu.Unlock()
}

// endCapture stops the runtime's crash output and removes its file: the
// run ended without a crash, or a report of its own was written.
func endCapture() {
	crashes.mu.Lock()
	defer crashes.mu.Unlock()
	if crashes.fatal == "" {
		return
	}
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_ = os.Remove(crashes.fatal)
	crashes.fatal = ""
}

// panicReport writes the report of a panic of the UI thread or of a UseGo
// goroutine (core.OnPanic), then stops the runtime's own output, which
// would write the same panic again when Tela panics again.
func panicReport(where string, v any, stack []byte) {
	crashes.mu.Lock()
	dir := crashes.dir
	crashes.mu.Unlock()
	if dir == "" {
		return
	}
	now := time.Now()
	text := reportHeader("time", now, "panic on "+where) + "\n" +
		"panic: " + panicText(v) + "\n\n" + string(stack)
	_ = writeNew(dir, "crash-"+stamp(now), ".txt", []byte(text))
	endCapture()
}

// panicText returns the value of a panic as Go prints it.
func panicText(v any) string {
	switch x := v.(type) {
	case error:
		return x.Error()
	case fmt.Stringer:
		return x.String()
	case string:
		return x
	}
	return fmt.Sprintf("%v", v)
}

// reportHeader returns the header of a report: lines of "key: value" up to
// a blank line, which the caller adds.
func reportHeader(timeKey string, t time.Time, kind string) string {
	version := appver.Version()
	if version == "" {
		version = "(unstamped)"
	}
	crashes.mu.Lock()
	app := crashes.app
	crashes.mu.Unlock()
	return fmt.Sprintf("Tela crash report\n%s: %s\nversion: %s\nos: %s/%s\ngo: %s\napp: %s\nkind: %s\n",
		timeKey, t.UTC().Format(time.RFC3339Nano), version, runtime.GOOS, runtime.GOARCH, runtime.Version(), app, kind)
}

// stamp formats t for a file name: it sorts as t does.
func stamp(t time.Time) string { return t.UTC().Format("20060102T150405.000Z") }

// writeNew creates dir/base+ext (or base-N+ext when it exists) with data.
func writeNew(dir, base, ext string, data []byte) string {
	for i := 0; i < 100; i++ {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return ""
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(path)
			return ""
		}
		return path
	}
	return ""
}

// onFirstCommit runs on the UI thread after the main window's first
// commit.
func onFirstCommit() {
	crashes.mu.Lock()
	crashes.ready = true
	crashes.mu.Unlock()
	deliverCrashes()
}

// handledKept is how many handled reports stay in LogDir.
const handledKept = 10

// staleFatal is how old an empty file of the runtime's output must be to
// be removed: a process that ended without cleaning up (killed), not
// another instance that still runs.
const staleFatal = 7 * 24 * time.Hour

// deliverCrashes gives the reports not handled yet to the function of
// OnCrash, oldest first, and marks each handled. UI thread only.
func deliverCrashes() {
	crashes.mu.Lock()
	f, own := crashes.cb, crashes.fatal
	crashes.mu.Unlock()
	if f == nil {
		return
	}
	dir, err := LogDir()
	if err != nil {
		return
	}
	collectFatal(dir, own)
	paths, _ := filepath.Glob(filepath.Join(dir, "crash-*.txt"))
	slices.Sort(paths)
	for _, p := range paths {
		c, ok := readCrash(p)
		if !ok {
			continue
		}
		func() {
			defer func() { _ = os.Rename(p, p+".handled") }()
			f(c)
		}()
	}
	pruneHandled(dir)
}

// collectFatal turns the files of the runtime's output that earlier runs
// left (not own, this run's) into reports: the ones with something after
// their header become crash-<time>.txt, the empty ones of runs that ended
// well are removed.
func collectFatal(dir, own string) {
	paths, _ := filepath.Glob(filepath.Join(dir, "fatal-*.tmp"))
	for _, p := range paths {
		if p == own {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if _, body, _ := strings.Cut(string(data), "\n\n"); strings.TrimSpace(body) == "" {
			if time.Since(st.ModTime()) > staleFatal {
				_ = os.Remove(p)
			}
			continue
		}
		if q := writeNew(dir, "crash-"+stamp(st.ModTime()), ".txt", data); q != "" {
			_ = os.Chtimes(q, st.ModTime(), st.ModTime()) // the time of the crash
			_ = os.Remove(p)
		}
	}
}

// readCrash reads the report at path.
func readCrash(path string) (Crash, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Crash{}, false
	}
	c := Crash{Report: string(data), Path: path}
	head, _, _ := strings.Cut(c.Report, "\n\n")
	for _, line := range strings.Split(head, "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch k {
		case "time":
			c.Time, _ = time.Parse(time.RFC3339Nano, v)
		case "version":
			if v != "(unstamped)" {
				c.Version = v
			}
		}
	}
	if c.Time.IsZero() { // a fatal error: the runtime wrote the file last then
		if st, err := os.Stat(path); err == nil {
			c.Time = st.ModTime().UTC()
		}
	}
	return c, true
}

// pruneHandled removes all but the newest handledKept handled reports.
func pruneHandled(dir string) {
	paths, _ := filepath.Glob(filepath.Join(dir, "crash-*.txt.handled"))
	slices.Sort(paths)
	for len(paths) > handledKept {
		_ = os.Remove(paths[0])
		paths = paths[1:]
	}
}
