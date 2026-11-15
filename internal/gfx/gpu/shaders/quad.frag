#version 450

// Fragment shader of the quad pipeline (ADR-282): rectangles and rounded
// rectangles with analytic coverage, rounded-rectangle shadows in closed
// form, solid and gradient paints, and one anti-aliased clip shape.
//
// It follows the CPU rasterizer (internal/gfx/raster), whose output is the
// reference: the same coverage (the exact area of the pixel inside the
// shape, rounded to 8 bits), the same shadow formula (raster/shadow.go),
// the same gradient geometry sampled at pixel centers (raster/gradient.go)
// with the ramps the rasterizer bakes. Every quantity is a function of the
// pixel's coordinates and of the instance only, so a render of a region
// gives the pixels of a full render.
//
// Compiled to quad.frag.spv by `go generate ./internal/gfx/gpu`.

// A rounded rectangle, in one of two forms (see Encoder):
//
//   device: rect is the rectangle in device pixels, snapped to 1/256 pixel
//           and relative to the instance's origin, and the radii belong to
//           the device corners. Coverage is the exact area.
//   local:  rect and radii are in the local space of the command, and
//           (inv, invT) maps a pixel center (relative to the origin) to
//           that space. Coverage comes from the distance to the boundary.
struct Shape {
    vec4 rect; // x0, y0, x1, y1
    vec4 radX; // top-left, top-right, bottom-right, bottom-left
    vec4 radY;
    vec4 inv;  // a, b, c, d: local = (a*X + c*Y + e, b*X + d*Y + f)
    vec4 invT; // e, f
};

// The layout is defined in instance.go; std430 gives every member 16 bytes.
struct Instance {
    vec4 bounds;  // the quad; bounds.xy is the origin of the instance
    uvec4 head;   // flags, first ramp row, first color, last color
    vec4 paint0;  // p = paint0.x*X + paint0.y*Y + paint0.z; w: conic start (turns)
    vec4 paint1;  // q likewise; w: unused
    vec4 paint2;  // focal: d.x, d.y, dr, r0
    vec4 paint3;  // focal: a; lo, hi: the first and last stop offsets
    Shape shape;  // the fill, or the box of a shadow
    Shape clip;
    Shape blur;   // the shape a shadow blurs, in the frame of its Gaussian
    vec4 sigma;   // shadow: sx, sy, and the size wx, wy of a pixel in that frame
};

layout(std430, set = 0, binding = 0) readonly buffer Instances {
    Instance items[];
} instances;

// One entry per ramp row: the offset of the segment's first stop and the
// factor from offsets to ramp entries.
layout(std430, set = 0, binding = 1) readonly buffer RampSegments {
    vec4 items[];
} segments;

// The ramps: 256 premultiplied colors per row.
layout(set = 0, binding = 2) uniform sampler2D ramps;

layout(location = 0) flat in int inIndex;
layout(location = 0) out vec4 outColor;

// Bits of head.x (instance.go).
const uint KIND_MASK = 3u;         // 0 fill, 1 outer shadow, 2 inset shadow
const uint KIND_OUTER = 1u;
const uint KIND_INSET = 2u;
const uint SHAPE_LOCAL = 4u;
const uint CLIP_SHIFT = 3u;        // 0 none, 1 device, 2 local
const uint PAINT_SHIFT = 5u;       // 0 solid, 1 linear, 2 radial, 3 conic, 4 focal
const uint SPREAD_SHIFT = 8u;      // 0 pad, 1 reflect, 2 repeat
const uint BLUR_SHIFT = 10u;       // 0 analytic, 1 hard device, 2 hard local, 3 empty
const uint SEGMENTS_SHIFT = 16u;

// ---------------------------------------------------------------------------
// Coverage
// ---------------------------------------------------------------------------

// to8 rounds a coverage to 8 bits (raster.unitTo8).
float to8(float c) {
    return floor(clamp(c, 0.0, 1.0) * 255.0 + 0.5);
}

// mul8 is round(a*b/255) for 8-bit values (raster.mul8).
float mul8(float a, float b) {
    return floor(a * b / 255.0 + 0.5);
}

