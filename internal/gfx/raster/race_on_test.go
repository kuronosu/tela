//go:build race

package raster

// raceOn reports that the tests run under the race detector, which makes
// rendering about ten times slower.
const raceOn = true
