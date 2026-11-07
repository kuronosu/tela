//go:build darwin

package cocoa

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// File dialogs (NSOpenPanel, NSSavePanel) and message boxes (NSAlert);
// dialog.go has the pure mapping of filters, buttons and kinds.
//
//   - With a parent window that is on screen, the dialog is a sheet on it
//     (beginSheetModalForWindow:completionHandler:, ADR-224): modal to that
//     window only, and the rest of the app keeps working.
//   - Without one, a panel is a separate, modeless window
//     (beginWithCompletionHandler:, as Electron and Chromium show detached
//     panels): no modal loop runs, so the shared loop goes on delivering
//     events as usual. An alert has no modeless form in AppKit
//     (-[NSAlert runModal] is app-modal: AppKit blocks input to every
//     other window of the app), so a detached alert is shown modeless by
//     hand (ADR-237), as a free-standing window: [NSAlert layout] builds
//     its panel, every button's target and action are retargeted to the
//     request's TelaPanelHelper (alertButton:, which finds the button's
//     index in NSAlert.buttons: the response NSAlertFirstButtonReturn +
//     index that runModal would return), and the panel is centered,
//     kept when the app deactivates (an NSPanel hides by default), and
//     made key and front with the app activated. No modal session runs:
//     the app's other windows keep their input, as with the drawn box on
//     Linux and a box of another window on Win32. Return and Escape work
//     through the buttons' key equivalents while the panel is key; the
//     panel has no close button. Several detached alerts can be open at
//     once.
//   - The completion handler runs on the main thread; it reads the result
//     and posts done to the loop, so done runs once, later, from a pass.
//     Cancel is nil paths (or "") and a nil error; a closed alert (its
//     sheet ended without a button) answers the set's DismissAnswer.
//   - When Run returns, open dialogs are ended and their done is dropped.

// dialogKind is what a dialogRequest shows.
type dialogKind uint8

const (
	dialogOpenFiles dialogKind = iota
	dialogSaveFile
	dialogPickFolder
	dialogMessageBox
)

// dialogRequest is an open panel or alert.
type dialogRequest struct {
	b      *Backend
	kind   dialogKind
	obj    darwin.ID // the NSOpenPanel, NSSavePanel or NSAlert (owned: +1)
	sheet  *Window   // the window the sheet is on; nil when detached
	helper darwin.ID // TelaPanelHelper: popup target, delegate, observer of the parent (owned), or 0
	// observing: the helper observes the sheet parent's
	// NSWindowWillCloseNotification.
	observing bool
	popup     darwin.ID // the format popup (owned by the accessory view), or 0

	filters  []panelFilter
	active   int  // the selected filter
	delegate bool // the helper is the panel's delegate (a filter has globs)

	ended    bool               // answered or abandoned
	detached bool               // a message box shown as a free-standing panel (no sheet, no modal session)
	finish   func(response int) // reads the result and posts done; main thread
}