const float GL_X[9] = float[9](
    -0.5773502691896257, 0.5773502691896257,
    -0.7745966692414834, 0.0, 0.7745966692414834,
    -0.8611363115940526, -0.3399810435848563, 0.3399810435848563, 0.8611363115940526);
const float GL_W[9] = float[9](
    1.0, 1.0,
    0.5555555555555556, 0.8888888888888888, 0.5555555555555556,
    0.3478548451374538, 0.6521451548625461, 0.6521451548625461, 0.3478548451374538);

// outsideUnit returns the area of the box [ul, uh] x [vl, vh] of the unit
// square that lies outside the unit disk: the integral over u of
// vh - clamp(sqrt(1 - u*u), vl, vh). Between the u where the arc enters the
// box (sh) and where it leaves it (sl) the integrand is
// vh - sqrt(1 - u*u) = (u - sh)*(u + sh) / (vh + sqrt(1 - u*u)), a form
// without cancellation, integrated with two Gauss-Legendre panels of four
// points; beyond sl the box is outside. The caller integrates along the
// axis where the arc is less steep.
float outsideUnit(float ul, float uh, float vl, float vh) {
    float sh = sqrt((1.0 - vh) * (1.0 + vh));
    float sl = sqrt((1.0 - vl) * (1.0 + vl));
    float a = clamp(sh, ul, uh);
    float b = clamp(sl, ul, uh);
    float area = (vh - vl) * (uh - b);
    if (b > a) {
        float half_ = 0.25 * (b - a);
        for (int k = 0; k < 2; k++) {
            float mid = a + (float(k) * 2.0 + 1.0) * half_;
            for (int i = 5; i < 9; i++) {
                float u = mid + half_ * GL_X[i];
                float f = sqrt((1.0 - u) * (1.0 + u));
                area += half_ * GL_W[i] * (u - sh) * (u + sh) / (vh + f);
            }
        }
    }
    return area;
}

// cornerOutside returns the area, in square pixels, of the pixel that lies
// in a corner's box but outside its ellipse (raster's corner.addOutside).
// d is the distance from the corner of the box to the near sides of the
// pixel, along both axes, and r the radii.
float cornerOutside(vec2 d, vec2 r) {
    vec2 a = max(d, 0.0);
    vec2 b = min(d + 1.0, r);
    if (!(a.x < b.x && a.y < b.y)) {
        return 0.0;
    }
    vec2 lo = clamp((r - b) / r, 0.0, 1.0);
    vec2 hi = clamp((r - a) / r, 0.0, 1.0);
    if (dot(hi, hi) <= 1.0) {
        return 0.0; // the pixel's part of the box is inside the ellipse
    }
    float o = lo.y >= lo.x ? outsideUnit(lo.x, hi.x, lo.y, hi.y) : outsideUnit(lo.y, hi.y, lo.x, hi.x);
    return o * r.x * r.y;
}

// coverDevice returns the area of the pixel whose corner is px (relative
// to the origin) inside a device shape: the rectangle's, a product of two
// overlaps that is exact on the 1/256 grid, minus the part outside each
// corner's ellipse.
float coverDevice(Shape s, vec2 px) {
    vec2 over = clamp(min(s.rect.zw, px + 1.0) - max(s.rect.xy, px), 0.0, 1.0);
    float c = over.x * over.y;
    if (c == 0.0) {
        return 0.0;
    }
    vec2 d0 = px - s.rect.xy;
    vec2 d1 = s.rect.zw - (px + 1.0);
    float sub = 0.0;
    if (s.radX.x > 0.0) sub += cornerOutside(d0, vec2(s.radX.x, s.radY.x));
    if (s.radX.y > 0.0) sub += cornerOutside(vec2(d1.x, d0.y), vec2(s.radX.y, s.radY.y));
    if (s.radX.z > 0.0) sub += cornerOutside(d1, vec2(s.radX.z, s.radY.z));
    if (s.radX.w > 0.0) sub += cornerOutside(vec2(d0.x, d1.y), vec2(s.radX.w, s.radY.w));
    return c - sub;
}

