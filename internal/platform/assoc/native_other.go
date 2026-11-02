//go:build !linux && !windows && !darwin

package assoc

import "github.com/kuronosu/tela/internal/platform"

// Native returns platform.UnsupportedAssociations: Tela registers schemes
// and file types on Linux, Windows and macOS only.
func Native() platform.Associations { return platform.UnsupportedAssociations{} }
