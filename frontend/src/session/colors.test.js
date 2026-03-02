import { describe, it, expect } from 'vitest';
import {
  MODEL_COLORS,
  DEFAULT_COLOR,
  getModelColor,
  hexToRgb,
  lightenHex,
  shortModelName,
} from './colors.js';

describe('getModelColor — exact model matches', () => {
  it('returns opus color for claude-opus-4-5-20251101', () => {
    const color = getModelColor('claude-opus-4-5-20251101', 'claude');
    expect(color).toBe(MODEL_COLORS['claude-opus-4-5-20251101']);
    expect(color.main).toBe('#a855f7');
  });

  it('returns sonnet color for claude-sonnet-4-5-20250929', () => {
    const color = getModelColor('claude-sonnet-4-5-20250929', 'claude');
    expect(color).toBe(MODEL_COLORS['claude-sonnet-4-5-20250929']);
    expect(color.main).toBe('#06b6d4');
  });

  it('returns haiku color for claude-haiku-4-5-20251001', () => {
    const color = getModelColor('claude-haiku-4-5-20251001', 'claude');
    expect(color).toBe(MODEL_COLORS['claude-haiku-4-5-20251001']);
    expect(color.main).toBe('#22c55e');
  });

  it('returns haiku color for claude-haiku-3-5-20241022', () => {
    const color = getModelColor('claude-haiku-3-5-20241022', 'claude');
    expect(color).toBe(MODEL_COLORS['claude-haiku-3-5-20241022']);
    expect(color.main).toBe('#22c55e');
  });

  it('returns sonnet color for claude-sonnet-4-20250514', () => {
    const color = getModelColor('claude-sonnet-4-20250514', 'claude');
    expect(color).toBe(MODEL_COLORS['claude-sonnet-4-20250514']);
    expect(color.main).toBe('#3b82f6');
  });
});

describe('getModelColor — fuzzy claude matches', () => {
  it('returns opus color for model containing "opus"', () => {
    const color = getModelColor('custom-opus-variant', 'claude');
    expect(color.main).toBe('#a855f7');
    expect(color.name).toBe('Opus');
  });

  it('returns sonnet color for model containing "sonnet"', () => {
    const color = getModelColor('my-sonnet-model', 'claude');
    expect(color.main).toBe('#06b6d4');
    expect(color.name).toBe('Sonnet');
  });

  it('returns haiku color for model containing "haiku"', () => {
    const color = getModelColor('custom-haiku', 'claude');
    expect(color.main).toBe('#22c55e');
    expect(color.name).toBe('Haiku');
  });
});

describe('getModelColor — gemini models', () => {
  it('returns blue color for gemini-2.0-flash', () => {
    const color = getModelColor('gemini-2.0-flash', 'gemini');
    expect(color.main).toBe('#4285f4');
    expect(color.name).toBe('Flash');
  });

  it('returns blue color for gemini-pro', () => {
    const color = getModelColor('gemini-pro', 'gemini');
    expect(color.main).toBe('#4285f4');
    expect(color.name).toBe('Pro');
  });

  it('returns blue color for generic gemini model', () => {
    const color = getModelColor('gemini-1.5-ultra', 'gemini');
    expect(color.main).toBe('#4285f4');
    expect(color.name).toBe('Gemini');
  });
});

describe('getModelColor — codex/openai models', () => {
  it('returns green color for o1-preview', () => {
    const color = getModelColor('o1-preview', 'codex');
    expect(color.main).toBe('#10b981');
  });

  it('returns green color for model containing "codex"', () => {
    const color = getModelColor('codex-davinci', 'codex');
    expect(color.main).toBe('#10b981');
  });

  it('returns green color for gpt-4 model', () => {
    const color = getModelColor('gpt-4', 'codex');
    expect(color.main).toBe('#10b981');
  });

  it('returns green color for o3-mini', () => {
    const color = getModelColor('o3-mini', 'codex');
    expect(color.main).toBe('#10b981');
  });
});