// halfPlane returns the area of a pixel on the inner side of a line: s is
// the distance from the pixel's center to the line, positive inside, and n
// the line's unit normal, both in device space. It is exact (the area of a
// unit square cut by a line).
float halfPlane(float s, vec2 n) {
    vec2 a = abs(n);
    float hi = max(a.x, a.y), lo = min(a.x, a.y);
    float h1 = 0.5 * (hi - lo), h2 = 0.5 * (hi + lo);
    float t = abs(s);
    float c; // the smaller part
    if (t >= h2) {
        c = 0.0;
    } else if (t <= h1) {
        c = 0.5 - t / hi;
    } else {
        float e = h2 - t;
        c = e * e / (2.0 * lo * hi);
    }
    return s >= 0.0 ? 1.0 - c : c;
}

// clampAvg returns the average of clamp(u, 0, 1) while u goes linearly
// from a to b.
float clampAvg(float a, float b) {
    float lo = min(a, b), hi = max(a, b);
    if (hi <= 0.0) {
        return 0.0;
    }
    if (lo >= 1.0) {
        return 1.0;
    }
    if (hi - lo < 1e-4) {
        return clamp(0.5 * (lo + hi), 0.0, 1.0);
    }
    // The integral of clamp(u, 0, 1): 0, u*u/2, then u - 1/2.
    float gl = lo <= 0.0 ? 0.0 : 0.5 * lo * lo;
    float gh = hi >= 1.0 ? hi - 0.5 : 0.5 * hi * hi;
    return (gh - gl) / (hi - lo);
}

// edgeArea returns the signed area of the unit pixel [0, 1]^2 to the right
// of the edge from a to b, the term each edge adds in the rasterizer's
// area accumulation: summed over the edges of a polygon it is the exact
// area of the pixel inside it.
float edgeArea(vec2 a, vec2 b) {
    float ya = clamp(a.y, 0.0, 1.0), yb = clamp(b.y, 0.0, 1.0);
    if (ya == yb) {
        return 0.0;
    }
    float slope = (b.x - a.x) / (b.y - a.y);
    float xa = a.x + (ya - a.y) * slope, xb = a.x + (yb - a.y) * slope;
    return (yb - ya) * clampAvg(1.0 - xa, 1.0 - xb);
}

// arcCover returns the area of the pixel inside an ellipse of radii r,
// near its boundary: w is the pixel center relative to the ellipse's
// center, in local space, and gx, gy the device gradients of the two local
// coordinates. In device space the ellipse is F = 0 for a quadratic F, so
// its crossing with the line through the pixel center along the gradient
// is exact; the boundary is then taken as the tangent there (halfPlane)
// bent by the curvature at that point, which takes away the sliver between
// the tangent and the arc over the chord inside the pixel.
float arcCover(vec2 w, vec2 r, vec2 gx, vec2 gy) {
    vec2 k = 2.0 / (r * r); // the local Hessian's diagonal
    vec2 wn = w / r;
    float f = dot(wn, wn) - 1.0;
    vec2 g = k.x * w.x * gx + k.y * w.y * gy;
    float gl = length(g);
    if (!(gl > 0.0)) {
        return 1.0; // the center of the ellipse
    }
    vec2 n = g / gl;
    vec2 jn = vec2(dot(gx, n), dot(gy, n)); // n in local space
    float hnn = k.x * jn.x * jn.x + k.y * jn.y * jn.y;
    float disc = gl * gl - 2.0 * hnn * f;
    // How far the boundary is from the center along n (negative: behind).
    float h = disc > 0.0 ? -2.0 * f / (gl + sqrt(disc)) : -f / gl;
    if (abs(h) >= 1.5) {
        return h > 0.0 ? 1.0 : 0.0; // the boundary is beyond the pixel
    }
    // The crossing point, and the gradient there.
    vec2 w1 = w + h * jn;
    vec2 g1 = k.x * w1.x * gx + k.y * w1.y * gy;
    float g1l = length(g1);
    vec2 n1 = g1 / g1l;
    vec2 t1 = vec2(-n1.y, n1.x);
    vec2 jt = vec2(dot(gx, t1), dot(gy, t1));
    float curv = (k.x * jt.x * jt.x + k.y * jt.y * jt.y) / g1l;
    float c = halfPlane(h * dot(n, n1), n1);
    // The chord of the tangent inside the pixel, from the crossing point
    // (the pixel is the square of half side 0.5 around the origin).
    vec2 p0 = h * n;
    vec2 inv = 1.0 / t1; // an infinity for an axis-aligned tangent
    vec2 ta = (-0.5 - p0) * inv, tb = (0.5 - p0) * inv;
    vec2 lo = min(ta, tb), hi = max(ta, tb);
    if (abs(t1.x) < 1e-6) {
        lo.x = abs(p0.x) <= 0.5 ? -4.0 : 4.0;
        hi.x = abs(p0.x) <= 0.5 ? 4.0 : -4.0;
    }
    if (abs(t1.y) < 1e-6) {
        lo.y = abs(p0.y) <= 0.5 ? -4.0 : 4.0;
        hi.y = abs(p0.y) <= 0.5 ? 4.0 : -4.0;
    }
    float s0 = max(lo.x, lo.y), s1 = min(hi.x, hi.y);
    if (s1 > s0) {
        c -= curv * (s1 * s1 * s1 - s0 * s0 * s0) * (1.0 / 6.0);
    }
    return clamp(c, 0.0, 1.0);
}

