//go:build darwin

package assoc

import "github.com/kuronosu/tela/internal/platform"

// Native returns the associations of the current macOS user: the app
// bundle's Info.plist and Launch Services (MacOS, macos.go).
func Native() platform.Associations { return MacOS(nil) }
