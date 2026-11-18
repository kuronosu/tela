package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Schema is the manifest schema this package reads and writes.
const Schema = 1

// Manifest is the update manifest of an app, schema 1 (ADR-296): a JSON
// file next to its detached signature, <manifest>.sig ([Sign]). Clients
// ignore the fields they do not know, so later versions of Tela add fields
// (and patch kinds) without a new schema; a manifest whose Schema is not 1
// is refused.
//
//	{
//	  "schema": 1,
//	  "appID": "com.example.notes",
//	  "published": "2026-11-17T10:00:00Z",
//	  "expires": "2027-02-15T10:00:00Z",
//	  "endOfSupport": {"below": "1.0.0", "message": "Download Notes 2 from the website.", "url": "https://example.com/notes"},
//	  "channels": {
//	    "stable": [{
//	      "version": "1.2.0",
//	      "date": "2026-11-17T10:00:00Z",
//	      "notes": "Faster search.",
//	      "rollout": 0.25,
//	      "activation": "next-start",
//	      "minVersion": "1.0.0",
//	      "artifacts": [{
//	        "os": "linux", "arch": "amd64", "format": "appimage",
//	        "url": "notes-1.2.0-linux-amd64.AppImage",
//	        "size": 48213504,
//	        "sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
//	        "patches": [{"from": "1.1.0", "fromSha256": "…", "kind": "zstd-patch", "url": "…", "size": 812, "sha256": "…"}]
//	      }]
//	    }]
//	  }
//	}
type Manifest struct {
	// Schema is 1.
	Schema int `json:"schema"`
	// AppID is the app's ID (tela.App.AppID); a client refuses the
	// manifest of another app.
	AppID string `json:"appID"`
	// Published is when the manifest was written. A client refuses a
	// manifest published before the newest one it has seen (rollback
	// protection), so a server cannot serve an old manifest again.
	Published time.Time `json:"published"`
	// Expires is when the manifest stops being valid. A client refuses an
	// expired manifest (freeze protection: a server cannot keep serving
	// one that hides a newer release), so sign a new one before then,
	// even without a release. Required.
	Expires time.Time `json:"expires"`
	// EndOfSupport, when set, names the versions that can no longer
	// update: [Check] returns an [*EndOfSupportError] for them.
	EndOfSupport *EndOfSupport `json:"endOfSupport,omitempty"`
	// Channels maps a channel name ("stable", "beta", any) to its
	// releases, in any order. A client reads only its own channel: list a
	// release in several channels to offer it in each.
	Channels map[string][]Release `json:"channels"`
}

// EndOfSupport says which versions can no longer update, and what to do.
type EndOfSupport struct {
	// Below: versions lower than this one are out of support.
	Below string `json:"below"`
	// Message is shown to the user ("Download Notes 2 from the website").
	Message string `json:"message,omitempty"`
	// URL is where to go next.
	URL string `json:"url,omitempty"`
}

// Release is one version of the app on a channel.
type Release struct {
	// Version is a Semantic Versioning version, without the "v"
	// ("1.2.0", "1.3.0-beta.1").
	Version string `json:"version"`
	// Date is when the release was made.
	Date time.Time `json:"date"`
	// Notes are the release notes, plain text or Markdown for the app to
	// show.
	Notes string `json:"notes,omitempty"`
	// Rollout is the share of installs offered the release, from 0 (none,
	// paused) to 1 (all); nil means 1. An install is in the share when
	// its bucket for the version is below Rollout ([Check] documents the
	// bucket), so raising the share keeps the installs already offered.
	Rollout *float64 `json:"rollout,omitempty"`
	// Activation is when the update takes effect: "next-start" (the
	// default), "app" (the app decides, for example with a banner) or
	// "now". Config.Policy overrides it. An unknown value is next-start.
	Activation Activation `json:"activation,omitempty"`
	// MinVersion, when set, is the lowest version that may update to this
	// release directly; lower versions are offered the newest release
	// they may update to, a stepping stone.
	MinVersion string `json:"minVersion,omitempty"`
	// Artifacts are the release's files, one per target.
	Artifacts []Artifact `json:"artifacts"`
}

// Artifact is the file of a release for one target.
type Artifact struct {
	// OS and Arch are GOOS and GOARCH values ("linux", "amd64").
	OS   string `json:"os"`
	Arch string `json:"arch"`
	// Format is the package format: "appimage", "deb", "flatpak",
	// "msix", "zip" (Windows), "app" (a zip of the macOS bundle) or any
	// other that tela package writes.
	Format string `json:"format"`
	// URL is where the file is, absolute or relative to the manifest's
	// URL.
	URL string `json:"url"`
	// Size is the file's length in bytes, and SHA256 its SHA-256 in
	// lowercase hex: a download that differs is discarded.
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	// Patches are the ways to make this file from an older one, for
	// delta updates. This version of Tela downloads the full file and
	// reads none; a client ignores kinds it does not know.
	Patches []Patch `json:"patches,omitempty"`
}