// psel holds the selectors of the panels and alerts.
var psel = struct {
	openPanel, savePanel, setTitle, setMessage, setDirectoryURL                        darwin.SEL
	setCanChooseFiles, setCanChooseDirectories, setAllowsMultipleSelection             darwin.SEL
	setCanCreateDirectories, setResolvesAliases, setNameFieldStringValue               darwin.SEL
	setAllowsOtherFileTypes, setAllowedContentTypes, setAllowedFileTypes               darwin.SEL
	typeWithFilenameExtension, setDelegate, validateVisibleColumns, url, urls          darwin.SEL
	beginSheet, begin, cancel, endSheetReturnCode, isVisible, window                   darwin.SEL
	setMessageText, setInformativeText, setAlertStyle, addButtonWithTitle              darwin.SEL
	setKeyEquivalent, labelWithString, initWithFramePullsDown, menu, addItemWithTitle  darwin.SEL
	selectItemAtIndex, indexOfSelectedItem, setTarget, setAction, sizeToFit, frame     darwin.SEL
	setFrameOrigin, addSubview, setAccessoryView, setAccessoryViewDisclosed, initFrame darwin.SEL
	layout, buttons, setHidesOnDeactivate, center, makeKeyAndOrderFront, orderOut      darwin.SEL
}{
	openPanel:                  darwin.Sel("openPanel"),
	savePanel:                  darwin.Sel("savePanel"),
	setTitle:                   darwin.Sel("setTitle:"),
	setMessage:                 darwin.Sel("setMessage:"),
	setDirectoryURL:            darwin.Sel("setDirectoryURL:"),
	setCanChooseFiles:          darwin.Sel("setCanChooseFiles:"),
	setCanChooseDirectories:    darwin.Sel("setCanChooseDirectories:"),
	setAllowsMultipleSelection: darwin.Sel("setAllowsMultipleSelection:"),
	setCanCreateDirectories:    darwin.Sel("setCanCreateDirectories:"),
	setResolvesAliases:         darwin.Sel("setResolvesAliases:"),
	setNameFieldStringValue:    darwin.Sel("setNameFieldStringValue:"),
	setAllowsOtherFileTypes:    darwin.Sel("setAllowsOtherFileTypes:"),
	setAllowedContentTypes:     darwin.Sel("setAllowedContentTypes:"),
	setAllowedFileTypes:        darwin.Sel("setAllowedFileTypes:"),
	typeWithFilenameExtension:  darwin.Sel("typeWithFilenameExtension:"),
	setDelegate:                darwin.Sel("setDelegate:"),
	validateVisibleColumns:     darwin.Sel("validateVisibleColumns"),
	url:                        darwin.Sel("URL"),
	urls:                       darwin.Sel("URLs"),
	beginSheet:                 darwin.Sel("beginSheetModalForWindow:completionHandler:"),
	begin:                      darwin.Sel("beginWithCompletionHandler:"),
	cancel:                     darwin.Sel("cancel:"),
	endSheetReturnCode:         darwin.Sel("endSheet:returnCode:"),
	isVisible:                  darwin.Sel("isVisible"),
	window:                     darwin.Sel("window"),
	setMessageText:             darwin.Sel("setMessageText:"),
	setInformativeText:         darwin.Sel("setInformativeText:"),
	setAlertStyle:              darwin.Sel("setAlertStyle:"),
	addButtonWithTitle:         darwin.Sel("addButtonWithTitle:"),
	setKeyEquivalent:           darwin.Sel("setKeyEquivalent:"),
	labelWithString:            darwin.Sel("labelWithString:"),
	initWithFramePullsDown:     darwin.Sel("initWithFrame:pullsDown:"),
	menu:                       darwin.Sel("menu"),
	addItemWithTitle:           darwin.Sel("addItemWithTitle:action:keyEquivalent:"),
	selectItemAtIndex:          darwin.Sel("selectItemAtIndex:"),
	indexOfSelectedItem:        darwin.Sel("indexOfSelectedItem"),
	setTarget:                  darwin.Sel("setTarget:"),
	setAction:                  darwin.Sel("setAction:"),
	sizeToFit:                  darwin.Sel("sizeToFit"),
	frame:                      darwin.Sel("frame"),
	setFrameOrigin:             darwin.Sel("setFrameOrigin:"),
	addSubview:                 darwin.Sel("addSubview:"),
	setAccessoryView:           darwin.Sel("setAccessoryView:"),
	setAccessoryViewDisclosed:  darwin.Sel("setAccessoryViewDisclosed:"),
	initFrame:                  darwin.Sel("initWithFrame:"),
	layout:                     darwin.Sel("layout"),
	buttons:                    darwin.Sel("buttons"),
	setHidesOnDeactivate:       darwin.Sel("setHidesOnDeactivate:"),
	center:                     darwin.Sel("center"),
	makeKeyAndOrderFront:       darwin.Sel("makeKeyAndOrderFront:"),
	orderOut:                   darwin.Sel("orderOut:"),
}

// OpenFiles shows an NSOpenPanel for files.
func (d desktop) OpenFiles(opts platform.FileDialogOptions, done func([]string, error)) error {
	if done == nil {
		panic("cocoa: OpenFiles called with a nil callback; pass a func(paths []string, err error)")
	}
	platform.CheckFileDialogOptions("OpenFiles", opts)
	return d.b.fileDialog(dialogOpenFiles, opts, done)
}

// SaveFile shows an NSSavePanel, which asks before replacing a file.
func (d desktop) SaveFile(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("cocoa: SaveFile called with a nil callback; pass a func(path string, err error)")
	}
	platform.CheckFileDialogOptions("SaveFile", opts)
	return d.b.fileDialog(dialogSaveFile, opts, func(paths []string, err error) { done(firstPath(paths), err) })
}

