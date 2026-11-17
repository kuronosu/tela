package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/update"
)

const updateUsage = `Usage:

	tela update <command> [arguments]

Commands:

	keygen      make an ed25519 key that signs update manifests
	manifest    add a release to an update manifest, from its files
	sign        sign an update manifest (writes <manifest>.sig)

The manifest and its signature are what package update reads (ADR-296):
upload them with the release files to a web server and give the manifest's
URL and the public key to update.Config.

Run "tela help update <command>" for details about a command.
`

const updateKeygenUsage = `Usage:

	tela update keygen [-o file]

Makes an ed25519 key pair and writes the private key to file (default
update.key) as a PKCS #8 PEM file readable only by you; it refuses to
overwrite a file. Keep the file secret and out of the repository: whoever
has it can publish updates for the app. It prints the public key in the
form update.Config takes:

	PublicKeys: []update.PublicKey{"ed25519:…"},

and its key ID, which signature files name. To rotate keys, make a new one,
sign manifests with both keys (tela update sign -key old.key -key new.key),
ship a release that trusts both, and later sign with the new one only.
`

const updateSignUsage = `Usage:

	tela update sign -key file [-key file...] [-o file] <manifest>

Signs the manifest's exact bytes with each key (made by tela update keygen)
and writes the detached signature to <manifest>.sig (or -o): a JSON file
with one signature per key, by key ID. Sign again after any change to the
manifest. The manifest is checked first: schema 1, an app ID, publication
and expiry times; an expired manifest is an error.
`

const updateManifestUsage = `Usage:

	tela update manifest [-m manifest.json] -version V [flags] <file>...
	tela update manifest [-m manifest.json] [-expires d]

Adds the release V to the manifest (made when missing; -app-id is then
required), with one artifact per file: its size and SHA-256, and its URL,
the file's name relative to the manifest, or under -base-url. A release
already in the channel keeps the fields not given as flags; an artifact for
the same target is replaced. Each run sets the publication time to now and
the expiry to now plus -expires, so run it without a release to extend a
manifest before it expires. Then sign it (or give -key here).

A file's target comes from its name as tela package writes it,
<name>-<version>-<os>-<arch>.<ext>: .AppImage is appimage, .deb deb,
.flatpak flatpak, .msix msix, .zip zip on windows and app on darwin (the
bundle in a zip), .dmg dmg. Give it explicitly as file=os/arch/format.

Flags:

	-m file           the manifest (default manifest.json)
	-app-id id        the app's ID, for a new manifest (tela.App.AppID)
	-version v        the release's version (Semantic Versioning)
	-channel name     the channel (default stable)
	-date t           the release date, RFC 3339 (default now)
	-notes text       the release notes, or -notes-file file
	-rollout x        the share of installs offered it, 0 to 1 (default 1)
	-activation a     next-start (default), app or now
	-min-version v    the lowest version that may update to it directly
	-base-url url     the URL of the files' folder, when not the manifest's
	-expires d        how long the manifest stays valid (default 90d; 36h,
	                  2160h, 30d)
	-end-of-support v versions below v can no longer update, with
	                  -end-of-support-message text and -end-of-support-url url
	-key file         also sign the manifest with this key (repeatable)
`

func runUpdate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, updateUsage)
		return 2
	}
	cmds := map[string]struct {
		usage string
		run   func([]string, io.Writer) error
	}{
		"keygen":   {updateKeygenUsage, updateKeygen},
		"sign":     {updateSignUsage, updateSign},
		"manifest": {updateManifestUsage, updateManifest},
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			if c, ok := cmds[args[1]]; ok {
				fmt.Fprint(stdout, c.usage)
				return 0
			}
		}
		fmt.Fprint(stdout, updateUsage)
		return 0
	}
	c, ok := cmds[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "tela update: unknown command %q\n\n%s", args[0], updateUsage)
		return 2
	}
	if err := c.run(args[1:], stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, c.usage)
			return 0
		}
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(stderr, "tela update %s: %v\n\n%s", args[0], err, c.usage)
			return 2
		}
		fmt.Fprintf(stderr, "tela update %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

// usageError is a mistake in the command line (exit status 2).
type usageError struct{ error }

func usagef(format string, a ...any) error { return usageError{fmt.Errorf(format, a...)} }

// multiFlag is a flag given several times.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func parseUpdateFlags(command string, args []string, setup func(fs *flag.FlagSet)) ([]string, error) {
	fs := flag.NewFlagSet("tela update "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	setup(fs)
	pos, err := parsePositionals("update "+command, fs, args)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		return nil, usageError{err}
	}
	return pos, err
}

