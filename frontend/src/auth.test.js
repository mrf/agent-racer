import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const AUTH_TOKEN_STORAGE_KEY = 'agent-racer-auth-token';

function makeSessionStorage(initial = {}) {
  const map = new Map(Object.entries(initial));
  return {
    getItem(key) {
      return map.has(key) ? map.get(key) : null;
    },
    setItem(key, value) {
      map.set(key, String(value));
    },
    removeItem(key) {
      map.delete(key);
    },
    clear() {
      map.clear();
    },
  };
}

// auth.js captures the token at module load time, so we need a fresh import
// per test to control URL and storage before evaluation.
async function loadAuth({ pathname = '/', search = '', hash = '', storedToken = '' } = {}) {
  vi.resetModules();

  const replaceState = vi.fn();
  vi.stubGlobal('location', { pathname, search, hash });
  vi.stubGlobal('history', { state: {}, replaceState });

  const initialStorage = {};
  if (storedToken) {
    initialStorage[AUTH_TOKEN_STORAGE_KEY] = storedToken;
  }
  const sessionStorage = makeSessionStorage(initialStorage);
  vi.stubGlobal('sessionStorage', sessionStorage);

  const mod = await import('./auth.js');
  return { ...mod, replaceState, sessionStorage };
}

beforeEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response()));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getAuthToken', () => {
  it('returns token from URL hash and scrubs it from browser URL', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState, sessionStorage } = await loadAuth({
      pathname: '/dashboard',
      hash: '#token=abc123',
    });
    expect(getAuthToken()).toBe('abc123');
    expect(sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe('abc123');
    expect(warn).toHaveBeenCalledWith(
      'Auth token was read from the URL. Avoid passing credentials in the URL when possible. The token was copied to sessionStorage for this tab.'
    );
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(replaceState.mock.calls[0][2]).toBe('/dashboard');
  });

  it('returns token from URL search params and strips token from URL', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState, sessionStorage } = await loadAuth({
      pathname: '/dashboard',
      search: '?token=abc%3D%3D123&foo=bar',
    });
    expect(getAuthToken()).toBe('abc==123');
    expect(sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe('abc==123');
    expect(warn).toHaveBeenCalledWith(
      'Auth token was read from the URL query string. Query-string tokens can leak through browser history, server logs, and referrer headers. The token was copied to sessionStorage for this tab.'
    );
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(replaceState.mock.calls[0][2]).toBe('/dashboard?foo=bar');
  });

  it('prefers hash token when both hash and query contain token', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState } = await loadAuth({
      pathname: '/dashboard',
      search: '?token=from-query&foo=bar',
      hash: '#token=from-hash&tab=1',
    });
    expect(getAuthToken()).toBe('from-hash');
    expect(warn).toHaveBeenCalledWith(
      'Auth token was read from the URL query string. Query-string tokens can leak through browser history, server logs, and referrer headers. The token was copied to sessionStorage for this tab.'
    );
    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(replaceState.mock.calls[0][2]).toBe('/dashboard?foo=bar#tab=1');
  });

  it('falls back to session storage when URL has no token', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState } = await loadAuth({
      pathname: '/dashboard',
      search: '?foo=bar',
      storedToken: 'saved-token',
    });
    expect(getAuthToken()).toBe('saved-token');
    expect(warn).not.toHaveBeenCalled();
    expect(replaceState).not.toHaveBeenCalled();
  });

  it('returns empty string when no token is present anywhere', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState } = await loadAuth({
      pathname: '/dashboard',
      search: '?foo=bar&baz=1',
      hash: '#tab=1',
    });
    expect(getAuthToken()).toBe('');
    expect(warn).not.toHaveBeenCalled();
    expect(replaceState).not.toHaveBeenCalled();
  });

  it('reuses a scrubbed URL token from session storage on the next load without warning again', async () => {
    const firstWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const firstLoad = await loadAuth({
      pathname: '/dashboard',
      search: '?token=persisted-token',
    });
    expect(firstLoad.getAuthToken()).toBe('persisted-token');
    expect(firstWarn).toHaveBeenCalledTimes(1);

    firstWarn.mockRestore();
    const secondWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { getAuthToken, replaceState } = await loadAuth({
      pathname: '/dashboard',
      storedToken: firstLoad.sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY) || '',
    });
    expect(getAuthToken()).toBe('persisted-token');
    expect(secondWarn).not.toHaveBeenCalled();
    expect(replaceState).not.toHaveBeenCalled();
  });
});

describe('authFetch', () => {
  it('adds Authorization header when token is present', async () => {
    const { authFetch } = await loadAuth({ hash: '#token=secret' });

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
    expect(new Headers(options.headers).get('Authorization')).toBe('Bearer secret');
  });

  it('does not add Authorization header when token is absent', async () => {
    const { authFetch } = await loadAuth();

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
    const headers = new Headers(options?.headers);
    expect(headers.get('Authorization')).toBeNull();
  });

  it('preserves existing headers when adding auth', async () => {
    const { authFetch } = await loadAuth({ hash: '#token=tk' });

    await authFetch('/api/data', {
      headers: { 'Content-Type': 'application/json' },
    });

    const [, options] = fetch.mock.calls[0];
    const headers = new Headers(options.headers);
    expect(headers.get('Authorization')).toBe('Bearer tk');
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('passes through options like method and body', async () => {
    const { authFetch } = await loadAuth({ hash: '#token=tk' });

    await authFetch('/api/data', { method: 'POST', body: '{}' });

    const [, options] = fetch.mock.calls[0];
    expect(options.method).toBe('POST');
    expect(options.body).toBe('{}');
  });

  it('works with no options argument', async () => {
    const { authFetch } = await loadAuth({ hash: '#token=tk' });

    await authFetch('/api/data');

    expect(fetch).toHaveBeenCalledTimes(1);
    const [url] = fetch.mock.calls[0];
    expect(url).toBe('/api/data');
  });

  it('returns the fetch response', async () => {
    const mockResponse = new Response('ok', { status: 200 });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

    const { authFetch } = await loadAuth({ hash: '#token=tk' });
    const result = await authFetch('/api/data');

    expect(result).toBe(mockResponse);
  });
});

describe('clearStoredAuthToken', () => {
  it('removes persisted token from session storage', async () => {
    const { clearStoredAuthToken, sessionStorage } = await loadAuth({
      storedToken: 'saved-token',
    });

    expect(sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe('saved-token');
    clearStoredAuthToken();
    expect(sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBeNull();
  });
});
