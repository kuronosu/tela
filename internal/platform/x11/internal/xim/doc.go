// Package xim is the wire format of the X Input Method protocol (XIM,
// protocol version 1.0, X11R6 "The Input Method Protocol"), with the parts
// of its X transport and the COMPOUND_TEXT decoding a client needs. It
// knows nothing about connections: the X11 backend (package x11, ime.go)
// moves the bytes and runs the client state machine; this package encodes
// and decodes messages and is tested without a server.
//
// Byte order: the client announces little-endian in XIM_CONNECT (Tela's
// platforms are little-endian), so every message both ways is
// little-endian.
//
// Layout rules used throughout (spec section 4):
//
//   - A message is a 4-byte header (major opcode, minor opcode, CARD16
//     length of the body in 4-byte units) and a body padded to 4 bytes.
//   - STRING is a CARD16 length and the bytes, padded so that length and
//     bytes together end on 4 bytes; STR (in lists of names) is a CARD8
//     length and the bytes, unpadded, the list padded at its end.
//   - An ATTRIBUTE is an attribute ID, a CARD16 value length, the value
//     and its padding to 4 bytes; a nested list (preeditAttributes) is an
//     attribute whose value is a list of attributes.
//   - XIMATTR (attribute descriptions in XIM_OPEN_REPLY) is an ID, a type
//     and a STRING name.
//
// Messages are written from the specification, cross-checked against
// libX11's modules/im/ximcp (the reference client), xcb-imdkit (the
// fcitx5 server side) and the xim crate (xim-rs, MIT). No code is ported;
// the tests use some of the xim crate's byte vectors (THIRD_PARTY.md).
package xim