describe('getModelColor — fallbacks', () => {
  it('returns DEFAULT_COLOR for unknown model with no source', () => {
    const color = getModelColor('totally-unknown-model', undefined);
    expect(color.main).toBe(DEFAULT_COLOR.main);
  });

  it('returns DEFAULT_COLOR for unknown model with empty string source', () => {
    const color = getModelColor('unknown-xyz', '');
    expect(color.main).toBe(DEFAULT_COLOR.main);
  });

  it('returns source-based fallback when model is null and source provided', () => {
    const color = getModelColor(null, 'custom-source');
    expect(color.main).toBe(DEFAULT_COLOR.main);
    expect(color.name).toBe('CUSTOM-SOURCE');
  });

  it('returns DEFAULT_COLOR when both model and source are null', () => {
    const color = getModelColor(null, null);
    expect(color).toBe(DEFAULT_COLOR);
  });

  it('returns DEFAULT_COLOR when both model and source are undefined', () => {
    const color = getModelColor(undefined, undefined);
    expect(color).toBe(DEFAULT_COLOR);
  });
});

describe('hexToRgb', () => {
  it('parses black correctly', () => {
    expect(hexToRgb('#000000')).toEqual({ r: 0, g: 0, b: 0 });
  });

  it('parses white correctly', () => {
    expect(hexToRgb('#ffffff')).toEqual({ r: 255, g: 255, b: 255 });
  });

  it('parses a855f7 (opus purple) correctly', () => {
    expect(hexToRgb('#a855f7')).toEqual({ r: 168, g: 85, b: 247 });
  });

  it('parses 06b6d4 (sonnet cyan) correctly', () => {
    expect(hexToRgb('#06b6d4')).toEqual({ r: 6, g: 182, b: 212 });
  });

  it('parses 22c55e (haiku green) correctly', () => {
    expect(hexToRgb('#22c55e')).toEqual({ r: 34, g: 197, b: 94 });
  });

  it('returns individual r, g, b channels', () => {
    const result = hexToRgb('#1a2b3c');
    expect(result).toHaveProperty('r');
    expect(result).toHaveProperty('g');
    expect(result).toHaveProperty('b');
    expect(result.r).toBe(0x1a);
    expect(result.g).toBe(0x2b);
    expect(result.b).toBe(0x3c);
  });
});

describe('lightenHex', () => {
  it('brightens a dark color by the given amount', () => {
    const result = lightenHex('#000000', 50);
    expect(result).toBe('rgb(50,50,50)');
  });

  it('clamps channels at 255', () => {
    const result = lightenHex('#ffffff', 100);
    expect(result).toBe('rgb(255,255,255)');
  });

  it('brightens each channel independently', () => {
    const result = lightenHex('#1a2b3c', 10);
    expect(result).toBe(`rgb(${0x1a + 10},${0x2b + 10},${0x3c + 10})`);
  });

  it('returns an rgb() string', () => {
    const result = lightenHex('#336699', 20);
    expect(result).toMatch(/^rgb\(\d+,\d+,\d+\)$/);
  });
});

describe('shortModelName', () => {
  it('returns "?" for null', () => {
    expect(shortModelName(null)).toBe('?');
  });

  it('returns "?" for undefined', () => {
    expect(shortModelName(undefined)).toBe('?');
  });

  it('returns "?" for empty string', () => {
    expect(shortModelName('')).toBe('?');
  });

  it('abbreviates claude models to the first segment', () => {
    expect(shortModelName('claude-sonnet-4-5-20250929')).toBe('CLAUDE');
  });

  it('abbreviates haiku models to the first segment', () => {
    expect(shortModelName('claude-haiku-4-5-20251001')).toBe('CLAUDE');
  });

  it('formats gemini-2.0-flash with version and tier', () => {
    const name = shortModelName('gemini-2.0-flash');
    expect(name).toBe('G2.0F');
  });

  it('formats gemini-pro with version and tier', () => {
    const name = shortModelName('gemini-pro');
    expect(name).toBe('G');
  });

  it('formats gemini-1.5 with version only', () => {
    const name = shortModelName('gemini-1.5');
    expect(name).toBe('G1.5');
  });

  it('formats o-series models as uppercase', () => {
    expect(shortModelName('o1')).toBe('O1');
    expect(shortModelName('o3-mini')).toBe('O3');
  });

  it('formats gpt models correctly', () => {
    const name = shortModelName('gpt-4');
    expect(name).toBe('GPT4');
  });

  it('limits gpt model name to 6 characters', () => {
    const name = shortModelName('gpt-4-turbo');
    expect(name.length).toBeLessThanOrEqual(6);
  });

  it('truncates unknown model names to 6 characters', () => {
    const name = shortModelName('verylongmodelname');
    expect(name.length).toBeLessThanOrEqual(6);
    expect(name).toBe('VERYLO');
  });
});
