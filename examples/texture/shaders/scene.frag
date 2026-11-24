#version 450
#extension GL_GOOGLE_include_directive : require

// The scenes of examples/texture: each pixel is computed from the uniforms and
// nothing else. shade.go is the same code in Go, for where there is no GPU;
// TestVulkanDrawsTheScene compares them.

layout(location = 0) out vec4 outColor;

layout(push_constant) uniform Uniforms { // Go: uniforms, in scene.go
    vec4 size; // width, height, pixels per unit of the scene, which scene
    vec4 v[7]; // the scene's, but for the last: the pixels of a side of the
               // tiles, 1 / pixels per unit of the scene
} u;

#include "common.glsl"
#include "planet.glsl"
#include "mountains.glsl"
#include "satin.glsl"

void main() {
    vec2 p = vec2(gl_FragCoord.x - 0.5 * u.size.x, 0.5 * u.size.y - gl_FragCoord.y) * u.v[6].y;
    vec3 c;
    if (u.size.w == 0.0) c = planetScene(p);
    else if (u.size.w == 1.0) c = mountainsScene(p);
    else c = satinScene(p);
    // A filmic curve, then sRGB, for which a square root is close enough, then
    // a rounding up or down at random, the same on every frame, so that slow
    // gradients show no steps.
    c = max(c, 0.0);
    c = c * (2.51 * c + 0.03) / (c * (2.43 * c + 0.59) + 0.14);
    float d = hash(int(gl_FragCoord.x), int(gl_FragCoord.y), 77);
    outColor = vec4(min(floor(sqrt(c) * 255.0 + d), 255.0) / 255.0, 1.0);
}
