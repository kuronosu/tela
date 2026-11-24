// What the scenes are made of (Go: shade.go): sums, products, divisions, square
// roots and integer hashes, on which every GPU and the Go code agree.

float clamp1(float x) { return clamp(x, 0.0, 1.0); }

float lerp(float a, float b, float t) { return a + (b - a) * t; }

vec3 lerp(vec3 a, vec3 b, float t) { return a + (b - a) * t; }

// smoothTo goes from 0 at a to 1 at b.
float smoothTo(float a, float b, float x) {
    float t = clamp1((x - a) * (1.0 / (b - a)));
    return t * t * (3.0 - 2.0 * t);
}

// quint is the curve noises are interpolated with, dquint its derivative.
float quint(float t) { return t * t * t * (t * (t * 6.0 - 15.0) + 10.0); }

float dquint(float t) {
    float k = t * (t - 1.0);
    return 30.0 * (k * k);
}

// wave is nearly a sine of period 1 (a parabola per half).
float wave(float t) {
    t = 2.0 * (t - floor(t)) - 1.0;
    return 4.0 * t * (abs(t) - 1.0);
}

// cosine is a cosine of period 1, and sine its sine.
float cosine(float t) {
    t = 0.25 - abs(t - floor(t + 0.5));
    float q = t * t;
    return t * (6.283185 + q * (-41.34148 + q * (81.59658 + q * -74.59)));
}

float sine(float t) { return cosine(t - 0.25); }

// decay is nearly exp(-x) where x is small, and 0 from 16 on.
float decay(float x) {
    x = max(1.0 - x * 0.0625, 0.0);
    x *= x;
    x *= x;
    x *= x;
    return x * x;
}

// hash is a number in [0, 1) that looks random. A noise takes it of cells that
// are neighbors: their products differ by one of these numbers.
const uint hashX = 0x8da6b343u, hashY = 0xd8163841u, hashZ = 0xcb1ab31fu;

float hash(uint h) {
    h ^= h >> 13;
    h *= 0x5bd1e995u;
    return float(h >> 8) * (1.0 / 16777216.0);
}

float hash(int x, int y, int z) { return hash(uint(x) * hashX ^ uint(y) * hashY ^ uint(z) * hashZ); }

// noise1 is a value noise over a line, one for each seed.
float noise1(float x, int seed) {
    float f = floor(x);
    uint i = uint(int(f)) * hashX, j = uint(seed) * hashY;
    return lerp(hash(i ^ j), hash(i + hashX ^ j), quint(x - f));
}

// noiseSoft is the mean of noise1 for a seed and the next one, whose values are
// less often extreme.
float noiseSoft(float x, int seed) {
    float f = floor(x);
    uint i = uint(int(f)) * hashX, j = uint(seed) * hashY, i1 = i + hashX, j1 = j + hashY;
    return lerp(0.5 * (hash(i ^ j) + hash(i ^ j1)), 0.5 * (hash(i1 ^ j) + hash(i1 ^ j1)), quint(x - f));
}

// noise2 is a value noise over a plane.
float noise2(vec2 p) {
    vec2 f = floor(p);
    uint i = uint(int(f.x)) * hashX, j = uint(int(f.y)) * hashY, i1 = i + hashX, j1 = j + hashY;
    p -= f;
    float u = quint(p.x);
    return lerp(lerp(hash(i ^ j), hash(i1 ^ j), u), lerp(hash(i ^ j1), hash(i1 ^ j1), u), quint(p.y));
}

// noise2d is noise2 with its derivatives.
vec3 noise2d(vec2 p) {
    vec2 f = floor(p);
    uint i = uint(int(f.x)) * hashX, j = uint(int(f.y)) * hashY, i1 = i + hashX, j1 = j + hashY;
    p -= f;
    float a = hash(i ^ j), b = hash(i1 ^ j), c = hash(i ^ j1), d = hash(i1 ^ j1);
    float u = quint(p.x), w = quint(p.y);
    float k1 = b - a, k2 = c - a, k3 = a - b - c + d;
    return vec3(a + k1 * u + k2 * w + k3 * u * w, dquint(p.x) * (k1 + k3 * w), dquint(p.y) * (k2 + k3 * u));
}

