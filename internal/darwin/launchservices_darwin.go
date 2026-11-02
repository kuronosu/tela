//go:build darwin

package darwin

import (
	"bytes"
	"errors"
	"sync"
)

// Launch Services (LaunchServices.framework, inside CoreServices) and the
// Uniform Type Identifier function of CoreServices, for the registration of
// URL schemes and file types (internal/platform/assoc). Opened on first use
// by LoadLaunchServices, not by Load: registration runs without a backend
// (an installer, a setup command) and needs neither AppKit nor the
// Objective-C runtime, so the CoreFoundation functions the wrappers use are
// bound here too, from CoreFoundation, independently of Load.
//
// The functions are plain C, thread-safe (they talk to the Launch Services
// database and its daemon), may block briefly, and need no autorelease
// pool: every object is created and released here. The wrappers take and
// return Go strings; a NULL result is "" or nil.
//
// Several of these functions are deprecated since macOS 12 in favor of
// NSWorkspace methods (the handler lists and defaults of schemes and content
// types, UTTypeCreatePreferredIdentifierForTag) but still exported and
// working; see package assoc for why they are used anyway.

// frameworkLaunchServices is the LaunchServices subframework; the
// CoreServices umbrella (frameworkCoreServices, carbon_darwin.go) is the
// fallback.
const frameworkLaunchServices = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/LaunchServices"

// LSRolesAll is kLSRolesAll: every role (viewer, editor, shell, none).
const LSRolesAll = 0xFFFFFFFF

// cfStringEncodingUTF8 is kCFStringEncodingUTF8.
const cfStringEncodingUTF8 = 0x08000100

// pathMax bounds the file system representation of a URL (PATH_MAX is 1024
// on macOS; the margin costs nothing).
const pathMax = 4096

// The bound functions (CF objects are uintptr; OSStatus is int32, 0 is
// noErr).
var ls struct {
	registerURL                func(url uintptr, update bool) int32
	copyAppURLsForBundleID     func(bundleID, outError uintptr) uintptr
	copyAllHandlersForScheme   func(scheme uintptr) uintptr
	copyDefaultHandlerForSch   func(scheme uintptr) uintptr
	setDefaultHandlerForScheme func(scheme, bundleID uintptr) int32
	copyAllRoleHandlers        func(contentType uintptr, roles uint32) uintptr
	copyDefaultRoleHandler     func(contentType uintptr, roles uint32) uintptr
	setDefaultRoleHandler      func(contentType uintptr, roles uint32, bundleID uintptr) int32
	utTypeForTag               func(tagClass, tag, conformingTo uintptr) uintptr

	release           func(cf uintptr)
	stringCreate      func(alloc uintptr, bytes *byte, n int, encoding uint32, external bool) uintptr
	stringLength      func(s uintptr) int
	stringMaxSize     func(length int, encoding uint32) int
	stringGetCString  func(s uintptr, buf *byte, size int, encoding uint32) bool
	arrayCount        func(a uintptr) int
	arrayValue        func(a uintptr, i int) uintptr
	urlCreateFromPath func(alloc uintptr, buf *byte, n int, isDir bool) uintptr
	urlGetPath        func(url uintptr, resolve bool, buf *byte, max int) bool
}

var (
	lsOnce sync.Once
	lsErr  error
)