// coverLocal returns the coverage of a pixel by a local shape under any
// invertible transform, where the rasterizer fills a path. The rectangle
// is a parallelogram in device space, whose area inside the pixel is
// exact (edgeArea). A pixel whose center lies in a corner, beyond the
// center of its ellipse on both axes, is also cut by the ellipse
// (arcCover). rel is the pixel center relative to the origin.
float coverLocal(Shape s, vec2 rel) {
    vec2 p = vec2(s.inv.x * rel.x + s.inv.z * rel.y + s.invT.x,
                  s.inv.y * rel.x + s.inv.w * rel.y + s.invT.y);
    // The corners of the rectangle relative to the pixel's corner, through
    // the inverse of inv.
    float det = s.inv.x * s.inv.w - s.inv.y * s.inv.z;
    vec2 l0 = s.rect.xy - p, l1 = s.rect.zw - p;
    vec2 ex = vec2(s.inv.w, -s.inv.y) / det, ey = vec2(-s.inv.z, s.inv.x) / det;
    vec2 v0 = l0.x * ex + l0.y * ey + 0.5;
    vec2 v1 = l1.x * ex + l0.y * ey + 0.5;
    vec2 v2 = l1.x * ex + l1.y * ey + 0.5;
    vec2 v3 = l0.x * ex + l1.y * ey + 0.5;
    float c = min(abs(edgeArea(v0, v1) + edgeArea(v1, v2) + edgeArea(v2, v3) + edgeArea(v3, v0)), 1.0);
    if (c == 0.0) {
        return 0.0;
    }
    // The device gradients of the local coordinates.
    vec2 gx = s.inv.xz, gy = s.inv.yw;
    for (int i = 0; i < 4; i++) {
        vec2 r = vec2(s.radX[i], s.radY[i]);
        if (!(r.x > 0.0 && r.y > 0.0)) {
            continue;
        }
        bool right = i == 1 || i == 2, bottom = i >= 2;
        // w: from the ellipse's center; in the corner it points to the
        // corner on both axes.
        vec2 w = p - vec2(right ? s.rect.z - r.x : s.rect.x + r.x, bottom ? s.rect.w - r.y : s.rect.y + r.y);
        if ((right ? w.x : -w.x) > 0.0 && (bottom ? w.y : -w.y) > 0.0) {
            c = min(c, arcCover(w, r, gx, gy));
        }
    }
    return c;
}

// cover8 returns the 8-bit coverage of a shape.
float cover8(Shape s, bool local, vec2 rel) {
    return to8(local ? coverLocal(s, rel) : coverDevice(s, rel - 0.5));
}

// ---------------------------------------------------------------------------
// Shadows (raster/shadow.go)
// ---------------------------------------------------------------------------

