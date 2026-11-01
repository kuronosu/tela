//go:build windows && (386 || arm)

package win32

// The IDropTarget methods that take a POINTL by value. On 32-bit Windows
// the structure takes two stack slots, x then y, and stdcall callbacks
// must declare every slot (the callee pops them).

// pointlArgs returns the slots of a POINTL argument, for calling an
// IDropTarget (the tests call the windows' targets the way OLE does).
func pointlArgs(p point) []uintptr { return []uintptr{uintptr(p.x), uintptr(p.y)} }

// IDropTarget::DragEnter(pDataObj, grfKeyState, pt, pdwEffect).
func dtDragEnter(this, dataObj, keyState, x, y, effect uintptr) uintptr {
	return dropTargetEnter(this, dataObj, uint32(keyState), point{int32(x), int32(y)}, effect)
}

// IDropTarget::DragOver(grfKeyState, pt, pdwEffect).
func dtDragOver(this, keyState, x, y, effect uintptr) uintptr {
	return dropTargetOver(this, uint32(keyState), point{int32(x), int32(y)}, effect)
}

// IDropTarget::Drop(pDataObj, grfKeyState, pt, pdwEffect).
func dtDrop(this, dataObj, keyState, x, y, effect uintptr) uintptr {
	return dropTargetDrop(this, dataObj, uint32(keyState), point{int32(x), int32(y)}, effect)
}
