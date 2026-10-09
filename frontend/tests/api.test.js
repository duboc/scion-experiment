// Run with: node --test tests/*.test.js   (Node 22+, no dependencies)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ApiError, createApi } from '../api.js';

/** Returns a fake fetch that records calls and replies with the given response. */
function fakeFetch(reply) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, init });
    if (reply instanceof Error) throw reply;
    return {
      ok: reply.status >= 200 && reply.status < 300,
      status: reply.status,
      json: async () => {
        if (reply.body === undefined) throw new SyntaxError('no body');
        return reply.body;
      },
    };
  };
  return { fn, calls };
}

test('GET requests use same-origin relative URLs and send no body', async () => {
  const { fn, calls } = fakeFetch({ status: 200, body: { incidents: [{ id: 'inc-1' }] } });
  const api = createApi(fn);
  assert.deepEqual(await api.incidents(), [{ id: 'inc-1' }]);
  assert.equal(calls[0].url, '/api/v1/incidents');
  assert.equal(calls[0].init.method, 'GET');
  assert.equal(calls[0].init.body, undefined);
  assert.equal(calls[0].init.headers['Content-Type'], undefined);
});

test('summary and services hit the right paths', async () => {
  const { fn, calls } = fakeFetch({ status: 200, body: { services: [] } });
  const api = createApi(fn);
  await api.summary();
  assert.deepEqual(await api.services(), []);
  assert.deepEqual(calls.map((c) => c.url), ['/api/v1/summary', '/api/v1/services']);
});

test('POSTs send JSON with Content-Type: application/json (backend CSRF rule)', async () => {
  const { fn, calls } = fakeFetch({ status: 201, body: { id: 'inc-3' } });
  const api = createApi(fn);
  const input = { title: 't', description: '', service_id: 'api', severity: 'sev2' };
  assert.deepEqual(await api.declareIncident(input), { id: 'inc-3' });
  assert.equal(calls[0].url, '/api/v1/incidents');
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(calls[0].init.body), input);
});

test('postUpdate encodes the incident id in the path', async () => {
  const { fn, calls } = fakeFetch({ status: 200, body: {} });
  await createApi(fn).postUpdate('inc/../1', { status: 'resolved', message: 'm' });
  assert.equal(calls[0].url, '/api/v1/incidents/inc%2F..%2F1/updates');
  assert.equal(calls[0].init.headers['Content-Type'], 'application/json');
});

test('error envelopes become ApiError with code and message', async () => {
  const { fn } = fakeFetch({
    status: 409,
    body: { error: { code: 'failed_precondition', message: 'incident "inc-1" is already resolved' } },
  });
  await assert.rejects(createApi(fn).postUpdate('inc-1', { status: 'monitoring', message: 'm' }), (err) => {
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 409);
    assert.equal(err.code, 'failed_precondition');
    assert.equal(err.message, 'incident "inc-1" is already resolved');
    return true;
  });
});

test('non-JSON error responses still produce a readable ApiError', async () => {
  const { fn } = fakeFetch({ status: 502 });
  await assert.rejects(createApi(fn).summary(), { name: 'ApiError', status: 502, code: 'internal', message: 'Request failed with status 502.' });
});

test('invalid success bodies are rejected', async () => {
  const { fn } = fakeFetch({ status: 200 });
  await assert.rejects(createApi(fn).summary(), { name: 'ApiError', code: 'internal' });
});

test('network failures become ApiError "unavailable"', async () => {
  const { fn } = fakeFetch(new TypeError('Failed to fetch'));
  await assert.rejects(createApi(fn).summary(), { name: 'ApiError', status: 0, code: 'unavailable' });
});
