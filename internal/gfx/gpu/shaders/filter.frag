#version 450

// Fragment shader of the filter pipeline (ADR-282): one step of a filter
// of a layer or of a backdrop filter, over a rectangle of a target, from
// the picture in another one. The steps are those of the CPU rasterizer's
// filters (internal/gfx/raster filter.go, blur.go, backdrop.go):
//
//   taps: one pass of a kernel along an axis (a blur, or the blocks of a
//         blur at a reduced resolution), the taps read from a buffer
//   up:   the blocks of a reduced blur interpolated back to the pixels
//   copy: the picture as it is
//
// The source may be a backdrop: the pixels of a rectangle, mirrored beyond
// it. The last step of a drop shadow also tints its result and puts it
// under the content, and the last step of a filter applies the color
// matrices that follow it, each on 8-bit values. Texels are read by their
// whole coordinates, never filtered, and the weights are data: a pixel's
// value is a function of the source's texels and of the pixel's
// coordinates in the space of its pass only, so a render of a region gives
// the pixels of a full render.
//
// Compiled to filter.frag.spv by `go generate ./internal/gfx/gpu`.

// The layout is defined in frame.go (the FilterSlot constants); std430
// gives every member 16 bytes.
struct Instance {
    vec4 bounds;    // the rectangle drawn
    uvec4 head;     // the step, the flags, the first tap, the number of taps
    ivec4 src;      // xy: what to add to the source's coordinates to get a texel; zw: the stride
    ivec4 srcRect;  // the coordinates that hold the source's picture: x0, y0, x1, y1
    ivec4 axis;     // taps: the step of a tap; up: the blocks' size, the shift
    ivec4 aux;      // xy: the content's texel for the pixel p is p + xy
    ivec4 auxRect;  // the coordinates that hold the content
    uvec4 color;    // a drop shadow's premultiplied color, 0 to 255
    ivec4 mirror;   // the first pixel and the size of the rectangle a backdrop is mirrored at
    uvec4 post;     // the first color matrix (in taps) and their number
    vec4 unused[14];
};

layout(std430, set = 0, binding = 0) readonly buffer Instances {
    Instance items[];
} instances;

// The kernels, an offset and a weight per tap, and the color matrices,
// 20 values each.
layout(std430, set = 0, binding = 5) readonly buffer Taps {
    vec2 items[];
} taps;

// The picture filtered, and the content a drop shadow goes under.
layout(set = 1, binding = 0) uniform sampler2D source;
layout(set = 1, binding = 1) uniform sampler2D content;

layout(push_constant) uniform Target {
    vec2 scale;
    vec2 origin; // the coordinates of the target's first pixel in the space of the pass
} target;

layout(location = 0) flat in int inIndex;
layout(location = 0) out vec4 outColor;

// The steps and the flags (frame.go).
const uint STEP_TAPS = 0u;
const uint STEP_UP = 1u;
const uint STEP_COPY = 2u;
const uint FLAG_ROUND = 1u;
const uint FLAG_DROP = 2u;
const uint FLAG_MIRROR = 4u;
const uint FLAG_MIRROR_AUX = 8u;

// Integer division and remainder are only relied on for operands that
// are not negative: coordinates are below 2^22 in magnitude, so they are
// moved up by a multiple of the divisor first.

// floorDiv returns floor(a / b) for b a power of two up to 2^23.
ivec2 floorDiv(ivec2 a, ivec2 b) {
    const int K = 1 << 23;
    return (a + K) / b - K / b;
}

// mirror maps i to [0, n) by reflecting at the ends, the edge pixel
// repeated (-1 is 0, n is n-1), any number of times (raster's mirror).
int mirror(int i, int n) {
    int period = 2 * n;
    i = (i + period * ((1 << 22) / period + 1)) % period;
    return i < n ? i : period - 1 - i;
}

// mirrored maps a pixel into the rectangle a backdrop is mirrored at. Most
// pixels are inside it: only the others pay for the reflection.
ivec2 mirrored(ivec2 q, ivec4 m) {
    ivec2 d = q - m.xy;
    if (d.x < 0 || d.x >= m.z) {
        d.x = mirror(d.x, m.z);
    }
    if (d.y < 0 || d.y >= m.w) {
        d.y = mirror(d.y, m.w);
    }
    return m.xy + d;
}

// The source of the instance, set by main before texel is called: the
// coordinates that hold its picture, what to add to them to get a texel,
// and the rectangle it is mirrored at, if it is.
ivec4 srcRect;
ivec2 srcOffset;
ivec4 srcMirror;
bool srcMirrored;

// texel returns the source at the coordinates q: transparent outside its
// picture.
vec4 texel(ivec2 q) {
    if (q.x < srcRect.x || q.y < srcRect.y || q.x >= srcRect.z || q.y >= srcRect.w) {
        return vec4(0.0);
    }
    if (srcMirrored) {
        q = mirrored(q, srcMirror);
    }
    return texelFetch(source, q + srcOffset, 0);
}

// unit8 rounds a value in [0, 1] to 8 bits (raster.unit8).
float unit8(float v) {
    return floor(clamp(v, 0.0, 1.0) * 255.0 + 0.5);
}

