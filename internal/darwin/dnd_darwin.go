//go:build darwin

package darwin

// AppKit constants of drag and drop (NSDragging.h, NSDraggingSession.h).
const (
	// NSDragOperation bits: what a source allows
	// (draggingSourceOperationMask) and what a destination performs.
	// Generic is what a plain drag does (Finder: a move within a volume);
	// Every is all of them (NSUIntegerMax).
	NSDragOperationNone    = 0
	NSDragOperationCopy    = 1
	NSDragOperationLink    = 2
	NSDragOperationGeneric = 4
	NSDragOperationPrivate = 8
	NSDragOperationMove    = 16
	NSDragOperationDelete  = 32
	NSDragOperationEvery   = ^uint64(0)

	// NSDraggingContext values (the second argument of
	// draggingSession:sourceOperationMaskForDraggingContext:).
	NSDraggingContextOutsideApplication = 0
	NSDraggingContextWithinApplication  = 1

	// NSDraggingFormationNone keeps every dragging item at its own frame
	// (draggingFormation of an NSDraggingSession).
	NSDraggingFormationNone = 1
)
