package assoc

// The return codes of GetCurrentPackageFullName (winerror.h) that
// packageResult reads.
const (
	errorInsufficientBuffer = 122
	appmodelErrorNoPackage  = 15700
)

// packageResult reports whether GetCurrentPackageFullName's return code,
// asked with a zero-length buffer, means a packaged process: 0 (the call
// succeeded, which a zero-length buffer should not allow, but the name is
// there) or ERROR_INSUFFICIENT_BUFFER (the full name does not fit). Any
// other code, APPMODEL_ERROR_NO_PACKAGE first, means no package.
func packageResult(r uintptr) bool { return r == 0 || r == errorInsufficientBuffer }
