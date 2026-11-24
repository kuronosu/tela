// Folds of satin under a warm light and a cool one (Go: satin.go). The
// uniforms:
//
//   v[0]  where the noises are that bend it
//   v[1]  the phases of the folds
//   v[2], v[3]  the directions to the lights

// folds returns the height of the cloth, and minus its slopes: the noise a
// bends the noise b, and both bend the folds.
vec3 folds(vec2 p) {
    vec3 n = slopes(vec2(p.x * 0.95 + u.v[0].x, p.y * 1.05 + u.v[0].y));
    float a = n.x, ax = n.y * 0.95, ay = n.z * 1.05;
    n = slopes(vec2(p.x * 0.9 + 2.2 * a + 4.0 + u.v[0].z, p.y + 1.8 * a + 1.0 + u.v[0].w));
    float b = n.x;
    float bx = n.y * (0.9 + 2.2 * ax) + n.z * (1.8 * ax);
    float by = n.y * (2.2 * ay) + n.z * (1.0 + 1.8 * ay);
    float f = 1.1 * p.x - 1.5 * p.y + 2.6 * b + 1.2 * a;
    float fx = 1.1 + 2.6 * bx + 1.2 * ax;
    float fy = 2.6 * by + 1.2 * ay - 1.5;
    float s = 0.4934 * f + u.v[1].x;
    float t = 0.2069 * f - 0.1273 * p.x + 0.3183 * a - u.v[1].y;
    float h = 0.275 * sine(s) + 0.30 * sine(t) + 0.18 * b;
    float k = 0.8525 * cosine(s), l = 0.30 * cosine(t);
    float hx = k * fx + l * (1.3 * fx - 0.8 + 2.0 * ax) + 0.18 * bx;
    float hy = k * fy + l * (1.3 * fy + 2.0 * ay) + 0.18 * by;
    return vec3(h, hx * -0.30, hy * -0.30);
}

// powers returns x to the power of 2, of 8 and of 32.
vec3 powers(float x) {
    x = max(x, 0.0);
    float p2 = x * x, p4 = p2 * p2, p8 = p4 * p4, p16 = p8 * p8;
    return vec3(p2, p8, p16 * p16);
}

vec3 unit(vec3 a) { return a * (1.0 / sqrt(dot(a, a))); }

vec3 satinScene(vec2 p) {
    vec3 t = tileMix(folds(tileCorner(0)), folds(tileCorner(1)), folds(tileCorner(2)));
    vec3 n = unit(vec3(t.yz, 1.0)), warm = u.v[2].xyz, cool = u.v[3].xyz;

    vec3 c = lerp(vec3(0.007, 0.0075, 0.0088), vec3(0.054, 0.053, 0.052), smoothTo(-0.55, 0.60, t.x));
    c *= 0.13 + 1.35 * max(dot(n, warm), 0.0) + 0.60 * max(dot(n, cool), 0.0);
    vec3 g = powers(dot(n, unit(warm + vec3(0.0, 0.0, 1.0))));
    c += vec3(1.0, 0.82, 0.62) * (0.46 * (g.z * (g.y * g.y)) + 0.080 * (g.y * g.x));
    g = powers(dot(n, unit(cool + vec3(0.0, 0.0, 1.0))));
    c += vec3(0.50, 0.66, 0.85) * (0.22 * (g.z * g.z) + 0.040 * g.y);
    float rim = 1.0 - n.z;
    c += vec3(0.8, 0.72, 0.62) * (0.09 * (rim * rim));
    return c * (1.0 - 0.19 * (p.x * p.x) - 0.46 * (p.y * p.y));
}
