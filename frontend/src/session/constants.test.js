import { describe, it, expect } from 'vitest';
import {
  TERMINAL_ACTIVITIES,
  PIT_ACTIVITIES,
  DEFAULT_CONTEXT_WINDOW,
  DATA_FRESHNESS_MS,
  isTerminalActivity,
} from './constants.js';

describe('TERMINAL_ACTIVITIES', () => {
  it('contains complete', () => {
    expect(TERMINAL_ACTIVITIES.has('complete')).toBe(true);
  });

  it('contains errored', () => {
    expect(TERMINAL_ACTIVITIES.has('errored')).toBe(true);
  });

  it('contains lost', () => {
    expect(TERMINAL_ACTIVITIES.has('lost')).toBe(true);
  });

  it('does not contain thinking', () => {
    expect(TERMINAL_ACTIVITIES.has('thinking')).toBe(false);
  });

  it('does not contain tool_use', () => {
    expect(TERMINAL_ACTIVITIES.has('tool_use')).toBe(false);
  });

  it('does not contain idle', () => {
    expect(TERMINAL_ACTIVITIES.has('idle')).toBe(false);
  });

  it('does not contain waiting', () => {
    expect(TERMINAL_ACTIVITIES.has('waiting')).toBe(false);
  });

  it('does not contain starting', () => {
    expect(TERMINAL_ACTIVITIES.has('starting')).toBe(false);
  });
});

describe('isTerminalActivity', () => {
  it('returns true for complete', () => {
    expect(isTerminalActivity('complete')).toBe(true);
  });

  it('returns true for errored', () => {
    expect(isTerminalActivity('errored')).toBe(true);
  });

  it('returns true for lost', () => {
    expect(isTerminalActivity('lost')).toBe(true);
  });

  it('returns false for thinking', () => {
    expect(isTerminalActivity('thinking')).toBe(false);
  });

  it('returns false for tool_use', () => {
    expect(isTerminalActivity('tool_use')).toBe(false);
  });

  it('returns false for idle', () => {
    expect(isTerminalActivity('idle')).toBe(false);
  });

  it('returns false for waiting', () => {
    expect(isTerminalActivity('waiting')).toBe(false);
  });

  it('returns false for starting', () => {
    expect(isTerminalActivity('starting')).toBe(false);
  });

  it('returns false for undefined', () => {
    expect(isTerminalActivity(undefined)).toBe(false);
  });

  it('returns false for empty string', () => {
    expect(isTerminalActivity('')).toBe(false);
  });
});

describe('PIT_ACTIVITIES', () => {
  it('contains idle', () => {
    expect(PIT_ACTIVITIES.has('idle')).toBe(true);
  });

  it('contains waiting', () => {
    expect(PIT_ACTIVITIES.has('waiting')).toBe(true);
  });

  it('contains starting', () => {
    expect(PIT_ACTIVITIES.has('starting')).toBe(true);
  });

  it('does not contain thinking', () => {
    expect(PIT_ACTIVITIES.has('thinking')).toBe(false);
  });

  it('does not contain complete', () => {
    expect(PIT_ACTIVITIES.has('complete')).toBe(false);
  });
});

describe('DEFAULT_CONTEXT_WINDOW', () => {
  it('is 1000000', () => {
    expect(DEFAULT_CONTEXT_WINDOW).toBe(1000000);
  });
});

describe('DATA_FRESHNESS_MS', () => {
  it('is 30000', () => {
    expect(DATA_FRESHNESS_MS).toBe(30_000);
  });
});