// LoadLaunchServices opens Launch Services and CoreFoundation and binds the
// functions of this file, once; an error means they are unavailable. Safe
// from any goroutine. Call it before any other function of this file.
func LoadLaunchServices() error {
	lsOnce.Do(func() {
		bindLS := func(lib uintptr) {
			bind(&ls.registerURL, lib, "LSRegisterURL")
			bind(&ls.copyAppURLsForBundleID, lib, "LSCopyApplicationURLsForBundleIdentifier")
			bind(&ls.copyAllHandlersForScheme, lib, "LSCopyAllHandlersForURLScheme")
			bind(&ls.copyDefaultHandlerForSch, lib, "LSCopyDefaultHandlerForURLScheme")
			bind(&ls.setDefaultHandlerForScheme, lib, "LSSetDefaultHandlerForURLScheme")
			bind(&ls.copyAllRoleHandlers, lib, "LSCopyAllRoleHandlersForContentType")
			bind(&ls.copyDefaultRoleHandler, lib, "LSCopyDefaultRoleHandlerForContentType")
			bind(&ls.setDefaultRoleHandler, lib, "LSSetDefaultRoleHandlerForContentType")
			bind(&ls.utTypeForTag, lib, "UTTypeCreatePreferredIdentifierForTag")
		}
		// The functions live in the LaunchServices subframework; the
		// umbrella re-exports them, which dlsym follows. Try the
		// subframework first.
		err1 := loadOptional(frameworkLaunchServices, bindLS)
		if err1 != nil {
			if err2 := loadOptional(frameworkCoreServices, bindLS); err2 != nil {
				lsErr = errors.Join(err1, err2)
				return
			}
		}
		lsErr = loadOptional(frameworkCF, func(lib uintptr) {
			bind(&ls.release, lib, "CFRelease")
			bind(&ls.stringCreate, lib, "CFStringCreateWithBytes")
			bind(&ls.stringLength, lib, "CFStringGetLength")
			bind(&ls.stringMaxSize, lib, "CFStringGetMaximumSizeForEncoding")
			bind(&ls.stringGetCString, lib, "CFStringGetCString")
			bind(&ls.arrayCount, lib, "CFArrayGetCount")
			bind(&ls.arrayValue, lib, "CFArrayGetValueAtIndex")
			bind(&ls.urlCreateFromPath, lib, "CFURLCreateFromFileSystemRepresentation")
			bind(&ls.urlGetPath, lib, "CFURLGetFileSystemRepresentation")
		})
	})
	return lsErr
}

// cfString returns a new CFString (+1) holding s (invalid UTF-8 and NUL
// characters become U+FFFD, as with CString).
func cfString(s string) uintptr {
	s = CString(s)
	b := []byte(s)
	var p *byte
	if len(b) > 0 {
		p = &b[0]
	}
	return ls.stringCreate(0, p, len(b), cfStringEncodingUTF8, false)
}

// goStringCF returns the text of the CFString s ("" for NULL).
func goStringCF(s uintptr) string {
	if s == 0 {
		return ""
	}
	n := ls.stringMaxSize(ls.stringLength(s), cfStringEncodingUTF8) + 1
	if n <= 1 {
		return ""
	}
	buf := make([]byte, n)
	if !ls.stringGetCString(s, &buf[0], n, cfStringEncodingUTF8) {
		return ""
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf)
}

// takeString returns the text of the +1 CFString s and releases it.
func takeString(s uintptr) string {
	if s == 0 {
		return ""
	}
	defer ls.release(s)
	return goStringCF(s)
}

