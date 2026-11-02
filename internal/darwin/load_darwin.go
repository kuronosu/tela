//go:build darwin

package darwin

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Paths of the libraries and frameworks Load opens. With CGO_ENABLED=0
// nothing is linked at build time, so a framework's classes and functions
// exist only once it is opened (Ebitengine's internal/cocoa notes the same).
const (
	libObjC         = "/usr/lib/libobjc.A.dylib"
	libSystem       = "/usr/lib/libSystem.B.dylib"
	frameworkCF     = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	frameworkCG     = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"
	frameworkQC     = "/System/Library/Frameworks/QuartzCore.framework/QuartzCore"
	frameworkFound  = "/System/Library/Frameworks/Foundation.framework/Foundation"
	frameworkAppKit = "/System/Library/Frameworks/AppKit.framework/AppKit"
)

var (
	loadOnce sync.Once
	loadErr  error
)

// Load opens the frameworks and binds every function and constant of the
// package. It runs once; later calls return the first result. An error
// means the system lacks something Tela needs (not a macOS with AppKit).
func Load() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

func load() (err error) {
	// purego's RegisterLibFunc panics when a symbol is missing.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("darwin: %v", r)
		}
	}()
	libs := map[string]uintptr{}
	for _, path := range []string{libObjC, libSystem, frameworkCF, frameworkCG, frameworkQC, frameworkFound, frameworkAppKit} {
		h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return fmt.Errorf("darwin: opening %s: %w", path, err)
		}
		libs[path] = h
	}
	if err := loadObjC(libs[libObjC], libs[libSystem]); err != nil {
		return err
	}
	loadCF(libs[frameworkCF])
	loadCG(libs[frameworkCG])
	loadConstants(libs[frameworkCF], libs[frameworkCG], libs[frameworkQC])
	return nil
}

// bind binds the C function name of lib to the Go function variable fptr.
func bind(fptr any, lib uintptr, name string) {
	purego.RegisterLibFunc(fptr, lib, name)
}

// symbolValue returns the pointer-sized value stored at the data symbol
// name of lib: the value of a global constant such as
// kCFRunLoopCommonModes (a CFStringRef).
func symbolValue(lib uintptr, name string) uintptr {
	p, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(fmt.Sprintf("looking up %s: %v", name, err))
	}
	// p is the address of a C global, outside the Go heap; the double
	// indirection keeps vet's unsafeptr check quiet about the conversion.
	return *(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
}

// NewCallback returns a C function pointer that calls fn (see purego's
// NewCallback: integer and pointer arguments, at most one integer result).
// Callbacks are never freed and purego allows about 2000 per process:
// create each one once.
func NewCallback(fn any) uintptr { return purego.NewCallback(fn) }