func updateKeygen(args []string, stdout io.Writer) error {
	out := "update.key"
	pos, err := parseUpdateFlags("keygen", args, func(fs *flag.FlagSet) { fs.StringVar(&out, "o", out, "") })
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected arguments %q", pos)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s exists; a new key cannot verify what the old one signed, so choose another -o", out)
		}
		return err
	}
	if err := pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote the private key to %s. Keep it secret: it signs the app's updates.\n\n", out)
	fmt.Fprintf(stdout, "Public key (key ID %s), for update.Config:\n\n\tPublicKeys: []update.PublicKey{%q},\n", update.KeyID(pub), update.NewPublicKey(pub))
	return nil
}

// readKey reads a private key written by keygen.
func readKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b, _ := pem.Decode(data)
	if b == nil || b.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%s is not a PEM private key (make one with tela update keygen)", path)
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	ek, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an ed25519 key", path)
	}
	return ek, nil
}

func updateSign(args []string, stdout io.Writer) error {
	var keys multiFlag
	out := ""
	pos, err := parseUpdateFlags("sign", args, func(fs *flag.FlagSet) {
		fs.Var(&keys, "key", "")
		fs.StringVar(&out, "o", "", "")
	})
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("want one <manifest>, got %d", len(pos))
	}
	if len(keys) == 0 {
		return usagef("missing -key")
	}
	return signManifest(pos[0], out, keys, stdout)
}

func signManifest(path, out string, keyFiles []string, stdout io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := checkManifest(data); err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	var keys []ed25519.PrivateKey
	for _, kf := range keyFiles {
		k, err := readKey(kf)
		if err != nil {
			return err
		}
		keys = append(keys, k)
	}
	sig, err := update.Sign(data, keys...)
	if err != nil {
		return err
	}
	if out == "" {
		out = path + ".sig"
	}
	if err := os.WriteFile(out, sig, 0o644); err != nil {
		return err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = update.KeyID(k.Public().(ed25519.PublicKey))
	}
	fmt.Fprintf(stdout, "Signed %s with key %s: wrote %s.\n", path, strings.Join(ids, ", "), out)
	return nil
}

// checkManifest parses manifest bytes and checks what a client requires.
func checkManifest(data []byte) (*update.Manifest, error) {
	var m update.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("not a manifest: %v", err)
	}
	switch {
	case m.Schema != update.Schema:
		return nil, fmt.Errorf("schema %d, want %d", m.Schema, update.Schema)
	case m.AppID == "":
		return nil, errors.New("no appID")
	case m.Published.IsZero():
		return nil, errors.New("no publication time (published)")
	case m.Expires.IsZero():
		return nil, errors.New("no expiry (expires)")
	case !time.Now().Before(m.Expires):
		return nil, fmt.Errorf("expired on %s; extend it with tela update manifest -m <manifest>", m.Expires.Format(time.RFC3339))
	}
	for ch, rs := range m.Channels {
		for _, r := range rs {
			if _, err := update.CompareVersions(r.Version, r.Version); err != nil {
				return nil, fmt.Errorf("channel %s: %v", ch, err)
			}
		}
	}
	return &m, nil
}

// manifestOptions are tela update manifest's flags.
type manifestOptions struct {
	path, appID, version, channel, date, notes, notesFile string
	rollout                                               float64
	activation, minVersion, baseURL, expires              string
	eosBelow, eosMessage, eosURL                          string
	keys                                                  multiFlag
	set                                                   map[string]bool
	files                                                 []string
}

