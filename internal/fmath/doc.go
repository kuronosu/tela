// Package fmath holds the elementary functions that every layer feeding the
// rasterizer needs with the same result, bit for bit, on every architecture
// (ADR-027, ADR-041).
//
// The Go specification allows a compiler to fuse x*y + z into one fused
// multiply-add, which skips the rounding of the product. Go does so on
// arm64, ppc64le, s390x, riscv64 and amd64 with GOAMD64=v3, also inside the
// standard library, and math.Exp and math.Log use assembly on some
// architectures only. Their results may therefore differ by an ulp between
// platforms, which can flip an 8-bit rounding and break Tela's
// same-pixels-everywhere rule (ARCHITECTURE section 3, principle 4).
//
// The functions here are ports of Go's pure-Go implementations in which
// every product that feeds an addition or subtraction is wrapped in an
// explicit float64 conversion. Such a conversion forces the rounding of the
// product, so the compiler cannot fuse it, and every remaining operation is
// a correctly rounded IEEE 754 operation: the result is the same everywhere.
// math.Sqrt, math.Floor, math.Ldexp, math.Frexp and the like are exact or
// correctly rounded on every architecture and may be used freely.
//
// Only finite arguments in the ranges documented on each function are
// supported; special cases (infinities, subnormal corner cases) are handled
// only as far as the callers need.
package fmath
