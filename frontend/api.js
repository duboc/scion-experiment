// Thin client for the JSON API in DESIGN.md §7. All URLs are same-origin
// and relative.

/** An API failure. code is the §7 error code, or "unavailable" for network errors. */
export class ApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

/**
 * Creates an API client. fetchFn is injectable for tests.
 * @param {typeof fetch} fetchFn
 */
export function createApi(fetchFn = (...args) => fetch(...args)) {
  async function request(path, { method = 'GET', body } = {}) {
    const init = { method, headers: { Accept: 'application/json' }, cache: 'no-store' };
    if (body !== undefined) {
      // The backend rejects any other Content-Type (CSRF protection).
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }

    let res;
    try {
      res = await fetchFn(path, init);
    } catch {
      throw new ApiError(0, 'unavailable', 'Unable to reach the server. Check your connection and try again.');
    }

    let data = null;
    try {
      data = await res.json();
    } catch {
      // Fall through: handled below for both success and error responses.
    }

    if (!res.ok) {
      const err = data && typeof data === 'object' ? data.error : null;
      throw new ApiError(
        res.status,
        err?.code ?? 'internal',
        err?.message ?? `Request failed with status ${res.status}.`,
      );
    }
    if (data === null || typeof data !== 'object') {
      throw new ApiError(res.status, 'internal', 'The server returned an invalid response.');
    }
    return data;
  }

  const incidentPath = (id) => `/api/v1/incidents/${encodeURIComponent(id)}`;

  return {
    summary: () => request('/api/v1/summary'),
    services: async () => (await request('/api/v1/services')).services,
    incidents: async () => (await request('/api/v1/incidents')).incidents,
    declareIncident: (input) => request('/api/v1/incidents', { method: 'POST', body: input }),
    postUpdate: (id, input) => request(`${incidentPath(id)}/updates`, { method: 'POST', body: input }),
  };
}
