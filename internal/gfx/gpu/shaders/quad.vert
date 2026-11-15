#version 450

// Vertex shader of the quad pipeline (ADR-282): one instance is one
// device-pixel rectangle, drawn as a triangle strip of four vertices
// without vertex buffers. Everything else about the instance is read by
// the fragment shader, by its index.
//
// Compiled to quad.vert.spv by `go generate ./internal/gfx/gpu`.

// The layout of an instance is defined in frame.go (InstanceSize bytes:
// 24 slots of 16 bytes); only the first slot matters here.
struct Instance {
    vec4 bounds;   // x0, y0, x1, y1 in device pixels (whole numbers)
    vec4 rest[23];
};

layout(std430, set = 0, binding = 0) readonly buffer Instances {
    Instance items[];
} instances;

layout(push_constant) uniform Target {
    vec2 scale; // device pixels to clip space: 2 / size
} target;

layout(location = 0) flat out int outIndex;

void main() {
    vec4 b = instances.items[gl_InstanceIndex].bounds;
    vec2 corner = vec2(float(gl_VertexIndex & 1), float(gl_VertexIndex >> 1));
    vec2 pixel = mix(b.xy, b.zw, corner);
    gl_Position = vec4(pixel * target.scale - 1.0, 0.0, 1.0);
    outIndex = gl_InstanceIndex;
}