func updateManifest(args []string, stdout io.Writer) error {
	o := manifestOptions{path: "manifest.json", channel: "stable", rollout: 1, expires: "90d"}
	var fs *flag.FlagSet
	pos, err := parseUpdateFlags("manifest", args, func(f *flag.FlagSet) {
		fs = f
		f.StringVar(&o.path, "m", o.path, "")
		f.StringVar(&o.appID, "app-id", "", "")
		f.StringVar(&o.version, "version", "", "")
		f.StringVar(&o.channel, "channel", o.channel, "")
		f.StringVar(&o.date, "date", "", "")
		f.StringVar(&o.notes, "notes", "", "")
		f.StringVar(&o.notesFile, "notes-file", "", "")
		f.Float64Var(&o.rollout, "rollout", o.rollout, "")
		f.StringVar(&o.activation, "activation", "", "")
		f.StringVar(&o.minVersion, "min-version", "", "")
		f.StringVar(&o.baseURL, "base-url", "", "")
		f.StringVar(&o.expires, "expires", o.expires, "")
		f.StringVar(&o.eosBelow, "end-of-support", "", "")
		f.StringVar(&o.eosMessage, "end-of-support-message", "", "")
		f.StringVar(&o.eosURL, "end-of-support-url", "", "")
		f.Var(&o.keys, "key", "")
	})
	if err != nil {
		return err
	}
	o.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	o.files = pos
	m, err := buildManifest(o, time.Now().UTC().Truncate(time.Second))
	if err != nil {
		return err
	}
	data, err := m.Encode()
	if err != nil {
		return err
	}
	if err := os.WriteFile(o.path, data, 0o644); err != nil {
		return err
	}
	if o.version != "" {
		fmt.Fprintf(stdout, "Wrote %s: %s %s with %d files, valid until %s.\n", o.path, o.channel, o.version, len(o.files), m.Expires.Format(time.RFC3339))
	} else {
		fmt.Fprintf(stdout, "Wrote %s, valid until %s.\n", o.path, m.Expires.Format(time.RFC3339))
	}
	if len(o.keys) > 0 {
		return signManifest(o.path, "", o.keys, stdout)
	}
	if _, err := os.Stat(o.path + ".sig"); err == nil {
		fmt.Fprintf(stdout, "%s.sig no longer matches: sign the manifest again (tela update sign -key <key> %s).\n", o.path, o.path)
	} else {
		fmt.Fprintf(stdout, "Sign it: tela update sign -key <key> %s\n", o.path)
	}
	return nil
}

