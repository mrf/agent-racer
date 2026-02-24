const params = new URLSearchParams(typeof location !== 'undefined' ? location.search : '');
const authToken = params.get('token') || '';

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
