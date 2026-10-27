//go:build linux

package freedesktop

import (
	"fmt"
	"net/url"
	"strings"
)

// uriPath converts a file: URI from the portal to a local path. The path is
// percent-decoded (file names may hold any byte but NUL and '/'); "file:"
// URIs with a host other than localhost, relative or opaque ones, and other
// schemes are errors.
func uriPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("freedesktop: the portal returned an invalid URI: %w", err)
	}
	switch {
	case !strings.EqualFold(u.Scheme, "file"):
		return "", fmt.Errorf("freedesktop: the portal returned %q, which is not a file: URI", uri)
	case u.Host != "" && !strings.EqualFold(u.Host, "localhost"):
		return "", fmt.Errorf("freedesktop: the portal returned %q, a file on another host", uri)
	case u.Opaque != "" || !strings.HasPrefix(u.Path, "/"):
		return "", fmt.Errorf("freedesktop: the portal returned %q, which has no absolute path", uri)
	case strings.ContainsRune(u.Path, 0):
		return "", fmt.Errorf("freedesktop: the portal returned %q, whose path holds a NUL byte", uri)
	}
	return u.Path, nil
}

// uriPaths converts the URIs of a portal answer to paths; any invalid URI
// fails the whole answer.
func uriPaths(uris []string) ([]string, error) {
	var paths []string
	for _, uri := range uris {
		p, err := uriPath(uri)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}
