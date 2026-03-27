import { describe, it, expect, vi, afterEach } from 'vitest';
import { TEMPLATES, pickTemplate, fillTemplate } from './templates.js';

describe('pickTemplate', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns a string from the pool for a known trigger', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const result = pickTemplate('session_start');
    expect(result).toBe(TEMPLATES.session_start[0]);
  });

  it('uses Math.random to select the template index', () => {
    // random() = 0.99 → floor(0.99 * 5) = 4 for a 5-element pool
    vi.spyOn(Math, 'random').mockReturnValue(0.99);
    const result = pickTemplate('overtake');
    expect(result).toBe(TEMPLATES.overtake[TEMPLATES.overtake.length - 1]);
  });

  it('returns null for an unknown trigger type', () => {
    expect(pickTemplate('nonexistent_trigger')).toBeNull();
  });

  it('returns a template from every known trigger type', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    for (const trigger of Object.keys(TEMPLATES)) {
      const result = pickTemplate(trigger);
      expect(result).toBe(TEMPLATES[trigger][0]);
    }
  });
});

describe('fillTemplate', () => {
  it('substitutes {name} placeholder', () => {
    expect(fillTemplate('{name} is racing!', { name: 'Alice' }))
      .toBe('Alice is racing!');
  });

  it('substitutes {other} placeholder', () => {
    expect(fillTemplate('{name} overtakes {other}!', { name: 'A', other: 'B' }))
      .toBe('A overtakes B!');
  });

  it('leaves unknown placeholders intact', () => {
    expect(fillTemplate('{name} and {unknown}', { name: 'Alice' }))
      .toBe('Alice and {unknown}');
  });

  it('handles template with no placeholders', () => {
    expect(fillTemplate('No placeholders here', { name: 'X' }))
      .toBe('No placeholders here');
  });

  it('replaces multiple occurrences of the same placeholder', () => {
    expect(fillTemplate('{name} vs {name}', { name: 'Z' }))
      .toBe('Z vs Z');
  });
});
