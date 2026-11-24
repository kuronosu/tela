// A giant planet the color of sand, with rings of fine grain and two moons
// (Go: planet.go). Lengths are in radii of it. The uniforms:
//
//   v[0]  the planet's axis, which is the normal of its rings; the phase of
//         the stars' twinkling
//   v[1]  where the longitude 0 of the planet is, as it turns
//   v[2]  and that of its finer clouds
//   v[3]  the direction to the light
//   v[4], v[5]  the moons: where they are, and their radii

const float planetR = 0.36;
const vec2 planetAt = vec2(0.10, -0.02);
const float ringIn = 1.30, ringOut = 2.31;

// ringDensity is how opaque the rings are at distance r of the center.
float ringDensity(float r, float soft) {
    float e = smoothTo(1.32 - soft, 1.35 + soft, r) * (1.0 - smoothTo(2.23, 2.28 + soft, r));
    if (e <= 0.0) return 0.0;
    float gap = smoothTo(0.012, 0.035 + soft, abs(r - 1.985));
    gap *= 1.0 - 0.75 * (1.0 - smoothTo(0.0, 0.02 + soft, abs(r - 2.17)));
    float d = 0.35 + 0.65 * noise1(r * 9.0, 3) + 0.35 * (noise1(r * 31.0, 7) - 0.5);
    float grain = clamp1(1.6 - soft * 100.0); // it is finer than a pixel of a small picture
    if (grain > 0.0) d += 0.22 * grain * (noise1(r * 97.0, 9) - 0.5);
    return e * gap * clamp1(d) * lerp(0.55, 0.95, smoothTo(1.4, 1.75, r));
}

// lit is how much light the planet lets through to q.
float lit(vec3 q) {
    float b = dot(q, u.v[3].xyz);
    if (b >= 0.0) return 1.0;
    float d = dot(q, q) - b * b;
    return d <= 0.0 ? 0.0 : smoothTo(0.90, 1.06, sqrt(d));
}

// dust is the color of the sky between the stars.
vec3 dust(vec2 p) {
    vec3 c = vec3(0.0040, 0.0048, 0.0064);
    c += vec3(0.030, 0.022, 0.016) * smoothTo(0.36, 0.72, fbm2(vec2(1.5 * p.x + 0.4 * p.y + 5.0, 1.9 * p.y - 0.5 * p.x + 1.0), 3));
    return c + vec3(0.010, 0.016, 0.024) * smoothTo(0.34, 0.78, fbm2(vec2(1.1 * p.x + 31.0, 1.3 * p.y + 4.0), 3));
}

// glow is the air around the planet, on the side of the light.
vec3 glow(vec2 p, float r) {
    if (r > 1.25) return vec3(0.0);
    float side = smoothTo(0.10, 0.95, dot(p, u.v[3].xy) / r);
    return vec3(0.42, 0.34, 0.26) * (0.34 * side * decay(abs(r - 1.0) * 34.0));
}

// ground is the color of the clouds: bands that the longitude bends.
vec3 ground(vec3 n, float lat) {
    vec3 a = u.v[0].xyz;
    float lx = dot(n, u.v[1].xyz), lz = dot(n, cross(a, u.v[1].xyz));
    float fx = dot(n, u.v[2].xyz), fz = dot(n, cross(a, u.v[2].xyz));
    float w1 = fbm3(vec3(lx * 2.2 + 3.0, lat * 5.5, lz * 2.2), 2) + 0.09;
    float w2 = fbm3(vec3(fx * 3.4 + 9.0 * w1, lat * 9.0 + 2.0 * w1, fz * 3.4 + 1.0), 3) + 0.03;
    float v = lat * 3.3 + 0.42 * (w1 - 0.47) + 0.30 * (w2 - 0.5);
    float f = 0.10 + 0.56 * noiseSoft(v * 1.9 + 10.0, 1) + 0.27 * noiseSoft(v * 5.3 + 40.0, 4)
        + 0.14 * noiseSoft(v * 14.0 + 70.0, 8) + 0.28 * (w2 - 0.5);
    vec3 c = lerp(vec3(0.20, 0.105, 0.060), vec3(0.52, 0.31, 0.17), smoothTo(0.15, 0.45, f));
    c = lerp(c, vec3(0.80, 0.60, 0.40), smoothTo(0.40, 0.62, f));
    c = lerp(c, vec3(0.93, 0.85, 0.72), smoothTo(0.60, 0.85, f));
    return lerp(c, vec3(0.30, 0.24, 0.20), 0.65 * smoothTo(0.62, 0.98, abs(lat)));
}