// noise3 is a value noise over space.
float noise3(vec3 p) {
    vec3 f = floor(p);
    uint i = uint(int(f.x)) * hashX, j = uint(int(f.y)) * hashY, k = uint(int(f.z)) * hashZ;
    uint i1 = i + hashX, j1 = j + hashY, k1 = k + hashZ;
    p -= f;
    float u = quint(p.x), w = quint(p.y);
    float a = lerp(lerp(hash(i ^ j ^ k), hash(i1 ^ j ^ k), u), lerp(hash(i ^ j1 ^ k), hash(i1 ^ j1 ^ k), u), w);
    float b = lerp(lerp(hash(i ^ j ^ k1), hash(i1 ^ j ^ k1), u), lerp(hash(i ^ j1 ^ k1), hash(i1 ^ j1 ^ k1), u), w);
    return lerp(a, b, quint(p.z));
}

// fbm2 and fbm3 are sums of noises, each of twice the detail of the one
// before, and half as strong.
float fbm2(vec2 p, int n) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < n; i++) {
        s += a * noise2(p);
        p = vec2(1.6 * p.x + 1.2 * p.y + 3.1, 1.6 * p.y - 1.2 * p.x + 7.7);
        a *= 0.5;
    }
    return s;
}

float fbm3(vec3 p, int n) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < n; i++) {
        s += a * noise3(p);
        p = p * 2.03 + vec3(1.7, 9.2, 4.3);
        a *= 0.5;
    }
    return s;
}

// slopes is fbm2 with its derivatives.
vec3 slopes(vec2 p) {
    vec3 s = vec3(0.0);
    float a = 0.5;
    float xx = 1.0, xy = 0.0, yx = 0.0, yy = 1.0; // how p changes with the first one
    for (int i = 0; i < 3; i++) {
        vec3 o = noise2d(p);
        s += a * vec3(o.x, o.y * xx + o.z * yx, o.y * xy + o.z * yy);
        p = vec2(1.6 * p.x + 1.2 * p.y + 3.1, 1.6 * p.y - 1.2 * p.x + 7.7);
        float xx2 = 1.6 * xx + 1.2 * yx, xy2 = 1.6 * xy + 1.2 * yy;
        yx = 1.6 * yx - 1.2 * xx;
        yy = 1.6 * yy - 1.2 * xy;
        xx = xx2;
        xy = xy2;
        a *= 0.5;
    }
    return s;
}

// stars are the stars of a layer at a point of its grid, whose cells have a
// star with a probability of rare. size is how small they are, and twinkle the
// phase of their twinkling.
vec3 stars(vec2 p, int layer, float rare, float size, float twinkle) {
    vec2 f = floor(p);
    int i = int(f.x), j = int(f.y);
    float h = hash(i, j, layer + 11);
    if (h >= rare) return vec3(0.0);
    vec2 d = p - f - vec2(0.2 + 0.6 * hash(i, j, layer + 21), 0.2 + 0.6 * hash(i, j, layer + 31));
    float g = 1.0 - dot(d, d) * size;
    if (g <= 0.0) return vec3(0.0);
    g *= g;
    g *= g;
    float m = 1.0 - h / rare;
    vec3 c = lerp(vec3(1.0, 0.82, 0.62), vec3(0.78, 0.88, 1.0), hash(i, j, layer + 51)) * (0.15 + 1.1 * (m * m * m));
    return c * (g * (0.8 + 0.2 * wave(twinkle + hash(i, j, layer + 41))));
}

// What changes slowly over the picture is computed where tiles of some pixels
// meet, and interpolated between, over each half of a tile (Go: grid). tileAt
// is where this pixel is in its tile, and tileCorner where a corner of the tile
// is in the scene: 0 is the one of its half, 1 and 2 are the next one along x
// and along y, which both halves have.
vec2 tileAt() {
    int n = int(u.v[6].x);
    ivec2 c = ivec2(gl_FragCoord.xy);
    return (vec2(c - c / n * n) + 0.5) * (1.0 / float(n));
}

vec2 tileCorner(int corner) {
    int n = int(u.v[6].x);
    vec2 t = tileAt();
    int near = t.x + t.y < 1.0 ? 0 : 1;
    ivec2 d = corner == 0 ? ivec2(near) : corner == 1 ? ivec2(1 - near, near) : ivec2(near, 1 - near);
    vec2 c = vec2((ivec2(gl_FragCoord.xy) / n + d) * n);
    return vec2(c.x - 0.5 * u.size.x, 0.5 * u.size.y - c.y) * u.v[6].y;
}

// tileMix interpolates the values at the corners.
vec3 tileMix(vec3 a, vec3 x, vec3 y) {
    vec2 t = tileAt();
    if (t.x + t.y >= 1.0) t = 1.0 - t;
    return a + (x - a) * t.x + (y - a) * t.y;
}
