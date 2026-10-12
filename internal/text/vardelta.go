// Copyright 2021 The go-text authors. Licensed under the BSD 3-Clause
// license (github.com/go-text/typesetting, SPDX: Unlicense OR BSD-3-Clause).
// Ported from font/opentype/tables/xvar_src.go (ItemVarStore.GetDelta,
// RegionAxisCoordinates.evaluate) with fusion-safe arithmetic; see
// THIRD_PARTY.md.

package text

import "github.com/go-text/typesetting/font/opentype/tables"

// varStoreDelta evaluates the delta at idx of an item variation store for
// the normalized coordinates coords, in the units of the varied value.
//
// go-text computes the same sum as delta += float32(d) * scalar, which Go may
// fuse into a multiply-add on arm64 (and amd64 with GOAMD64=v3), changing
// the result across architectures. Here every product is rounded explicitly
// before it is added (ADR-027), in float64.
func varStoreDelta(store *tables.ItemVarStore, idx tables.VariationStoreIndex, coords []tables.Coord) float64 {
	if store == nil || len(coords) == 0 || int(idx.DeltaSetOuter) >= len(store.ItemVariationDatas) {
		return 0
	}
	data := store.ItemVariationDatas[idx.DeltaSetOuter]
	if int(idx.DeltaSetInner) >= len(data.DeltaSets) {
		return 0
	}
	row := data.DeltaSets[idx.DeltaSetInner]
	regions := store.VariationRegionList.VariationRegions
	var delta float64
	for i, ri := range data.RegionIndexes {
		if i >= len(row) || int(ri) >= len(regions) {
			continue
		}
		s := regionScalar(regions[ri].RegionAxes, coords)
		if s == 0 {
			continue
		}
		delta += float64(float64(row[i]) * s)
	}
	return delta
}

// regionScalar is the scalar of a variation region at coords (OpenType
// "Algorithm for interpolation of instance values"). Axes the region does
// not describe, or coordinates beyond the region's axis count, count as 1.
func regionScalar(axes []tables.RegionAxisCoordinates, coords []tables.Coord) float64 {
	s := 1.0
	for i, ra := range axes {
		var coord int
		if i < len(coords) {
			coord = int(coords[i])
		}
		start, peak, end := int(ra.StartCoord), int(ra.PeakCoord), int(ra.EndCoord)
		if peak == 0 || coord == peak {
			continue
		}
		// Invalid regions are ignored, as in HarfBuzz.
		if start > peak || peak > end || (start < 0 && end > 0) {
			continue
		}
		if coord <= start || end <= coord {
			return 0
		}
		if coord < peak {
			s *= float64(coord-start) / float64(peak-start)
		} else {
			s *= float64(end-coord) / float64(end-peak)
		}
	}
	return s
}

// tupleScalar is the scalar of a gvar tuple variation at coords. peak,
// start and end are indexed by axis; start and end are nil when the tuple
// has no intermediate region.
func tupleScalar(coords, peak, start, end []tables.Coord) float64 {
	s := 1.0
	for i := range coords {
		if i >= len(peak) {
			break
		}
		v, p := int(coords[i]), int(peak[i])
		if p == 0 || v == p {
			continue
		}
		if start != nil && i < len(start) && i < len(end) {
			st, en := int(start[i]), int(end[i])
			if st > p || p > en || (st < 0 && en > 0 && p != 0) {
				continue
			}
			if v < st || v > en {
				return 0
			}
			if v < p {
				if p != st {
					s *= float64(v-st) / float64(p-st)
				}
			} else if p != en {
				s *= float64(en-v) / float64(en-p)
			}
			continue
		}
		if v == 0 || v < min(0, p) || v > max(0, p) {
			return 0
		}
		s *= float64(v) / float64(p)
	}
	return s
}
