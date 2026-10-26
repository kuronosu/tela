package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/watch"
)

// The instance of tela mcp: what the package is, how it is built (go test
// -c with the overlay of mcp_runner.go) and the child process that serves
// it. Everything here runs on the server's tool-call goroutine.

// mcpCallTimeout bounds one command to the instance; an instance that does
// not answer (an infinite loop in the app) is killed and started again.
const mcpCallTimeout = 60 * time.Second

// mcpTarget is the package the server serves.
type mcpTarget struct {
	pkgArg  string    // the package as the user named it
	pkg     goPackage // from go list, with the test files
	modDir  string
	arg     string // the package argument of go test -c, relative to modDir
	goflags string
	set     *watchSet
}

// resolveMCPTarget finds the package and the files that decide when it
// rebuilds.
func resolveMCPTarget(goBin, pkgArg, tmp string) (*mcpTarget, error) {
	here := readGoEnv(goBin, "", goEnv())
	pkgs, err := goList(goBin, "", here.flags, pkgArg)
	if err != nil {
		return nil, fmt.Errorf("cannot find package %q: %s", pkgArg, err)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("%q does not name exactly one package (go list printed %d); give a single package, without wildcards", pkgArg, len(pkgs))
	}
	p := pkgs[0]
	if p.Dir == "" || p.Name == "" {
		msg := "no Go files"
		if p.Error != nil {
			msg = p.Error.Err
		}
		return nil, fmt.Errorf("cannot find package %q: %s", pkgArg, msg)
	}
	if p.Module == nil || p.Module.Dir == "" {
		return nil, fmt.Errorf("package %s (%s) is not in a module; tela mcp serves packages of a module that requires github.com/kuronosu/tela (tela new creates one)", p.Name, pkgArg)
	}
	t := &mcpTarget{pkgArg: pkgArg, modDir: p.Module.Dir, goflags: here.flags, set: newWatchSet(tmp)}
	if !here.sameIn(t.modDir) {
		t.goflags = readGoEnv(goBin, t.modDir, goEnv()).flags
	}
	rel, err := filepath.Rel(t.modDir, p.Dir)
	if err != nil {
		return nil, err
	}
	t.arg = "./" + filepath.ToSlash(rel)
	t.pkg = goPackage{Dir: p.Dir, Name: p.Name}
	t.refreshDeps(goBin)
	return t, nil
}

// refreshDeps updates what the fingerprint covers: the modules the package
// builds from source and the files they embed, as for tela dev.
func (t *mcpTarget) refreshDeps(goBin string) {
	dt := &devTarget{pkg: listedPackage{Dir: t.pkg.Dir, ImportPath: t.arg}, modDir: t.modDir, goflags: t.goflags}
	deps, err := listDeps(goBin, dt)
	if err != nil {
		deps = depSet{modules: []string{t.modDir}, pkgDirs: []string{t.pkg.Dir}}
	}
	t.set.setDeps(deps)
}

// fingerprint hashes the sources the build depends on: tela dev's watch
// set (non-test Go files, go.mod and go.sum of the modules built from
// source, embedded files) and the package's own _test.go files, which go
// test -c compiles in.
func (t *mcpTarget) fingerprint() uint64 {
	dir := filepath.Clean(t.pkg.Dir)
	filter := func(path string, isDir bool) bool {
		if strings.HasSuffix(path, "_test.go") && filepath.Dir(path) == dir {
			return true
		}
		return t.set.relevant(path, isDir)
	}
	return watch.Fingerprint(watch.Options{Filter: filter, SkipDir: t.set.skipDir}, t.set.roots()...)
}

// mcpBuildFlags are the flags of the instance's builds: those of tela dev's
// builds without its tag (devBuildFlags), and no vet.
func mcpBuildFlags(goflags string) []string {
	var flags []string
	if !hasGoFlag(goflags, "vet") {
		flags = append(flags, "-vet=off")
	}
	if !hasGoFlag(goflags, "buildvcs") {
		flags = append(flags, "-buildvcs=false")
	}
	if !hasGoFlag(goflags, "ldflags") {
		flags = append(flags, "-ldflags=-s -w")
	}
	if !hasGoFlag(goflags, "gcflags") {
		flags = append(flags, "-gcflags=-l")
	}
	return flags
}

// buildInstance compiles the instance of t into dir as build gen. On
// failure it returns the go command's output as the error.
func buildInstance(ctx context.Context, goBin string, t *mcpTarget, c contract, hasApp bool, dir string, gen int) (string, error) {
	src := filepath.Join(dir, fmt.Sprintf("runner-%d.go", gen))
	if err := os.WriteFile(src, generateMCPSource(t.pkg.Name, c, hasApp), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(src)
	overlay := filepath.Join(dir, fmt.Sprintf("overlay-%d.json", gen))
	js, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(t.pkg.Dir, mcpGeneratedFile): src}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(overlay, js, 0o644); err != nil {
		return "", err
	}
	defer os.Remove(overlay)
	exe := filepath.Join(dir, fmt.Sprintf("instance-%d", gen))
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	args := append([]string{"test", "-c", "-o", exe, "-overlay", overlay}, mcpBuildFlags(t.goflags)...)
	args = append(args, t.arg)
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = t.modDir
	cmd.Env = goEnv()
	cmd.Cancel = func() error { return interruptProcess(cmd.Process) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		os.Remove(exe)
		msg := strings.TrimRight(out.String(), "\n")
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(cleanBuildOutput(msg))
	}
	if _, err := os.Stat(exe); err != nil {
		// go test -c writes nothing for a package without test files,
		// which the overlay rules out; say what happened anyway.
		return "", fmt.Errorf("go test -c wrote no binary: %s", strings.TrimSpace(out.String()))
	}
	return exe, nil
}

