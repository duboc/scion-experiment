// Incident Status Dashboard UI (DESIGN.md §8).
//
// Rendering rules:
// - API data is only ever inserted as text (dom.js), never as HTML.
// - Incident cards are keyed by id and updated in place on every poll, so
//   expanded timelines, half-typed update messages and focus survive refreshes.

import { createApi, ApiError } from './api.js';
import { el, replaceChildren, setText } from './dom.js';
import {
  INCIDENT_STATUS_LABELS,
  OVERALL_STATUS_LABELS,
  SERVICE_STATUS_LABELS,
  SEVERITY_LABELS,
  absoluteTime,
  fieldForError,
  labelFor,
  openCountText,
  relativeTime,
  splitIncidents,
} from './format.js';

const POLL_INTERVAL_MS = 15_000;
const RESOLVED_LIMIT = 10;
const DECLARE_FIELDS = ['title', 'service_id', 'severity', 'description'];
const UPDATE_FIELDS = ['status', 'message'];

const api = createApi();

const byTestId = (id) => document.querySelector(`[data-testid="${id}"]`);

const state = {
  /** @type {Map<string, string>} service id -> display name */
  serviceNames: new Map(),
  servicesLoaded: false,
  /** Incremented per refresh; stale responses are dropped. */
  refreshSeq: 0,
  pollTimer: 0,
  /** @type {Map<string, IncidentView>} */
  incidentViews: new Map(),
};

// ---------------------------------------------------------------------------
// Data loading and polling

async function refresh() {
  const seq = ++state.refreshSeq;
  try {
    if (!state.servicesLoaded) await loadServices();
    const [summary, incidents] = await Promise.all([api.summary(), api.incidents()]);
    if (seq !== state.refreshSeq) return; // superseded by a newer refresh
    renderSummary(summary);
    renderIncidents(incidents);
    setConnectionError('');
    setText(byTestId('last-updated'), `Updated ${new Date().toLocaleTimeString()}`);
  } catch (err) {
    if (seq !== state.refreshSeq) return;
    console.error('refresh failed', err);
    setConnectionError(`Could not refresh status: ${errorMessage(err)} Showing the last known data; retrying automatically.`);
  }
}

function schedulePoll() {
  clearTimeout(state.pollTimer);
  state.pollTimer = setTimeout(async () => {
    await refresh();
    schedulePoll();
  }, POLL_INTERVAL_MS);
}

/** Refreshes immediately and restarts the poll interval. */
async function refreshNow() {
  await refresh();
  schedulePoll();
}

async function loadServices() {
  const services = await api.services();
  const select = byTestId('declare-service');
  const placeholder = select.options[0];
  const selected = select.value;
  replaceChildren(select, placeholder, ...services.map((svc) => el('option', { value: svc.id }, svc.name)));
  select.value = selected;
  for (const svc of services) state.serviceNames.set(svc.id, svc.name);
  state.servicesLoaded = true;
}

function setConnectionError(message) {
  const box = byTestId('connection-error');
  setText(box, message);
  box.hidden = message === '';
}

function errorMessage(err) {
  return err instanceof ApiError ? err.message : 'Unexpected error.';
}

function announce(message) {
  // Clear first so repeating the same message is still announced.
  const live = byTestId('announcer');
  live.textContent = '';
  setTimeout(() => { live.textContent = message; }, 50);
}

// ---------------------------------------------------------------------------
// Summary and services

function renderSummary(summary) {
  const overall = summary.overall_status;
  byTestId('status-banner').dataset.status = overall;
  const overallEl = byTestId('overall-status');
  overallEl.dataset.status = overall;
  setText(overallEl, labelFor(OVERALL_STATUS_LABELS, overall));
  setText(byTestId('open-count'), openCountText(summary.open_incidents));

  for (const svc of summary.services) state.serviceNames.set(svc.id, svc.name);
  replaceChildren(byTestId('services'), ...summary.services.map(serviceCard));
}

function serviceCard(svc) {
  return el('li', { class: 'service-card', 'data-testid': `service-card-${svc.id}`, 'data-status': svc.status },
    el('h3', { class: 'service-name' }, svc.name),
    statusBadge(svc.status, labelFor(SERVICE_STATUS_LABELS, svc.status), `service-status-${svc.id}`),
  );
}

/** A status pill: icon + text so status is never conveyed by colour alone. */
function statusBadge(status, label, testId) {
  return el('span', { class: 'badge', 'data-status': status, 'data-testid': testId },
    el('span', { class: 'status-icon', 'aria-hidden': 'true' }),
    label,
  );
}

// ---------------------------------------------------------------------------
// Incidents

