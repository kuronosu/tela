//go:build linux

package assoc

import "github.com/kuronosu/tela/internal/platform"

// Native returns the associations of this session: Linux with the XDG
// directories of the environment (LinuxDirsFromEnv).
func Native() platform.Associations { return Linux(LinuxDirsFromEnv()) }
