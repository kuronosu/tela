// Package gputest opens the GPU devices of the machine for the tests of
// the packages above the renderer (internal/core, the examples): each test
// runs on every physical device, and under TELA_VK_VALIDATE=1 fails on
// anything the Khronos validation layer reports, an object left alive when
// its device closes included. Its code needs the build tag telagpu, on
// Linux or Windows; the tests that use it have the same constraint.
//
// The tolerance the tests compare GPU frames with the CPU rasterizer's
// under is gfxtest's (GPULevels, GPUShare, GPUHard; ADR-282).
package gputest
