package desktop

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// The log file rotates when it would pass logMaxSize bytes: <name>.log
// becomes <name>.log.1, that one <name>.log.2, and so on; the file that
// would be number logKeep+1 is removed. At most (logKeep+1)*logMaxSize
// bytes of log stay on disk.
const (
	logMaxSize = 5 << 20
	logKeep    = 3
)

// What the logger writes its text to, and whether the process has it:
// replaced by the tests.
var (
	stderrOut io.Writer = os.Stderr
	hasStderr           = stderrUsable
)

// logging is the process's logger (ADR-297).
var logging struct {
	once  sync.Once
	level slog.LevelVar
	file  *rotatingFile
	// fileH writes the JSON lines; Tela's own messages go through it alone
	// (they already reached the standard error).
	fileH  slog.Handler
	logger *slog.Logger
}

// Logger returns the app's logger, a [log/slog] logger that writes:
//
//   - JSON lines to LogDir()/<app ID>.log ([LogDir], [AppID]): the file
//     opens when the first record is written, and a failure to open it
//     (no folder, no space) leaves the standard error alone, without
//     panicking or returning an error. It rotates by size: at 5 MB the
//     file becomes <app ID>.log.1, the older ones shift up, and three old
//     files are kept. [FlushAll] (and so the end of tela.Run) closes it,
//     and the next record opens it again. On Windows it is opened shared
//     for deletion, so other programs can rename and delete it as on
//     Unix.
//   - text to the standard error, when the process has one: a program
//     started from a terminal, not a Windows GUI program (built with
//     -ldflags=-H=windowsgui, ADR-127) or one whose standard error is
//     closed.
//
// Records below the level of [SetLogLevel] (Info at first) are dropped.
// Tela's own messages (dev warnings, platform errors) are written to the
// file too, at the Warn level (the Error level for the errors of the
// app's content) with the attribute component=tela, once tela.Run
// started; they keep their place on the standard error as they were. The
// logger is safe from any goroutine, and is the same value on every call.
//
//	desktop.Logger().Info("note saved", "id", note.ID, "bytes", len(body))
//
// Name the app with [SetAppID] (or tela.App.AppID) before the first
// record: the file is named after the app ID the first record finds.
func Logger() *slog.Logger {
	logging.once.Do(initLogging)
	return logging.logger
}

// SetLogLevel sets the least severe level [Logger] writes (slog.LevelInfo
// at first; slog.LevelDebug for everything). It also decides which of
// Tela's own messages reach the file (they are warnings and errors). Safe
// from any goroutine.
func SetLogLevel(l slog.Level) { logging.level.Set(l) }

func initLogging() {
	logging.file = &rotatingFile{
		open:   func() (string, error) { return logFilePath() },
		max:    logMaxSize,
		keep:   logKeep,
		report: stderrOut,
	}
	opts := &slog.HandlerOptions{Level: &logging.level}
	logging.fileH = slog.NewJSONHandler(logging.file, opts)
	hs := []slog.Handler{logging.fileH}
	if hasStderr() {
		hs = append(hs, slog.NewTextHandler(stderrOut, opts))
	}
	logging.logger = slog.New(slog.NewMultiHandler(hs...))
}

// logFilePath returns the path of the log file: LogDir()/<app ID>.log.
func logFilePath() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppID()+".log"), nil
}

// logTela writes a message of Tela's own to the log file, not to the
// standard error, where it was printed already.
func logTela(level slog.Level, msg string) {
	logging.once.Do(initLogging)
	if level < logging.level.Level() {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(slog.String("component", "tela"))
	_ = logging.fileH.Handle(context.Background(), r)
}

// rotatingFile is the log file: an io.Writer that opens its file on the
// first write and rotates it by size. A file that cannot be opened is
// reported once on report and every write is dropped, as are the writes
// that fail later: logging never fails the app.
type rotatingFile struct {
	// open returns the path of the file, when it is needed.
	open func() (string, error)
	max  int64
	keep int
	// report receives the failure to open the file; nil: nothing.
	report io.Writer

	mu     sync.Mutex
	path   string
	f      *os.File
	size   int64
	failed bool
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failed {
		return len(p), nil
	}
	if r.f == nil {
		if err := r.start(); err != nil {
			r.failed = true
			if r.report != nil {
				fmt.Fprintf(r.report, "desktop: the log file cannot be opened (%v); logging to the standard error only\n", err)
			}
			return len(p), nil
		}
	}
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		r.rotate()
	}
	if r.f == nil {
		return len(p), nil
	}
	n, _ := r.f.Write(p)
	r.size += int64(n)
	return len(p), nil
}

// start opens the file for appending.
func (r *rotatingFile) start() error {
	path, err := r.open()
	if err != nil {
		return err
	}
	r.path = path
	return r.reopen()
}

func (r *rotatingFile) reopen() error {
	f, err := openLogFile(r.path)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

// close closes the file (FlushAll): the next record opens it again, at
// the path LogDir and AppID give then, and a file that could not be
// opened is tried again, since the folder may have changed (a test's
// temporary folders, telatest.TempDesktopDirs).
func (r *rotatingFile) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
	r.failed = false
}

// closeLog closes the process's log file, when it is open (FlushAll).
func closeLog() {
	logging.once.Do(initLogging)
	logging.file.close()
}

// rotate shifts the files up and starts a new one. This process closes
// its file first, as Windows renames no file a process holds without
// FILE_SHARE_DELETE (openLogFile shares it, so another process of the
// app holding it does not stop the rotation either). When the renames
// fail anyway, the log goes on in the file that is open, or stops when it
// cannot be reopened.
func (r *rotatingFile) rotate() {
	r.f.Close()
	r.f = nil
	_ = os.Remove(r.path + "." + strconv.Itoa(r.keep))
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(r.path+"."+strconv.Itoa(i), r.path+"."+strconv.Itoa(i+1))
	}
	if r.keep > 0 {
		_ = os.Rename(r.path, r.path+".1")
	} else {
		_ = os.Remove(r.path)
	}
	if err := r.reopen(); err != nil {
		r.failed = true
	}
}
