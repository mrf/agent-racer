import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  formatTokens,
  formatBurnRate,
  formatTime,
  formatElapsed,
  basename,
  esc,
} from './formatters.js';

describe('formatTokens', () => {
  it('returns raw number below 1000', () => {
    expect(formatTokens(0)).toBe('0');
    expect(formatTokens(1)).toBe('1');
    expect(formatTokens(999)).toBe('999');
  });

  it('returns K suffix at exactly 1000', () => {
    expect(formatTokens(1000)).toBe('1K');
  });

  it('rounds to nearest K for values above 1000', () => {
    expect(formatTokens(1499)).toBe('1K');
    expect(formatTokens(1500)).toBe('2K');
    expect(formatTokens(54321)).toBe('54K');
  });

  it('handles large values in K (no M suffix)', () => {
    // NOTE: unlike TeamGarage.formatTokens, ui/formatters has no millions tier
    expect(formatTokens(1_000_000)).toBe('1000K');
    expect(formatTokens(2_500_000)).toBe('2500K');
  });
});

describe('formatBurnRate', () => {
  it('returns dash for falsy or non-positive values', () => {
    expect(formatBurnRate(0)).toBe('-');
    expect(formatBurnRate(null)).toBe('-');
    expect(formatBurnRate(undefined)).toBe('-');
    expect(formatBurnRate(-5)).toBe('-');
  });

  it('formats millions with M/min suffix', () => {
    expect(formatBurnRate(1_000_000)).toBe('1.0M/min');
    expect(formatBurnRate(1_250_000)).toBe('1.3M/min');
    expect(formatBurnRate(9_999_999)).toBe('10.0M/min');
  });

  it('formats 100K-999K as rounded K without decimal', () => {
    expect(formatBurnRate(100_000)).toBe('100K/min');
    expect(formatBurnRate(566_093.5)).toBe('566K/min');
    expect(formatBurnRate(999_999)).toBe('1000K/min');
  });

  it('formats 1K-99K with one decimal place', () => {
    expect(formatBurnRate(1000)).toBe('1.0K/min');
    expect(formatBurnRate(43_955.1)).toBe('44.0K/min');
    expect(formatBurnRate(99_999)).toBe('100.0K/min');
  });

  it('rounds sub-thousand rates to whole numbers', () => {
    expect(formatBurnRate(1)).toBe('1/min');
    expect(formatBurnRate(500)).toBe('500/min');
    expect(formatBurnRate(987.6)).toBe('988/min');
    expect(formatBurnRate(999)).toBe('999/min');
  });
});

describe('formatTime', () => {
  it('returns dash for falsy input', () => {
    expect(formatTime(null)).toBe('-');
    expect(formatTime(undefined)).toBe('-');
    expect(formatTime('')).toBe('-');
  });

  it('formats a valid ISO date string via toLocaleTimeString', () => {
    const result = formatTime('2026-03-27T14:30:00Z');
    // toLocaleTimeString is locale-dependent, so just verify it returns
    // a non-empty string that is not the dash placeholder
    expect(result).not.toBe('-');
    expect(result.length).toBeGreaterThan(0);
  });
});

describe('formatElapsed', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns dash for falsy input', () => {
    expect(formatElapsed(null)).toBe('-');
    expect(formatElapsed(undefined)).toBe('-');
    expect(formatElapsed('')).toBe('-');
  });

  it('formats elapsed seconds and minutes', () => {
    const now = new Date('2026-03-27T12:05:30Z');
    vi.setSystemTime(now);

    // 90 seconds ago => 1m 30s
    expect(formatElapsed('2026-03-27T12:04:00Z')).toBe('1m 30s');
  });

  it('formats zero elapsed time', () => {
    const now = new Date('2026-03-27T12:00:00Z');
    vi.setSystemTime(now);

    expect(formatElapsed('2026-03-27T12:00:00Z')).toBe('0m 0s');
  });

  it('formats large elapsed times', () => {
    const now = new Date('2026-03-27T14:00:00Z');
    vi.setSystemTime(now);

    // 2 hours = 120m 0s
    expect(formatElapsed('2026-03-27T12:00:00Z')).toBe('120m 0s');
  });
});

describe('basename', () => {
  it('returns the last segment of a Unix path', () => {
    expect(basename('/home/user/file.txt')).toBe('file.txt');
    expect(basename('/a/b/c')).toBe('c');
  });

  it('returns the string itself when no slashes', () => {
    expect(basename('file.txt')).toBe('file.txt');
  });

  it('handles trailing slash by returning empty string', () => {
    expect(basename('/home/user/')).toBe('');
  });

  it('handles single slash', () => {
    expect(basename('/')).toBe('');
  });
});

describe('esc', () => {
  it('returns empty string for falsy input', () => {
    expect(esc(null)).toBe('');
    expect(esc(undefined)).toBe('');
    expect(esc('')).toBe('');
    expect(esc(0)).toBe('');
  });

  it('escapes ampersands', () => {
    expect(esc('a&b')).toBe('a&amp;b');
  });

  it('escapes angle brackets', () => {
    expect(esc('<script>')).toBe('&lt;script&gt;');
  });

  it('escapes double quotes', () => {
    expect(esc('"hello"')).toBe('&quot;hello&quot;');
  });

  it('escapes single quotes', () => {
    expect(esc("it's")).toBe('it&#39;s');
  });

  it('escapes all special characters together', () => {
    expect(esc('<a href="x">&\'</a>')).toBe(
      '&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;'
    );
  });

  it('passes through safe strings unchanged', () => {
    expect(esc('hello world 123')).toBe('hello world 123');
  });

  it('coerces non-string input to string', () => {
    expect(esc(42)).toBe('42');
    expect(esc(true)).toBe('true');
  });
});

// --- Duplication note ---
// canvas/TeamGarage.js contains private formatTokens and formatBurnRate functions
// that diverge from ui/formatters.js:
//
// formatTokens differences:
//   - TeamGarage adds a >= 1_000_000 tier returning "X.XM" (ui/ has no millions tier)
//
// formatBurnRate differences:
//   - TeamGarage returns '' (empty string) for falsy rates; ui/ returns '-'
//   - TeamGarage uses '/m' suffix; ui/ uses '/min'
//   - TeamGarage has only 2 tiers (>=1000 K/m, else /m); ui/ has 4 tiers
//
// Recommendation: deduplicate by reusing ui/formatters.js in TeamGarage, adding
// the millions tier to formatTokens, and parameterizing suffix/placeholder if needed.
