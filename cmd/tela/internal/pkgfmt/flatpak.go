package pkgfmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

// flatpakFormat writes a flatpak-builder manifest and the folder it
// builds from, <package>-<version>-linux-<arch>-flatpak:
//
//	<ID>.json          the manifest
//	files/<package>    the program
//	files/...          the desktop entry, metainfo, MIME package, icons
//
// and, when flatpak-builder and flatpak are installed and the target is
// this machine's architecture, the bundle
// <package>-<version>-linux-<arch>.flatpak (flatpak-builder, then flatpak
// build-bundle; its bytes are flatpak's, not deterministic). The manifest
// uses the runtime org.freedesktop.Platform at FlatpakRuntimeVersion and
// a simple build that installs the prebuilt files. flatpak manages the
// install, so package update reports ErrManaged for it.
type flatpakFormat struct{}

// FlatpakRuntimeVersion is the branch of org.freedesktop.Platform and
// .Sdk the manifest names: 25.08, the Freedesktop SDK release of August
// 2025, supported until August 2027. The program is static and needs
// nothing of the runtime but the sandbox; change runtime-version in the
// manifest for a newer branch.
const FlatpakRuntimeVersion = "25.08"

func (flatpakFormat) Name() string { return "flatpak" }
func (flatpakFormat) OS() string   { return "linux" }

// flatpakArches are flatpak's names of the Go architectures.
var flatpakArches = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}

func (flatpakFormat) Check(app App, t Target) error {
	if _, ok := flatpakArches[t.Arch]; !ok {
		return fmt.Errorf("flatpak packages are built for linux/amd64 and linux/arm64, not %s", t)
	}
	return CheckFlatpakID(app.ID)
}

// CheckFlatpakID checks flatpak's rules for an application ID: three
// parts or more separated by periods, each of ASCII letters, digits and
// '_' (and '-' in the last one), none starting with a digit, at most 255
// characters.
func CheckFlatpakID(id string) error {
	parts := strings.Split(id, ".")
	bad := len(id) > 255 || len(parts) < 3
	for i, p := range parts {
		if p == "" || p[0] >= '0' && p[0] <= '9' {
			bad = true
		}
		for _, r := range p {
			if !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' && i == len(parts)-1)) {
				bad = true
			}
		}
	}
	if bad {
		return fmt.Errorf("the app ID %q is not a flatpak application ID: give three parts or more, such as \"com.example.Notes\", of letters, digits and '_' ('-' only in the last), none starting with a digit", id)
	}
	return nil
}

// FinishArgs returns the sandbox permissions of the app: the display
// (Wayland, X11 as a fallback, IPC for X11's shared memory), the GPU, the
// tray (Tela's StatusNotifierItem owns org.kde.StatusNotifierItem-<pid>-<n>,
// which flatpak allows through the org.kde.* prefix, and talks to
// org.kde.StatusNotifierWatcher), and from the declaration's Services:
// Network --share=network, Notifications the
// org.freedesktop.Notifications bus name, Secrets org.freedesktop.secrets,
// Files --filesystem=home, Microphone --socket=pulseaudio, Bluetooth
// --allow=bluetooth. The rest needs no permission (the camera, the
// location and global shortcuts go through portals, which ask the user;
// dialogs, settings, the trash and opening URLs too; accessibility goes
// through the bus flatpak proxies by itself), or has no flatpak mapping
// (contacts, calendars, photos), which notes returns.
func FinishArgs(app App) (args, notes []string) {
	args = []string{"--socket=wayland", "--socket=fallback-x11", "--share=ipc", "--device=dri",
		"--talk-name=org.kde.StatusNotifierWatcher", "--own-name=org.kde.*"}
	s := app.Services
	if s.Network {
		args = append(args, "--share=network")
	}
	if s.Notifications {
		args = append(args, "--talk-name=org.freedesktop.Notifications")
	}
	if s.Secrets {
		args = append(args, "--talk-name=org.freedesktop.secrets")
	}
	if s.Files {
		args = append(args, "--filesystem=home")
	}
	if s.Microphone != "" {
		args = append(args, "--socket=pulseaudio")
	}
	if s.Bluetooth != "" {
		args = append(args, "--allow=bluetooth")
	}
	for _, n := range []struct{ name, use, note string }{
		{"Camera", s.Camera, "through the Camera portal, which asks the user: no permission"},
		{"Location", s.Location, "through the Location portal, which asks the user: no permission"},
		{"Contacts", s.Contacts, "flatpak has no permission for it: not mapped"},
		{"Calendars", s.Calendars, "flatpak has no permission for it: not mapped"},
		{"Photos", s.Photos, "flatpak has no permission for it (files come through the file chooser portal): not mapped"},
	} {
		if n.use != "" {
			notes = append(notes, "Services."+n.name+": "+n.note)
		}
	}
	return args, notes
}

type flatpakManifest struct {
	ID             string          `json:"id"`
	Runtime        string          `json:"runtime"`
	RuntimeVersion string          `json:"runtime-version"`
	SDK            string          `json:"sdk"`
	Command        string          `json:"command"`
	FinishArgs     []string        `json:"finish-args"`
	Modules        []flatpakModule `json:"modules"`
}

