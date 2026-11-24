#version 450

// The scene of examples/texture: a ringed planet with four moons, each pixel
// computed from the uniforms and nothing else. shade.go is the same code in
// Go, for where there is no GPU; TestVulkanDrawsTheScene compares them.

layout(location = 0) out vec4 outColor;

layout(push_constant) uniform Uniforms { // Go: uniforms, in scene.go
    vec4 size;     // width, height, pixels per unit of the scene
    vec4 axis;     // the planet's axis, which is the normal of its ring
    vec4 meridian; // where its longitude 0 is, as it spins
    vec4 light;    // the direction to the light
    vec4 moons[4]; // x, y, z, radius
} u;

const float planetR = 0.62, ringIn = 0.80, ringOut = 1.22;

const vec3 moonColors[4] = vec3[](
    vec3(0.85, 0.70, 0.55), vec3(0.45, 0.65, 0.85), vec3(0.75, 0.75, 0.78), vec3(0.80, 0.45, 0.35));

// wave is nearly a sine of period 1 (a parabola per half), made of
// multiplications so that every GPU and the Go code agree on it.
float wave(float t) {
    t = 2.0 * (t - floor(t)) - 1.0;
    return 4.0 * t * (abs(t) - 1.0);
}

float ramp(float lo, float hi, float x) { return clamp((x - lo) / (hi - lo), 0.0, 1.0); }

// lit is how much light the planet lets through to point q.
float lit(vec3 q) {
    float b = dot(q, u.light.xyz);
    if (b >= 0.0) return 1.0;
    return ramp(planetR - 0.05, planetR + 0.02, sqrt(max(dot(q, q) - b * b, 0.0)));
}

// ringDensity is how opaque the ring is at distance r of the center.
float ringDensity(float r, float soft) {
    float edge = ramp(ringIn - soft, ringIn + soft, r) * (1.0 - ramp(ringOut - soft, ringOut + soft, r));
    float gap = ramp(0.0, 0.015 + soft, abs(r - 1.06));
    return edge * gap * (0.62 + 0.24 * wave(r * 11.0) + 0.10 * wave(r * 37.0));
}

vec3 sky(vec2 p, float t) {
    vec3 c = mix(vec3(0.010, 0.014, 0.045), vec3(0.030, 0.012, 0.050), t);
    c += vec3(0.085, 0.040, 0.160) / (1.0 + 2.2 * dot(p, p));
    vec2 q = p - vec2(1.5, -0.9);
    return c + vec3(0.010, 0.075, 0.095) / (1.0 + 1.6 * dot(q, q));
}

vec3 moon(vec3 c, vec2 p, int i) {
    vec4 m = u.moons[i];
    vec2 q = p - m.xy;
    float dd = dot(q, q), far = m.w + 1.0 / u.size.z;
    if (dd > far * far) return c;
    vec3 n = vec3(q, sqrt(max(m.w * m.w - dd, 0.0))) / m.w;
    float diff = max(dot(n, u.light.xyz), 0.0) * lit(m.xyz + n * m.w);
    float cover = clamp((m.w - sqrt(dd)) * u.size.z + 0.5, 0.0, 1.0);
    return mix(c, moonColors[i] * (0.03 + 0.97 * diff), cover);
}

vec3 planet(vec3 c, vec2 p) {
    float dd = dot(p, p), far = planetR + 1.0 / u.size.z;
    if (dd > far * far) return c;
    vec3 a = u.axis.xyz, l = u.light.xyz;
    vec3 n = vec3(p, sqrt(max(planetR * planetR - dd, 0.0))) / planetR;
    float lat = dot(n, a), x = dot(n, u.meridian.xyz), z = dot(n, cross(a, u.meridian.xyz));
    // Bands that the longitude bends, and storms between them.
    float band = wave(lat * 2.3 + 0.07 * wave(x * 1.4) + 0.05 * wave(z * 1.9 + lat));
    float storm = wave(x * 1.7 + 0.2 * band) * wave(z * 1.3 - lat);
    vec3 s = mix(vec3(0.34, 0.13, 0.42), vec3(0.95, 0.58, 0.30), 0.5 + 0.5 * band);
    s = mix(s, vec3(0.98, 0.86, 0.70), 0.35 * max(storm, 0.0));
    float diff = max(dot(n, l), 0.0);
    // The shadow of the ring.
    float k = dot(a, l);
    float hit = -planetR * lat / (abs(k) < 0.001 ? 0.001 : k);
    if (hit > 0.0) {
        vec3 q = n * planetR + l * hit;
        diff *= 1.0 - 0.75 * ringDensity(sqrt(dot(q, q)), 0.02);
    }
    float rim = 1.0 - n.z;
    s = s * (0.025 + 0.95 * diff) + vec3(0.25, 0.30, 0.65) * (rim * rim * rim * (0.15 + 0.85 * diff));
    float g = max(dot(n, normalize(l + vec3(0.0, 0.0, 1.0))), 0.0);
    g *= g;
    g *= g;
    g *= g;
    g *= g;
    g *= g;
    s += vec3(0.16 * g * diff);
    float cover = clamp((planetR - sqrt(dd)) * u.size.z + 0.5, 0.0, 1.0);
    return mix(c, s, cover);
}

vec3 ring(vec3 c, vec2 p, bool front) {
    vec3 a = u.axis.xyz;
    float z = -(a.x * p.x + a.y * p.y) / a.z;
    if ((z > 0.0) != front) return c;
    vec3 q = vec3(p, z);
    float rr = dot(q, q);
    if (rr < 0.5 || rr > 1.7) return c;
    float d = ringDensity(sqrt(rr), 1.0 / (u.size.z * a.z));
    return mix(c, vec3(0.86, 0.74, 0.62) * (0.04 + 0.80 * lit(q)), d);
}

void main() {
    vec2 p = (gl_FragCoord.xy - 0.5 * u.size.xy) / u.size.z;
    p.y = -p.y;
    vec3 c = sky(p, gl_FragCoord.y / u.size.y);
    for (int i = 0; i < 4; i++)
        if (u.moons[i].z < 0.0) c = moon(c, p, i);
    c = ring(c, p, false);
    c = planet(c, p);
    c = ring(c, p, true);
    for (int i = 0; i < 4; i++)
        if (u.moons[i].z >= 0.0) c = moon(c, p, i);
    // Textures take sRGB: a square root is close enough to its curve.
    outColor = vec4(sqrt(clamp(c, 0.0, 1.0)), 1.0);
}