const float SHADOW_K = 4.0;  // raster.ShadowCutoff
const float PANEL_LEN = 2.0; // the longest quadrature panel, in standard deviations
const float MAX_PANELS = 6.0;
const float INV_SQRT_2PI = 0.3989422804014327;

// gauss returns the standard normal CDF and density at t. The CDF uses
// Abramowitz and Stegun 7.1.26 (error below 1e-7), which shares its
// exponential with the density.
void gauss(float t, out float cdf, out float pdf) {
    float e = exp(-0.5 * t * t);
    pdf = INV_SQRT_2PI * e;
    float k = 1.0 / (1.0 + 0.3275911 * abs(t) * 0.7071067811865476);
    float tail = 0.5 * e * k * (0.254829592 + k * (-0.284496736 + k * (1.421413741 + k * (-1.453152027 + k * 1.061405429))));
    cdf = t > 0.0 ? 1.0 - tail : tail;
}

// stepAvg returns the average over [p, p+w] of Phi((x - a)/s): a blurred
// step at a, box-filtered over the pixel. A pixel narrow against s takes
// the midpoint value with its second-order term, which avoids the
// cancellation of the closed form s*(I(t1) - I(t0))/w.
float stepAvg(float p, float w, float a, float s) {
    float ks = SHADOW_K * s;
    if (p + w <= a - ks) {
        return 0.0;
    }
    if (p >= a + ks) {
        return 1.0;
    }
    float t0 = (p - a) / s, h = w / s;
    float c0, d0, c1, d1;
    if (h < 0.5) {
        float tm = t0 + 0.5 * h;
        gauss(tm, c0, d0);
        return c0 - h * h * (1.0 / 24.0) * tm * d0;
    }
    float t1 = t0 + h;
    gauss(t0, c0, d0);
    gauss(t1, c1, d1);
    return ((t1 * c1 + d1) - (t0 * c0 + d0)) / h;
}

// densAvg returns the average over [p, p+w] of the Gaussian density of
// standard deviation s centered at v.
float densAvg(float p, float w, float v, float s) {
    float ks = SHADOW_K * s;
    if (p + w <= v - ks || p >= v + ks) {
        return 0.0;
    }
    float t0 = (p - v) / s, h = w / s;
    float c0, d0, c1, d1;
    if (h < 0.5) {
        float tm = t0 + 0.5 * h;
        gauss(tm, c0, d0);
        return d0 / s * (1.0 + h * h * (1.0 / 24.0) * (tm * tm - 1.0));
    }
    gauss(t0, c0, d0);
    gauss(t0 + h, c1, d1);
    return (c1 - c0) / w;
}

// interval returns the box-filtered blurred indicator of the interval
// between a and b for the pixel [p, p+w].
float interval(float p, float w, float a, float b, float s) {
    return stepAvg(p, w, min(a, b), s) - stepAvg(p, w, max(a, b), s);
}

// piece integrates one curved piece of a corner's deficit: along the axis
// u, between lo and hi, the density of the node times the interval between
// the box's edge and the arc on the other axis. Only the part within the
// Gaussian's reach of the pixel counts; it is split into panels of at most
// PANEL_LEN standard deviations (MAX_PANELS at most), with 2 to 4
// Gauss-Legendre points each as in the rasterizer. The arc at u is
// c2 + gr2*sqrt(1 - ((u - c1)/r1)^2).
float piece(float pu, float wu, float su, float pv, float wv, float sv,
            float lo, float hi, float c1, float r1, float c2, float gr2, float edge) {
    float a = max(lo, pu - SHADOW_K * su);
    float b = min(hi, pu + wu + SHADOW_K * su);
    if (!(a < b)) {
        return 0.0;
    }
    float n = clamp(ceil((b - a) / (PANEL_LEN * su)), 1.0, MAX_PANELS);
    float ph = (b - a) / n;
    int first = 5, last = 9;
    if (ph <= 0.5 * su) {
        first = 0; last = 2;
    } else if (ph <= su) {
        first = 2; last = 5;
    }
    float half_ = 0.5 * ph;
    float sum = 0.0;
    for (float k = 0.0; k < n; k += 1.0) {
        float mid = a + (k + 0.5) * ph;
        for (int i = first; i < last; i++) {
            float u = mid + half_ * GL_X[i];
            float dn = densAvg(pu, wu, u, su);
            if (dn != 0.0) {
                float q = clamp(abs(u - c1) / r1, 0.0, 1.0);
                float arc = c2 + gr2 * sqrt((1.0 - q) * (1.0 + q));
                sum += half_ * GL_W[i] * dn * interval(pv, wv, edge, arc, sv);
            }
        }
    }
    return sum;
}

