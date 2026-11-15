#version 450

// Fragment shader of the smoke test of internal/vk (TestRender): a
// premultiplied color times a texture.
//
// Compiled to quad.frag.spv by `go generate ./internal/vk` (glslangValidator).

layout(location = 0) in vec2 inUV;

layout(set = 0, binding = 1) uniform sampler2D tex;

layout(push_constant) uniform Draw {
    vec4 color;
    vec4 rect;
    vec2 instanceStep;
} draw;

layout(location = 0) out vec4 outColor;

void main() {
    outColor = draw.color * texture(tex, inUV);
}
