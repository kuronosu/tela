package pkgfmt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DefaultTimestampURL is the RFC 3161 time-stamp server signtool uses
// without -timestamp-url: DigiCert's, free and public. A time stamp keeps
// a signature valid after the certificate expires.
const DefaultTimestampURL = "http://timestamp.digicert.com"

// Signtool is -sign for Windows targets on a Windows host: it runs
//
//	signtool sign /fd SHA256 /tr <TimestampURL> /td SHA256 <certificate> <file>
//
// on each program (.exe, SignProgram) and MSIX package (SignPackage of a
// .msix); other files (the zip) are not signtool's. The certificate is
// /f CertFile, with /p and the password read from PasswordFile, or /sha1
// CertSHA1 in the certificate store, or /a, the best one signtool finds in
// the store. An MSIX's publisher must be the certificate's subject
// (-msix-publisher).
type Signtool struct {
	// Path is signtool.exe's.
	Path string
	// CertFile, PasswordFile and CertSHA1 choose the certificate.
	CertFile, PasswordFile, CertSHA1 string
	// TimestampURL is the time-stamp server; "" is DefaultTimestampURL.
	TimestampURL string
	// Out receives signtool's output.
	Out io.Writer
}

// Args returns signtool's arguments to sign path, password being the
// content of PasswordFile.
func (s Signtool) Args(path, password string) []string {
	ts := s.TimestampURL
	if ts == "" {
		ts = DefaultTimestampURL
	}
	args := []string{"sign", "/fd", "SHA256", "/tr", ts, "/td", "SHA256"}
	switch {
	case s.CertFile != "":
		args = append(args, "/f", s.CertFile)
		if s.PasswordFile != "" {
			args = append(args, "/p", password)
		}
	case s.CertSHA1 != "":
		args = append(args, "/sha1", s.CertSHA1)
	default:
		args = append(args, "/a")
	}
	return append(args, path)
}

func (s Signtool) Sign(path, what string, t Target) (bool, error) {
	if t.OS != "windows" {
		return false, nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !(what == SignProgram && ext == ".exe" || what == SignPackage && ext == ".msix") {
		return false, nil
	}
	password := ""
	if s.PasswordFile != "" {
		b, err := os.ReadFile(s.PasswordFile)
		if err != nil {
			return false, fmt.Errorf("-cert-password-file: %v", err)
		}
		password = strings.TrimRight(string(b), "\r\n")
	}
	cmd := exec.Command(s.Path, s.Args(path, password)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if s.Out != nil {
		s.Out.Write(out.Bytes())
	}
	if err != nil {
		msg := fmt.Sprintf("signtool sign %s: %v", filepath.Base(path), err)
		if ext == ".msix" && strings.Contains(out.String(), "0x8007000B") {
			msg += "; the package's publisher must be the certificate's subject: pass it with -msix-publisher"
		}
		return false, errors.New(msg)
	}
	return true, nil
}

// FindSigntool returns signtool.exe's path: in PATH, else the newest
// Windows SDK's under the Windows Kits folder (for goarch, the host's
// architecture, else x64 or x86), else "".
func FindSigntool(goarch string) string {
	if p, err := exec.LookPath("signtool"); err == nil {
		return p
	}
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[goarch]
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		root := os.Getenv(env)
		if root == "" {
			continue
		}
		vers, _ := filepath.Glob(filepath.Join(root, "Windows Kits", "10", "bin", "10.*"))
		sort.Slice(vers, func(i, j int) bool { return versionLess(filepath.Base(vers[j]), filepath.Base(vers[i])) })
		for _, v := range vers {
			for _, a := range []string{arch, "x64", "x86"} {
				if p := filepath.Join(v, a, "signtool.exe"); a != "" && isFile(p) {
					return p
				}
			}
		}
		if p := filepath.Join(root, "Windows Kits", "10", "App Certification Kit", "signtool.exe"); isFile(p) {
			return p
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// versionLess compares dotted numbers ("10.0.22621.0").
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.Atoi(as[i])
		y, _ := strconv.Atoi(bs[i])
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}
