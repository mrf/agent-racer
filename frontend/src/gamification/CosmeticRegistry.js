// CosmeticRegistry maps reward IDs to render parameters.
// Consumed by Racer.js (paint, body), Particles.js (trail), Dashboard.js (badge, title).
// Hydrates equipped state from the backend /api/stats endpoint on page load.
import { authFetch } from '../auth.js';

// ── Paint definitions ──────────────────────────────────────────────────
const PAINTS = {
  rookie_paint:      { fill: '#4a4a4a', stroke: '#333333', pattern: null },
  matte_black_paint: { fill: '#1a1a1a', stroke: '#0a0a0a', pattern: null },
  century_paint:     { fill: '#c0c0c0', stroke: '#808080', pattern: 'chrome' },
  chrome_paint:      { fill: '#c0c0c0', stroke: '#808080', pattern: 'chrome' },
  gemini_paint:      { fill: '#00bcd4', stroke: '#008a9e', pattern: null },
  codex_paint:       { fill: '#4caf50', stroke: '#357a38', pattern: null },
  deep_purple_paint: { fill: '#673ab7', stroke: '#4a2a85', pattern: null },
  electric_blue_paint: { fill: '#2196f3', stroke: '#1769aa', pattern: null },
  lime_green_paint:  { fill: '#8bc34a', stroke: '#618834', pattern: null },
  endurance_orange_paint: { fill: '#ff9800', stroke: '#b36a00', pattern: null },
  racing_stripe_paint: { fill: null, stroke: null, pattern: 'racing_stripe', stripeColor: '#ffffff' },
  gold_stripe_paint: { fill: null, stroke: null, pattern: 'gold_stripe', stripeColor: '#ffd700' },
  holographic_paint: { fill: null, stroke: null, pattern: 'holographic' },
  spectrum_paint:    { fill: '#ff6fd8', stroke: '#be3fff', pattern: 'gradient' },
  clean_sweep_paint: { fill: '#e94560', stroke: '#a8324a', pattern: 'stripe' },
  metallic_paint:    { fill: '#b0c4de', stroke: '#7a8ea8', pattern: 'metallic' },
};

// ── Trail definitions ──────────────────────────────────────────────────
// Fields override the base particle config in Particles.js createParticle.
// If `preset` is set, the particle system uses that preset instead of overriding.
const TRAILS = {
  home_trail: {
    color: { r: 168, g: 85, b: 247 },
    colorEnd: { r: 124, g: 58, b: 237 },
    size: 5,
    decay: 0.018,
    sizeMultiplier: 'bloom',
  },
  haiku_trail: {
    color: { r: 34, g: 197, b: 94 },
    colorEnd: { r: 22, g: 163, b: 74 },
    size: 4,
    decay: 0.025,
    sizeMultiplier: 'bloom',
  },
  redline_trail: {
    color: { r: 255, g: 60, b: 60 },
    colorEnd: { r: 200, g: 30, b: 30 },
    size: 3,
    decay: 0.03,
    gravity: 0.04,
    layer: 'front',
  },
  on_a_roll_trail: {
    color: { r: 255, g: 215, b: 0 },
    colorEnd: { r: 200, g: 160, b: 0 },
    size: 5,
    decay: 0.02,
    sizeMultiplier: 'bloom',
  },
  crash_survivor_trail: {
    color: { r: 255, g: 140, b: 20 },
    colorEnd: { r: 180, g: 60, b: 10 },
    size: 6,
    decay: 0.015,
    sizeMultiplier: 'bloom',
    gravity: 0.02,
  },
  spark_trail: {
    color: { r: 255, g: 255, b: 100 },
    colorEnd: { r: 255, g: 120, b: 20 },
    size: 2,
    decay: 0.04,
    gravity: 0.05,
    layer: 'front',
  },
  flame_trail: {
    color: { r: 255, g: 200, b: 50 },
    colorEnd: { r: 255, g: 80, b: 20 },
    size: 6,
    decay: 0.02,
    sizeMultiplier: 'bloom',
  },
  blue_flame_trail: { preset: 'blueFlame' },
  red_sparks_trail: { preset: 'redSparks' },
  rainbow_trail: { preset: 'rainbow' },
  afterburn_trail: { preset: 'afterburn' },
  prismatic_trail: { preset: 'prismatic' },
  confetti_burst_trail: { preset: 'confettiBurst' },
  tire_smoke_trail: { preset: 'tireSmoke' },
  snowfall_trail: { preset: 'snowfall' },
  sakura_trail: { preset: 'sakura' },
  autumn_trail: { preset: 'autumn' },
};

