package main

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"github.com/kuronosu/tela/cmd/tela/internal/appbundle"
)

const buildUsage = `Usage:

	tela build [-o path] [-id ID] [-name NAME] [-tags list] [-ldflags flags]
	           [-gcflags flags] [-trimpath] [-v] [pkg]

Builds the program of a package main (default ".") with CGO_ENABLED=0 for
the target GOOS and GOARCH of the environment, so GOOS=darwin
GOARCH=arm64 tela build works from Linux or Windows.

When the target is darwin, the program is wrapped in a macOS application
bundle, <Name>.app:

	<Name>.app/Contents/Info.plist
	<Name>.app/Contents/MacOS/<Name>

The Info.plist holds the app's ID (CFBundleIdentifier), name, version and
the system version Tela needs; macOS services that belong to an app (user
notifications, URL schemes and file types, the keychain) need it. The ID and
the name are read from the package's tela.App, without running it: AppID
and Title of the literal passed to tela.Main in func main, or of the
package-level var App. A literal, a constant or a concatenation of them
works, and so does an App that a function wraps, as in
tela.Main(desktop.SingleInstance(App)). The flags -id and -name replace
them; without an ID the command stops and says so. The name defaults to the
package's name. The version is the module's, when the build knows it, else
0.0.0.

On any other target the output is the plain executable, like go build, and
-id and -name are ignored.

Rebuilding an existing bundle writes only what changed, so Launch Services
keeps seeing the same app, and it keeps the keys a running app added to the
Info.plist (registered URL schemes and file types).

Signing: tela build does not run codesign. The Go linker ad-hoc signs
darwin/arm64 executables, and that signature covers the executable only,
not the Info.plist or the resources. The bundle is for the machine that
built it: it has no developer identity and is not notarized. Distribution
(icons, signing, notarization, installers) is the packaging step.

Flags:

	-o path        where to write. For a bundle: a folder to create
	               <Name>.app in, or a path ending in .app (default: the
	               current folder). Otherwise the executable file, or a folder
	               to write it in (default: named after the package, in the
	               current folder)
	-id ID         the app's ID (CFBundleIdentifier): letters, digits,
	               hyphens and periods, such as com.example.notes
	-name NAME     the app's name (CFBundleName and the bundle's file name)
	-tags list     build tags, comma-separated, as go build -tags
	-ldflags f     linker flags, as go build -ldflags
	-gcflags f     compiler flags, as go build -gcflags
	-trimpath      remove file system paths from the program, as go build
	-v             say where the ID and the name came from

The build uses go build's defaults otherwise: no -tags teladev, no stripping.
GOFLAGS applies as for any go command.
`

const runUsage = `Usage:

	tela run [-id ID] [-name NAME] [-tags list] [-ldflags flags]
	         [-gcflags flags] [-trimpath] [-v] [pkg [args...]]

Builds the program of a package main (default ".") and runs it, like go
run, with CGO_ENABLED=0. Everything after pkg goes to the program as its
arguments. The program keeps the terminal: standard input, output and
error, and Ctrl+C. Its exit status is tela run's.

On macOS the program runs as an application bundle. tela run builds the
same <Name>.app as tela build, in the user's cache folder, under a folder
of the app's ID that stays the same from run to run, so Launch Services sees
one bundle per app:

	<user cache>/tela/run/<AppID>/<Name>.app

then runs the executable inside it directly, not through open. The bundle
is what makes NSBundle.mainBundle the app: notifications, URL schemes, the
keychain and the Dock name need it. The ID and the name come from the
package's tela.App or from -id and -name (see "tela help build"). A
rebuild writes only what changed.

On the other systems tela run builds the program in a temporary folder and
runs it; -id and -name are ignored. The target is always the machine's own:
tela run does not run a cross-built program.

Flags:

	-id ID         the app's ID (macOS), such as com.example.notes
	-name NAME     the app's name (macOS)
	-tags list     build tags, comma-separated, as go build -tags
	-ldflags f     linker flags, as go build -ldflags
	-gcflags f     compiler flags, as go build -gcflags
	-trimpath      remove file system paths from the program, as go build
	-v             say where the ID and the name came from, and where the
	               bundle is

Flags go before pkg: after it, everything belongs to the program.
`

