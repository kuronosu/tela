// Ranges in the mist at dusk, over still water (Go: mountains.go). The
// uniforms:
//
//   v[0]  where the sun is; the phase of the stars' twinkling; how far the
//         viewer went along the ranges
//   v[1]  how far the clouds, the mist and the waves went, toward the viewer
//         and along the shore

// The ranges, the farthest first: where they stand, how tall they are, how
// many mountains there are, and how fast they go by.
const int ranges = 6;
const float rangeBase[ranges] = float[](0.030, 0.012, -0.012, -0.050, -0.100, -0.172);
const float rangeTall[ranges] = float[](0.17, 0.20, 0.20, 0.19, 0.17, 0.075);
const float rangeScale[ranges] = float[](1.5, 1.7, 1.9, 2.1, 2.3, 3.6);
const float rangeSpeed[ranges] = float[](1.0, 1.6, 2.3, 3.1, 4.0, 5.0);
const vec3 rangeColor[ranges] = vec3[](vec3(0.40, 0.235, 0.140), vec3(0.235, 0.145, 0.095), vec3(0.120, 0.082, 0.062),
    vec3(0.052, 0.042, 0.037), vec3(0.019, 0.0175, 0.0175), vec3(0.0052, 0.0054, 0.0060));
const vec3 rangeMist[ranges] = vec3[](vec3(0.74, 0.44, 0.25), vec3(0.60, 0.36, 0.215), vec3(0.43, 0.27, 0.175),
    vec3(0.27, 0.185, 0.135), vec3(0.155, 0.118, 0.097), vec3(0.075, 0.063, 0.057));

// waterY is the shore, at the end of the water.
const float waterY = -0.205;

// ridge is the outline of a range, a sum of sharp crests, or, far, without the
// finer ones.
float ridge(float x, int seed, bool far) {
    float s = 0.0, a = 0.5;
    int n = far ? 2 : 6;
    for (int o = 0; o < n; o++) {
        float v = 1.0 - abs(2.0 * noise1(x, seed + 16 * o) - 1.0);
        s += a * (v * v);
        a *= 0.47;
        precise float next = x * 2.07 + 3.7;
        x = next;
    }
    return far ? s + 0.06 : s;
}

float rangeMistAt(float x, int i) { return noise1(x * 2.1 + u.v[1].y * (1.0 + 0.3 * float(i)) + 9.1 * float(i), 100 + i); }

// cloud and wisp are how much cloud there is at a point, and how much mist.
float cloud(vec2 p) {
    float v = 0.5 * fbm2(vec2(p.x * 2.2, p.y * 4.0), 2);
    return fbm2(vec2(p.x * 1.35 + u.v[1].x, p.y * 10.0 + 3.0 + v), 3);
}

float wisp(vec2 p) { return fbm2(vec2(p.x * 2.6 + 1.7 * u.v[1].y, p.y * 15.0 - u.v[1].y), 2) * 1.333333; }

vec3 sky(vec2 p, bool far) {
    float h = clamp1((p.y + 0.06) * 1.8);
    vec3 c = lerp(vec3(0.78, 0.36, 0.155), vec3(0.225, 0.150, 0.115), smoothTo(0.0, 0.40, h));
    c = lerp(c, vec3(0.011, 0.016, 0.0215), smoothTo(0.18, 0.95, h));
    vec2 q = p - u.v[0].xy;
    float d = sqrt(dot(q, q));
    c += vec3(1.0, 0.50, 0.20) * (0.62 * decay(d * 6.5) + 0.14 * decay(d * 2.6));
    if (far) return c;
    float dark = smoothTo(0.16, 0.46, p.y);
    if (dark > 0.0) {
        vec3 t = stars(p * 28.0, 0, 0.13, 70.0, u.v[0].z) + stars(p * 60.0, 1, 0.15, 42.0, u.v[0].z);
        c += t * (0.75 * dark * (1.0 - decay(d * 1.5)));
    }
    // The sun, behind a little of the haze near the horizon.
    c = lerp(c, vec3(3.2, 2.0, 1.05), clamp1((0.046 - d) * (0.5 * u.size.z) + 0.5));
    // Streaks of cloud, lit from below.
    float band = smoothTo(0.01, 0.14, p.y) * (1.0 - smoothTo(0.26, 0.48, p.y));
    if (band > 0.0) {
        float n = tileMix(vec3(cloud(tileCorner(0))), vec3(cloud(tileCorner(1))), vec3(cloud(tileCorner(2)))).x;
        c = lerp(c, lerp(vec3(0.105, 0.078, 0.068), vec3(1.15, 0.52, 0.22), decay(d * 3.1)), 0.72 * (smoothTo(0.42, 0.72, n) * band));
    }
    return c;
}

