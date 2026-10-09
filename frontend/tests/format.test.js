// Run with: node --test tests/*.test.js   (Node 22+, no dependencies)
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  INCIDENT_STATUS_LABELS,
  OVERALL_STATUS_LABELS,
  SERVICE_STATUS_LABELS,
  absoluteTime,
  fieldForError,
  labelFor,
  openCountText,
  relativeTime,
  splitIncidents,
} from '../format.js';

const NOW = Date.parse('2026-10-02T12:00:00Z');

test('labels cover every status in DESIGN.md §6', () => {
  for (const s of ['operational', 'degraded', 'partial_outage', 'major_outage']) {
    assert.ok(SERVICE_STATUS_LABELS[s], s);
    assert.ok(OVERALL_STATUS_LABELS[s], s);
  }
  for (const s of ['investigating', 'identified', 'monitoring', 'resolved']) {
    assert.ok(INCIDENT_STATUS_LABELS[s], s);
  }
});

test('labelFor falls back to the raw value and ignores prototype keys', () => {
  assert.equal(labelFor(SERVICE_STATUS_LABELS, 'major_outage'), 'Major outage');
  assert.equal(labelFor(SERVICE_STATUS_LABELS, 'brand_new'), 'brand_new');
  assert.equal(labelFor(SERVICE_STATUS_LABELS, 'toString'), 'toString');
});

test('openCountText pluralises', () => {
  assert.equal(openCountText(0), '0 open incidents');
  assert.equal(openCountText(1), '1 open incident');
  assert.equal(openCountText(2), '2 open incidents');
});

test('relativeTime', () => {
  const ago = (s) => new Date(NOW - s * 1000).toISOString();
  const cases = [
    [ago(0), 'just now'],
    [ago(44), 'just now'],
    [ago(-30), 'just now'], // small clock skew into the future
    [ago(60), '1 minute ago'],
    [ago(45 * 60), '45 minutes ago'],
    [ago(3600), '1 hour ago'],
    [ago(3 * 3600), '3 hours ago'],
    [ago(86400), 'yesterday'],
    [ago(3 * 86400), '3 days ago'],
    [ago(-10 * 60), 'in 10 minutes'],
    ['not a date', ''],
  ];
  for (const [iso, want] of cases) {
    assert.equal(relativeTime(iso, NOW), want, iso);
  }
});

test('absoluteTime formats UTC without fractional seconds', () => {
  assert.equal(absoluteTime('2026-10-02T10:00:00Z'), '2026-10-02 10:00:00 UTC');
  assert.equal(absoluteTime('garbage'), '');
});

test('splitIncidents keeps server order', () => {
  const incs = [
    { id: 'inc-4', status: 'investigating' },
    { id: 'inc-3', status: 'resolved' },
    { id: 'inc-2', status: 'monitoring' },
    { id: 'inc-1', status: 'resolved' },
  ];
  const { open, resolved } = splitIncidents(incs);
  assert.deepEqual(open.map((i) => i.id), ['inc-4', 'inc-2']);
  assert.deepEqual(resolved.map((i) => i.id), ['inc-3', 'inc-1']);
});

test('fieldForError maps backend validation messages to fields', () => {
  const fields = ['title', 'service_id', 'severity', 'description'];
  assert.equal(fieldForError('title is required', fields), 'title');
  assert.equal(fieldForError('title must be at most 120 characters', fields), 'title');
  assert.equal(fieldForError('unknown service_id "nope"', fields), 'service_id');
  assert.equal(fieldForError('severity must be one of sev1, sev2, sev3, sev4', fields), 'severity');
  assert.equal(fieldForError('description must be at most 2000 characters', fields), 'description');
  assert.equal(fieldForError('Content-Type must be application/json', fields), null);
  assert.equal(fieldForError('malformed JSON at offset 3', fields), null);
  // "status" must not match the "status filter" text unless it is a field.
  assert.equal(fieldForError('message is required', ['status', 'message']), 'message');
});
