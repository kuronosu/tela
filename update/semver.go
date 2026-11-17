package update

import (
	"fmt"
	"strconv"
	"strings"
)

// semver is a parsed Semantic Versioning 2.0.0 version.
type semver struct {
	major, minor, patch uint64
	pre                 []string // pre-release identifiers, nil for a release
}

// parseSemver parses "1.2.3", "1.2.3-beta.1" or "1.2.3+build", with or
// without a leading "v". Build metadata is accepted and ignored, as the
// specification says for precedence.
func parseSemver(s string) (semver, error) {
	in := s
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		if !validIdents(s[i+1:], false) {
			return semver{}, fmt.Errorf("version %q: invalid build metadata", in)
		}
		s = s[:i]
	}
	var v semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		if !validIdents(pre, true) {
			return semver{}, fmt.Errorf("version %q: invalid pre-release %q", in, pre)
		}
		v.pre = strings.Split(pre, ".")
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", in)
	}
	nums := [3]*uint64{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || len(p) > 1 && p[0] == '0' {
			return semver{}, fmt.Errorf("version %q: invalid number %q", in, p)
		}
		*nums[i] = n
	}
	return v, nil
}

// validIdents reports whether s is a dot-separated list of identifiers of
// [0-9A-Za-z-]; numeric identifiers of a pre-release have no leading zero.
func validIdents(s string, pre bool) bool {
	if s == "" {
		return false
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		numeric := true
		for i := 0; i < len(id); i++ {
			c := id[i]
			switch {
			case c >= '0' && c <= '9':
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-':
				numeric = false
			default:
				return false
			}
		}
		if pre && numeric && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

// compare returns -1, 0 or +1 as v is lower than, equal to or higher than
// w in Semantic Versioning precedence: a pre-release is lower than its
// release (1.2.0-beta.1 < 1.2.0), and pre-release identifiers compare
// numerically when both are numbers, else as ASCII, a number below a word.
func (v semver) compare(w semver) int {
	for _, d := range [...][2]uint64{{v.major, w.major}, {v.minor, w.minor}, {v.patch, w.patch}} {
		if d[0] != d[1] {
			return cmpOrder(d[0] < d[1])
		}
	}
	switch {
	case len(v.pre) == 0 && len(w.pre) == 0:
		return 0
	case len(v.pre) == 0:
		return 1
	case len(w.pre) == 0:
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(w.pre); i++ {
		a, b := v.pre[i], w.pre[i]
		if a == b {
			continue
		}
		na, ea := strconv.ParseUint(a, 10, 64)
		nb, eb := strconv.ParseUint(b, 10, 64)
		switch {
		case ea == nil && eb == nil:
			return cmpOrder(na < nb)
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		}
		return cmpOrder(a < b)
	}
	switch {
	case len(v.pre) < len(w.pre):
		return -1
	case len(v.pre) > len(w.pre):
		return 1
	}
	return 0
}

func cmpOrder(less bool) int {
	if less {
		return -1
	}
	return 1
}

// CompareVersions compares two Semantic Versioning versions ("1.2.0",
// "v1.3.0-beta.2"): -1 when a is lower than b, 0 when they are equal in
// precedence, +1 when a is higher. It returns an error when either is not
// a version.
func CompareVersions(a, b string) (int, error) {
	va, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	vb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	return va.compare(vb), nil
}