// ── Body definitions ───────────────────────────────────────────────────
// Each body returns an array of polygon vertices as { x, y } offsets from car center.
// Coordinates use the same scale as Racer.js drawCar (pre-CAR_SCALE transform).
// L is the LIMO_STRETCH value, passed at render time.
const BODIES = {
  triple_body: (L) => [
    // Formula style: low, wide, aggressive
    { x: -17 - L, y: 2 },
    { x: -17 - L, y: -1 },
    { x: -14 - L, y: -5 },
    { x: -4 - L, y: -6 },
    { x: 3, y: -6 },
    { x: 12, y: -4 },
    { x: 19, y: -2 },
    { x: 23, y: 0 },
    { x: 21, y: 1 },
    { x: 18, y: 2 },
  ],
  connoisseur_body: (L) => [
    // Luxury sedan: longer, rounded, refined
    { x: -18 - L, y: 2 },
    { x: -18 - L, y: -2 },
    { x: -15 - L, y: -6 },
    { x: -6 - L, y: -10 },
    { x: 2, y: -10 },
    { x: 8, y: -7 },
    { x: 14, y: -4 },
    { x: 20, y: -2 },
    { x: 22, y: 0 },
    { x: 20, y: 1 },
    { x: 18, y: 2 },
  ],
  tool_fiend_body: (L) => [
    // Armored: angular, aggressive
    { x: -17 - L, y: 2 },
    { x: -18 - L, y: -1 },
    { x: -15 - L, y: -8 },
    { x: -10 - L, y: -10 },
    { x: -2 - L, y: -10 },
    { x: 4, y: -10 },
    { x: 10, y: -8 },
    { x: 16, y: -4 },
    { x: 21, y: -1 },
    { x: 23, y: 0 },
    { x: 21, y: 1 },
    { x: 18, y: 2 },
  ],
  aero_body: (L) => [
    // Aero: sleek, tapered, aerodynamic
    { x: -16 - L, y: 2 },
    { x: -17 - L, y: 0 },
    { x: -14 - L, y: -6 },
    { x: -5 - L, y: -8 },
    { x: 3, y: -8 },
    { x: 11, y: -5 },
    { x: 18, y: -2 },
    { x: 24, y: 0 },
    { x: 22, y: 1 },
    { x: 18, y: 2 },
  ],
};

// ── Badge definitions ──────────────────────────────────────────────────
// Each badge returns { emoji, label } for rendering next to session names.
// A future version may add sprite paths.
const BADGES = {
  pit_badge:        { emoji: '\u{1F3CE}', label: 'Pit' },       // racing car
  bronze_badge:     { emoji: '\u{1F949}', label: 'Bronze' },     // 3rd place medal
  silver_badge:     { emoji: '\u{1F948}', label: 'Silver' },     // 2nd place medal
  gold_badge:       { emoji: '\u{1F947}', label: 'Gold' },       // 1st place medal
  collector_badge:  { emoji: '\u{1F4E6}', label: 'Collector' },  // package
  grid_badge:       { emoji: '\u{1F3C1}', label: 'Grid' },       // checkered flag
  hat_trick_badge:  { emoji: '\u2B50\u2B50\u2B50', label: 'Hat Trick' },
};

// ── Registry state ─────────────────────────────────────────────────────
let equipped = {
  paint: '',
  trail: '',
  body: '',
  badge: '',
  sound: '',
  theme: '',
  title: '',
};

let hydrated = false;
const listeners = new Set();

function notifyListeners() {
  for (const fn of listeners) fn(equipped);
}

// ── Public API ─────────────────────────────────────────────────────────

/**
 * Returns paint render params for the equipped paint, or null if no paint equipped.
 * @param {string} [id] - Reward ID. Defaults to currently equipped paint.
 * @returns {{ fill: string, stroke: string, pattern: string|null } | null}
 */
export function getEquippedPaint(id) {
  const key = id ?? equipped.paint;
  return PAINTS[key] ?? null;
}

/**
 * Returns particle config overrides for the equipped trail, or null if none.
 * Merge the returned object into a base particle to apply the trail.
 * @param {string} [id] - Reward ID. Defaults to currently equipped trail.
 * @returns {object|null}
 */
export function getEquippedTrail(id) {
  const key = id ?? equipped.trail;
  const trail = TRAILS[key];
  return trail ? { ...trail } : null;
}

/**
 * Returns body polygon vertices for the equipped body, or null for default.
 *
 * Supports two calling conventions:
 *   getEquippedBody(id, L) - look up a specific body
 *   getEquippedBody(L)     - use the currently equipped body
 *
 * @param {string|number} [id] - Reward ID, or LIMO_STRETCH if called without an id.
 * @param {number} [L] - LIMO_STRETCH value from Racer.js (when id is provided).
 * @returns {Array<{x: number, y: number}>|null}
 */
export function getEquippedBody(id, L) {
  const key = typeof id === 'string' ? id : equipped.body;
  const limoStretch = typeof id === 'number' ? id : L;
  const factory = BODIES[key];
  return factory ? factory(limoStretch ?? 0) : null;
}

/**
 * Returns badge info for the equipped badge, or null if none.
 * @param {string} [id] - Reward ID. Defaults to currently equipped badge.
 * @returns {{ emoji: string, label: string } | null}
 */
export function getEquippedBadge(id) {
  const key = id ?? equipped.badge;
  return BADGES[key] ?? null;
}

/**
 * Returns the equipped title string, or empty string if none.
 * @returns {string}
 */
export function getEquippedTitle() {
  return equipped.title || '';
}

/**
 * Returns the full equipped loadout (all slots).
 * @returns {Readonly<typeof equipped>}
 */
export function getEquippedLoadout() {
  return { ...equipped };
}

/**
 * Updates the local equipped state. Called when the user equips/unequips
 * a cosmetic via the RewardSelector or when receiving a WebSocket update.
 * @param {Partial<typeof equipped>} slots
 */
export function setEquipped(slots) {
  Object.assign(equipped, slots);
  notifyListeners();
}

/**
 * Subscribe to equipped state changes.
 * @param {(equipped: typeof equipped) => void} fn
 * @returns {() => void} Unsubscribe function.
 */
export function onEquippedChange(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/**
 * Fetch equipped state from the backend and populate the registry.
 * Call once on page load. Safe to call multiple times (idempotent after first success).
 */
export async function hydrate() {
  if (hydrated) return;
  try {
    const resp = await authFetch('/api/stats');
    if (!resp.ok) return;
    const stats = await resp.json();
    if (stats.equipped) {
      Object.assign(equipped, stats.equipped);
      hydrated = true;
      notifyListeners();
    }
  } catch {
    // Silently fail; consumers fall back to defaults.
  }
}

/**
 * Returns true once hydration has completed successfully.
 */
export function isHydrated() {
  return hydrated;
}
