import { describe, expect, it } from 'vitest';
import { formatBurnRate } from './formatters.js';

describe('formatBurnRate', () => {
  it('returns a placeholder for empty burn rates', () => {
    expect(formatBurnRate(0)).toBe('-');
    expect(formatBurnRate(null)).toBe('-');
  });

  it('formats millions with M suffix', () => {
    expect(formatBurnRate(1_250_000)).toBe('1.3M/min');
  });

  it('formats high thousands without a decimal', () => {
    expect(formatBurnRate(566093.5)).toBe('566K/min');
  });

  it('formats smaller thousands with one decimal', () => {
    expect(formatBurnRate(43955.1)).toBe('44.0K/min');
  });

  it('rounds sub-thousand rates to whole numbers', () => {
    expect(formatBurnRate(987.6)).toBe('988/min');
  });
});
