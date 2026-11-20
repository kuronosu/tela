//go:build windows

package win32

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"unsafe"

	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces of platform contract v16 (ADR-298) on Windows (the
// data is in surfaces.go):
//
//   - Start at login: the Run value (setRunAtLogin). A packaged app
//     (MSIX) gets ErrUnsupported: its registry is virtualized, and only a
//     StartupTask of its manifest starts it at logon.
//   - Tasks: ICustomDestinationList of the app's AppUserModelID (the AUMID
//     SetAppID made the process's explicit one, toastAUMID), as
//     Chromium's JumpList and Electron's app.setUserTasks do: BeginList,
//     the Recent known category when the app registered file types
//     (Software\<ID>\Capabilities\FileAssociations has values; the shell
//     lists only files of types the app handles), AddUserTasks with an
//     IObjectCollection of IShellLinkW (Exec, the quoted Args, the
//     description, PKEY_Title through IPropertyStore, an .ico in the
//     user's cache folder as the icon), CommitList. No tasks is
//     DeleteList: the default jump list comes back.
//   - Recent documents: SHAddToRecentDocs with SHARD_APPIDINFO (the file's
//     IShellItem and the AUMID), so the file lands in the app's jump list
//     whatever the process's AUMID; clearing is
//     IApplicationDestinations.RemoveAllDestinations of the AUMID, which
//     empties the app's Recent and Frequent lists and nothing else
//     (SHAddToRecentDocs(SHARD_PIDL, NULL) would clear every app's).
//
// The COM calls run like MoveToTrash's: on a goroutine locked to a thread
// of its own, in a single-threaded apartment, so they are safe from any
// goroutine and leave no COM state on a thread of the pool.

var procSHAddToRecentDocs = shell32.NewProc("SHAddToRecentDocs")

var (
	clsidDestinationList         = guidOf("77f10cf0-3db5-4966-b520-b7c54fd35ed6")
	iidCustomDestinationList     = guidOf("6332debf-87b5-4670-90c0-5e57b408a49e")
	clsidEnumerableObjCollection = guidOf("2d3468c1-36a7-43b6-ac24-d3f02fd9607a")
	iidObjectCollection          = guidOf("5632b1a4-e38a-400a-928a-d4cd63230295")
	iidObjectArray               = guidOf("92ca9dcd-5622-4bba-a805-5e9f541bd8c9")
	clsidShellLink               = guidOf("00021401-0000-0000-c000-000000000046")
	iidShellLinkW                = guidOf("000214f9-0000-0000-c000-000000000046")
	iidPropertyStore             = guidOf("886d8eeb-8cf2-4446-8d02-cdba1dbdcf99")
	clsidApplicationDestinations = guidOf("86c14003-4d6b-4ef3-a7b4-0506663b2e68")
	iidApplicationDestinations   = guidOf("12337d35-94c6-48a0-bce7-6a9c69d4d600")

	// pkeyTitle is PKEY_Title, the label of a jump list task.
	pkeyTitle = propertyKey{fmtid: guidOf("f29f85e0-4ff9-1068-ab91-08002b27b3d9"), pid: 2}
)

const (
	// ICustomDestinationList (after IUnknown).
	vtCDLSetAppID           = 3
	vtCDLBeginList          = 4
	vtCDLAppendKnownCategor = 6
	vtCDLAddUserTasks       = 7
	vtCDLCommitList         = 8
	vtCDLDeleteList         = 10
	vtCDLAbortList          = 11
	// IObjectCollection (IObjectArray's GetCount and GetAt, then AddObject).
	vtObjCollAddObject = 5
	// IShellLinkW.
	vtSLSetDescription  = 7
	vtSLSetArguments    = 11
	vtSLSetIconLocation = 17
	vtSLSetPath         = 20
	// IPropertyStore.
	vtPSSetValue = 6
	vtPSCommit   = 7
	// IApplicationDestinations.
	vtADSetAppID              = 3
	vtADRemoveAllDestinations = 5

	kdcRecent      = 2 // KNOWNDESTCATEGORY KDC_RECENT
	vtLPWSTR       = 31
	shardAppIDInfo = 4 // SHARD_APPIDINFO
)

// propertyKey is PROPERTYKEY.
type propertyKey struct {
	fmtid guid
	pid   uint32
}

// propVariant is a PROPVARIANT holding a string (VT_LPWSTR): 16 bytes on
// 386, 24 on 64-bit systems (the union's largest member is two pointers).
type propVariant struct {
	vt       uint16
	reserved [3]uint16
	val      uintptr
	_        uintptr
}

// shardAppIDInfoArg is SHARDAPPIDINFO.
type shardAppIDInfoArg struct {
	item  uintptr // IShellItem
	appID *uint16
}

// inSTA runs f on a new goroutine locked to its own OS thread, in a
// single-threaded apartment, and returns its error (see the top of the
// file).
func inSTA(f func() error) error {
	done := make(chan error, 1)
	go func() {
		// Never unlocked: the thread ends with the goroutine.
		runtime.LockOSThread()
		r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded|coinitDisableOLE1DDE)
		if hr := int32(uint32(r)); hr < 0 {
			done <- hresultError("CoInitializeEx", hr)
			return
		}
		defer procCoUninitialize.Call()
		done <- f()
	}()
	return <-done
}