// cleanBuildOutput drops the noise of go test from a compile error: the
// "FAIL ... [build failed]" line and paths of the generated file, which
// the user never wrote.
func cleanBuildOutput(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "FAIL\t") && strings.HasSuffix(l, "[build failed]") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

// mcpChild is a running instance.
type mcpChild struct {
	exe     string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan mcpReply
	done    chan struct{} // closed when the process has exited
	exitErr error
	tail    *tailWriter
	nextID  int
	logf    func(format string, args ...any)
}

// startChild starts the instance binary exe in the package's directory.
// The child's standard error goes to stderr (nil: kept only in the tail
// for crash reports).
func startChild(exe, dir string, stderr io.Writer, logf func(string, ...any)) (*mcpChild, error) {
	cmd := exec.Command(exe, "-test.run=^TestTelaMCP$", "-test.timeout=0", "-test.count=1")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.SysProcAttr = childSysProcAttr()
	c := &mcpChild{exe: exe, cmd: cmd, replies: make(chan mcpReply, 4), done: make(chan struct{}), tail: &tailWriter{w: stderr}, logf: logf}
	cmd.Stderr = c.tail
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.stdin = stdin
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		r := bufio.NewReaderSize(stdout, 64<<10)
		for {
			line, err := r.ReadBytes('\n')
			if rest, ok := bytes.CutPrefix(line, []byte(mcpReplyPrefix)); ok {
				var rep mcpReply
				if jerr := json.Unmarshal(rest, &rep); jerr != nil {
					logf("unreadable reply of the instance: %v", jerr)
				} else {
					select {
					case c.replies <- rep:
					default:
						// Only the late reply of a call that gave up can find
						// the queue full; the reader never blocks on it.
						logf("dropped a late reply of the instance")
					}
				}
			} else if s := strings.TrimSpace(string(line)); s != "" {
				logf("instance: %s", s)
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		<-readDone
		c.exitErr = cmd.Wait()
		close(c.done)
	}()
	return c, nil
}

// alive reports whether the process is still running.
func (c *mcpChild) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// errInstanceGone is a call to an instance that exited or stopped
// answering.
type errInstanceGone struct{ msg string }

func (e *errInstanceGone) Error() string { return e.msg }

// call sends req and waits for its reply.
func (c *mcpChild) call(ctx context.Context, req mcpReq) (mcpReply, error) {
	c.nextID++
	req.ID = c.nextID
	b, err := json.Marshal(req)
	if err != nil {
		return mcpReply{}, err
	}
	start := time.Now()
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return mcpReply{}, c.gone()
	}
	timer := time.NewTimer(mcpCallTimeout)
	defer timer.Stop()
	for {
		select {
		case rep := <-c.replies:
			if rep.ID != req.ID {
				continue // the late reply of a call that timed out
			}
			c.logf("instance %s in %v", req.Op, time.Since(start).Round(time.Millisecond))
			return rep, nil
		case <-c.done:
			return mcpReply{}, c.gone()
		case <-timer.C:
			c.kill()
			return mcpReply{}, &errInstanceGone{fmt.Sprintf("the instance did not answer %s within %v (an endless loop in a handler, a build or an effect?); it was stopped and starts again on the next call, with fresh state", req.Op, mcpCallTimeout)}
		case <-ctx.Done():
			c.kill()
			return mcpReply{}, ctx.Err()
		}
	}
}

// gone describes an instance that exited, with the end of its standard
// error (a panic in a goroutine of the app, a fatal error).
func (c *mcpChild) gone() error {
	return &errInstanceGone{"the instance exited (" + c.exitReport() + ")\nIt starts again on the next call, with fresh state."}
}

// exitReport says how the exited instance ended: its exit status and the
// end of its standard error, where the Go runtime writes a panic of
// another goroutine or a fatal error.
func (c *mcpChild) exitReport() string {
	<-c.done
	c.tail.mu.Lock()
	tail := strings.TrimSpace(string(c.tail.buf))
	c.tail.mu.Unlock()
	if i := strings.LastIndex(tail, "\npanic: "); i >= 0 {
		tail = tail[i+1:]
	} else if i := strings.LastIndex(tail, "\nfatal error: "); i >= 0 {
		tail = tail[i+1:]
	}
	if len(tail) > 3000 {
		tail = tail[:3000] + "\n..."
	}
	msg := "it ended"
	if c.exitErr != nil {
		msg = c.exitErr.Error()
	}
	if tail != "" {
		msg += ":\n" + tail
	}
	return msg
}

// stop ends the instance: its standard input closes, which ends its
// command loop, and it is killed if it does not exit soon.
func (c *mcpChild) stop() {
	c.stdin.Close()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		c.kill()
	}
}

func (c *mcpChild) kill() {
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	<-c.done
}
