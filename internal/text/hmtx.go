// Copyright 2021 The go-text authors. Licensed under the BSD 3-Clause
// license (github.com/go-text/typesetting, SPDX: Unlicense OR BSD-3-Clause).
// The reading of the table follows font/opentype/tables (ParseHmtx and
// Hmtx's Advance and SideBearing: the last long metric's advance for the
// glyphs past it, and the results of short tables), on the raw table
// instead of parsed arrays; see THIRD_PARTY.md.

package text

import (
	"encoding/binary"

	"github.com/go-text/typesetting/font/opentype/tables"
)

// hmtx is the raw hmtx table, read like tables.Hmtx (which the eager parse
// kept as two arrays of every glyph's metrics).
type hmtx struct {
	data            []byte
	nMetrics, nLSBs int // long metrics, then left side bearings only
}

// newHmtx returns the hmtx table src with nMetrics long metrics followed by
// nLSBs side bearings. Like tables.ParseHmtx, whose result the eager parse
// kept even on an error, a table too short for the long metrics is empty
// and one too short for the side bearings has only the long metrics.
func newHmtx(src []byte, nMetrics, nLSBs int) hmtx {
	switch {
	case len(src) < 4*nMetrics:
		return hmtx{}
	case len(src) < 4*nMetrics+2*nLSBs:
		nLSBs = 0
	}
	return hmtx{data: src, nMetrics: nMetrics, nLSBs: nLSBs}
}

// IsEmpty reports whether the font has no horizontal metrics.
func (h hmtx) IsEmpty() bool { return h.nMetrics+h.nLSBs == 0 }

// Advance returns the advance width of gid: a glyph past the long metrics
// has the last one's, and an invalid glyph 0.
func (h hmtx) Advance(gid tables.GlyphID) uint16 {
	i := int(gid)
	switch {
	case i < h.nMetrics:
		return binary.BigEndian.Uint16(h.data[4*i:])
	case i < h.nMetrics+h.nLSBs && h.nMetrics > 0:
		return binary.BigEndian.Uint16(h.data[4*(h.nMetrics-1):])
	}
	return 0
}

// SideBearing returns the left side bearing of gid, 0 for an invalid glyph.
func (h hmtx) SideBearing(gid tables.GlyphID) int16 {
	i := int(gid)
	switch {
	case i < h.nMetrics:
		return int16(binary.BigEndian.Uint16(h.data[4*i+2:]))
	case i < h.nMetrics+h.nLSBs:
		return int16(binary.BigEndian.Uint16(h.data[4*h.nMetrics+2*(i-h.nMetrics):]))
	}
	return 0
}