// PickFolder shows an NSOpenPanel for one folder (with New Folder).
func (d desktop) PickFolder(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("cocoa: PickFolder called with a nil callback; pass a func(path string, err error)")
	}
	platform.CheckFileDialogOptions("PickFolder", opts)
	return d.b.fileDialog(dialogPickFolder, opts, func(paths []string, err error) { done(firstPath(paths), err) })
}

func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// fileDialog creates, configures and begins a panel.
func (b *Backend) fileDialog(kind dialogKind, opts platform.FileDialogOptions, done func([]string, error)) error {
	if b.closed {
		return errClosed
	}
	if err := setupDesktop(); err != nil {
		return errNoHelpers("file dialog", err)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	var panel darwin.ID
	if kind == dialogSaveFile {
		panel = darwin.SendClass(darwin.GetClass("NSSavePanel"), psel.savePanel)
	} else {
		panel = darwin.SendClass(darwin.GetClass("NSOpenPanel"), psel.openPanel)
	}
	if panel == 0 {
		return fmt.Errorf("cocoa: creating the file panel failed: %w", platform.ErrUnsupported)
	}
	r := &dialogRequest{b: b, kind: kind, obj: darwin.Retain(panel)}
	if opts.Title != "" {
		// Panels have no visible title since macOS 10.11; the message is
		// shown at their top, so the title goes there too.
		darwin.Send(panel, psel.setTitle, darwin.AutoString(opts.Title))
		darwin.Send(panel, psel.setMessage, darwin.AutoString(opts.Title))
	}
	if opts.Directory != "" {
		if dir, err := filepath.Abs(opts.Directory); err == nil {
			darwin.Send(panel, psel.setDirectoryURL, darwin.FileURL(dir, true))
		}
	}
	switch kind {
	case dialogOpenFiles:
		darwin.Send(panel, psel.setCanChooseFiles, true)
		darwin.Send(panel, psel.setCanChooseDirectories, false)
		darwin.Send(panel, psel.setAllowsMultipleSelection, opts.Multiple)
		darwin.Send(panel, psel.setResolvesAliases, true)
	case dialogPickFolder:
		darwin.Send(panel, psel.setCanChooseFiles, false)
		darwin.Send(panel, psel.setCanChooseDirectories, true)
		darwin.Send(panel, psel.setAllowsMultipleSelection, false)
		darwin.Send(panel, psel.setCanCreateDirectories, true)
	case dialogSaveFile:
		darwin.Send(panel, psel.setCanCreateDirectories, true)
		// A typed extension other than the selected type's is kept (the
		// panel asks which one to use), like Win32's dialog.
		darwin.Send(panel, psel.setAllowsOtherFileTypes, true)
		if opts.Name != "" {
			darwin.Send(panel, psel.setNameFieldStringValue, darwin.AutoString(opts.Name))
		}
	}
	if kind != dialogPickFolder && len(opts.Filters) > 0 {
		r.filters = panelFilters(opts.Filters)
		r.newHelper()
		if kind == dialogOpenFiles {
			for _, f := range r.filters {
				if _, del := f.openTypes(); del {
					r.delegate = true
				}
			}
			if r.delegate {
				darwin.Send(panel, psel.setDelegate, r.helper)
			}
		}
		if len(r.filters) > 1 {
			r.addPopup()
		}
		r.applyFilter(0)
	}
	r.finish = func(response int) {
		var paths []string
		if response == darwin.NSModalResponseOK {
			if kind == dialogSaveFile {
				if p := darwin.URLPath(darwin.Send(r.obj, psel.url)); p != "" {
					paths = []string{p}
				}
			} else {
				for _, u := range darwin.ArrayItems(darwin.Send(r.obj, psel.urls)) {
					if p := darwin.URLPath(u); p != "" {
						paths = append(paths, p)
					}
				}
			}
		}
		b.answer(func() { done(paths, nil) })
	}
	r.begin(opts.Parent)
	return nil
}

// addPopup gives the panel a "Format:" popup of the filters in its
// accessory view, its target the helper.
func (r *dialogRequest) addPopup() {
	const margin, gap = 20.0, 8.0
	label := darwin.SendClass(darwin.GetClass("NSTextField"), psel.labelWithString, darwin.AutoString("Format:"))
	popup := darwin.Send(darwin.SendClass(darwin.GetClass("NSPopUpButton"), sel.alloc), psel.initWithFramePullsDown, darwin.CGRect{}, false)
	if label == 0 || popup == 0 {
		darwin.Release(popup)
		return
	}
	// The menu's own method keeps items of the same title, which
	// addItemWithTitle: of the popup would merge.
	menu := darwin.Send(popup, psel.menu)
	for _, f := range r.filters {
		darwin.Send(menu, psel.addItemWithTitle, darwin.AutoString(f.label), darwin.SEL(0), darwin.AutoString(""))
	}
	darwin.Send(popup, psel.selectItemAtIndex, 0)
	darwin.Send(popup, psel.setTarget, r.helper)
	darwin.Send(popup, psel.setAction, deskClasses.selFilterChanged)
	darwin.Send(label, psel.sizeToFit)
	darwin.Send(popup, psel.sizeToFit)
	lf, pf := darwin.SendRect(label, psel.frame), darwin.SendRect(popup, psel.frame)
	h := max(lf.Size.Height, pf.Size.Height) + 2*gap
	w := margin + lf.Size.Width + gap + pf.Size.Width + margin
	view := darwin.Send(darwin.SendClass(app.nsView, sel.alloc), psel.initFrame, darwin.CGRect{Size: darwin.CGSize{Width: w, Height: h}})
	darwin.Send(label, psel.setFrameOrigin, darwin.CGPoint{X: margin, Y: (h - lf.Size.Height) / 2})
	darwin.Send(popup, psel.setFrameOrigin, darwin.CGPoint{X: margin + lf.Size.Width + gap, Y: (h - pf.Size.Height) / 2})
	darwin.Send(view, psel.addSubview, label)
	darwin.Send(view, psel.addSubview, popup)
	darwin.Release(popup) // the view keeps it
	darwin.Send(r.obj, psel.setAccessoryView, view)
	darwin.Release(view) // the panel keeps it
	if r.kind == dialogOpenFiles {
		darwin.Send(r.obj, psel.setAccessoryViewDisclosed, true)
	}
	r.popup = popup
}

// applyFilter makes filter i the panel's: its content types, and the
// delegate's filter for an open panel.
func (r *dialogRequest) applyFilter(i int) {
	if i < 0 || i >= len(r.filters) {
		return
	}
	r.active = i
	f := r.filters[i]
	exts := f.saveTypes()
	if r.kind != dialogSaveFile {
		exts, _ = f.openTypes()
	}
	setPanelTypes(r.obj, exts)
	if r.delegate {
		darwin.Send(r.obj, psel.validateVisibleColumns)
	}
}

// setPanelTypes sets a panel's allowed content types to the UTTypes of the
// extensions (none: every file), or its allowed file types where UTType
// is missing.
func setPanelTypes(panel darwin.ID, exts []string) {
	if darwin.LoadUniformTypeIdentifiers() {
		types := darwin.NewMutableArray()
		utType := darwin.GetClass("UTType")
		for _, ext := range exts {
			if t := darwin.SendClass(utType, psel.typeWithFilenameExtension, darwin.AutoString(ext)); t != 0 {
				darwin.ArrayAdd(types, t)
			}
		}
		darwin.Send(panel, psel.setAllowedContentTypes, types)
		return
	}
	var list darwin.ID // nil: every file
	if len(exts) > 0 {
		list = darwin.NewMutableArray()
		for _, ext := range exts {
			darwin.ArrayAdd(list, darwin.AutoString(ext))
		}
	}
	darwin.Send(panel, psel.setAllowedFileTypes, list)
}

// begin shows the dialog: a sheet on the parent when it is on screen, else
// a separate panel; an alert without a sheet is a free-standing panel.
func (r *dialogRequest) begin(parent platform.Window) {
	b := r.b
	b.desk.dialogs = append(b.desk.dialogs, r)
	w := b.sheetParent(parent)
	if w == nil && r.kind == dialogMessageBox {
		r.showDetached()
		return
	}
	handler := darwin.NewBlock(func(_ darwin.Block, response int) {
		b.guard(func() { r.end(response) })
	})
	defer handler.Release()
	if w != nil {
		r.sheet = w
		// A window closed by the app (Window.Close) with a sheet on it
		// would never end the sheet: its close notification does.
		r.newHelper()
		darwin.Send(notificationCenter(), setSel.addObserverSelector, r.helper, deskClasses.selParentWillClose,
			appKitName("NSWindowWillCloseNotification"), w.ns)
		r.observing = true
		darwin.Send(r.obj, psel.beginSheet, w.ns, handler)
	} else {
		darwin.Send(r.obj, psel.begin, handler)
	}
}

// showDetached shows an alert without a sheet as a modeless panel: its
// buttons answer through the helper's alertButton: instead of runModal's
// modal session.
func (r *dialogRequest) showDetached() {
	r.detached = true
	r.newHelper()
	darwin.Send(r.obj, psel.layout)
	for _, btn := range darwin.ArrayItems(darwin.Send(r.obj, psel.buttons)) {
		darwin.Send(btn, psel.setTarget, r.helper)
		darwin.Send(btn, psel.setAction, deskClasses.selAlertButton)
	}
	win := r.window()
	darwin.Send(win, psel.setHidesOnDeactivate, false)
	darwin.Send(win, psel.center)
	activateApp()
	darwin.Send(win, psel.makeKeyAndOrderFront, darwin.ID(0))
}

// alertButtonPressed is the action of a detached alert's buttons: the
// answer of the button's position, as runModal would return it.
func alertButtonPressed(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	r := panelRequest(self)
	if r == nil || r.ended || !r.detached {
		return
	}
	r.b.guard(func() {
		i := slices.Index(darwin.ArrayItems(darwin.Send(r.obj, psel.buttons)), sender)
		if i < 0 {
			return
		}
		darwin.Send(r.window(), psel.orderOut, darwin.ID(0))
		r.end(alertButtonResponse(i))
	})
}

// newHelper gives the request its TelaPanelHelper, once.
func (r *dialogRequest) newHelper() {
	if r.helper != 0 {
		return
	}
	r.helper = darwin.Send(darwin.SendClass(deskClasses.panelHelper, sel.alloc), sel.init)
	deskObjects.Lock()
	deskObjects.panels[r.helper] = r
	deskObjects.Unlock()
}

func notificationCenter() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSNotificationCenter"), setSel.defaultCenter)
}

