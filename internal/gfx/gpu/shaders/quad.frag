#version 450

// Fragment shader of the quad pipeline (ADR-282): rectangles and rounded
// rectangles with analytic coverage, rounded-rectangle shadows in closed
// form, coverage read from the atlas (glyphs, paths, strokes), solid and
// gradient paints, images, image patterns and sprites, one anti-aliased
// clip shape and one clip mask; and the pixels of a layer or of a filtered
// backdrop, with an opacity, a mask and a blend mode (a Porter-Duff operator
// among them).
//
// It follows the CPU rasterizer (internal/gfx/raster), whose output is the
// reference: the same coverage (the exact area of the pixel inside the
// shape, rounded to 8 bits; the rasterizer's own for what comes from the
// atlas), the same shadow formula (raster/shadow.go), the same gradient
// geometry sampled at pixel centers (raster/gradient.go) with the ramps
// the rasterizer bakes, the same image sampling (raster/image.go,
// raster/pattern.go). Every quantity is a function of the pixel's
// coordinates and of the instance only, so a render of a region gives the
// pixels of a full render.
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

// The layout is defined in frame.go; std430 gives every member 16 bytes.
struct Instance {
    vec4 bounds;  // the quad
    uvec4 head;   // flags, first ramp row (a layer: its opacity, blend mode and mask), first color, last color
    vec4 paint0;  // p = paint0.x*X + paint0.y*Y + paint0.z; w: conic start (turns)
    vec4 paint1;  // q likewise; w: unused. A layer: pixel + xy is the mask's texel, pixel + zw the backdrop's
    vec4 paint2;  // focal: d.x, d.y, dr, r0; image: the clamp range of u, then of v
    vec4 paint3;  // focal: a; lo, hi: the first and last stop offsets
    Shape shape;  // the fill, or the box of a shadow
    Shape clip;
    Shape blur;   // the shape a shadow blurs, in the frame of its Gaussian
    vec4 sigma;   // shadow: sx, sy, and the size wx, wy of a pixel in that frame
    vec4 origin;  // xy: the origin of the coordinates; z: the atlas page; w: the clip mask's page
    ivec4 tex;    // pixel + tex.xy: the texel in the atlas, or of a layer; pixel + tex.zw: the clip mask's
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

// The atlases: pages of 8-bit coverage, and of premultiplied colors. They
// are read texel by texel.
layout(set = 0, binding = 3) uniform sampler2DArray coverage;
layout(set = 0, binding = 4) uniform sampler2DArray sprites;

// The image of the batch, or the layer its instances paint, read texel by
// texel; the mask of the layer; and the copy of the pixels under it that a
// blend mode reads.
layout(set = 1, binding = 0) uniform sampler2D image;
layout(set = 1, binding = 1) uniform sampler2D layerMask;
layout(set = 1, binding = 2) uniform sampler2D backdrop;

layout(push_constant) uniform Target {
    vec2 scale;
    vec2 origin; // the device coordinates of the target's first pixel
} target;

layout(location = 0) flat in int inIndex;

// The instance being drawn. Its members are read where they are used:
// a path of the shader pays for the ones it needs.
#define IT instances.items[inIndex]
layout(location = 0) out vec4 outColor;

// Bits of head.x (frame.go).
const uint KIND_MASK = 3u;         // 0 fill, 1 outer shadow, 2 inset shadow, 3 atlas
const uint KIND_OUTER = 1u;
const uint KIND_INSET = 2u;
const uint KIND_ATLAS = 3u;
const uint SHAPE_LOCAL = 4u;
const uint CLIP_SHIFT = 3u;        // 0 none, 1 device, 2 local
const uint PAINT_SHIFT = 5u;       // 0 solid, 1 linear, 2 radial, 3 conic, 4 focal, 5 image, 6 image (nearest), 7 sprite
const uint SPREAD_SHIFT = 8u;      // 0 pad, 1 reflect, 2 repeat
const uint BLUR_SHIFT = 10u;       // 0 analytic, 1 hard device, 2 hard local, 3 empty
const uint CLIP_MASK = 4096u;      // a clip mask in the coverage atlas
const uint LAYER = 8192u;          // the paint is the pixels of a layer
const uint FULL = 16384u;          // every pixel of the quad is covered entirely: no shape, no clip
const uint OPACITY = 32768u;       // a solid fill faded by the opacity in head.y (a layer of one rectangle)
const uint SEGMENTS_SHIFT = 16u;
// In the place of the segments, for an image: it is a pattern, and the axes
// along which that repeats.
const uint PATTERN = 65536u;
const uint REPEAT_X = 131072u;
const uint REPEAT_Y = 262144u;

const uint PAINT_IMAGE = 5u;
const uint PAINT_NEAREST = 6u;
const uint PAINT_SPRITE = 7u;

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
vec4 rampColor(float t) {
    if (!(t >= IT.paint3.y)) { // also NaN
        return unpackUnorm4x8(IT.head.z);
    }
    if (t >= IT.paint3.z) {
        return unpackUnorm4x8(IT.head.w);
    }
    uint row = IT.head.y;
    uint n = IT.head.x >> SEGMENTS_SHIFT;
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
float focalT(float u, float v, out bool ok) {
    float dx = IT.paint2.x, dy = IT.paint2.y, dr = IT.paint2.z, r0 = IT.paint2.w;
    float qa = IT.paint3.x;
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

// texel8 returns the 8-bit values of a texel of the image, clamped to it.
uvec4 texel8(ivec2 p, ivec2 size) {
    return uvec4(texelFetch(image, clamp(p, ivec2(0), size - 1), 0) * 255.0 + 0.5);
}

// imageColor samples the batch's image at the image coordinates uv: the
// pixel that contains the point, or the four around it with weights
// quantized to 1/256 and summed in integers. A DrawImage (raster's
// imageShader) is clamped to the range of its source rectangle (paint2). A
// pattern (raster's patternShader) wraps around along an axis that repeats,
// where the encoder keeps the taps at -1 or above, and is transparent beyond
// one that does not, for each of the four pixels.
vec4 imageColor(vec2 uv, vec2 rel, uint flags, bool nearest) {
    ivec2 size = textureSize(image, 0);
    ivec2 i, j;   // the taps: columns i.x and j.x, rows i.y and j.y,
    uvec2 wi, wj; // and their weights
    if ((flags & PATTERN) == 0u) {
        vec2 lo = IT.paint2.xz, hi = IT.paint2.yw;
        if (nearest) {
            return texelFetch(image, clamp(ivec2(clamp(floor(uv), lo, hi)), ivec2(0), size - 1), 0);
        }
        vec2 f = floor(clamp(uv - 0.5, lo, hi) * 256.0);
        i = ivec2(f) >> 8;
        wj = uvec2(ivec2(f) & 255);
        wi = 256u - wj;
        // The second tap has weight 0 where it would leave the image.
        j = i + ivec2(wj.x == 0u ? 0 : 1, wj.y == 0u ? 0 : 1);
    } else {
        // The coordinates come in two parts (splitCoord): uv, which float32
        // holds without rounding, and a rest, small where that matters, added
        // to what uv is above a whole number. The half pixel between a center
        // and the taps around it is in them. (The bounds keep coordinates of
        // no meaning inside an int.)
        precise vec2 rest = vec2(IT.paint0.w * rel.x + IT.paint2.x * rel.y + IT.paint2.y,
                                 IT.paint1.w * rel.x + IT.paint2.z * rel.y + IT.paint2.w);
        vec2 whole = clamp(floor(uv), -2097152.0, 2097152.0);
        precise vec2 part = uv - whole;
        precise vec2 f = floor((part + rest) * 256.0);
        f = clamp(f, -536870912.0, 536870912.0);
        i = ivec2(whole) * 256 + ivec2(f);
        wj = nearest ? uvec2(0u) : uvec2(i & 255); // nearest: bilinear at a pixel's center
        wi = 256u - wj;
        i = i >> 8;
        bvec2 rep = bvec2((flags & REPEAT_X) != 0u, (flags & REPEAT_Y) != 0u);
        if (rep.x) {
            i.x = int(uint(max(i.x + size.x, 0)) % uint(size.x));
        }
        if (rep.y) {
            i.y = int(uint(max(i.y + size.y, 0)) % uint(size.y));
        }
        j = i + 1;
        j.x = rep.x && j.x == size.x ? 0 : j.x;
        j.y = rep.y && j.y == size.y ? 0 : j.y;
        // A tap beyond the image has no weight (its fetch is clamped).
        wi.x = i.x >= 0 && i.x < size.x ? wi.x : 0u;
        wi.y = i.y >= 0 && i.y < size.y ? wi.y : 0u;
        wj.x = j.x >= 0 && j.x < size.x ? wj.x : 0u;
        wj.y = j.y >= 0 && j.y < size.y ? wj.y : 0u;
    }
    uvec4 sum = texel8(i, size) * (wi.x * wi.y)
              + texel8(ivec2(j.x, i.y), size) * (wj.x * wi.y)
              + texel8(ivec2(i.x, j.y), size) * (wi.x * wj.y)
              + texel8(j, size) * (wj.x * wj.y);
    return vec4((sum + 32768u) >> 16) / 255.0;
}

// paintColor returns the premultiplied color of the instance's paint at
// the pixel center rel (relative to the origin) of the pixel px.
vec4 paintColor(vec2 rel, ivec2 px) {
    uint kind = (IT.head.x >> PAINT_SHIFT) & 7u;
    if (kind == 0u) {
        return unpackUnorm4x8(IT.head.z);
    }
    if (kind == PAINT_SPRITE) {
        return texelFetch(sprites, ivec3(px + IT.tex.xy, int(IT.origin.z)), 0);
    }
    uint spread = (IT.head.x >> SPREAD_SHIFT) & 3u;
    float p = IT.paint0.x * rel.x + IT.paint0.y * rel.y + IT.paint0.z;
    if (kind == 1u) {
        return rampColor(spreadT(p, spread));
    }
    float q = IT.paint1.x * rel.x + IT.paint1.y * rel.y + IT.paint1.z;
    if (kind == PAINT_IMAGE || kind == PAINT_NEAREST) {
        return imageColor(vec2(p, q), rel, IT.head.x, kind == PAINT_NEAREST);
    }
    if (kind == 2u) {
        return rampColor(spreadT(sqrt(p * p + q * q), spread));
    }
    if (kind == 3u) {
        // The angle from up (-Y), clockwise on screen.
        float t = atan(p, -q) * 0.15915494309189535 - IT.paint0.w;
        return rampColor(t - floor(t));
    }
    bool ok;
    float t = focalT(p, q, ok);
    if (!ok) {
        return vec4(0.0);
    }
    return rampColor(spreadT(t, spread));
}

// ---------------------------------------------------------------------------
// Layers (raster/blend.go)
// ---------------------------------------------------------------------------

// The blend modes (gfx.BlendMode).
const uint BLEND_MULTIPLY = 1u;
const uint BLEND_SCREEN = 2u;
const uint BLEND_OVERLAY = 3u;
const uint BLEND_DARKEN = 4u;
const uint BLEND_LIGHTEN = 5u;
const uint BLEND_COLOR_DODGE = 6u;
const uint BLEND_COLOR_BURN = 7u;
const uint BLEND_HARD_LIGHT = 8u;
const uint BLEND_SOFT_LIGHT = 9u;
const uint BLEND_DIFFERENCE = 10u;
const uint BLEND_EXCLUSION = 11u;
const uint BLEND_HUE = 12u;
const uint BLEND_SATURATION = 13u;
const uint BLEND_COLOR = 14u;
const uint BLEND_LUMINOSITY = 15u;
const uint BLEND_PLUS_LIGHTER = 16u;
// The Porter-Duff operators.
const uint BLEND_SRC_IN = 17u;
const uint BLEND_SRC_OUT = 18u;
const uint BLEND_SRC_ATOP = 19u;
const uint BLEND_DST_OVER = 20u;
const uint BLEND_DST_IN = 21u;
const uint BLEND_DST_OUT = 22u;
const uint BLEND_DST_ATOP = 23u;
const uint BLEND_COPY = 24u;
const uint BLEND_XOR = 25u;

// Sets of operators, a bit each: those whose factor Fa is the alpha of the
// backdrop, what that lacks of 1, and 1 (0 for the others), and whose factor
// Fb is that of the source, what that lacks of 1, and 1 (raster's
// porterDuff).
const uint FA_ALPHA = 1u << BLEND_SRC_IN | 1u << BLEND_SRC_ATOP;
const uint FA_REST = 1u << BLEND_SRC_OUT | 1u << BLEND_DST_OVER | 1u << BLEND_DST_ATOP | 1u << BLEND_XOR;
const uint FA_ONE = 1u << BLEND_COPY;
const uint FB_ALPHA = 1u << BLEND_DST_IN | 1u << BLEND_DST_ATOP;
const uint FB_REST = 1u << BLEND_SRC_ATOP | 1u << BLEND_DST_OUT | 1u << BLEND_XOR;
const uint FB_ONE = 1u << BLEND_DST_OVER;
// The operators that change the backdrop where the source is transparent
// (gfx.BlendMode.Unbounded).
const uint UNBOUNDED = 1u << BLEND_SRC_IN | 1u << BLEND_SRC_OUT | 1u << BLEND_DST_IN | 1u << BLEND_DST_ATOP | 1u << BLEND_COPY;

// porterDuff composites the premultiplied 8-bit source s onto the backdrop d
// with an operator: the sum of the two times their factors over 255, rounded
// once.
ivec4 porterDuff(uint mode, ivec4 s, ivec4 d) {
    uint m = 1u << mode;
    int fa = (m & FA_ALPHA) != 0u ? d.a : (m & FA_REST) != 0u ? 255 - d.a : (m & FA_ONE) != 0u ? 255 : 0;
    int fb = (m & FB_ALPHA) != 0u ? s.a : (m & FB_REST) != 0u ? 255 - s.a : (m & FB_ONE) != 0u ? 255 : 0;
    return (s * fa + d * fb + 127) / 255; // 255 is odd: no ties
}

// imul8 is round(a*b/255) for 8-bit values, in integers (raster.mul8).
int imul8(int a, int b) {
    int t = a * b + 128;
    return (t + (t >> 8)) >> 8;
}

// rdiv255 is round(v / 255) for v >= 0 (255 is odd: no ties).
int rdiv255(int v) {
    return (v + 127) / 255;
}

// chan8 rounds a color numerator over 255² to 8 bits, at most alpha a.
int chan8(int v, int a) {
    return min(rdiv255(max(v, 0)), a);
}

// hardLight returns as*ab*HardLight(Cb, Cs) over 255² for premultiplied
// s, d.
int hardLight(int s, int sa, int d, int da) {
    return 2 * s <= sa ? 2 * s * d : sa * da - 2 * (da - d) * (sa - s);
}

// The non-separable helpers of W3C Compositing and Blending Level 1.
float lum(vec3 c) {
    return 0.3 * c.r + 0.59 * c.g + 0.11 * c.b;
}

vec3 setLum(vec3 c, float l) {
    c += l - lum(c);
    l = lum(c);
    float n = min(c.r, min(c.g, c.b));
    float x = max(c.r, max(c.g, c.b));
    if (n < 0.0 && l - n != 0.0) {
        c = l + (c - l) * l / (l - n);
    }
    if (x > 1.0 && x - l != 0.0) {
        c = l + (c - l) * (1.0 - l) / (x - l);
    }
    return c;
}

float sat(vec3 c) {
    return max(c.r, max(c.g, c.b)) - min(c.r, min(c.g, c.b));
}

vec3 setSat(vec3 c, float s) {
    float mn = min(c.r, min(c.g, c.b));
    float mx = max(c.r, max(c.g, c.b));
    return mx > mn ? (c - mn) * s / (mx - mn) : vec3(0.0);
}

// blendPixel composites the premultiplied 8-bit source s onto the backdrop
// d with a blend mode other than source-over (raster's blendRow and
// blendFloat): exact integers, rounded once, for the modes whose formula
// is a polynomial and for the operators; floats for the others.
ivec4 blendPixel(uint mode, ivec4 s, ivec4 d) {
    if (mode > BLEND_PLUS_LIGHTER) {
        return porterDuff(mode, s, d);
    }
    if (mode == BLEND_PLUS_LIGHTER) {
        return min(s + d, ivec4(255));
    }
    if (s.a == 0) {
        return d;
    }
    if (d.a == 0) {
        return s;
    }
    int sa = s.a, da = d.a;
    int a = rdiv255(255 * (sa + da) - sa * da);
    ivec4 o = ivec4(0, 0, 0, a);
    bool exact = mode <= BLEND_LIGHTEN || mode == BLEND_HARD_LIGHT || mode == BLEND_DIFFERENCE || mode == BLEND_EXCLUSION;
    if (exact) {
        for (int c = 0; c < 3; c++) {
            int sc = s[c], dc = d[c];
            int v;
            if (mode == BLEND_MULTIPLY) {
                v = sc * (255 - da) + dc * (255 - sa) + sc * dc;
            } else if (mode == BLEND_SCREEN) {
                v = 255 * (sc + dc) - sc * dc;
            } else if (mode == BLEND_OVERLAY) {
                v = sc * (255 - da) + dc * (255 - sa) + hardLight(dc, da, sc, sa);
            } else if (mode == BLEND_HARD_LIGHT) {
                v = sc * (255 - da) + dc * (255 - sa) + hardLight(sc, sa, dc, da);
            } else if (mode == BLEND_DARKEN) {
                v = 255 * (sc + dc) - max(sc * da, dc * sa);
            } else if (mode == BLEND_LIGHTEN) {
                v = 255 * (sc + dc) - min(sc * da, dc * sa);
            } else if (mode == BLEND_DIFFERENCE) {
                v = 255 * (sc + dc) - 2 * min(sc * da, dc * sa);
            } else {
                v = 255 * (sc + dc) - 2 * sc * dc;
            }
            o[c] = chan8(v, a);
        }
        return o;
    }
    float fsa = float(sa) / 255.0, fda = float(da) / 255.0;
    vec3 ps = vec3(s.rgb) / 255.0, pd = vec3(d.rgb) / 255.0;
    // Unpremultiplied; 1 exactly when a channel equals its alpha.
    vec3 cs = vec3(1.0), cb = vec3(1.0);
    for (int c = 0; c < 3; c++) {
        if (s[c] < sa) {
            cs[c] = float(s[c]) / float(sa);
        }
        if (d[c] < da) {
            cb[c] = float(d[c]) / float(da);
        }
    }
    vec3 bl = vec3(0.0);
    if (mode == BLEND_COLOR_DODGE) {
        for (int c = 0; c < 3; c++) {
            bl[c] = cb[c] == 0.0 ? 0.0 : (cs[c] >= 1.0 ? 1.0 : min(1.0, cb[c] / (1.0 - cs[c])));
        }
    } else if (mode == BLEND_COLOR_BURN) {
        for (int c = 0; c < 3; c++) {
            bl[c] = cb[c] >= 1.0 ? 1.0 : (cs[c] <= 0.0 ? 0.0 : 1.0 - min(1.0, (1.0 - cb[c]) / cs[c]));
        }
    } else if (mode == BLEND_SOFT_LIGHT) {
        for (int c = 0; c < 3; c++) {
            float b = cb[c], f = cs[c];
            if (f <= 0.5) {
                bl[c] = b - (1.0 - 2.0 * f) * b * (1.0 - b);
            } else {
                float dd = b <= 0.25 ? ((16.0 * b - 12.0) * b + 4.0) * b : sqrt(b);
                bl[c] = b + (2.0 * f - 1.0) * (dd - b);
            }
        }
    } else if (mode == BLEND_HUE) {
        bl = setLum(setSat(cs, sat(cb)), lum(cb));
    } else if (mode == BLEND_SATURATION) {
        bl = setLum(setSat(cb, sat(cs)), lum(cb));
    } else if (mode == BLEND_COLOR) {
        bl = setLum(cs, lum(cb));
    } else {
        bl = setLum(cb, lum(cs));
    }
    vec3 v = ps * (1.0 - fda) + pd * (1.0 - fsa) + fsa * fda * bl;
    o.rgb = min(ivec3(floor(clamp(v, 0.0, 1.0) * 255.0 + 0.5)), ivec3(a));
    return o;
}

// ---------------------------------------------------------------------------

void main() {
    uint flags = IT.head.x;
    vec2 center = gl_FragCoord.xy + target.origin; // in device pixels: whole numbers and a half
    vec2 rel = center - IT.origin.xy;
    ivec2 px = ivec2(floor(center));

    uint kind = flags & KIND_MASK;
    float cov;
    if ((flags & FULL) != 0u) {
        // The inside of a shape, where the shape and the clips cover
        // every pixel entirely: the values the paths below compute there.
        cov = 255.0;
    } else if (kind == KIND_ATLAS) {
        // The rasterizer's own coverage.
        cov = floor(texelFetch(coverage, ivec3(px + IT.tex.xy, int(IT.origin.z)), 0).r * 255.0 + 0.5);
    } else {
        cov = cover8(IT.shape, (flags & SHAPE_LOCAL) != 0u, rel);
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
                vec2 w = IT.sigma.zw;
                vec2 p = vec2(IT.blur.inv.x * rel.x + IT.blur.inv.z * rel.y + IT.blur.invT.x,
                              IT.blur.inv.y * rel.x + IT.blur.inv.w * rel.y + IT.blur.invT.y) - 0.5 * w;
                v = clamp(blurred(IT.blur, IT.sigma.xy, w, p), 0.0, 1.0);
            } else if (mode != 3u) {
                v = cover8(IT.blur, mode == 2u, rel) / 255.0;
            }
            cov = floor((kind == KIND_INSET ? 1.0 - v : v) * box + 0.5);
        }
    }
    if (((flags >> PAINT_SHIFT) & 7u) == PAINT_SPRITE) {
        // A sprite is composited at the alpha of its run.
        cov = mul8(cov, float(IT.head.z >> 24));
    }

    uint clip = (flags >> CLIP_SHIFT) & 3u;
    if (clip != 0u) {
        cov = mul8(cov, cover8(IT.clip, clip == 2u, rel));
    }
    if ((flags & CLIP_MASK) != 0u) {
        cov = mul8(cov, floor(texelFetch(coverage, ivec3(px + IT.tex.zw, int(IT.origin.w)), 0).r * 255.0 + 0.5));
    }
    if ((flags & (LAYER | OPACITY)) == OPACITY) {
        // The rectangle is all its layer holds: the layer's opacity
        // multiplies the coverage, as it does when a layer is composited.
        cov = mul8(cov, float(IT.head.y & 255u));
    }
    if (cov == 0.0) {
        discard;
    }
    if ((flags & LAYER) == 0u) {
        outColor = paintColor(rel, px) * (cov / 255.0);
        return;
    }

    // The pixels of a layer (raster's composite and compositeLayer) or of
    // a filtered backdrop (drawBackdrop), on 8-bit values.
    uint lp = IT.head.y;
    int op = int(lp & 255u);
    uint mode = (lp >> 8) & 255u;
    uint maskKind = (lp >> 16) & 255u;
    ivec4 s = ivec4(0);
    if (op != 0) { // else nothing of the source shows, and there may be none
        s = ivec4(texelFetch(image, px + IT.tex.xy, 0) * 255.0 + 0.5);
    }
    int k = op; // the opacity, times the mask's coverage
    if (maskKind != 0u) {
        uvec4 m = uvec4(texelFetch(layerMask, px + ivec2(IT.paint1.xy), 0) * 255.0 + 0.5);
        // The alpha, or the luminance of the premultiplied color in the
        // rasterizer's 16-bit fixed point.
        uint mc = maskKind == 2u ? (13933u * m.r + 46871u * m.g + 4732u * m.b + 32768u) >> 16 : m.a;
        k = imul8(k, int(mc));
    }
    int c = int(cov); // of the shape and the clips
    if (mode == 0u) {
        // Source-over: the device blends.
        if ((lp >> 24) != 0u) {
            // A backdrop filter's opacity scales its colors.
            s = ivec4(imul8(s.r, k), imul8(s.g, k), imul8(s.b, k), imul8(s.a, k));
        } else {
            c = imul8(k, c);
        }
        if (c == 0 || s.a == 0) {
            discard;
        }
        outColor = vec4(s) * (float(c) / 65025.0);
        return;
    }
    // A blend mode: the source scaled by the opacity and the mask, blended
    // with the copy of the backdrop, and the clip's coverage interpolates
    // between the backdrop and the result. Nothing blends this output.
    if (k != 255) {
        s = ivec4(imul8(s.r, k), imul8(s.g, k), imul8(s.b, k), imul8(s.a, k));
    }
    if (s.a == 0 && s.rgb == ivec3(0) && (1u << mode & UNBOUNDED) == 0u) {
        discard; // the mode leaves the backdrop
    }
    ivec4 d = ivec4(texelFetch(backdrop, px + ivec2(IT.paint1.zw), 0) * 255.0 + 0.5);
    ivec4 r = blendPixel(mode, s, d);
    outColor = vec4((r * c + d * (255 - c) + 127) / 255) / 255.0;
}