vec3 planet(vec2 p, float dd) {
    vec3 l = u.v[3].xyz, n = vec3(p, sqrt(max(1.0 - dd, 0.0)));
    float ndl = dot(n, l);
    vec3 c = vec3(0.0036, 0.0030, 0.0024); // the night
    if (ndl <= -0.15) return c;
    float lat = dot(n, u.v[0].xyz), rim = 1.0 - n.z;
    c += vec3(0.85, 0.62, 0.42) * (0.14 * (rim * rim) * smoothTo(-0.05, 0.6, ndl));
    c += vec3(0.55, 0.26, 0.12) * (0.11 * smoothTo(-0.10, 0.04, ndl) * (1.0 - smoothTo(0.04, 0.3, ndl)));
    float diff = 1.2 * smoothTo(-0.06, 1.0, ndl);
    if (diff <= 0.0) return c;
    float hit = -lat / dot(u.v[0].xyz, l);
    if (hit > 0.0) { // the shadow of the rings
        vec3 q = n + l * hit;
        float r = sqrt(dot(q, q));
        if (r > ringIn && r < ringOut) diff *= 1.0 - 0.72 * ringDensity(r, 0.03);
    }
    return c + ground(n, lat) * (diff * (0.55 + 0.45 * sqrt(n.z)));
}

// ring lays the rings over c, at a point of their plane r from the center.
vec3 ring(vec3 c, vec3 q, float r, float pixel) {
    float a = ringDensity(r, 0.8 * pixel / u.v[0].z);
    if (a <= 0.0) return c;
    float k = 0.03 + 0.95 * lit(q);
    return lerp(c, lerp(vec3(0.50, 0.40, 0.31), vec3(0.88, 0.80, 0.69), smoothTo(1.35, 2.0, r)) * k, a);
}

vec3 moon(vec3 c, vec2 p, int i, bool front, float pixel, float pixels) {
    vec4 m = u.v[4 + i];
    if ((m.z >= 0.0) != front) return c;
    vec2 q = p - m.xy;
    if (abs(q.x) >= m.w + pixel || abs(q.y) >= m.w + pixel) return c;
    float dd = dot(q, q);
    vec3 n = vec3(q, sqrt(max(m.w * m.w - dd, 0.0))) * (1.0 / m.w);
    float k = 0.45 + 1.1 * fbm3(vec3(n.x * 2.6 + 9.0, n.y * 2.6 + 4.0 * float(i), n.z * 2.6), 2);
    vec3 color = i == 0 ? vec3(0.62, 0.54, 0.45) : vec3(0.52, 0.53, 0.54);
    vec3 s = vec3(0.003) + color * (k * smoothTo(-0.05, 1.0, dot(n, u.v[3].xyz)) * lit(m.xyz));
    return lerp(c, s, clamp1((m.w - sqrt(dd)) * pixels + 0.5));
}

vec3 planetScene(vec2 at) {
    float pixels = u.size.z * planetR, pixel = 1.0 / pixels;
    vec2 p = (at - planetAt) * (1.0 / planetR);
    float dd = dot(p, p), r = 2.0, cover = 0.0; // of the planet
    if (dd < 2.0) {
        r = sqrt(dd);
        cover = clamp1((1.0 - r) * pixels + 0.5);
    }
    // Where the ray meets the plane of the rings.
    vec3 axis = u.v[0].xyz;
    vec3 q = vec3(p, (-axis.x / axis.z) * p.x + (-axis.y / axis.z) * p.y);
    float qq = dot(q, q), rr = 0.0;
    bool ringed = qq > ringIn * ringIn && qq < ringOut * ringOut;
    if (ringed) rr = sqrt(qq);
    vec3 c = vec3(0.0);
    if (cover < 1.0) {
        c = tileMix(dust(tileCorner(0)), dust(tileCorner(1)), dust(tileCorner(2)));
        if (!ringed) { // no stars behind the rings
            c += stars(at * 26.0, 0, 0.13, 70.0, u.v[0].w);
            c += stars(at * 55.0, 1, 0.15, 42.0, u.v[0].w);
        }
        for (int i = 0; i < 2; i++) c = moon(c, p, i, false, pixel, pixels);
        if (ringed && q.z <= 0.0) c = ring(c, q, rr, pixel);
        c += glow(p, r);
    }
    if (cover > 0.0) c = lerp(c, planet(p, dd), cover);
    if (ringed && q.z > 0.0) c = ring(c, q, rr, pixel);
    for (int i = 0; i < 2; i++) c = moon(c, p, i, true, pixel, pixels);
    return c;
}
