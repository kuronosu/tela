//go:build windows

package assoc

import "github.com/kuronosu/tela/internal/platform"

// Native returns the associations of the current Windows user:
// HKEY_CURRENT_USER through advapi32.
func Native() platform.Associations { return Windows(nil) }