// buildOptions are the arguments of tela build and tela run.
type buildOptions struct {
	Pkg      string
	Out      string // build only
	ID, Name string
	Tags     string
	Ldflags  string
	Gcflags  string
	Trimpath bool
	Verbose  bool
	// Args are the program's arguments (run only).
	Args []string
}

// register adds the flags both commands take.
func (o *buildOptions) register(fs *flag.FlagSet) {
	fs.StringVar(&o.ID, "id", "", "")
	fs.StringVar(&o.Name, "name", "", "")
	fs.StringVar(&o.Tags, "tags", "", "")
	fs.StringVar(&o.Ldflags, "ldflags", "", "")
	fs.StringVar(&o.Gcflags, "gcflags", "", "")
	fs.BoolVar(&o.Trimpath, "trimpath", false, "")
	fs.BoolVar(&o.Verbose, "v", false, "")
	fs.BoolVar(&o.Verbose, "verbose", false, "")
}

// parseBuildArgs parses the arguments of tela build. Flags and the package
// may come in any order; the package defaults to ".".
func parseBuildArgs(args []string) (buildOptions, error) {
	var o buildOptions
	fs := flag.NewFlagSet("tela build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o.register(fs)
	fs.StringVar(&o.Out, "o", "", "")
	pkgs, err := parsePositionals("build", fs, args)
	if err != nil {
		return o, err
	}
	switch len(pkgs) {
	case 0:
		o.Pkg = "."
	case 1:
		o.Pkg = pkgs[0]
	default:
		return o, fmt.Errorf("expected at most one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
	}
	return o, o.validate("build")
}

// parseRunArgs parses the arguments of tela run: flags, then the package,
// then the program's arguments, which are not parsed.
func parseRunArgs(args []string) (buildOptions, error) {
	var o buildOptions
	fs := flag.NewFlagSet("tela run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o.register(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, err
		}
		return o, fmt.Errorf("%v (see \"tela help run\")", err)
	}
	o.Pkg = "."
	if rest := fs.Args(); len(rest) > 0 {
		o.Pkg = rest[0]
		if len(rest) > 1 {
			o.Args = rest[1:]
		}
	}
	return o, o.validate("run")
}

func (o *buildOptions) validate(command string) error {
	if o.Pkg == "" {
		return fmt.Errorf("empty <pkg>: give a package path or a directory such as ./cmd/app, or leave it out for the current one")
	}
	if strings.HasPrefix(o.Pkg, "-") {
		return fmt.Errorf("%q looks like a flag, and flags go before the package (see \"tela help %s\")", o.Pkg, command)
	}
	return nil
}

// runBuild implements tela build.
func runBuild(args []string, stdout, stderr io.Writer) int {
	opts, err := parseBuildArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, buildUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela build: %v\n\n%s", err, buildUsage)
		return 2
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "tela build: the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
		return 1
	}
	out, err := buildProgram(goBin, opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "tela build: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", out)
	return 0
}