// sheetParent returns the window a dialog with parent p is a sheet on: p
// when it is a visible window of the backend (a sheet needs a window on
// screen, not a hidden or minimized one), else nil.
func (b *Backend) sheetParent(p platform.Window) *Window {
	w := b.parentOf(p)
	if w == nil || !darwin.SendBool(w.ns, psel.isVisible) {
		return nil
	}
	return w
}

// end answers the dialog once: called by its completion handler, or by a
// detached alert's button.
func (r *dialogRequest) end(response int) {
	if r.ended {
		return
	}
	r.ended = true
	r.finish(response)
	r.release()
}

// abandon ends the dialog without an answer (Run returns): a sheet is
// ended, a panel canceled; their completion handlers then do nothing.
func (r *dialogRequest) abandon() {
	if r.ended {
		return
	}
	r.ended = true
	switch {
	case r.sheet != nil:
		darwin.Send(r.sheet.ns, psel.endSheetReturnCode, r.window(), darwin.NSModalResponseCancel)
	case r.detached:
		darwin.Send(r.window(), psel.orderOut, darwin.ID(0))
	case r.kind != dialogMessageBox:
		darwin.Send(r.obj, psel.cancel, darwin.ID(0))
	}
	r.release()
}

// window returns the dialog's window: the panel itself, an alert's
// window.
func (r *dialogRequest) window() darwin.ID {
	if r.kind == dialogMessageBox {
		return darwin.Send(r.obj, psel.window)
	}
	return r.obj
}

