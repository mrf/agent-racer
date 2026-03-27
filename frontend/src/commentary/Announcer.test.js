import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Announcer } from './Announcer.js';

describe('Announcer', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-03-07T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts with no message', () => {
    const announcer = new Announcer();
    expect(announcer._message).toBeNull();
    expect(announcer._alpha).toBe(0);
  });

  it('setMessage sets message and alpha to 1', () => {
    const announcer = new Announcer();
    announcer.setMessage('Hello race fans!');
    expect(announcer._message).toBe('Hello race fans!');
    expect(announcer._alpha).toBe(1);
  });

  it('setMessage resets lines for recalculation', () => {
    const announcer = new Announcer();
    announcer._lines = ['old line'];
    announcer.setMessage('New message');
    expect(announcer._lines).toEqual([]);
  });

  it('update is a no-op when no message is set', () => {
    const announcer = new Announcer();
    announcer.update();
    expect(announcer._message).toBeNull();
    expect(announcer._alpha).toBe(0);
  });

  it('alpha stays 1 during the display period', () => {
    const announcer = new Announcer();
    announcer.setMessage('Visible');

    // Advance 7 seconds (under 8s display duration)
    vi.advanceTimersByTime(7000);
    announcer.update();

    expect(announcer._alpha).toBe(1);
    expect(announcer._message).toBe('Visible');
  });

  it('alpha fades after display duration expires', () => {
    const announcer = new Announcer();
    announcer.setMessage('Fading');

    // Advance past 8s display + halfway through 500ms fade
    vi.advanceTimersByTime(8250);
    announcer.update();

    expect(announcer._alpha).toBeCloseTo(0.5, 1);
    expect(announcer._message).toBe('Fading');
  });

  it('clears message when fade completes', () => {
    const announcer = new Announcer();
    announcer.setMessage('Gone');

    // Advance past 8s display + full 500ms fade
    vi.advanceTimersByTime(8600);
    announcer.update();

    expect(announcer._alpha).toBe(0);
    expect(announcer._message).toBeNull();
  });

  it('setMessage during fade resets to full alpha', () => {
    const announcer = new Announcer();
    announcer.setMessage('First');

    vi.advanceTimersByTime(8300);
    announcer.update();
    expect(announcer._alpha).toBeLessThan(1);

    announcer.setMessage('Second');
    expect(announcer._alpha).toBe(1);
    expect(announcer._message).toBe('Second');
  });
});
