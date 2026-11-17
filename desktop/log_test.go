package desktop

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetLogging gives the test a fresh logger in its own folders: the app
// ID is id, the standard error is the returned buffer (present or not),
// and everything is undone when the test ends.
func resetLogging(t *testing.T, id string, stderr bool) *lockedBuffer {
	t.Helper()
	t.Setenv(DirEnv, t.TempDir())
	SetAppIDForTest(t, id)
	buf := &lockedBuffer{}
	reset := func() {
		if logging.file != nil {
			logging.file.mu.Lock()
			if logging.file.f != nil {
				logging.file.f.Close()
			}
			logging.file.mu.Unlock()
		}
		logging.once = sync.Once{}
		logging.file = nil
		logging.level.Set(slog.LevelInfo)
	}
	reset()
	stderrOut, hasStderr = buf, func() bool { return stderr }
	t.Cleanup(func() {
		reset()
		stderrOut, hasStderr = os.Stderr, stderrUsable
	})
	return buf
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// readLines returns the JSON lines of the log file.
func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("%q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

// The logger writes JSON lines to LogDir()/<app ID>.log, opens the file on
// the first record, and writes text to the standard error when there is
// one.
func TestLoggerWritesJSONLinesAndText(t *testing.T) {
	stderr := resetLogging(t, "com.example.logger", true)
	dir, _ := LogDir()
	path := filepath.Join(dir, "com.example.logger.log")
	l := Logger()
	if l != Logger() {
		t.Error("Logger returns a new logger on each call")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the file exists before the first record")
	}
	l.Info("note saved", "id", 7)
	lines := readLines(t, path)
	if len(lines) != 1 || lines[0]["msg"] != "note saved" || lines[0]["level"] != "INFO" || lines[0]["id"] != float64(7) {
		t.Errorf("file = %v", lines)
	}
	if !strings.Contains(stderr.String(), "msg=\"note saved\"") || !strings.Contains(stderr.String(), "id=7") {
		t.Errorf("standard error = %q", stderr.String())
	}
}

// Without a standard error (a Windows GUI program), only the file gets
// the records.
func TestLoggerWithoutStderr(t *testing.T) {
	stderr := resetLogging(t, "com.example.nostderr", false)
	Logger().Warn("only the file")
	dir, _ := LogDir()
	if got := readLines(t, filepath.Join(dir, "com.example.nostderr.log")); len(got) != 1 {
		t.Errorf("file = %v", got)
	}
	if stderr.String() != "" {
		t.Errorf("standard error = %q", stderr.String())
	}
}

func TestSetLogLevel(t *testing.T) {
	resetLogging(t, "com.example.level", false)
	dir, _ := LogDir()
	path := filepath.Join(dir, "com.example.level.log")
	l := Logger()
	l.Debug("hidden")
	if _, err := os.Stat(path); err == nil {
		t.Error("a record below the level opened the file")
	}
	l.Info("shown")
	SetLogLevel(slog.LevelDebug)
	l.Debug("debug now")
	SetLogLevel(slog.LevelError)
	l.Warn("hidden again")
	l.Error("error")
	var msgs []string
	for _, m := range readLines(t, path) {
		msgs = append(msgs, m["msg"].(string))
	}
	if got := strings.Join(msgs, ","); got != "shown,debug now,error" {
		t.Errorf("messages = %s", got)
	}
}

// A folder that cannot be made leaves the standard error alone, once, and
// never panics or fails the app.
func TestLoggerFallsBackWhenTheFileCannotOpen(t *testing.T) {
	stderr := resetLogging(t, "com.example.nofile", true)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DirEnv, filepath.Join(blocker, "sub")) // under a file: no folder can be made
	l := Logger()
	l.Info("first")
	l.Info("second")
	out := stderr.String()
	if !strings.Contains(out, "msg=first") || !strings.Contains(out, "msg=second") {
		t.Errorf("the records did not reach the standard error: %q", out)
	}
	if n := strings.Count(out, "the log file cannot be opened"); n != 1 {
		t.Errorf("the failure was reported %d times, want once: %q", n, out)
	}
}

// Many goroutines log while the file rotates: nothing is lost or torn
// (run with -race).
func TestLoggerConcurrent(t *testing.T) {
	resetLogging(t, "com.example.race", false)
	dir, _ := LogDir()
	path := filepath.Join(dir, "com.example.race.log")
	Logger() // creates the handlers
	logging.file.max, logging.file.keep = 4<<10, 100
	const goroutines, each = 16, 100
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				Logger().Info("record", "g", g, "i", i, "pad", strings.Repeat("x", 40))
				if i%10 == 0 {
					SetLogLevel(slog.LevelInfo)
				}
			}
		}()
	}
	wg.Wait()
	files, _ := filepath.Glob(path + "*")
	total := 0
	for _, f := range files {
		total += len(readLines(t, f))
	}
	if total != goroutines*each {
		t.Errorf("%d records on disk in %d files, want %d", total, len(files), goroutines*each)
	}
	if len(files) < 2 {
		t.Errorf("the file did not rotate: %v", files)
	}
}

// Rotation by size: the files shift up, the oldest goes, a record is never
// split.
func TestRotatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	r := &rotatingFile{open: func() (string, error) { return path, nil }, max: 100, keep: 2}
	for i := range 12 {
		line := fmt.Sprintf("%02d %s\n", i, strings.Repeat("x", 27)) // 31 bytes: three fit
		if n, err := r.Write([]byte(line)); n != len(line) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			return "<" + err.Error() + ">"
		}
		var ids []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
			ids = append(ids, line[:2])
		}
		return strings.Join(ids, ",")
	}
	if got := read(path); got != "09,10,11" {
		t.Errorf("app.log = %s", got)
	}
	if got := read(path + ".1"); got != "06,07,08" {
		t.Errorf("app.log.1 = %s", got)
	}
	if got := read(path + ".2"); got != "03,04,05" {
		t.Errorf("app.log.2 = %s", got)
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error("app.log.3 exists, keep is 2")
	}
	// A new process appends to the file where it was, and rotates it when
	// the record does not fit.
	r.f.Close()
	r2 := &rotatingFile{open: func() (string, error) { return path, nil }, max: 100, keep: 2}
	r2.Write([]byte("12 " + strings.Repeat("x", 27) + "\n"))
	if got := read(path); got != "12" {
		t.Errorf("app.log = %s (it had 93 bytes, so the record rotates it)", got)
	}
	r2.f.Close()
}

// The messages of Tela go to the file alone, at the level they come with,
// marked as Tela's.
func TestLogTela(t *testing.T) {
	stderr := resetLogging(t, "com.example.tela", true)
	dir, _ := LogDir()
	path := filepath.Join(dir, "com.example.tela.log")
	logTela(slog.LevelWarn, "tela: something odd")
	SetLogLevel(slog.LevelError)
	logTela(slog.LevelWarn, "tela: below the level")
	logTela(slog.LevelError, "tela: broken")
	lines := readLines(t, path)
	if len(lines) != 2 || lines[0]["msg"] != "tela: something odd" || lines[0]["level"] != "WARN" || lines[0]["component"] != "tela" || lines[1]["level"] != "ERROR" {
		t.Errorf("file = %v", lines)
	}
	if stderr.String() != "" {
		t.Errorf("Tela's message was printed twice: %q", stderr.String())
	}
}
