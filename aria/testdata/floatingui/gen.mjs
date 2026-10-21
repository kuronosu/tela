// Generates cases.json, the oracle of the Go port of Floating UI's
// computePosition and middleware (aria/floating_*.go): random rectangles,
// placements and middleware stacks, run through the real @floating-ui/core.
//
//   cd aria/testdata/floatingui
//   npm install            # @floating-ui/core 1.7.5 (see package.json)
//   node gen.mjs 2400 > cases.json
//
// The output is deterministic (a seeded generator). Coordinates are
// multiples of 0.25 so that float32 and float64 agree exactly.
//
// Floating UI is Copyright (c) 2021-present Floating UI contributors, MIT.

import {
  computePosition, offset, flip, shift, limitShift, size, arrow, hide, autoPlacement,
} from '@floating-ui/core';

function mulberry32(a) {
  return function () {
    a |= 0; a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const rng = mulberry32(20261021);
const rnd = () => rng();
const int = (lo, hi) => lo + Math.floor(rnd() * (hi - lo + 1));
const q = (lo, hi) => int(lo * 4, hi * 4) / 4;
const pick = (a) => a[int(0, a.length - 1)];
const chance = (p) => rnd() < p;

const placements = [
  'top', 'top-start', 'top-end', 'right', 'right-start', 'right-end',
  'bottom', 'bottom-start', 'bottom-end', 'left', 'left-start', 'left-end',
];

function randomRect(win, minW, maxW, minH, maxH) {
  const w = q(minW, maxW), h = q(minH, maxH);
  return [q(win.x - 20, win.x + win.w - w + 20), q(win.y - 20, win.y + win.h - h + 20), w, h];
}

function randomOffset() {
  const k = int(0, 3);
  if (k === 0) return q(-6, 24);
  if (k === 1) return { mainAxis: q(0, 16), crossAxis: q(-12, 12) };
  if (k === 2) return { mainAxis: q(0, 16), alignmentAxis: q(-12, 12) };
  return { mainAxis: q(0, 12), crossAxis: q(-8, 8), alignmentAxis: chance(0.5) ? q(-8, 8) : null };
}

function randomFlip() {
  const o = {};
  if (chance(0.5)) o.padding = q(0, 16);
  if (chance(0.2)) o.mainAxis = false;
  if (chance(0.25)) o.crossAxis = false;
  else if (chance(0.2)) o.crossAxis = 'alignment';
  if (chance(0.25)) {
    const n = int(0, 3);
    o.fallbackPlacements = Array.from({ length: n }, () => pick(placements));
  }
  if (chance(0.3)) o.fallbackStrategy = pick(['bestFit', 'initialPlacement']);
  if (chance(0.3)) o.fallbackAxisSideDirection = pick(['none', 'start', 'end']);
  if (chance(0.25)) o.flipAlignment = false;
  return o;
}

function randomShift() {
  const o = {};
  if (chance(0.7)) o.padding = q(0, 16);
  if (chance(0.15)) o.mainAxis = false;
  if (chance(0.3)) o.crossAxis = true;
  if (chance(0.4)) {
    const l = {};
    const k = int(0, 2);
    if (k === 1) l.offset = q(-10, 20);
    if (k === 2) l.offset = { mainAxis: q(-10, 20), crossAxis: q(-10, 20) };
    if (chance(0.2)) l.mainAxis = false;
    if (chance(0.2)) l.crossAxis = false;
    o.limiter = l;
  }
  return o;
}

function randomAuto() {
  const o = {};
  if (chance(0.4)) o.padding = q(0, 16);
  if (chance(0.3)) o.crossAxis = true;
  if (chance(0.3)) o.alignment = pick(['start', 'end']);
  if (chance(0.3)) {
    const n = int(1, 6);
    o.allowedPlacements = Array.from({ length: n }, () => pick(placements));
  }
  if (chance(0.25)) o.autoAlignment = false;
  return o;
}

function randomMiddleware(withArrow) {
  const list = [];
  const n = int(0, 5);
  for (let i = 0; i < n; i++) {
    const kind = pick(['offset', 'offset', 'flip', 'flip', 'shift', 'shift', 'size', 'hide', 'autoPlacement', 'none']);
    switch (kind) {
      case 'offset': list.push({ name: 'offset', options: randomOffset() }); break;
      case 'flip': list.push({ name: 'flip', options: randomFlip() }); break;
      case 'shift': list.push({ name: 'shift', options: randomShift() }); break;
      case 'size': list.push({ name: 'size', options: chance(0.4) ? { padding: q(0, 12) } : {} }); break;
      case 'hide': {
        const o = {};
        if (chance(0.5)) o.strategy = pick(['referenceHidden', 'escaped']);
        if (chance(0.3)) o.padding = q(0, 8);
        list.push({ name: 'hide', options: o });
        break;
      }
      case 'autoPlacement': if (chance(0.5)) list.push({ name: 'autoPlacement', options: randomAuto() }); break;
      default: break;
    }
  }
  if (withArrow) {
    const a = { name: 'arrow', options: chance(0.5) ? { padding: q(0, 12) } : {} };
    if (chance(0.85)) list.push(a); else list.splice(int(0, list.length), 0, a);
  }
  return list;
}

// The typical stacks, so that the common case is covered often.
function typicalMiddleware(withArrow) {
  const off = chance(0.8) ? [{ name: 'offset', options: q(2, 12) }] : [];
  const stack = [...off, { name: 'flip', options: chance(0.3) ? randomFlip() : {} }, { name: 'shift', options: chance(0.7) ? { padding: q(0, 12) } : randomShift() }];
  if (chance(0.3)) stack.splice(int(0, stack.length), 0, { name: 'size', options: {} });
  if (withArrow) stack.push({ name: 'arrow', options: chance(0.5) ? { padding: q(0, 10) } : {} });
  if (chance(0.15)) stack.push({ name: 'hide', options: {} });
  return stack;
}

function build(spec, ctx) {
  const o = spec.options ?? {};
  switch (spec.name) {
    case 'offset': return offset(o);
    case 'flip': return flip(o);
    case 'shift': return shift({ ...o, limiter: o.limiter ? limitShift(o.limiter) : undefined });
    case 'size': return size({
      ...o,
      apply: (a) => {
        ctx.sizeCalls.push({
          availableWidth: a.availableWidth,
          availableHeight: a.availableHeight,
          reference: [a.rects.reference.x, a.rects.reference.y, a.rects.reference.width, a.rects.reference.height],
          placement: a.placement,
        });
        if (ctx.emulate) ctx.limit = { w: Math.max(a.availableWidth, 0), h: Math.max(a.availableHeight, 0) };
      },
    });
    case 'arrow': return arrow({ ...o, element: ctx.arrowEl });
    case 'hide': return hide(o);
    case 'autoPlacement': return autoPlacement(o);
  }
  throw new Error('unknown middleware ' + spec.name);
}

async function run(c) {
  const ctx = { sizeCalls: [], emulate: !!c.emulate, limit: null, arrowEl: { id: 'arrow' } };
  const referenceEl = { id: 'reference' };
  const floatingEl = { id: 'floating' };
  const [rx, ry, rw, rh] = c.reference;
  const dims = () => {
    let [w, h] = c.floating;
    if (ctx.limit) { w = Math.min(w, ctx.limit.w); h = Math.min(h, ctx.limit.h); }
    return { width: w, height: h };
  };
  const rectOf = ([x, y, w, h]) => ({ x, y, width: w, height: h });
  const platform = {
    getElementRects: async () => ({ reference: { x: rx, y: ry, width: rw, height: rh }, floating: { x: 0, y: 0, ...dims() } }),
    getDimensions: async (el) => (el === ctx.arrowEl ? { width: c.arrow[0], height: c.arrow[1] } : dims()),
    isRTL: async () => c.rtl,
    getClippingRect: async ({ element }) => rectOf(element === floatingEl ? c.clip : c.referenceClip),
  };
  const middleware = c.middleware.map((m) => build(m, ctx));
  const r = await computePosition(referenceEl, floatingEl, { placement: c.placement, strategy: 'fixed', middleware, platform });
  const d = r.middlewareData;
  const out = { x: r.x, y: r.y, placement: r.placement, size: ctx.sizeCalls };
  if (d.arrow) out.arrow = { x: d.arrow.x, y: d.arrow.y, centerOffset: d.arrow.centerOffset, alignmentOffset: d.arrow.alignmentOffset };
  if (d.hide) out.hide = { referenceHidden: d.hide.referenceHidden, escaped: d.hide.escaped };
  if (ctx.limit) out.limit = [ctx.limit.w, ctx.limit.h];
  return out;
}

const cases = [];
const N = Number(process.argv[2] ?? 2400);
for (let i = 0; i < N; i++) {
  const win = { x: 0, y: 0, w: int(200, 1200), h: int(150, 900) };
  const hasArrow = chance(0.4);
  const typical = chance(0.5);
  const c = {
    id: i,
    reference: randomRect(win, 4, 200, 4, 80),
    floating: [q(20, 300), q(10, 400)],
    clip: [win.x, win.y, win.w, win.h],
    referenceClip: [win.x, win.y, win.w, win.h],
    rtl: chance(0.25),
    arrow: hasArrow ? [q(2, 20), q(2, 20)] : [0, 0],
    placement: pick(placements),
    emulate: false,
  };
  // An inner scroll container cuts the clipping rectangle sometimes.
  if (chance(0.25)) {
    const x = q(0, win.w / 2), y = q(0, win.h / 2);
    c.clip = [x, y, q(20, win.w - x), q(20, win.h - y)];
  }
  if (chance(0.3)) {
    const x = q(0, win.w / 2), y = q(0, win.h / 2);
    c.referenceClip = [x, y, q(10, win.w - x), q(10, win.h - y)];
  }
  c.middleware = typical ? typicalMiddleware(hasArrow) : randomMiddleware(hasArrow);
  // Size middleware with real limits: the floating element follows them.
  if (c.middleware.some((m) => m.name === 'size') && chance(0.4)) c.emulate = true;
  c.expect = await run(c);
  cases.push(c);
}

process.stdout.write(JSON.stringify({ floatingUI: '@floating-ui/core 1.7.5', cases }) + '\n');
