//go:build darwin

package darwin

import (
	"sync"

	"github.com/ebitengine/purego/objc"
)

// Foundation helpers of the desktop services: file URLs, NSError and
// dictionaries. Results are autoreleased: call them inside an autorelease
// pool.

var (
	selFileURLWithPathIsDir = objc.RegisterName("fileURLWithPath:isDirectory:")
	selPath                 = objc.RegisterName("path")
	selDomain               = objc.RegisterName("domain")
	selCode                 = objc.RegisterName("code")
	selLocalizedDescription = objc.RegisterName("localizedDescription")
	selDictionary           = objc.RegisterName("dictionary")
	selSetObjectForKey      = objc.RegisterName("setObject:forKey:")
	selNumberWithBool       = objc.RegisterName("numberWithBool:")
	selStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	selArrayWithCapacity    = objc.RegisterName("arrayWithCapacity:")
	selAddObject            = objc.RegisterName("addObject:")
	selCount                = objc.RegisterName("count")
	selObjectAtIndex        = objc.RegisterName("objectAtIndex:")
)

// foundationClasses are looked up once: the helpers are called from any
// goroutine (the secrets and the trash).
var foundationClasses = sync.OnceValue(func() (c struct{ url, dict, number, array, str Class }) {
	c.url = objc.GetClass("NSURL")
	c.dict = objc.GetClass("NSMutableDictionary")
	c.number = objc.GetClass("NSNumber")
	c.array = objc.GetClass("NSMutableArray")
	c.str = objc.GetClass("NSString")
	return c
})

// FileURL returns an autoreleased file NSURL of path, a folder when dir is
// set (fileURLWithPath:isDirectory:, which does not touch the file system
// and does not resolve symbolic links).
func FileURL(path string, dir bool) ID {
	return ID(foundationClasses().url).Send(selFileURLWithPathIsDir, AutoString(path), dir)
}

// URLPath returns the path of the file URL u ("" for nil).
func URLPath(u ID) string {
	if u == 0 {
		return ""
	}
	return GoString(u.Send(selPath))
}

// ErrorInfo returns the domain, code and localized description of the
// NSError e (zero values for nil).
func ErrorInfo(e ID) (domain string, code int, desc string) {
	if e == 0 {
		return "", 0, ""
	}
	return GoString(e.Send(selDomain)), sendInt(e, selCode), GoString(e.Send(selLocalizedDescription))
}

// NewDictionary returns an autoreleased, empty NSMutableDictionary
// (toll-free bridged with CFMutableDictionaryRef).
func NewDictionary() ID {
	return ID(foundationClasses().dict).Send(selDictionary)
}

// DictionarySet sets key to value in the NSMutableDictionary d; the
// dictionary retains both.
func DictionarySet(d, key, value ID) { d.Send(selSetObjectForKey, value, key) }

// NSBool returns the shared NSNumber of b (kCFBooleanTrue or
// kCFBooleanFalse, toll-free bridged).
func NSBool(b bool) ID {
	return ID(foundationClasses().number).Send(selNumberWithBool, b)
}

// AutoString returns an autoreleased NSString holding s (see NSString for
// invalid UTF-8 and NUL characters).
func AutoString(s string) ID {
	return ID(foundationClasses().str).Send(selStringWithUTF8String, CString(s))
}

// NewMutableArray returns an autoreleased, empty NSMutableArray.
func NewMutableArray() ID {
	return ID(foundationClasses().array).Send(selArrayWithCapacity, uint(0))
}

// ArrayAdd appends obj to the NSMutableArray a, which retains it.
func ArrayAdd(a, obj ID) { a.Send(selAddObject, obj) }

// ArrayItems returns the objects of the NSArray a, borrowed (nil for nil).
func ArrayItems(a ID) []ID {
	if a == 0 {
		return nil
	}
	n := sendInt(a, selCount)
	out := make([]ID, 0, n)
	for i := range n {
		out = append(out, a.Send(selObjectAtIndex, uint(i)))
	}
	return out
}
