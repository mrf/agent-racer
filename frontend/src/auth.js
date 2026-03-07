const AUTH_TOKEN_STORAGE_KEY = 'agent-racer-auth-token';

function resolveAuthToken() {
  const hasLocation = typeof location !== 'undefined';
  const hasStorage = typeof sessionStorage !== 'undefined';

  let searchParams = new URLSearchParams();
  let hashParams = new URLSearchParams();
  let sawTokenInURL = false;
  let token = '';

  if (hasLocation) {
    hashParams = new URLSearchParams((location.hash || '').replace(/^#/, ''));
    if (hashParams.has('token')) {
      token = hashParams.get('token') || '';
      hashParams.delete('token');
      sawTokenInURL = true;
    }

    searchParams = new URLSearchParams(location.search || '');
    if (searchParams.has('token')) {
      if (!token) {
        token = searchParams.get('token') || '';
      }
      searchParams.delete('token');
      sawTokenInURL = true;
    }
  }

  if (!token && hasStorage) {
    token = sessionStorage.getItem(AUTH_TOKEN_STORAGE_KEY) || '';
  }

  if (token && hasStorage) {
    sessionStorage.setItem(AUTH_TOKEN_STORAGE_KEY, token);
  }

  if (sawTokenInURL && hasLocation && typeof history !== 'undefined' && typeof history.replaceState === 'function') {
    const pathname = location.pathname || '/';
    const search = searchParams.toString();
    const hash = hashParams.toString();
    const cleanURL = `${pathname}${search ? `?${search}` : ''}${hash ? `#${hash}` : ''}`;
    history.replaceState(history.state, '', cleanURL);
  }

  return token;
}

const authToken = resolveAuthToken();

/** Wraps fetch, injecting an Authorization header when a token is configured. */
export function authFetch(url, options = {}) {
  if (authToken) {
    const headers = new Headers(options.headers);
    headers.set('Authorization', `Bearer ${authToken}`);
    options = { ...options, headers };
  }
  return fetch(url, options);
}

export function getAuthToken() {
  return authToken;
}

export function clearStoredAuthToken() {
  if (typeof sessionStorage !== 'undefined') {
    sessionStorage.removeItem(AUTH_TOKEN_STORAGE_KEY);
  }
}
