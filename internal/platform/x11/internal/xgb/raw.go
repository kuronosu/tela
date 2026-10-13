// Tela patch (ADR-025): types the patched reader in xgb.go uses for the
// events and errors it has no generated decoder for. This file is Tela's
// own; the rest of the package is vendored from github.com/jezek/xgb
// v1.3.1 (see LICENSE and THIRD_PARTY.md).

package xgb

// GenericEventCode is the event code of an X GenericEvent (the Generic
// Event Extension, used by XInput2 and Present). Its 32-byte header is
// followed by Get32(header[4:]) more 4-byte units.
const GenericEventCode = 35

// RawEvent is an event as read from the wire, delivered by WaitForEvent
// when the reader has no decoder for it: a GenericEvent with all its extra
// bytes (byte 1 is the extension's major opcode, bytes 8-9 its event
// type), or an event of an extension without a Go package, such as XKB
// (32 bytes; byte 1 is the XKB event type).
type RawEvent []byte

// Code returns the event code, without the SendEvent bit.
func (e RawEvent) Code() byte { return e[0] & 127 }

// Bytes returns the event's bytes.
func (e RawEvent) Bytes() []byte { return e }

func (e RawEvent) String() string {
	if e.Code() == GenericEventCode && len(e) >= 10 {
		return Sprintf("GenericEvent {Extension: %d, EventType: %d, Length: %d}", e[1], Get16(e[8:]), len(e))
	}
	return Sprintf("RawEvent {Code: %d, Detail: %d}", e.Code(), e[1])
}

// RawError is an X error the reader has no constructor for (an error of an
// extension without a Go package, such as XInput2's BadDevice). It is
// matched to its request like any other error.
type RawError []byte

func newRawError(buf []byte) Error { return RawError(buf) }

// Code returns the error code.
func (e RawError) Code() byte { return e[1] }

// SequenceId returns the sequence number of the failed request.
func (e RawError) SequenceId() uint16 { return Get16(e[2:]) }

// BadId returns the resource id or value the error is about.
func (e RawError) BadId() uint32 { return Get32(e[4:]) }

// MajorOpcode returns the major opcode of the failed request.
func (e RawError) MajorOpcode() byte { return e[10] }

// MinorOpcode returns the minor opcode of the failed request.
func (e RawError) MinorOpcode() uint16 { return Get16(e[8:]) }

func (e RawError) Error() string {
	return Sprintf("X error %d (request %d.%d, sequence %d, value %#x)",
		e.Code(), e.MajorOpcode(), e.MinorOpcode(), e.SequenceId(), e.BadId())
}
