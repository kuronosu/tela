package assoc

import "testing"

// GetCurrentPackageFullName's codes: success and a full name that does
// not fit mean a package; APPMODEL_ERROR_NO_PACKAGE and other errors do
// not.
func TestPackageResult(t *testing.T) {
	for _, tc := range []struct {
		r    uintptr
		want bool
	}{
		{0, true},
		{errorInsufficientBuffer, true},
		{appmodelErrorNoPackage, false},
		{87, false}, // ERROR_INVALID_PARAMETER
	} {
		if got := packageResult(tc.r); got != tc.want {
			t.Errorf("packageResult(%d) = %v, want %v", tc.r, got, tc.want)
		}
	}
	// The tests do not run from a package.
	if InPackage() {
		t.Error("InPackage is true for the test binary")
	}
}
