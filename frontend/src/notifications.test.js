import { describe, it, expect, vi, beforeEach } from 'vitest';

// Fresh module import per test to reset `permissionGranted` state
async function loadModule() {
  vi.resetModules();
  return import('./notifications.js');
}

function stubNotification(permission, requestResult) {
  const ctor = Object.assign(vi.fn(), {
    permission,
    requestPermission: requestResult !== undefined
      ? vi.fn().mockResolvedValue(requestResult)
      : vi.fn(),
  });
  globalThis.Notification = ctor;
  return ctor;
}

beforeEach(() => {
  vi.restoreAllMocks();
  delete globalThis.Notification;
  // The production code checks `'Notification' in window`, so provide window
  globalThis.window = globalThis;
});

describe('requestPermission', () => {
  it('does nothing when Notification API is not available', async () => {
    const { requestPermission } = await loadModule();
    await requestPermission();
  });

  it('sets permissionGranted when already granted', async () => {
    stubNotification('granted');

    const { requestPermission, notifyCompletion } = await loadModule();
    await requestPermission();

    // Verify permission was recorded by calling notifyCompletion
    notifyCompletion('test', 'complete');
    expect(Notification).toHaveBeenCalledWith(
      'Session Complete: test',
      { body: 'test finished successfully', tag: 'race-test' },
    );
  });

  it('requests permission and records granted result', async () => {
    stubNotification('default', 'granted');

    const { requestPermission, notifyCompletion } = await loadModule();
    await requestPermission();

    notifyCompletion('agent', 'complete');
    expect(Notification).toHaveBeenCalled();
  });

  it('requests permission and records denied result', async () => {
    stubNotification('default', 'denied');

    const { requestPermission, notifyCompletion } = await loadModule();
    await requestPermission();

    notifyCompletion('agent', 'complete');
    expect(Notification).not.toHaveBeenCalled();
  });

  it('does not request permission when already denied', async () => {
    const ctor = stubNotification('denied');

    const { requestPermission } = await loadModule();
    await requestPermission();
    expect(ctor.requestPermission).not.toHaveBeenCalled();
  });
});

describe('notifyCompletion', () => {
  async function setupGranted() {
    stubNotification('granted');
    const mod = await loadModule();
    await mod.requestPermission();
    return mod;
  }

  it('does nothing when permission was not granted', async () => {
    stubNotification('default', 'denied');

    const { requestPermission, notifyCompletion } = await loadModule();
    await requestPermission();
    notifyCompletion('bot', 'complete');
    expect(Notification).not.toHaveBeenCalled();
  });

  it('constructs correct title and body for complete activity', async () => {
    const { notifyCompletion } = await setupGranted();
    notifyCompletion('alpha', 'complete');

    expect(Notification).toHaveBeenCalledWith(
      'Session Complete: alpha',
      { body: 'alpha finished successfully', tag: 'race-alpha' },
    );
  });

  it('constructs correct title and body for errored activity', async () => {
    const { notifyCompletion } = await setupGranted();
    notifyCompletion('beta', 'errored');

    expect(Notification).toHaveBeenCalledWith(
      'Session Error: beta',
      { body: 'beta encountered an error', tag: 'race-beta' },
    );
  });

  it('constructs correct title and body for lost activity', async () => {
    const { notifyCompletion } = await setupGranted();
    notifyCompletion('gamma', 'lost');

    expect(Notification).toHaveBeenCalledWith(
      'Session Lost: gamma',
      { body: 'gamma disappeared or crashed', tag: 'race-gamma' },
    );
  });

  it('catches errors from Notification constructor', async () => {
    globalThis.Notification = Object.assign(
      vi.fn(() => { throw new Error('not allowed'); }),
      { permission: 'granted', requestPermission: vi.fn() },
    );

    const { requestPermission, notifyCompletion } = await loadModule();
    await requestPermission();

    expect(() => notifyCompletion('x', 'complete')).not.toThrow();
  });
});