type flatpakModule struct {
	Name          string          `json:"name"`
	Buildsystem   string          `json:"buildsystem"`
	BuildCommands []string        `json:"build-commands"`
	Sources       []flatpakSource `json:"sources"`
}

type flatpakSource struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

func (f flatpakFormat) Write(job Job) ([]Output, error) {
	app := job.App
	prog, err := readProgram(job)
	if err != nil {
		return nil, err
	}
	// The files under files/, and where the build installs each.
	type inst struct {
		src, dst string
		mode     os.FileMode
		data     []byte
	}
	list := []inst{
		{app.Package, "/app/bin/" + app.Package, 0o755, prog},
		{app.ID + ".desktop", "/app/share/applications/" + app.ID + ".desktop", 0o644, DesktopEntry(app, app.Package)},
		{app.ID + ".metainfo.xml", "/app/share/metainfo/" + app.ID + ".metainfo.xml", 0o644, Metainfo(app, job.Time)},
	}
	if m := MIMEPackage(app); m != nil {
		list = append(list, inst{app.ID + ".xml", "/app/share/mime/packages/" + app.ID + ".xml", 0o644, m})
	}
	icons, err := Icons(app, "share")
	if err != nil {
		return nil, err
	}
	for _, ic := range icons {
		rel := strings.TrimPrefix(ic.path, "share/icons/hicolor/")
		list = append(list, inst{"icons/" + rel, "/app/" + ic.path, 0o644, ic.data})
	}

	dir := filepath.Join(job.OutDir, FileName(app, job.Target, "-flatpak"))
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	var cmds []string
	for _, in := range list {
		p := filepath.Join(dir, "files", filepath.FromSlash(in.src))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, in.data, in.mode); err != nil {
			return nil, err
		}
		if err := os.Chmod(p, in.mode); err != nil {
			return nil, err
		}
		os.Chtimes(p, job.Time, job.Time)
		cmds = append(cmds, fmt.Sprintf("install -Dm%o %s %s", in.mode, shellQuote(in.src), shellQuote(in.dst)))
	}
	args, notes := FinishArgs(app)
	for _, n := range notes {
		fmt.Fprintf(job.Log, "tela package: flatpak: %s\n", n)
	}
	m := flatpakManifest{
		ID: app.ID, Runtime: "org.freedesktop.Platform", RuntimeVersion: FlatpakRuntimeVersion,
		SDK: "org.freedesktop.Sdk", Command: app.Package, FinishArgs: args,
		Modules: []flatpakModule{{
			Name: app.Package, Buildsystem: "simple", BuildCommands: cmds,
			Sources: []flatpakSource{{Type: "dir", Path: "files"}},
		}},
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	manifest := filepath.Join(dir, app.ID+".json")
	if err := os.WriteFile(manifest, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	os.Chtimes(manifest, job.Time, job.Time)
	outs := []Output{{Path: dir, Note: "flatpak-builder manifest and its files"}}

	bundle := FileName(app, job.Target, ".flatpak")
	builder, err1 := exec.LookPath("flatpak-builder")
	fp, err2 := exec.LookPath("flatpak")
	switch {
	case err1 != nil || err2 != nil:
		outs[0].Note += fmt.Sprintf("; no %s: flatpak-builder is not installed (build it with flatpak-builder --repo=repo build %s, then flatpak build-bundle repo %s %s)", bundle, filepath.Base(manifest), bundle, app.ID)
	case job.Target.Arch != runtime.GOARCH:
		outs[0].Note += fmt.Sprintf("; no %s: flatpak-builder builds for this machine's architecture (%s); build it on a linux/%s machine", bundle, runtime.GOARCH, job.Target.Arch)
	default:
		p, err := buildBundle(job, builder, fp, manifest, bundle)
		if err != nil {
			return nil, err
		}
		o, err := sign(job, p)
		if err != nil {
			return nil, err
		}
		outs = append(outs, o)
	}
	return outs, nil
}

// buildBundle runs flatpak-builder and flatpak build-bundle.
func buildBundle(job Job, builder, fp, manifest, bundle string) (string, error) {
	work := job.Work
	repo := filepath.Join(work, "repo")
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = filepath.Dir(manifest)
		cmd.Env = append(os.Environ(), "SOURCE_DATE_EPOCH="+strconv.FormatInt(job.Time.Unix(), 10))
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %v\n%s", path.Base(name), err, out.String())
		}
		return nil
	}
	if err := run(builder, "--force-clean", "--disable-rofiles-fuse", "--arch="+flatpakArches[job.Target.Arch],
		"--state-dir="+filepath.Join(work, "state"), "--repo="+repo, filepath.Join(work, "build"), manifest); err != nil {
		return "", err
	}
	out := filepath.Join(job.OutDir, bundle)
	if err := run(fp, "build-bundle", "--arch="+flatpakArches[job.Target.Arch], repo, out, job.App.ID); err != nil {
		return "", err
	}
	return out, nil
}

// shellQuote quotes s for the build commands, which flatpak-builder runs
// with sh -c.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("/._-+", r)))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