// takeStrings returns the CFStrings of the +1 CFArray a and releases it.
func takeStrings(a uintptr) []string {
	if a == 0 {
		return nil
	}
	defer ls.release(a)
	n := ls.arrayCount(a)
	out := make([]string, 0, n)
	for i := range n {
		if s := goStringCF(ls.arrayValue(a, i)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// urlPathCF returns the file system path of the CFURL u ("" when it has
// none).
func urlPathCF(u uintptr) string {
	if u == 0 {
		return ""
	}
	buf := make([]byte, pathMax)
	if !ls.urlGetPath(u, true, &buf[0], len(buf)) {
		return ""
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf)
}

// withString calls f with a temporary CFString of s.
func withString[T any](s string, f func(uintptr) T) T {
	cf := cfString(s)
	defer ls.release(cf)
	return f(cf)
}

// LSRegisterBundle registers the application bundle at path (a folder)
// with Launch Services, updating its record when it is known already:
// LSRegisterURL(url, inUpdate true). It returns the OSStatus (0: noErr).
func LSRegisterBundle(path string) int32 {
	b := []byte(path)
	if len(b) == 0 {
		return -50 // paramErr
	}
	u := ls.urlCreateFromPath(0, &b[0], len(b), true)
	if u == 0 {
		return -50
	}
	defer ls.release(u)
	return ls.registerURL(u, true)
}

// LSApplicationPaths returns the paths of the application bundles Launch
// Services knows with the bundle identifier id
// (LSCopyApplicationURLsForBundleIdentifier), nil when it knows none.
func LSApplicationPaths(id string) []string {
	a := withString(id, func(s uintptr) uintptr { return ls.copyAppURLsForBundleID(s, 0) })
	if a == 0 {
		return nil
	}
	defer ls.release(a)
	n := ls.arrayCount(a)
	out := make([]string, 0, n)
	for i := range n {
		if p := urlPathCF(ls.arrayValue(a, i)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LSSchemeHandlers returns the bundle identifiers of the applications that
// handle the URL scheme (LSCopyAllHandlersForURLScheme).
func LSSchemeHandlers(scheme string) []string {
	return takeStrings(withString(scheme, ls.copyAllHandlersForScheme))
}

// LSDefaultSchemeHandler returns the bundle identifier of the default
// handler of the URL scheme, "" when there is none
// (LSCopyDefaultHandlerForURLScheme).
func LSDefaultSchemeHandler(scheme string) string {
	return takeString(withString(scheme, ls.copyDefaultHandlerForSch))
}

// LSSetDefaultSchemeHandler makes the application with the bundle
// identifier id the default handler of the URL scheme
// (LSSetDefaultHandlerForURLScheme); it returns the OSStatus.
func LSSetDefaultSchemeHandler(scheme, id string) int32 {
	return withString(scheme, func(s uintptr) int32 {
		return withString(id, func(b uintptr) int32 { return ls.setDefaultHandlerForScheme(s, b) })
	})
}

// LSContentTypeHandlers returns the bundle identifiers of the applications
// that handle the content type (a UTI) in any role
// (LSCopyAllRoleHandlersForContentType with kLSRolesAll).
func LSContentTypeHandlers(uti string) []string {
	return takeStrings(withString(uti, func(s uintptr) uintptr { return ls.copyAllRoleHandlers(s, LSRolesAll) }))
}

// LSDefaultContentTypeHandler returns the bundle identifier of the default
// handler of the content type in any role, "" when there is none
// (LSCopyDefaultRoleHandlerForContentType with kLSRolesAll).
func LSDefaultContentTypeHandler(uti string) string {
	return takeString(withString(uti, func(s uintptr) uintptr { return ls.copyDefaultRoleHandler(s, LSRolesAll) }))
}

// LSSetDefaultContentTypeHandler makes the application with the bundle
// identifier id the default handler of the content type in every role
// (LSSetDefaultRoleHandlerForContentType with kLSRolesAll); it returns the
// OSStatus.
func LSSetDefaultContentTypeHandler(uti, id string) int32 {
	return withString(uti, func(s uintptr) int32 {
		return withString(id, func(b uintptr) int32 { return ls.setDefaultRoleHandler(s, LSRolesAll, b) })
	})
}

// UTTypeForFilenameExtension returns the preferred Uniform Type Identifier
// of a file name extension given without its dot ("note"):
// UTTypeCreatePreferredIdentifierForTag with kUTTagClassFilenameExtension.
// A declared type ("public.plain-text", an app's exported or imported
// type) when one claims the extension, else a dynamic one ("dyn.…").
func UTTypeForFilenameExtension(ext string) string {
	return withString("public.filename-extension", func(class uintptr) string {
		return takeString(withString(ext, func(tag uintptr) uintptr { return ls.utTypeForTag(class, tag, 0) }))
	})
}
