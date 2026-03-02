import { describe, it, expect, vi, beforeEach } from 'vitest';

// auth.js captures the token at module load time, so we need a fresh import
// per test to control `location.search` before evaluation.
async function loadAuth(search = '') {
  vi.resetModules();
  vi.stubGlobal('location', { search });
  return import('./auth.js');
}

beforeEach(() => {
  vi.restoreAllMocks();
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response()));
});

describe('getAuthToken', () => {
  it('returns token from URL search params', async () => {
    const { getAuthToken } = await loadAuth('?token=abc123');
    expect(getAuthToken()).toBe('abc123');
  });

  it('returns empty string when no token param is present', async () => {
    const { getAuthToken } = await loadAuth('');
    expect(getAuthToken()).toBe('');
  });

  it('returns empty string when search has other params but no token', async () => {
    const { getAuthToken } = await loadAuth('?foo=bar&baz=1');
    expect(getAuthToken()).toBe('');
  });

  it('handles token with special characters', async () => {
    const { getAuthToken } = await loadAuth('?token=abc%3D%3D123');
    expect(getAuthToken()).toBe('abc==123');
  });
});

describe('authFetch', () => {
  it('adds Authorization header when token is present', async () => {
    const { authFetch } = await loadAuth('?token=secret');

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
    expect(new Headers(options.headers).get('Authorization')).toBe('Bearer secret');
  });

  it('does not add Authorization header when token is absent', async () => {
    const { authFetch } = await loadAuth('');

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
    // options may be the original empty object — no headers injected
    const headers = new Headers(options?.headers);
    expect(headers.get('Authorization')).toBeNull();
  });

  it('preserves existing headers when adding auth', async () => {
    const { authFetch } = await loadAuth('?token=tk');

    await authFetch('/api/data', {
      headers: { 'Content-Type': 'application/json' },
    });

    const [, options] = fetch.mock.calls[0];
    const headers = new Headers(options.headers);
    expect(headers.get('Authorization')).toBe('Bearer tk');
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('passes through options like method and body', async () => {
    const { authFetch } = await loadAuth('?token=tk');

    await authFetch('/api/data', { method: 'POST', body: '{}' });

    const [, options] = fetch.mock.calls[0];
    expect(options.method).toBe('POST');
    expect(options.body).toBe('{}');
  });

  it('works with no options argument', async () => {
    const { authFetch } = await loadAuth('?token=tk');

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
  });

  it('returns the fetch response', async () => {
    const mockResponse = new Response('ok', { status: 200 });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

    const { authFetch } = await loadAuth('?token=tk');
    const result = await authFetch('/api/data');

    expect(result).toBe(mockResponse);
  });
});