// create calls CoCreateInstance.
func create(clsid, iid *guid, what string) (comObject, error) {
	var o comObject
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&o)))
	if hr := int32(uint32(r)); hr < 0 {
		if hr == hrClassNotRegistered {
			return 0, fmt.Errorf("win32: the shell has no %s: %w", what, platform.ErrUnsupported)
		}
		return 0, hresultError("CoCreateInstance("+what+")", hr)
	}
	return o, nil
}

// --- start at login ---

func (d desktop) SetStartAtLogin(app platform.HandlerApp, on bool) error {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	if packaged() {
		return fmt.Errorf("win32: SetStartAtLogin: a packaged (MSIX) app starts at logon through a StartupTask of its manifest: %w", platform.ErrUnsupported)
	}
	_, err := setRunAtLogin(hkcuRegistry{}, app, on)
	if err != nil {
		return fmt.Errorf("win32: SetStartAtLogin: %w", err)
	}
	return nil
}

func (d desktop) StartsAtLogin(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	if packaged() {
		return false, fmt.Errorf("win32: StartsAtLogin: %w", platform.ErrUnsupported)
	}
	on, err := runsAtLogin(hkcuRegistry{}, app)
	if err != nil {
		return false, fmt.Errorf("win32: StartsAtLogin: %w", err)
	}
	return on, nil
}

// Binary returns a REG_BINARY value of HKEY_CURRENT_USER.
func (hkcuRegistry) Binary(key, name string) ([]byte, bool, error) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE)
	if regMissing(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(name)
	switch {
	case regMissing(err), errors.Is(err, winreg.ErrUnexpectedType):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	return v, true, nil
}

// DeleteValue removes a value of HKEY_CURRENT_USER; a missing one is not
// an error.
func (hkcuRegistry) DeleteValue(key, name string) error {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.SET_VALUE)
	if regMissing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !regMissing(err) {
		return err
	}
	return nil
}

// --- tasks ---

