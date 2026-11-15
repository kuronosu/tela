#version 450

// Vertex shader of the smoke test of internal/vk (TestRender): a rectangle
// in pixels, placed by push constants and repeated by instance, mapped to
// clip space by a uniform buffer.
//
// Compiled to quad.vert.spv by `go generate ./internal/vk` (glslangValidator).

layout(location = 0) in vec2 inCorner; // a corner of the unit square
layout(location = 1) in vec2 inUV;

layout(set = 0, binding = 0) uniform Target {
    vec2 scale;     // pixels to clip space: 2 / size
    vec2 translate; // and -1
} target;

layout(push_constant) uniform Draw {
    vec4 color;        // premultiplied
    vec4 rect;         // x, y, width, height in pixels
    vec2 instanceStep; // pixels from one instance to the next
} draw;

layout(location = 0) out vec2 outUV;

void main() {
    vec2 pixel = draw.rect.xy + inCorner * draw.rect.zw + draw.instanceStep * float(gl_InstanceIndex);
    gl_Position = vec4(pixel * target.scale + target.translate, 0.0, 1.0);
    outUV = inUV;
}