/**
 * @typedef {object} IncidentView
 * @property {HTMLLIElement} root
 * @property {HTMLElement} service
 * @property {HTMLElement} severity
 * @property {HTMLElement} status
 * @property {HTMLTimeElement} time
 * @property {HTMLElement} timePrefix
 * @property {HTMLElement} description
 * @property {HTMLElement} timelineSummary
 * @property {HTMLOListElement} timeline
 * @property {HTMLFormElement|null} form
 */

function renderIncidents(incidents) {
  const { open, resolved } = splitIncidents(incidents);
  const recent = resolved.slice(0, RESOLVED_LIMIT);

  const keep = new Set();
  placeIncidents(byTestId('open-incidents'), open, keep);
  placeIncidents(byTestId('resolved-incidents'), recent, keep);
  for (const [id, view] of state.incidentViews) {
    if (!keep.has(id)) {
      view.root.remove();
      state.incidentViews.delete(id);
    }
  }

  byTestId('open-empty').hidden = open.length > 0;
  byTestId('resolved-empty').hidden = recent.length > 0;
  setText(byTestId('open-heading-count'), `(${open.length})`);
  setText(byTestId('resolved-heading-count'), `(${recent.length})`);
}

/** Puts one card per incident into list, in order, reusing existing cards. */
function placeIncidents(list, incidents, keep) {
  incidents.forEach((inc, i) => {
    let view = state.incidentViews.get(inc.id);
    if (!view) {
      view = createIncidentView(inc);
      state.incidentViews.set(inc.id, view);
    }
    updateIncidentView(view, inc);
    keep.add(inc.id);
    // Only move nodes that are out of place: moving a node blurs focus inside it.
    if (list.children[i] !== view.root) list.insertBefore(view.root, list.children[i] ?? null);
  });
}

function createIncidentView(inc) {
  const titleId = `incident-title-${inc.id}`;
  const view = {
    service: el('span', { class: 'incident-service', 'data-testid': `incident-service-${inc.id}` }),
    severity: el('span', { class: 'badge severity', 'data-testid': `incident-severity-${inc.id}` }),
    status: el('span', { class: 'badge', 'data-testid': `incident-status-${inc.id}` }),
    timePrefix: el('span'),
    time: el('time', { 'data-testid': `incident-time-${inc.id}` }),
    description: el('p', { class: 'incident-description' }),
    timelineSummary: el('summary', { 'data-testid': `timeline-toggle-${inc.id}` }),
    timeline: el('ol', { class: 'timeline', 'data-testid': `timeline-${inc.id}` }),
    form: null,
  };
  view.root = el('li', { class: 'incident', 'data-testid': `incident-${inc.id}` },
    el('article', { 'aria-labelledby': titleId },
      el('header', { class: 'incident-header' },
        el('h3', { id: titleId, class: 'incident-title' }, inc.title),
        el('p', { class: 'incident-meta' },
          view.service, view.severity, view.status,
          el('span', { class: 'incident-time' }, view.timePrefix, ' ', view.time),
        ),
      ),
      view.description,
      el('details', { class: 'timeline-details' }, view.timelineSummary, view.timeline),
    ),
  );
  return view;
}

function updateIncidentView(view, inc) {
  const isOpen = inc.status !== 'resolved';
  view.root.dataset.status = inc.status;
  view.root.dataset.severity = inc.severity;

  setText(view.service, state.serviceNames.get(inc.service_id) ?? inc.service_id);

  view.severity.dataset.severity = inc.severity;
  setText(view.severity, labelFor(SEVERITY_LABELS, inc.severity));

  view.status.dataset.status = inc.status;
  replaceChildren(view.status,
    el('span', { class: 'status-icon', 'aria-hidden': 'true' }),
    labelFor(INCIDENT_STATUS_LABELS, inc.status));

  const when = isOpen ? inc.created_at : (inc.resolved_at ?? inc.updated_at);
  setText(view.timePrefix, isOpen ? 'Declared' : 'Resolved');
  view.time.dateTime = when;
  view.time.title = absoluteTime(when);
  setText(view.time, relativeTime(when));

  setText(view.description, inc.description);
  view.description.hidden = inc.description === '';

  const n = inc.updates.length;
  setText(view.timelineSummary, `Timeline (${n} ${n === 1 ? 'update' : 'updates'})`);
  replaceChildren(view.timeline, ...inc.updates.map(timelineEntry));

  if (isOpen && !view.form) {
    view.form = createUpdateForm(inc);
    view.root.firstElementChild.append(view.form);
  } else if (!isOpen && view.form) {
    view.form.remove();
    view.form = null;
  }
}