// release forgets the request and releases its objects at the next pass
// (the completion handler that ended it is a method of the panel).
func (r *dialogRequest) release() {
	b := r.b
	b.desk.dialogs = slices.DeleteFunc(b.desk.dialogs, func(o *dialogRequest) bool { return o == r })
	if r.helper != 0 {
		deskObjects.Lock()
		delete(deskObjects.panels, r.helper)
		deskObjects.Unlock()
		if r.observing {
			darwin.Send(notificationCenter(), setSel.removeObserver, r.helper)
		}
		if r.delegate {
			darwin.Send(r.obj, psel.setDelegate, darwin.ID(0))
		}
		if r.popup != 0 {
			darwin.Send(r.popup, psel.setTarget, darwin.ID(0))
		}
		if r.detached {
			for _, btn := range darwin.ArrayItems(darwin.Send(r.obj, psel.buttons)) {
				darwin.Send(btn, psel.setTarget, darwin.ID(0))
			}
		}
		b.releaseLater(r.helper)
	}
	b.releaseLater(r.obj)
}

// panelRequest returns the request of a TelaPanelHelper.
func panelRequest(helper darwin.ID) *dialogRequest {
	deskObjects.Lock()
	defer deskObjects.Unlock()
	return deskObjects.panels[helper]
}

