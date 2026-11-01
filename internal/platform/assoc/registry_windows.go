//go:build windows

package assoc

import (
	"errors"
	"slices"

	syswin "golang.org/x/sys/windows"
	winreg "golang.org/x/sys/windows/registry"
)

func init() { nativeRegistry = func() Registry { return hkcu{} } }

// hkcu is the real HKEY_CURRENT_USER: the Registry the Windows
// associations write, and, as a Shell and MachineReader, what they notify
// and read beside it. Everything goes through advapi32 (RegOpenKeyExW,
// RegCreateKeyExW, RegQueryValueExW, RegSetValueExW, RegEnumKeyExW,
// RegEnumValueW, RegDeleteValueW, RegDeleteKeyW) by way of
// golang.org/x/sys/windows/registry, and shell32 (SHChangeNotify,
// ShellExecuteW). It opens HKEY_LOCAL_MACHINE for reading only, so it
// never needs administrator rights.
type hkcu struct{}

var (
	_ Registry      = hkcu{}
	_ Shell         = hkcu{}
	_ MachineReader = hkcu{}
)

func missing(err error) bool {
	return errors.Is(err, winreg.ErrNotExist) || errors.Is(err, syswin.ERROR_PATH_NOT_FOUND)
}

func stringIn(root winreg.Key, key, name string) (string, bool, error) {
	k, err := winreg.OpenKey(root, key, winreg.QUERY_VALUE)
	if missing(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	switch {
	case missing(err), errors.Is(err, winreg.ErrUnexpectedType):
		return "", false, nil // not there, or not a string: nothing to compare with
	case err != nil:
		return "", false, err
	}
	return v, true, nil
}

func (hkcu) String(key, name string) (string, bool, error) {
	return stringIn(winreg.CURRENT_USER, key, name)
}

func (hkcu) MachineString(key, name string) (string, bool, error) {
	return stringIn(winreg.LOCAL_MACHINE, key, name)
}

func (hkcu) SetString(key, name, value string) error {
	k, _, err := winreg.CreateKey(winreg.CURRENT_USER, key, winreg.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (hkcu) SubKeys(key string) ([]string, error) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.ENUMERATE_SUB_KEYS)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	slices.Sort(names)
	return names, err
}

func (hkcu) ValueNames(key string) ([]string, error) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	return k.ReadValueNames(-1)
}

func (hkcu) DeleteValue(key, name string) error {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.SET_VALUE)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !missing(err) {
		return err
	}
	return nil
}

// DeleteKey removes key and the keys below it, children first
// (RegDeleteKeyW deletes only a key without subkeys).
func (r hkcu) DeleteKey(key string) error {
	subs, err := r.SubKeys(key)
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := r.DeleteKey(key + `\` + s); err != nil {
			return err
		}
	}
	if err := winreg.DeleteKey(winreg.CURRENT_USER, key); err != nil && !missing(err) {
		return err
	}
	return nil
}

var procSHChangeNotify = syswin.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

const (
	shcneAssocChanged = 0x08000000
	shcnfIDList       = 0x0000
	swShowNormal      = 1
)

func (hkcu) AssociationsChanged() {
	procSHChangeNotify.Call(shcneAssocChanged, shcnfIDList, 0, 0)
}

func (hkcu) OpenSettings(uri string) error {
	verb, err := syswin.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := syswin.UTF16PtrFromString(uri)
	if err != nil {
		return err
	}
	return syswin.ShellExecute(0, verb, file, nil, nil, swShowNormal)
}