function timelineEntry(update) {
  return el('li', { class: 'timeline-entry' },
    el('div', { class: 'timeline-head' },
      statusBadge(update.status, labelFor(INCIDENT_STATUS_LABELS, update.status)),
      el('time', { datetime: update.created_at, title: absoluteTime(update.created_at) }, relativeTime(update.created_at)),
    ),
    el('p', { class: 'timeline-message' }, update.message),
  );
}

// ---------------------------------------------------------------------------
// Forms

function createUpdateForm(inc) {
  const id = inc.id;
  const statusSelect = el('select', { id: `update-status-${id}`, name: 'status', 'data-testid': `update-status-${id}` },
    ...Object.entries(INCIDENT_STATUS_LABELS).map(([value, label]) =>
      el('option', { value }, value === 'resolved' ? `${label} (closes the incident)` : label)),
  );
  statusSelect.value = inc.status;

  const form = el('form', {
    class: 'update-form',
    'data-testid': `update-form-${id}`,
    'aria-label': `Post an update for ${inc.title}`,
    novalidate: true,
  },
    el('div', { class: 'field' },
      el('label', { for: `update-status-${id}` }, 'New status'),
      statusSelect,
    ),
    el('div', { class: 'field field-grow' },
      el('label', { for: `update-message-${id}` }, 'Update message'),
      el('textarea', {
        id: `update-message-${id}`, name: 'message', rows: 2, maxlength: 1000, required: true,
        'data-testid': `update-message-${id}`,
      }),
    ),
    el('p', { class: 'form-error', id: `update-error-${id}`, 'data-testid': `update-error-${id}`, role: 'alert', hidden: true }),
    el('button', { type: 'submit', 'data-testid': `update-submit-${id}` }, 'Post update'),
  );
  form.addEventListener('submit', (ev) => onUpdateSubmit(ev, id, inc.title));
  return form;
}

async function onUpdateSubmit(ev, id, title) {
  ev.preventDefault();
  const form = /** @type {HTMLFormElement} */ (ev.currentTarget);
  const errorEl = form.querySelector('.form-error');
  const status = form.elements.namedItem('status').value;
  await submitForm(form, errorEl, UPDATE_FIELDS, 'Posting…', async () => {
    await api.postUpdate(id, { status, message: form.elements.namedItem('message').value });
    form.elements.namedItem('message').value = '';
    announce(status === 'resolved' ? `Resolved “${title}”.` : `Update posted to “${title}”.`);
  });
}

async function onDeclareSubmit(ev) {
  ev.preventDefault();
  const form = /** @type {HTMLFormElement} */ (ev.currentTarget);
  const success = byTestId('declare-success');
  success.hidden = true;
  const value = (name) => form.elements.namedItem(name).value;
  await submitForm(form, byTestId('declare-error'), DECLARE_FIELDS, 'Declaring…', async () => {
    const inc = await api.declareIncident({
      title: value('title'),
      description: value('description'),
      service_id: value('service_id'),
      severity: value('severity'),
    });
    form.reset();
    setText(success, `Declared “${inc.title}” (${inc.id}).`);
    success.hidden = false;
  });
}

/**
 * Runs action with the submit button disabled. On ApiError, shows the API's
 * message inline and marks the field it refers to as invalid. Always
 * refreshes the dashboard afterwards (also after a 409, whose incident has
 * changed under us).
 */
async function submitForm(form, errorEl, fieldNames, busyLabel, action) {
  const button = form.querySelector('button[type="submit"]');
  const idleLabel = button.textContent;
  clearFormError(form, errorEl);
  button.disabled = true;
  setText(button, busyLabel);
  try {
    await action();
  } catch (err) {
    showFormError(form, errorEl, fieldNames, err);
  } finally {
    button.disabled = false;
    setText(button, idleLabel);
  }
  await refreshNow();
}

function clearFormError(form, errorEl) {
  errorEl.hidden = true;
  setText(errorEl, '');
  for (const field of form.querySelectorAll('[aria-invalid]')) {
    field.removeAttribute('aria-invalid');
    field.removeAttribute('aria-describedby');
  }
}

function showFormError(form, errorEl, fieldNames, err) {
  if (!(err instanceof ApiError)) console.error(err);
  setText(errorEl, errorMessage(err));
  errorEl.hidden = false;
  const name = err instanceof ApiError ? fieldForError(err.message, fieldNames) : null;
  const field = name ? form.elements.namedItem(name) : null;
  if (field) {
    field.setAttribute('aria-invalid', 'true');
    field.setAttribute('aria-describedby', errorEl.id);
    field.focus();
  }
}

// ---------------------------------------------------------------------------
// Startup

function start() {
  byTestId('declare-form').addEventListener('submit', onDeclareSubmit);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') refreshNow();
  });
  refreshNow();
}

start();
