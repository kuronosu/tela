package pkgfmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Notarizer is a Signer that also has files notarized: Notarize uploads
// the file upload (a zip of a bundle, a dmg) to the notary service, waits
// for the verdict and staples the ticket to staple (the bundle, the dmg).
// It returns false when it does nothing for t.
type Notarizer interface {
	Notarize(upload, staple string, t Target) (bool, error)
}

// Notarize runs the signers of the chain that notarize.
func (ch Chain) Notarize(upload, staple string, t Target) (bool, error) {
	done := false
	for _, s := range ch {
		if n, ok := s.(Notarizer); ok {
			ok, err := n.Notarize(upload, staple, t)
			if err != nil {
				return done, err
			}
			done = done || ok
		}
	}
	return done, nil
}

// notarize has upload notarized and stapled to staple when job's signer
// notarizes.
func notarize(job Job, upload, staple string) (bool, error) {
	n, ok := job.Sign.(Notarizer)
	if !ok {
		return false, nil
	}
	return n.Notarize(upload, staple, job.Target)
}

// MacSigner is -sign for darwin targets on a macOS host: Apple's codesign
// with a Developer ID, and the notary service through notarytool.
//
//   - A program (SignProgram) and a bundle (SignBundle): codesign --force
//     --options runtime --timestamp --entitlements <file> -s <identity>,
//     the hardened runtime notarization requires, with the app's
//     entitlements (Entitlements). A bundle is then checked with codesign
//     --verify --deep --strict.
//   - A dmg (SignPackage): codesign --force --timestamp -s <identity>; a
//     disk image has no runtime or entitlements. Other packages (the zip of
//     the app format, which holds the signed bundle) are not signed.
//   - Notarize, with NotaryProfile: xcrun notarytool submit <file>
//     --keychain-profile <profile> --wait, then xcrun stapler staple. The
//     profile holds the Apple ID or API key, stored once with xcrun
//     notarytool store-credentials.
type MacSigner struct {
	// Identity is the signing identity, as codesign -s takes it
	// ("Developer ID Application: Example Ltd (TEAMID)"); "-" signs ad
	// hoc, which Gatekeeper refuses on other Macs but shows that the
	// bundle signs and runs with the hardened runtime.
	Identity string
	// NotaryProfile is the keychain profile of notarytool; "" does not
	// notarize.
	NotaryProfile string
	// Entitlements is the entitlements file of programs and bundles.
	Entitlements string
	// Out receives the tools' output.
	Out io.Writer
	// Run runs a command and returns its standard output; nil runs it
	// (tests replace it).
	Run func(name string, args ...string) ([]byte, error)
}

func (s MacSigner) run(name string, args ...string) ([]byte, error) {
	if s.Run != nil {
		return s.Run(name, args...)
	}
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = s.Out
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out.String())
	}
	return out.Bytes(), err
}

func (s MacSigner) Sign(path, what string, t Target) (bool, error) {
	if t.OS != "darwin" {
		return false, nil
	}
	// The identity "-" signs ad hoc (a local check of the hardened
	// runtime), which cannot carry a secure timestamp.
	ts := "--timestamp"
	if s.Identity == "-" {
		ts = "--timestamp=none"
	}
	switch what {
	case SignProgram, SignBundle:
		args := []string{"--force", "--options", "runtime", ts}
		if s.Entitlements != "" {
			args = append(args, "--entitlements", s.Entitlements)
		}
		args = append(args, "-s", s.Identity, path)
		if _, err := s.run("codesign", args...); err != nil {
			return false, err
		}
		if what == SignBundle {
			if _, err := s.run("codesign", "--verify", "--deep", "--strict", path); err != nil {
				return false, err
			}
		}
		return true, nil
	case SignPackage:
		if !strings.HasSuffix(path, ".dmg") {
			return false, nil
		}
		if _, err := s.run("codesign", "--force", ts, "-s", s.Identity, path); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s MacSigner) Notarize(upload, staple string, t Target) (bool, error) {
	if t.OS != "darwin" || s.NotaryProfile == "" {
		return false, nil
	}
	out, err := s.run("xcrun", "notarytool", "submit", upload, "--keychain-profile", s.NotaryProfile, "--wait", "--output-format", "json")
	if err != nil {
		return false, err
	}
	// notarytool exits 0 when the submission ends Invalid too: the status
	// says.
	var res struct{ ID, Status, Message string }
	if err := json.Unmarshal(bytes.TrimSpace(out), &res); err != nil {
		return false, fmt.Errorf("notarytool printed %q: %v", out, err)
	}
	if res.Status != "Accepted" {
		return false, fmt.Errorf("the notary service answered %s for %s (%s); see why with: xcrun notarytool log %s --keychain-profile %s",
			res.Status, upload, res.Message, res.ID, s.NotaryProfile)
	}
	if _, err := s.run("xcrun", "stapler", "staple", staple); err != nil {
		return false, err
	}
	return true, nil
}