// Patch is a delta that turns the artifact of an older version into this
// artifact.
type Patch struct {
	// From is the older version, and FromSHA256 the SHA-256 of its
	// artifact for the same target, which the patch applies to.
	From       string `json:"from"`
	FromSHA256 string `json:"fromSha256"`
	// Kind names the patch algorithm. A client skips kinds it does not
	// know.
	Kind string `json:"kind"`
	// URL, Size and SHA256 are the patch file's, as an Artifact's.
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// maxManifest and maxSignature bound what Check reads.
const (
	maxManifest  = 8 << 20
	maxSignature = 64 << 10
)

// parseManifest parses verified manifest bytes and checks the fields
// every client relies on. Unknown fields are ignored.
func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("update: the manifest is not valid JSON: %w", err)
	}
	if m.Schema != Schema {
		return nil, fmt.Errorf("update: the manifest has schema %d; this version of Tela reads schema %d", m.Schema, Schema)
	}
	if m.AppID == "" {
		return nil, fmt.Errorf("update: the manifest has no appID")
	}
	if m.Published.IsZero() {
		return nil, fmt.Errorf("update: the manifest has no publication time (published); without one an older manifest could be served again")
	}
	if m.Expires.IsZero() {
		return nil, fmt.Errorf("update: the manifest has no expiry date (expires); a manifest without one could be served forever")
	}
	return &m, nil
}

// Encode returns m as indented JSON, the form tela update manifest writes:
// the releases of each channel sorted newest first, so the bytes are the
// same for the same manifest.
func (m *Manifest) Encode() ([]byte, error) {
	sorted := *m
	sorted.Channels = make(map[string][]Release, len(m.Channels))
	for name, rs := range m.Channels {
		rs = slices.Clone(rs)
		slices.SortStableFunc(rs, func(a, b Release) int {
			c, err := CompareVersions(b.Version, a.Version)
			if err != nil {
				return strings.Compare(b.Version, a.Version)
			}
			return c
		})
		sorted.Channels[name] = rs
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&sorted); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rolloutBucket returns the bucket of an install for a version, in [0, 1):
// the first 8 bytes of SHA-256(installID, a zero byte, version) as a
// big-endian integer, scaled down. Hashing the version too reshuffles the
// installs at every release, so the same installs are not always first.
func rolloutBucket(installID, version string) float64 {
	h := sha256.New()
	h.Write([]byte(installID))
	h.Write([]byte{0})
	h.Write([]byte(version))
	sum := h.Sum(nil)
	return float64(binary.BigEndian.Uint64(sum[:8])>>11) / (1 << 53)
}

// inRollout reports whether the install with this bucket is offered r.
func (r *Release) inRollout(bucket float64) bool {
	if r.Rollout == nil {
		return true
	}
	share := *r.Rollout
	if math.IsNaN(share) {
		return false
	}
	return bucket < share
}

// target is what a client looks for in a manifest.
type target struct {
	os, arch, format string
	current          semver
	installID        string
	skip             []string // versions rolled back on this install
}

// choose returns the release to offer and its artifact, or nil when none
// is newer than the current version, in rollout, reachable from it
// (MinVersion) and built for the target. Releases with invalid versions
// are an error: the manifest was signed, so they are a publishing mistake.
func choose(rs []Release, t target) (*Release, *Artifact, error) {
	type cand struct {
		r *Release
		v semver
	}
	var cands []cand
	for i := range rs {
		r := &rs[i]
		v, err := parseSemver(r.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("update: manifest release: %w", err)
		}
		cands = append(cands, cand{r, v})
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return b.v.compare(a.v) })
	for _, c := range cands {
		if c.v.compare(t.current) <= 0 {
			break
		}
		if slices.Contains(t.skip, c.r.Version) || !c.r.inRollout(rolloutBucket(t.installID, c.r.Version)) {
			continue
		}
		if c.r.MinVersion != "" {
			min, err := parseSemver(c.r.MinVersion)
			if err != nil {
				return nil, nil, fmt.Errorf("update: release %s minVersion: %w", c.r.Version, err)
			}
			if t.current.compare(min) < 0 {
				continue
			}
		}
		for j := range c.r.Artifacts {
			a := &c.r.Artifacts[j]
			if a.OS == t.os && a.Arch == t.arch && a.Format == t.format {
				return c.r, a, nil
			}
		}
	}
	return nil, nil, nil
}

// resolveURL returns ref resolved against the manifest's URL, refusing
// schemes other than http and https.
func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("update: manifest URL: %w", err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("update: artifact URL %q: %w", ref, err)
	}
	u := b.ResolveReference(r)
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("update: artifact URL %q: only http and https are supported", u)
	}
	return u.String(), nil
}