// blurred returns the shape of a shadow convolved with its Gaussian and
// averaged over the pixel box whose corner is p, in the frame of the
// Gaussian (raster's rrBlur.at): the rectangle's closed form minus, at
// each corner, the blurred region between the corner's box and its
// ellipse, split where the arc's slope is 1.
float blurred(Shape sh, vec2 s, vec2 w, vec2 p) {
    vec4 r = sh.rect;
    float fx = stepAvg(p.x, w.x, r.x, s.x) - stepAvg(p.x, w.x, r.z, s.x);
    if (fx == 0.0) {
        return 0.0;
    }
    float fy = stepAvg(p.y, w.y, r.y, s.y) - stepAvg(p.y, w.y, r.w, s.y);
    float v = fx * fy;
    vec2 reach = SHADOW_K * s + w;
    for (int i = 0; i < 4; i++) {
        vec2 rad = vec2(sh.radX[i], sh.radY[i]);
        if (!(rad.x > 0.0 && rad.y > 0.0)) {
            continue;
        }
        bool right = i == 1 || i == 2, bottom = i >= 2;
        vec2 g = vec2(right ? 1.0 : -1.0, bottom ? 1.0 : -1.0);
        vec2 box = vec2(right ? r.z : r.x, bottom ? r.w : r.y); // the box corner
        vec2 c = box - g * rad;                                   // the ellipse's center
        vec2 zlo = min(box, c) - reach, zhi = max(box, c) + reach;
        if (p.x + w.x <= zlo.x || p.x >= zhi.x || p.y + w.y <= zlo.y || p.y >= zhi.y) {
            continue;
        }
        // The split point: slope 1 in coordinates scaled by the standard
        // deviations.
        vec2 nr = rad / s;
        vec2 split = c + g * rad * (nr / length(nr));
        float d = interval(p.x, w.x, box.x, split.x, s.x) * interval(p.y, w.y, box.y, split.y, s.y);
        d += piece(p.x, w.x, s.x, p.y, w.y, s.y, min(split.x, c.x), max(split.x, c.x), c.x, rad.x, c.y, g.y * rad.y, box.y);
        d += piece(p.y, w.y, s.y, p.x, w.x, s.x, min(split.y, c.y), max(split.y, c.y), c.y, rad.y, c.x, g.x * rad.x, box.x);
        v -= d;
    }
    return v;
}

// ---------------------------------------------------------------------------
// Paints (raster/gradient.go)
// ---------------------------------------------------------------------------

// rampColor returns the color at the gradient parameter t.
vec4 rampColor(Instance it, float t) {
    if (!(t >= it.paint3.y)) { // also NaN
        return unpackUnorm4x8(it.head.z);
    }
    if (t >= it.paint3.z) {
        return unpackUnorm4x8(it.head.w);
    }
    uint row = it.head.y;
    uint n = it.head.x >> SEGMENTS_SHIFT;
    uint k = 0u;
    while (k + 1u < n && t >= segments.items[row + k + 1u].x) {
        k++;
    }
    vec4 seg = segments.items[row + k];
    int i = clamp(int((t - seg.x) * seg.y + 0.5), 0, 255);
    return texelFetch(ramps, ivec2(i, int(row + k)), 0);
}

float spreadT(float t, uint spread) {
    if (spread == 2u) {
        return t - floor(t);
    }
    if (spread == 1u) {
        float u = t - 2.0 * floor(t * 0.5);
        return u > 1.0 ? 2.0 - u : u;
    }
    return t;
}