// land is the ranges before the sky, with outlines as soft as soft; far is
// what water shows of them, without detail.
vec3 land(vec2 p, float soft, bool far) {
    vec3 c = vec3(0.0);
    float sharp = 1.0 / soft, thin = far ? 0.32 : 1.0;
    float left = 1.0; // of what lies behind
    float mist = -1.0;
    float sun = decay(abs(p.x - u.v[0].x) * 3.2);
    for (int i = ranges - 1; i >= 0; i--) {
        if (p.y > rangeBase[i] + rangeTall[i] * 0.6 + soft) continue; // above it
        float k = float(i);
        // precise, here and in ridge: where it is far along the range, fusing
        // these would move the fine crests.
        precise float x = p.x * rangeScale[i] + u.v[0].w * rangeSpeed[i] + 7.3 * k;
        float height = rangeBase[i] + rangeTall[i] * (ridge(x, i, far) - 0.42);
        float cover = clamp1((height - p.y) * sharp + 0.5);
        if (cover <= 0.0) continue;
        if (mist < 0.0)
            mist = far ? 0.5 : tileMix(vec3(wisp(tileCorner(0))), vec3(wisp(tileCorner(1))), vec3(wisp(tileCorner(2)))).x;
        float up = max(p.y - rangeBase[i] + 0.085, 0.0);
        float fog = clamp1(thin * decay(up * (17.0 - 1.2 * k)) * (0.25 + 0.85 * rangeMistAt(p.x, i) + 0.75 * mist));
        vec3 s = lerp(rangeColor[i], rangeMist[i], fog) + vec3(0.55, 0.27, 0.11) * (sun * (0.42 - 0.08 * k));
        c += s * (left * cover);
        left *= 1.0 - cover;
        if (left <= 0.0) return c;
    }
    return c + sky(p, far) * left;
}

// water is the land upside down, in waves that the distance makes finer, and
// the path of the sun on them.
vec3 water(vec2 p, float pixel) {
    float dp = waterY - p.y;
    float near = clamp1(dp * 3.3);
    float z = 1.0 / sqrt(dp + 0.012);
    // How much of a wave a pixel covers: those smaller than it are not seen.
    float foot = 0.5 * (z * z) * z * pixel;
    float x = p.x * z, waves = u.v[1].z, drift = u.v[1].w;
    float w1 = noise2(vec2(x * 0.8 + drift, z * 9.0 + waves)) - 0.5;
    float w2 = noise2(vec2(x * 1.9 - drift, z * 21.0 + 1.5 * waves + 5.0)) - 0.5;
    float w3 = noise2(vec2(x * 4.4 + 1.7 * drift, z * 47.0 + 2.1 * waves + 9.0)) - 0.5;
    float fine = smoothTo(0.3, 0.9, foot * 21.0);
    w1 *= 1.0 - smoothTo(0.3, 0.9, foot * 9.0);
    w2 *= 1.0 - fine;
    w3 *= 1.0 - smoothTo(0.3, 0.9, foot * 47.0);
    float w = lerp(0.14, 0.50, fine) * w1 + 0.34 * w2 + 0.40 * w3;

    float up = dp * 0.95 + (0.60 * dp + 0.010) * w;
    vec3 c = land(vec2(p.x, waterY + up), pixel + 0.018 + 0.36 * dp, true);
    // The water darkens toward the viewer, who looks into it there, and the
    // back of the waves more.
    c *= lerp(0.78, 0.17, smoothTo(0.0, 0.9, near)) * (1.0 + 1.1 * w * near);
    c += vec3(0.0040, 0.0045, 0.0052);

    // The path of the sun.
    float side = abs(p.x - u.v[0].x) / (0.018 + 0.30 * dp);
    float glint = smoothTo(0.10, 0.34, 0.65 * w1 + 0.85 * w2 + 0.95 * w3 + 0.24 - 0.20 * side);
    c += vec3(1.35, 0.62, 0.24) * (glint * smoothTo(0.0, 0.05, dp) * lerp(0.95, 0.42, near));
    float glow = max(1.0 - side * side * 0.04, 0.0);
    c += vec3(0.55, 0.26, 0.11) * (0.16 * (glow * glow));
    // Mist at the shore.
    return c + vec3(0.20, 0.135, 0.098) * ((0.55 + 0.5 * rangeMistAt(p.x, 4)) * decay(dp * 60.0));
}

vec3 mountainsScene(vec2 p) {
    float pixel = 1.0 / u.size.z;
    return p.y >= waterY ? land(p, pixel, false) : water(p, pixel);
}
