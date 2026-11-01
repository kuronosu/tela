//go:build !linux && !windows

package assoc

import "github.com/kuronosu/tela/internal/platform"

// Native returns platform.UnsupportedAssociations: macOS registers schemes
// and document types through the app bundle's Info.plist (phase 14).
func Native() platform.Associations { return platform.UnsupportedAssociations{} }
