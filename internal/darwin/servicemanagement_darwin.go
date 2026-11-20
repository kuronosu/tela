//go:build darwin

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The ServiceManagement framework, home of SMAppService (macOS 13+), the
// app's login item (platform contract v16, ADR-298). Like every framework
// it must be opened before its classes can be found without cgo.

const frameworkServiceManagement = "/System/Library/Frameworks/ServiceManagement.framework/ServiceManagement"

// SMAppServiceStatus values (SMAppService.h).
const (
	SMAppServiceStatusNotRegistered    = 0
	SMAppServiceStatusEnabled          = 1
	SMAppServiceStatusRequiresApproval = 2
	SMAppServiceStatusNotFound         = 3
)

var (
	smOnce sync.Once
	smOK   bool
)

// LoadServiceManagement opens ServiceManagement once and reports whether
// its SMAppService class exists (macOS 13 and later).
func LoadServiceManagement() bool {
	smOnce.Do(func() {
		if _, err := purego.Dlopen(frameworkServiceManagement, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
		smOK = objc.GetClass("SMAppService") != 0
	})
	return smOK
}

// The four-character codes of the launch Apple event (AppleEvents.h,
// AERegistry.h): an 'oapp' event whose keyAEPropData parameter is
// keyAELaunchedAsLogInItem says that the system started the app as a login
// item (what sindresorhus/LaunchAtLogin-Modern reads with SMAppService).
const (
	KAEOpenApplication       = 0x6f617070 // 'oapp'
	KeyAEPropData            = 0x70726474 // 'prdt'
	KeyAELaunchedAsLogInItem = 0x6c676974 // 'lgit'
)