// colorMatrix applies the color matrix at a place of the taps to an 8-bit
// premultiplied color (raster's colorMatrix): unpremultiply, transform,
// clamp, premultiply.
uvec4 colorMatrix(uvec4 s, uint at) {
    vec4 c = vec4(0.0);
    if (s.a > 0u) {
        c = vec4(vec3(s.rgb) / float(s.a), float(s.a) / 255.0);
    }
    vec4 o;
    for (uint row = 0u; row < 4u; row++) {
        // The row's five values: two taps and a half.
        uint i = row * 5u;
        vec2 a = taps.items[at + (i >> 1)];
        vec2 b = taps.items[at + (i >> 1) + 1u];
        vec2 d = taps.items[at + (i >> 1) + 2u];
        vec4 k = (i & 1u) == 0u ? vec4(a, b) : vec4(a.y, b, d.x);
        float last = (i & 1u) == 0u ? d.x : d.y;
        o[row] = clamp(k.x * c.r + k.y * c.g + k.z * c.b + k.w * c.a + last, 0.0, 1.0);
    }
    return uvec4(unit8(o.r * o.a), unit8(o.g * o.a), unit8(o.b * o.a), unit8(o.a));
}

void main() {
    Instance it = instances.items[inIndex];
    uint step_ = it.head.x;
    uint flags = it.head.y;
    ivec2 p = ivec2(floor(gl_FragCoord.xy + target.origin));
    vec4 v = vec4(0.0);
    srcRect = it.srcRect;
    srcOffset = it.src.xy;
    srcMirror = it.mirror;
    srcMirrored = (flags & FLAG_MIRROR) != 0u;

    if (step_ == STEP_TAPS) {
        ivec2 base = p * it.src.zw;
        uint first = it.head.z, n = it.head.w;
        // The taps are in the order of their offsets. Where the first and
        // the last one read inside the picture (and inside the rectangle a
        // backdrop is mirrored at), every one does: most pixels take the
        // loop without the tests, which sums the same values in the same
        // order.
        ivec2 a = base + it.axis.xy * int(taps.items[first].x);
        ivec2 b = base + it.axis.xy * int(taps.items[first + n - 1u].x);
        ivec4 r = srcRect;
        if (srcMirrored) {
            r = ivec4(max(r.xy, srcMirror.xy), min(r.zw, srcMirror.xy + srcMirror.zw));
        }
        if (n > 0u && a.x >= r.x && a.y >= r.y && b.x < r.z && b.y < r.w) {
            base += srcOffset;
            for (uint k = 0u; k < n; k++) {
                vec2 tw = taps.items[first + k];
                v += tw.y * texelFetch(source, base + it.axis.xy * int(tw.x), 0);
            }
        } else {
            for (uint k = 0u; k < n; k++) {
                vec2 tw = taps.items[first + k];
                v += tw.y * texel(base + it.axis.xy * int(tw.x));
            }
        }
    } else if (step_ == STEP_UP) {
        // raster's blurDown: with t = 2(X - shift) + 1 - f, the blocks
        // floor(t / 2f) and the next, with weights 2f - n and n over 2f,
        // n = t mod 2f; a block of one pixel is the pixel.
        ivec2 f = it.axis.xy;
        ivec2 t = 2 * (p - it.axis.zw) + 1 - f;
        ivec2 i = floorDiv(t, 2 * f);
        vec2 w1 = vec2(t - 2 * f * i) / vec2(2 * f);
        vec2 w0 = 1.0 - w1;
        if (i.x >= srcRect.x && i.y >= srcRect.y && i.x + 1 < srcRect.z && i.y + 1 < srcRect.w) {
            // The four blocks are inside the picture (it is never a
            // backdrop): the same sum without the tests.
            ivec2 q = i + srcOffset;
            v = w0.y * (w0.x * texelFetch(source, q, 0) + w1.x * texelFetch(source, q + ivec2(1, 0), 0))
              + w1.y * (w0.x * texelFetch(source, q + ivec2(0, 1), 0) + w1.x * texelFetch(source, q + ivec2(1, 1), 0));
        } else {
            v = w0.y * (w0.x * texel(i) + w1.x * texel(i + ivec2(1, 0)))
              + w1.y * (w0.x * texel(i + ivec2(0, 1)) + w1.x * texel(i + ivec2(1, 1)));
        }
    } else {
        v = texel(p * it.src.zw);
    }

    if ((flags & FLAG_DROP) != 0u) {
        // raster's dropShadow: the shadow is the color times the blurred
        // alpha, under the content, rounded once over 255².
        ivec2 q = p;
        uvec4 c = uvec4(0u);
        if (q.x >= it.auxRect.x && q.y >= it.auxRect.y && q.x < it.auxRect.z && q.y < it.auxRect.w) {
            if ((flags & FLAG_MIRROR_AUX) != 0u) {
                q = mirrored(q, it.mirror);
            }
            c = uvec4(texelFetch(content, q + it.aux.xy, 0) * 255.0 + 0.5);
        }
        uint k = uint(unit8(v.a)) * (255u - c.a);
        v = vec4((c * 65025u + it.color * k + 32512u) / 65025u) / 255.0;
    } else if ((flags & FLAG_ROUND) != 0u) {
        v = floor(clamp(v, 0.0, 1.0) * 255.0 + 0.5) / 255.0;
    }
    // The color matrices that follow, each on the 8-bit result of what
    // is before it.
    for (uint k = 0u; k < it.post.y; k++) {
        v = vec4(colorMatrix(uvec4(v * 255.0 + 0.5), it.post.x + 10u * k)) / 255.0;
    }
    outColor = v;
}
