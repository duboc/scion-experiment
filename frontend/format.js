// Pure formatting and data helpers. No DOM access, so these run under
// `node --test` as well as in the browser.

export const SERVICE_STATUS_LABELS = Object.freeze({
  operational: 'Operational',
  degraded: 'Degraded performance',
  partial_outage: 'Partial outage',
  major_outage: 'Major outage',
});

export const OVERALL_STATUS_LABELS = Object.freeze({
  operational: 'All systems operational',
  degraded: 'Degraded performance',
  partial_outage: 'Partial outage',
  major_outage: 'Major outage',
});

export const INCIDENT_STATUS_LABELS = Object.freeze({
  investigating: 'Investigating',
  identified: 'Identified',
  monitoring: 'Monitoring',
  resolved: 'Resolved',
});

export const SEVERITY_LABELS = Object.freeze({
  sev1: 'SEV1 · Critical',
  sev2: 'SEV2 · Major',
  sev3: 'SEV3 · Minor',
  sev4: 'SEV4 · Low',
});

/** Returns the label for value in labels, falling back to the raw value. */
export function labelFor(labels, value) {
  return Object.hasOwn(labels, value) ? labels[value] : String(value);
}

/** Returns a short phrase such as "the open incident count". */
export function openCountText(n) {
  return n === 1 ? '1 open incident' : `${n} open incidents`;
}

const UNITS = [
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
];

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

/**
 * Formats an RFC3339 timestamp relative to now, e.g. "5 minutes ago".
 * Returns "just now" for anything under 45 seconds (including small clock
 * skew into the future) and "" for unparseable input.
 */
export function relativeTime(iso, now = Date.now()) {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const seconds = Math.round((t - now) / 1000);
  if (Math.abs(seconds) < 45) return 'just now';
  for (const [unit, size] of UNITS) {
    if (Math.abs(seconds) >= size || unit === 'minute') {
      return rtf.format(Math.round(seconds / size), unit);
    }
  }
  return '';
}

/** Formats an RFC3339 timestamp as an absolute UTC string for tooltips. */
export function absoluteTime(iso) {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  return new Date(t).toISOString().replace('T', ' ').replace(/\.\d+Z$/, ' UTC');
}

/**
 * Splits incidents into open and resolved lists, keeping the server's
 * newest-first order (timestamps have only second precision, so re-sorting
 * client-side could reorder same-second incidents).
 */
export function splitIncidents(incidents) {
  const open = [];
  const resolved = [];
  for (const inc of incidents) {
    (inc.status === 'resolved' ? resolved : open).push(inc);
  }
  return { open, resolved };
}

/**
 * Guesses which form field an API validation message refers to. Store
 * messages start with the field name ("title is required",
 * "unknown service_id ..."). Returns null when no field matches.
 */
export function fieldForError(message, fieldNames) {
  const words = String(message).split(/[\s"]+/);
  return fieldNames.find((name) => words.includes(name)) ?? null;
}
