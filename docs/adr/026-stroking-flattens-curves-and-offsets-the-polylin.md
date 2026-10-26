# ADR-026: Stroking flattens curves and offsets the polyline

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Stroking flattens curves and offsets the polyline (true-tangent ends, round joins between pieces) instead of kurbo's cubic offset fitting; joins, caps and dashes follow kurbo

## Rationale

The CPU rasterizer fills lines anyway; simpler and provably within tolerance, checked against geometric oracles
