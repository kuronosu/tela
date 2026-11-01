//go:build windows && !386 && !arm

package win32

// The IDropTarget methods that take a POINTL by value. On 64-bit Windows
// (x64 and ARM64) an 8-byte structure is passed like an integer, in one
// register: x in the low 32 bits, y in the high ones.

func pointlFrom(v uintptr) point { return point{int32(uint32(v)), int32(uint32(v >> 32))} }

// pointlArgs is the inverse of pointlFrom, for calling an IDropTarget
// (the tests call the windows' targets the way OLE does).
func pointlArgs(p point) []uintptr { return []uintptr{uintptr(uint32(p.x)) | uintptr(uint32(p.y))<<32} }

// IDropTarget::DragEnter(pDataObj, grfKeyState, pt, pdwEffect).
func dtDragEnter(this, dataObj, keyState, pt, effect uintptr) uintptr {
	return dropTargetEnter(this, dataObj, uint32(keyState), pointlFrom(pt), effect)
}

// IDropTarget::DragOver(grfKeyState, pt, pdwEffect).
func dtDragOver(this, keyState, pt, effect uintptr) uintptr {
	return dropTargetOver(this, uint32(keyState), pointlFrom(pt), effect)
}

// IDropTarget::Drop(pDataObj, grfKeyState, pt, pdwEffect).
func dtDrop(this, dataObj, keyState, pt, effect uintptr) uintptr {
	return dropTargetDrop(this, dataObj, uint32(keyState), pointlFrom(pt), effect)
}
