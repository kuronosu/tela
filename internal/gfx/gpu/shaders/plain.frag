#version 450

// Fragment shader of the plain pipeline: the insides of shapes that
// quad.frag would draw without coverage, clips or a paint of its own
// (FULL), in a shader small enough for the device to keep many pixels in
// flight. It draws two kinds of instances of the quad's layout:
//
//   - a solid fill (paint 0), faded by an opacity when OPACITY says so;
//   - the pixels of a layer or of a filtered backdrop (LAYER), composited
//     source-over without a mask, or copied (COPY, without blending).
//
// Its arithmetic is quad.frag's for these instances, operation for
// operation, so the pixels are the same (TestPlain).
//
// Compiled to plain.frag.spv by `go generate ./internal/gfx/gpu`.

struct Instance {
    vec4 bounds;
    uvec4 head; // flags, a layer's opacity (and flags), the color of a fill
    vec4 rest[21];
    ivec4 tex;  // pixel + tex.xy: the texel of a layer
};

layout(std430, set = 0, binding = 0) readonly buffer Instances {
    Instance items[];
} instances;

layout(set = 1, binding = 0) uniform sampler2D image;

layout(push_constant) uniform Target {
    vec2 scale;
    vec2 origin;
} target;

layout(location = 0) flat in int inIndex;
layout(location = 0) out vec4 outColor;

#define IT instances.items[inIndex]

const uint LAYER = 8192u;
const uint OPACITY = 32768u;

// Bits of head.y for a layer: the opacity scales the colors (a filtered
// backdrop), and a copy of pixels between targets (PipelineOpaque), which
// writes transparent pixels too.
const uint FADE = 1u << 24;
const uint COPY = 1u << 25;

// imul8 is round(a*b/255) for 8-bit integers (raster.mul8).
int imul8(int a, int b) {
    int t = a * b + 128;
    return (t + (t >> 8)) >> 8;
}

void main() {
    uint flags = IT.head.x;
    if ((flags & LAYER) == 0u) {
        // quad.frag: cov is 255, or mul8(255, opacity), which is the
        // opacity; the color times cov / 255.
        float cov = 255.0;
        if ((flags & OPACITY) != 0u) {
            cov = float(IT.head.y & 255u);
        }
        outColor = unpackUnorm4x8(IT.head.z) * (cov / 255.0);
        return;
    }
    vec2 center = gl_FragCoord.xy + target.origin;
    ivec2 px = ivec2(floor(center));
    uint lp = IT.head.y;
    int k = int(lp & 255u);
    ivec4 s = ivec4(texelFetch(image, px + IT.tex.xy, 0) * 255.0 + 0.5);
    int c = 255;
    if ((lp & FADE) != 0u) {
        // A backdrop filter's opacity scales its colors.
        s = ivec4(imul8(s.r, k), imul8(s.g, k), imul8(s.b, k), imul8(s.a, k));
    } else {
        c = imul8(k, c);
    }
    if ((lp & COPY) == 0u && (c == 0 || s.a == 0)) {
        discard;
    }
    outColor = vec4(s) * (float(c) / 65025.0);
}