// runRun implements tela run.
func runRun(args []string, stdout, stderr io.Writer) int {
	opts, err := parseRunArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, runUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela run: %v\n\n%s", err, runUsage)
		return 2
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "tela run: the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
		return 1
	}
	code, err := runProgram(goBin, opts, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "tela run: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// buildTarget is the package to build and the platform to build it for.
type buildTarget struct {
	goos, goarch string
	pkg          listedPackage
	// deps are the packages go list printed besides pkg, by import path
	// (only when the app's source is read).
	deps map[string]listedPackage
}

// buildListFields are the fields of go list for a build.
const buildListFields = listFields + ",DepOnly"

// resolveBuildTarget finds the package, checks it is a program and reads
// the target platform. For a darwin target whose ID or name is not given
// by flags it lists the dependencies too, for reading the App.
func resolveBuildTarget(goBin string, opts buildOptions, command string) (*buildTarget, error) {
	t := &buildTarget{}
	cmd := exec.Command(goBin, "env", "GOOS", "GOARCH")
	cmd.Env = goEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %v", err)
	}
	if f := strings.Fields(string(out)); len(f) == 2 {
		t.goos, t.goarch = f[0], f[1]
	} else {
		return nil, fmt.Errorf("go env GOOS GOARCH printed %q", strings.TrimSpace(string(out)))
	}

	args := []string{"list", "-e", "-json=" + buildListFields}
	if t.goos == "darwin" && (opts.ID == "" || opts.Name == "") {
		args = append(args, "-deps")
	}
	if opts.Tags != "" {
		args = append(args, "-tags", opts.Tags)
	}
	args = append(args, "--", opts.Pkg)
	cmd = exec.Command(goBin, args...)
	cmd.Env = goEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./cmd/app", opts.Pkg, msg)
	}
	var pkgs []listedPackage
	for dec := json.NewDecoder(&stdout); ; {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("unreadable go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	t.deps = map[string]listedPackage{}
	var targets []listedPackage
	for _, p := range pkgs {
		if p.DepOnly {
			t.deps[p.ImportPath] = p
		} else {
			targets = append(targets, p)
		}
	}
	if len(targets) != 1 {
		return nil, fmt.Errorf("%q does not name exactly one package (go list printed %d); give a single package, without wildcards", opts.Pkg, len(targets))
	}
	t.pkg = targets[0]
	if t.pkg.Error != nil && t.pkg.Name == "" {
		return nil, fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./cmd/app", opts.Pkg, strings.TrimSpace(t.pkg.Error.Err))
	}
	if t.pkg.Name != "main" {
		return nil, fmt.Errorf("package %s (%s) is not a package main, so there is no program to %s; put func main (calling tela.Main) in a package main, or use \"tela dev %s\" to run its screen", t.pkg.Name, opts.Pkg, command, opts.Pkg)
	}
	return t, nil
}

// identity is the ID and name of a bundle.
type identity struct {
	ID, Name string
}

// resolveIdentity decides the ID and the name of the bundle, from the flags
// and the package's App.
func resolveIdentity(opts buildOptions, t *buildTarget, command string, log io.Writer) (identity, error) {
	var src appSource
	if opts.ID == "" || opts.Name == "" {
		var err error
		if src, err = readAppSource(t.pkg, t.deps); err != nil {
			return identity{}, err
		}
	}
	var id identity
	from := "-id"
	id.ID = opts.ID
	if id.ID == "" {
		from = "App.AppID"
		switch {
		case src.IDErr != nil:
			return identity{}, fmt.Errorf("%v\npass the ID with -id instead: tela %s -id com.example.app %s", src.IDErr, command, opts.Pkg)
		case src.ID == "" && !src.Found:
			return identity{}, fmt.Errorf("package %s has no app ID: macOS services (notifications, URL schemes and file types, the keychain) need the app's ID, and tela found no tela.App to read it from (it looks at the argument of tela.Main in func main, and at the package-level var App); set AppID in your tela.App, for example AppID: \"com.example.notes\", or pass -id", t.pkg.ImportPath)
		case src.ID == "":
			return identity{}, fmt.Errorf("package %s has no app ID: macOS services (notifications, URL schemes and file types, the keychain) need the app's ID, so set tela.App.AppID, for example AppID: \"com.example.notes\", or pass -id", t.pkg.ImportPath)
		}
		id.ID = src.ID
	}
	if err := appbundle.ValidID(id.ID); err != nil {
		return identity{}, fmt.Errorf("%v (from %s)", err, from)
	}
	nameFrom := "-name"
	id.Name = opts.Name
	if id.Name == "" {
		nameFrom = "App.Title"
		id.Name = strings.TrimSpace(src.Title)
	}
	if id.Name == "" {
		nameFrom = "the package name"
		id.Name = packageBaseName(t.pkg)
	}
	if opts.Verbose {
		fmt.Fprintf(log, "app ID %q (from %s), name %q (from %s)\n", id.ID, from, id.Name, nameFrom)
	}
	return id, nil
}

var majorSuffix = regexp.MustCompile(`^v[0-9]+$`)

// packageBaseName names a package main for the executable and the bundle
// by its import path, as go build does: the last element, but not a /vN
// major version suffix.
func packageBaseName(p listedPackage) string {
	elems := strings.Split(p.ImportPath, "/")
	name := elems[len(elems)-1]
	if len(elems) > 1 && majorSuffix.MatchString(name) {
		name = elems[len(elems)-2]
	}
	if name == "" || name == "." || strings.ContainsAny(name, " \t") {
		name = filepath.Base(p.Dir)
	}
	return name
}

// goBuild runs go build for opts into the file out.
func goBuild(goBin string, opts buildOptions, out string, stderr io.Writer) error {
	args := []string{"build", "-o", out}
	if opts.Tags != "" {
		args = append(args, "-tags", opts.Tags)
	}
	if opts.Ldflags != "" {
		args = append(args, "-ldflags", opts.Ldflags)
	}
	if opts.Gcflags != "" {
		args = append(args, "-gcflags", opts.Gcflags)
	}
	if opts.Trimpath {
		args = append(args, "-trimpath")
	}
	args = append(args, "--", opts.Pkg)
	cmd := exec.Command(goBin, args...)
	cmd.Env = goEnv()
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %v", err)
	}
	return nil
}