func (d desktop) SetTasks(app platform.HandlerApp, tasks []platform.Task) error {
	platform.CheckHandlerApp("SetTasks", app, true)
	platform.CheckTasks("SetTasks", tasks)
	icons, err := writeTaskIcons(app.ID, tasks)
	if err != nil {
		return err
	}
	aumid := toastAUMID(app.ID)
	names, _ := hkcuRegistry{}.ValueNames(`Software\` + app.ID + `\Capabilities\FileAssociations`)
	recent := len(names) > 0
	return inSTA(func() error { return setJumpList(aumid, app.Exec, tasks, icons, recent) })
}

// setJumpList builds and commits the app's jump list on an STA thread.
func setJumpList(aumid, exe string, tasks []platform.Task, icons []string, recent bool) error {
	list, err := create(&clsidDestinationList, &iidCustomDestinationList, "DestinationList")
	if err != nil {
		return err
	}
	defer list.release()
	id := utf16(aumid)
	if hr := list.call(vtCDLSetAppID, uintptr(unsafe.Pointer(id))); hr < 0 {
		return hresultError("ICustomDestinationList.SetAppID", hr)
	}
	if len(tasks) == 0 {
		if hr := list.call(vtCDLDeleteList, uintptr(unsafe.Pointer(id))); hr < 0 {
			return hresultError("ICustomDestinationList.DeleteList", hr)
		}
		return nil
	}
	var slots uint32
	var removed comObject
	if hr := list.call(vtCDLBeginList, uintptr(unsafe.Pointer(&slots)), uintptr(unsafe.Pointer(&iidObjectArray)), uintptr(unsafe.Pointer(&removed))); hr < 0 {
		return hresultError("ICustomDestinationList.BeginList", hr)
	}
	removed.release()
	committed := false
	defer func() {
		if !committed {
			list.call(vtCDLAbortList)
		}
	}()
	if recent {
		// Fails when the shell knows no file type of the app's; the
		// tasks still go.
		list.call(vtCDLAppendKnownCategor, kdcRecent)
	}
	coll, err := create(&clsidEnumerableObjCollection, &iidObjectCollection, "EnumerableObjectCollection")
	if err != nil {
		return err
	}
	defer coll.release()
	for i, t := range tasks {
		link, err := shellLink(exe, t, icons[i])
		if err != nil {
			return err
		}
		hr := coll.call(vtObjCollAddObject, uintptr(link))
		link.release()
		if hr < 0 {
			return hresultError("IObjectCollection.AddObject", hr)
		}
	}
	var arr comObject
	if hr := coll.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidObjectArray)), uintptr(unsafe.Pointer(&arr))); hr < 0 {
		return hresultError("IObjectCollection.QueryInterface(IObjectArray)", hr)
	}
	defer arr.release()
	if hr := list.call(vtCDLAddUserTasks, uintptr(arr)); hr < 0 {
		return hresultError("ICustomDestinationList.AddUserTasks", hr)
	}
	if hr := list.call(vtCDLCommitList); hr < 0 {
		return hresultError("ICustomDestinationList.CommitList", hr)
	}
	committed = true
	return nil
}

// shellLink returns an IShellLinkW (+1) of a task.
func shellLink(exe string, t platform.Task, icon string) (comObject, error) {
	link, err := create(&clsidShellLink, &iidShellLinkW, "ShellLink")
	if err != nil {
		return 0, err
	}
	ok := false
	defer func() {
		if !ok {
			link.release()
		}
	}()
	if hr := link.call(vtSLSetPath, uintptr(unsafe.Pointer(utf16(exe)))); hr < 0 {
		return 0, hresultError("IShellLinkW.SetPath", hr)
	}
	if hr := link.call(vtSLSetArguments, uintptr(unsafe.Pointer(utf16(quoteArgs(t.Args))))); hr < 0 {
		return 0, hresultError("IShellLinkW.SetArguments", hr)
	}
	if t.Description != "" {
		if hr := link.call(vtSLSetDescription, uintptr(unsafe.Pointer(utf16(t.Description)))); hr < 0 {
			return 0, hresultError("IShellLinkW.SetDescription", hr)
		}
	}
	if icon != "" {
		if hr := link.call(vtSLSetIconLocation, uintptr(unsafe.Pointer(utf16(icon))), 0); hr < 0 {
			return 0, hresultError("IShellLinkW.SetIconLocation", hr)
		}
	}
	var store comObject
	if hr := link.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidPropertyStore)), uintptr(unsafe.Pointer(&store))); hr < 0 {
		return 0, hresultError("IShellLinkW.QueryInterface(IPropertyStore)", hr)
	}
	defer store.release()
	title := utf16(t.Title)
	pv := &propVariant{vt: vtLPWSTR, val: uintptr(unsafe.Pointer(title))}
	hr := store.call(vtPSSetValue, uintptr(unsafe.Pointer(&pkeyTitle)), uintptr(unsafe.Pointer(pv)))
	runtime.KeepAlive(title)
	if hr < 0 {
		return 0, hresultError("IPropertyStore.SetValue(PKEY_Title)", hr)
	}
	if hr := store.call(vtPSCommit); hr < 0 {
		return 0, hresultError("IPropertyStore.Commit", hr)
	}
	ok = true
	return link, nil
}

// writeTaskIcons writes each task's icon as an .ico file in the user's
// cache folder (%LOCALAPPDATA%\tela\tasks\<ID>), which the shell reads
// when it shows the jump list, and returns the paths ("" for none).
func writeTaskIcons(id string, tasks []platform.Task) ([]string, error) {
	paths := make([]string, len(tasks))
	var dir string
	for i, t := range tasks {
		if len(t.Icon) == 0 {
			continue
		}
		if dir == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return nil, fmt.Errorf("win32: SetTasks: the cache folder for the icons: %w", err)
			}
			dir = filepath.Join(cache, "tela", "tasks", id)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("win32: SetTasks: %w", err)
			}
		}
		data, err := icoFile(t.Icon)
		if err != nil {
			return nil, fmt.Errorf("win32: SetTasks: the icon of %q: %w", t.Title, err)
		}
		p := filepath.Join(dir, strconv.Itoa(i+1)+".ico")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			return nil, fmt.Errorf("win32: SetTasks: %w", err)
		}
		paths[i] = p
	}
	return paths, nil
}

// --- recent documents ---

func (d desktop) AddRecentDocument(app platform.HandlerApp, path string) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	clean, _, err := platform.CheckRecent(path)
	if err != nil {
		return err
	}
	aumid := toastAUMID(app.ID)
	return inSTA(func() error {
		var item comObject
		r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(utf16(clean))), 0,
			uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
		if hr := int32(uint32(r)); hr < 0 {
			return hresultError("SHCreateItemFromParsingName", hr)
		}
		defer item.release()
		info := &shardAppIDInfoArg{item: uintptr(item), appID: utf16(aumid)}
		procSHAddToRecentDocs.Call(shardAppIDInfo, uintptr(unsafe.Pointer(info)))
		runtime.KeepAlive(info)
		return nil
	})
}

func (d desktop) ClearRecentDocuments(app platform.HandlerApp) error {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	aumid := toastAUMID(app.ID)
	return inSTA(func() error {
		ad, err := create(&clsidApplicationDestinations, &iidApplicationDestinations, "ApplicationDestinations")
		if err != nil {
			return err
		}
		defer ad.release()
		if hr := ad.call(vtADSetAppID, uintptr(unsafe.Pointer(utf16(aumid)))); hr < 0 {
			return hresultError("IApplicationDestinations.SetAppID", hr)
		}
		if hr := ad.call(vtADRemoveAllDestinations); hr < 0 {
			return hresultError("IApplicationDestinations.RemoveAllDestinations", hr)
		}
		return nil
	})
}