// focalT solves the two-point conical gradient for the local point
// Focal + (u, v): the largest t whose circle passes through it with a
// radius of 0 or more. ok is false outside the cone.
float focalT(Instance it, float u, float v, out bool ok) {
    float dx = it.paint2.x, dy = it.paint2.y, dr = it.paint2.z, r0 = it.paint2.w;
    float qa = it.paint3.x;
    float b = u * dx + v * dy + r0 * dr;
    float c = u * u + v * v - r0 * r0;
    if (qa == 0.0) {
        if (b == 0.0) {
            ok = false;
            return 0.0;
        }
        float t = c / (2.0 * b);
        ok = t * dr + r0 >= 0.0;
        return t;
    }
    float disc = b * b - qa * c;
    if (!(disc >= 0.0)) {
        ok = false;
        return 0.0;
    }
    float q = b + (b < 0.0 ? -sqrt(disc) : sqrt(disc));
    float t1 = 0.0, t2 = 0.0;
    if (q != 0.0) {
        t1 = q / qa;
        t2 = c / q;
    }
    float hi = max(t1, t2), lo = min(t1, t2);
    if (hi * dr + r0 >= 0.0) {
        ok = true;
        return hi;
    }
    ok = lo * dr + r0 >= 0.0;
    return lo;
}

// paintColor returns the premultiplied color of the instance's paint at
// the pixel center rel (relative to the origin).
vec4 paintColor(Instance it, vec2 rel) {
    uint kind = (it.head.x >> PAINT_SHIFT) & 7u;
    if (kind == 0u) {
        return unpackUnorm4x8(it.head.z);
    }
    uint spread = (it.head.x >> SPREAD_SHIFT) & 3u;
    float p = it.paint0.x * rel.x + it.paint0.y * rel.y + it.paint0.z;
    if (kind == 1u) {
        return rampColor(it, spreadT(p, spread));
    }
    float q = it.paint1.x * rel.x + it.paint1.y * rel.y + it.paint1.z;
    if (kind == 2u) {
        return rampColor(it, spreadT(sqrt(p * p + q * q), spread));
    }
    if (kind == 3u) {
        // The angle from up (-Y), clockwise on screen.
        float t = atan(p, -q) * 0.15915494309189535 - it.paint0.w;
        return rampColor(it, t - floor(t));
    }
    bool ok;
    float t = focalT(it, p, q, ok);
    if (!ok) {
        return vec4(0.0);
    }
    return rampColor(it, spreadT(t, spread));
}

// ---------------------------------------------------------------------------

void main() {
    Instance it = instances.items[inIndex];
    uint flags = it.head.x;
    vec2 rel = gl_FragCoord.xy - it.bounds.xy; // the pixel's center

    float cov = cover8(it.shape, (flags & SHAPE_LOCAL) != 0u, rel);
    uint kind = flags & KIND_MASK;
    if (kind != 0u) {
        // A shadow paints outside its box (outer) or inside it (inset):
        // box is the part of the pixel where it may paint.
        float box = kind == KIND_INSET ? cov : 255.0 - cov;
        if (box == 0.0) {
            discard;
        }
        uint mode = (flags >> BLUR_SHIFT) & 3u;
        float v = 0.0;
        if (mode == 0u) {
            vec2 w = it.sigma.zw;
            vec2 p = vec2(it.blur.inv.x * rel.x + it.blur.inv.z * rel.y + it.blur.invT.x,
                          it.blur.inv.y * rel.x + it.blur.inv.w * rel.y + it.blur.invT.y) - 0.5 * w;
            v = clamp(blurred(it.blur, it.sigma.xy, w, p), 0.0, 1.0);
        } else if (mode != 3u) {
            v = cover8(it.blur, mode == 2u, rel) / 255.0;
        }
        cov = floor((kind == KIND_INSET ? 1.0 - v : v) * box + 0.5);
    }

    uint clip = (flags >> CLIP_SHIFT) & 3u;
    if (clip != 0u) {
        cov = mul8(cov, cover8(it.clip, clip == 2u, rel));
    }
    if (cov == 0.0) {
        discard;
    }
    outColor = paintColor(it, rel) * (cov / 255.0);
}