var versionCore = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)`)

// moduleVersion returns the numeric version (1.2.3) of the main module of
// the Go program at path, as the build recorded it (a tag, or the tag a
// pseudo-version starts from), and "" when the build did not know one
// ("(devel)"). It reads the build information from the file, so it works
// for programs of other platforms.
func moduleVersion(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}
	return numericVersion(info.Main.Version)
}

// numericVersion returns the X.Y.Z at the start of a module version, which
// is all CFBundleShortVersionString takes ("v1.2.3" and
// "v1.2.4-0.2026...-abc+dirty" give 1.2.3 and 1.2.4), or "".
func numericVersion(v string) string {
	if m := versionCore.FindStringSubmatch(v); m != nil {
		return m[1]
	}
	return ""
}

// stageBundle builds the program of t into a temporary folder and writes
// the bundle at bundlePath from it. It returns the executable inside.
func stageBundle(goBin string, opts buildOptions, id identity, bundlePath string, stderr io.Writer) (appbundle.Result, error) {
	stage, err := os.MkdirTemp("", "tela-bundle-")
	if err != nil {
		return appbundle.Result{}, err
	}
	defer os.RemoveAll(stage)
	exe := filepath.Join(stage, "program")
	if err := goBuild(goBin, opts, exe, stderr); err != nil {
		return appbundle.Result{}, err
	}
	return appbundle.Write(bundlePath, appbundle.Spec{
		ID:      id.ID,
		Name:    id.Name,
		Version: moduleVersion(exe),
	}, exe)
}

// buildProgram implements tela build and returns the path it wrote.
func buildProgram(goBin string, opts buildOptions, stderr io.Writer) (string, error) {
	t, err := resolveBuildTarget(goBin, opts, "build")
	if err != nil {
		return "", err
	}
	if t.goos != "darwin" {
		return buildPlain(goBin, opts, t, stderr)
	}
	id, err := resolveIdentity(opts, t, "build", stderr)
	if err != nil {
		return "", err
	}
	bundle, err := bundlePath(opts.Out, id.Name)
	if err != nil {
		return "", err
	}
	if _, err := stageBundle(goBin, opts, id, bundle, stderr); err != nil {
		return "", err
	}
	return bundle, nil
}

// bundlePath is where tela build writes the bundle: out when it ends in
// .app, else <out>/<Name>.app (out defaults to the current folder).
func bundlePath(out, name string) (string, error) {
	if out == "" {
		out = "."
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(abs, ".app") {
		abs = filepath.Join(abs, appbundle.FileName(name)+".app")
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return "", fmt.Errorf("%s exists and is not a folder; remove it or choose another -o", abs)
	}
	return abs, nil
}

// buildPlain builds the executable for a target that has no bundle.
func buildPlain(goBin string, opts buildOptions, t *buildTarget, stderr io.Writer) (string, error) {
	name := packageBaseName(t.pkg)
	if t.goos == "windows" {
		name += ".exe"
	}
	out := opts.Out
	if out == "" {
		out = name
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && st.IsDir() {
		abs = filepath.Join(abs, name)
	}
	if err := goBuild(goBin, opts, abs, stderr); err != nil {
		return "", err
	}
	return abs, nil
}

// runProgram implements tela run and returns the program's exit status.
func runProgram(goBin string, opts buildOptions, stdout, stderr io.Writer) (int, error) {
	probe, err := resolveBuildTarget(goBin, opts, "run")
	if err != nil {
		return 1, err
	}
	if probe.goos != runtime.GOOS || probe.goarch != runtime.GOARCH {
		return 1, fmt.Errorf("the target %s/%s is not this machine (%s/%s), and tela run runs the program here; unset GOOS and GOARCH, or use tela build", probe.goos, probe.goarch, runtime.GOOS, runtime.GOARCH)
	}
	var exe string
	if probe.goos == "darwin" {
		id, err := resolveIdentity(opts, probe, "run", stderr)
		if err != nil {
			return 1, err
		}
		bundle, err := runBundlePath(id)
		if err != nil {
			return 1, err
		}
		res, err := stageBundle(goBin, opts, id, bundle, stderr)
		if err != nil {
			return 1, err
		}
		removeOtherBundles(filepath.Dir(bundle), bundle)
		if opts.Verbose {
			fmt.Fprintf(stderr, "bundle %s (created %v, Info.plist written %v, executable written %v)\n", bundle, res.Created, res.InfoChanged, res.ExeChanged)
		}
		exe = res.Exe
	} else {
		dir, err := os.MkdirTemp("", "tela-run-")
		if err != nil {
			return 1, err
		}
		defer os.RemoveAll(dir)
		exe = filepath.Join(dir, packageBaseName(probe.pkg))
		if probe.goos == "windows" {
			exe += ".exe"
		}
		if err := goBuild(goBin, opts, exe, stderr); err != nil {
			return 1, err
		}
	}
	cmd := exec.Command(exe, opts.Args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	return runChild(cmd), nil
}

// userCacheDir is os.UserCacheDir; tests replace it.
var userCacheDir = os.UserCacheDir

// runBundlePath is the bundle tela run keeps for an app: a folder named by
// the app's ID in the user's cache, so one app has one bundle there.
func runBundlePath(id identity) (string, error) {
	cache, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache folder for the bundle (%v); set HOME (or XDG_CACHE_HOME), or use tela build", err)
	}
	return filepath.Join(cache, "tela", "run", id.ID, appbundle.FileName(id.Name)+".app"), nil
}

// removeOtherBundles deletes the bundles in dir other than keep: the app was
// renamed, and Launch Services must see one bundle for its ID.
func removeOtherBundles(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if p != keep && strings.HasSuffix(e.Name(), ".app") {
			os.RemoveAll(p)
		}
	}
}

// runChild runs cmd as the user's program: Ctrl+C reaches it through the
// terminal (it is in the same process group), a SIGTERM or SIGHUP sent to
// tela run is forwarded, and its exit status is the result.
func runChild(cmd *exec.Cmd) int {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(cmd.Stderr, "tela run: starting %s: %v\n", cmd.Path, err)
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case s := <-sigs:
			if s != os.Interrupt {
				cmd.Process.Signal(s)
			}
		case err := <-done:
			return exitStatus(err)
		}
	}
}

// exitStatus is the exit code for the result of Wait: the program's code,
// or 128 plus the signal that ended it, as a shell reports it.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
	}
	return 1
}
