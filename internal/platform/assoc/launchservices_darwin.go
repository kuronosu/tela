//go:build darwin

package assoc

import (
	"fmt"

	"github.com/kuronosu/tela/internal/darwin"
)

func init() { nativeLaunchServices = func() LaunchServices { return realLaunchServices{} } }

// realLaunchServices is the LaunchServices of the system, through
// internal/darwin (ADR-222: purego stays there). Every call is safe from
// any goroutine and needs no running app.
type realLaunchServices struct{}

func lsLoad() error {
	if err := darwin.LoadLaunchServices(); err != nil {
		return fmt.Errorf("the Launch Services framework is unavailable: %w", err)
	}
	return nil
}

// osStatus turns an OSStatus into an error.
func osStatus(what string, st int32) error {
	if st == 0 {
		return nil
	}
	return fmt.Errorf("%s failed with OSStatus %d", what, st)
}

func (realLaunchServices) RegisterBundle(path string) error {
	if err := lsLoad(); err != nil {
		return err
	}
	return osStatus("LSRegisterURL", darwin.LSRegisterBundle(path))
}

func (realLaunchServices) BundlePaths(id string) ([]string, error) {
	if err := lsLoad(); err != nil {
		return nil, err
	}
	return darwin.LSApplicationPaths(id), nil
}

func (realLaunchServices) SchemeHandlers(scheme string) ([]string, error) {
	if err := lsLoad(); err != nil {
		return nil, err
	}
	return darwin.LSSchemeHandlers(scheme), nil
}

func (realLaunchServices) DefaultSchemeHandler(scheme string) (string, error) {
	if err := lsLoad(); err != nil {
		return "", err
	}
	return darwin.LSDefaultSchemeHandler(scheme), nil
}

func (realLaunchServices) SetDefaultSchemeHandler(scheme, id string) error {
	if err := lsLoad(); err != nil {
		return err
	}
	return osStatus("LSSetDefaultHandlerForURLScheme", darwin.LSSetDefaultSchemeHandler(scheme, id))
}

func (realLaunchServices) TypeHandlers(uti string) ([]string, error) {
	if err := lsLoad(); err != nil {
		return nil, err
	}
	return darwin.LSContentTypeHandlers(uti), nil
}

func (realLaunchServices) DefaultTypeHandler(uti string) (string, error) {
	if err := lsLoad(); err != nil {
		return "", err
	}
	return darwin.LSDefaultContentTypeHandler(uti), nil
}

func (realLaunchServices) SetDefaultTypeHandler(uti, id string) error {
	if err := lsLoad(); err != nil {
		return err
	}
	return osStatus("LSSetDefaultRoleHandlerForContentType", darwin.LSSetDefaultContentTypeHandler(uti, id))
}

func (realLaunchServices) TypeOfExtension(ext string) (string, error) {
	if err := lsLoad(); err != nil {
		return "", err
	}
	return darwin.UTTypeForFilenameExtension(ext), nil
}