// panelFilterChanged is the action of the format popup.
func panelFilterChanged(self darwin.ID, _ darwin.SEL, sender darwin.ID) {
	r := panelRequest(self)
	if r == nil || r.ended {
		return
	}
	r.b.guard(func() { r.applyFilter(darwin.SendInt(sender, psel.indexOfSelectedItem)) })
}

// panelParentWillClose observes the close of a sheet's parent: the sheet
// ends as canceled (a file dialog answers nil, a message box its
// DismissAnswer).
func panelParentWillClose(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
	r := panelRequest(self)
	if r == nil || r.ended || r.sheet == nil {
		return
	}
	r.b.guard(func() {
		w, win := r.sheet, r.window()
		r.end(darwin.NSModalResponseCancel)
		darwin.Send(w.ns, psel.endSheetReturnCode, win, darwin.NSModalResponseCancel)
	})
}

// panelShouldEnableURL is the open panel's delegate method: folders are
// enabled (the user browses them), files when they match the selected
// filter's patterns.
func panelShouldEnableURL(self darwin.ID, _ darwin.SEL, _, url darwin.ID) bool {
	r := panelRequest(self)
	if r == nil || r.ended || r.active >= len(r.filters) {
		return true
	}
	enable := true
	r.b.guard(func() {
		f := r.filters[r.active]
		if _, del := f.openTypes(); !del {
			return
		}
		p := darwin.URLPath(url)
		if fi, err := os.Stat(p); p == "" || (err == nil && fi.IsDir()) {
			return
		}
		enable = f.match(filepath.Base(p))
	})
	return enable
}

// --- message boxes ---

// MessageBox shows an NSAlert: a sheet on opts.Parent, or a modeless
// free-standing panel.
func (d desktop) MessageBox(opts platform.MessageBoxOptions, done func(platform.MessageAnswer, error)) error {
	if done == nil {
		panic("cocoa: MessageBox called with a nil callback; pass a func(answer platform.MessageAnswer, err error)")
	}
	platform.CheckMessageBoxOptions(opts)
	b := d.b
	if b.closed {
		return errClosed
	}
	if err := setupDesktop(); err != nil {
		return errNoHelpers("MessageBox", err)
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	alert := darwin.Send(darwin.SendClass(darwin.GetClass("NSAlert"), sel.alloc), sel.init)
	if alert == 0 {
		return fmt.Errorf("cocoa: creating the NSAlert failed: %w", platform.ErrUnsupported)
	}
	message, informative := alertText(opts.Title, opts.Text)
	darwin.Send(alert, psel.setMessageText, darwin.AutoString(message))
	darwin.Send(alert, psel.setInformativeText, darwin.AutoString(informative))
	darwin.Send(alert, psel.setAlertStyle, uint(alertStyle(opts.Kind)))
	for _, bt := range alertButtons(opts.Buttons) {
		btn := darwin.Send(alert, psel.addButtonWithTitle, darwin.AutoString(bt.title))
		if bt.escape && btn != 0 {
			darwin.Send(btn, psel.setKeyEquivalent, darwin.AutoString("\x1b"))
		}
	}
	r := &dialogRequest{b: b, kind: dialogMessageBox, obj: alert}
	r.finish = func(response int) {
		a := alertAnswer(opts.Buttons, response)
		b.answer(func() { done(a, nil) })
	}
	r.begin(opts.Parent)
	return nil
}