// buildManifest reads the manifest at o.path (or starts one) and applies
// o at time now.
func buildManifest(o manifestOptions, now time.Time) (*update.Manifest, error) {
	m := &update.Manifest{Schema: update.Schema, Channels: map[string][]update.Release{}}
	if data, err := os.ReadFile(o.path); err == nil {
		m = &update.Manifest{}
		if err := json.Unmarshal(data, m); err != nil {
			return nil, fmt.Errorf("%s: %v", o.path, err)
		}
		if m.Schema != update.Schema {
			return nil, fmt.Errorf("%s has schema %d; this tela writes schema %d", o.path, m.Schema, update.Schema)
		}
		if m.Channels == nil {
			m.Channels = map[string][]update.Release{}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if o.appID != "" {
		if m.AppID != "" && m.AppID != o.appID {
			return nil, usagef("-app-id %s, but %s is the manifest of %s", o.appID, o.path, m.AppID)
		}
		m.AppID = o.appID
	}
	if m.AppID == "" {
		return nil, usagef("a new manifest needs -app-id (the app's tela.App.AppID)")
	}
	valid, err := parseDays(o.expires)
	if err != nil {
		return nil, usagef("-expires: %v", err)
	}
	m.Published, m.Expires = now, now.Add(valid)
	if o.eosBelow != "" {
		if _, err := update.CompareVersions(o.eosBelow, o.eosBelow); err != nil {
			return nil, usagef("-end-of-support: %v", err)
		}
		m.EndOfSupport = &update.EndOfSupport{Below: o.eosBelow, Message: o.eosMessage, URL: o.eosURL}
	}

	if o.version == "" {
		if len(o.files) > 0 {
			return nil, usagef("files without -version")
		}
		return m, nil
	}
	if _, err := update.CompareVersions(o.version, o.version); err != nil {
		return nil, usagef("-version: %v", err)
	}
	o.version = strings.TrimPrefix(o.version, "v")
	rs := m.Channels[o.channel]
	idx, isNew := -1, false
	for i := range rs {
		if c, err := update.CompareVersions(rs[i].Version, o.version); err == nil && c == 0 {
			idx = i
		}
	}
	if idx < 0 {
		rs = append(rs, update.Release{Version: o.version, Date: now})
		idx, isNew = len(rs)-1, true
	}
	r := &rs[idx]
	if o.set["date"] {
		d, err := time.Parse(time.RFC3339, o.date)
		if err != nil {
			return nil, usagef("-date: %v", err)
		}
		r.Date = d
	}
	if o.set["notes-file"] {
		b, err := os.ReadFile(o.notesFile)
		if err != nil {
			return nil, err
		}
		r.Notes = strings.TrimSpace(string(b))
	} else if o.set["notes"] {
		r.Notes = o.notes
	}
	if o.set["rollout"] || isNew {
		if !(o.rollout >= 0 && o.rollout <= 1) {
			return nil, usagef("-rollout %v: use a share from 0 to 1", o.rollout)
		}
		x := o.rollout
		r.Rollout = &x
	}
	if o.set["activation"] {
		switch a := update.Activation(o.activation); a {
		case update.ActivateNextStart, update.ActivateByApp, update.ActivateNow:
			r.Activation = a
		default:
			return nil, usagef("-activation %q: use next-start, app or now", o.activation)
		}
	}
	if o.set["min-version"] {
		if o.minVersion != "" {
			if _, err := update.CompareVersions(o.minVersion, o.minVersion); err != nil {
				return nil, usagef("-min-version: %v", err)
			}
		}
		r.MinVersion = o.minVersion
	}
	for _, f := range o.files {
		a, err := artifactOf(f, o.version, o.baseURL)
		if err != nil {
			return nil, err
		}
		replaced := false
		for i := range r.Artifacts {
			if x := &r.Artifacts[i]; x.OS == a.OS && x.Arch == a.Arch && x.Format == a.Format {
				r.Artifacts[i], replaced = a, true
			}
		}
		if !replaced {
			r.Artifacts = append(r.Artifacts, a)
		}
	}
	m.Channels[o.channel] = rs
	return m, nil
}

// parseDays parses a Go duration or a number of days ("90d").
func parseDays(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		d, err := strconv.Atoi(n)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("%q is not a number of days", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q is not a duration such as 90d or 2160h", s)
	}
	return d, nil
}

// artifactOf returns the artifact of a file argument, path or
// path=os/arch/format.
func artifactOf(arg, version, baseURL string) (update.Artifact, error) {
	path, tgt, explicit := strings.Cut(arg, "=")
	var a update.Artifact
	if explicit {
		parts := strings.Split(tgt, "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return a, usagef("%s: give the target as os/arch/format, such as linux/amd64/appimage", arg)
		}
		a.OS, a.Arch, a.Format = parts[0], parts[1], parts[2]
	} else {
		var err error
		if a.OS, a.Arch, a.Format, err = targetOfName(filepath.Base(path), version); err != nil {
			return a, err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return a, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return a, err
	}
	a.Size, a.SHA256 = n, hex.EncodeToString(h.Sum(nil))
	name := filepath.Base(path)
	a.URL = url.PathEscape(name)
	if baseURL != "" {
		u, err := url.JoinPath(baseURL, name)
		if err != nil {
			return a, usagef("-base-url: %v", err)
		}
		a.URL = u
	}
	return a, nil
}

// targetOfName reads os, arch and format from a file name of tela
// package, <name>-<version>-<os>-<arch>.<ext>.
func targetOfName(name, version string) (goos, goarch, format string, err error) {
	exts := []struct{ ext, format string }{
		{".AppImage", "appimage"}, {".deb", "deb"}, {".flatpak", "flatpak"}, {".msix", "msix"}, {".zip", "zip"}, {".dmg", "dmg"},
	}
	stem := ""
	for _, e := range exts {
		if s, ok := strings.CutSuffix(name, e.ext); ok {
			stem, format = s, e.format
			break
		}
	}
	fields := strings.Split(stem, "-")
	if format == "" || len(fields) < 3 {
		return "", "", "", usagef("%s: cannot tell its target from the name (<name>-<version>-<os>-<arch>.<ext>); give it as %s=os/arch/format", name, name)
	}
	goos, goarch = fields[len(fields)-2], fields[len(fields)-1]
	switch goos {
	case "linux", "windows", "darwin":
	default:
		return "", "", "", usagef("%s: %q is not linux, windows or darwin; give the target as %s=os/arch/format", name, goos, name)
	}
	if format == "zip" && goos == "darwin" {
		format = "app"
	}
	if version != "" && !strings.Contains(stem, "-"+strings.TrimPrefix(version, "v")+"-") {
		return "", "", "", usagef("%s: the name does not carry version %s", name, version)
	}
	return goos, goarch, format, nil
}
