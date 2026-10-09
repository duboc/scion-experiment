# Incident Status Dashboard: Frontend

A dashboard UI with no build step, written in plain HTML, CSS and ES-module JavaScript with no npm
dependencies. It implements [`/workspace/DESIGN.md`](../DESIGN.md) §8 against the API in §7.

## Run

The Go backend serves this directory at `/`:

```bash
cd /workspace/backend
go run . -addr :8080 -static ../frontend
# open http://localhost:8080
```

The UI calls the API with same-origin relative URLs (`/api/v1/...`), so it has to be served by
the backend, not opened as a `file://` page.

## Files

| File          | Purpose |
|---------------|---------|
| `index.html`  | Static page shell: banner, services, open and resolved incidents, and the declare form. Loads only `styles.css` and `app.js`. |
| `styles.css`  | All styling. There are no inline styles; status colours come from `data-status` and `data-severity` attributes. |
| `app.js`      | Entry module: polling, rendering, and form handling. |
| `api.js`      | API client (`createApi(fetchFn)`). Every POST sends `Content-Type: application/json`, which the backend requires as CSRF protection. Error envelopes become `ApiError{status, code, message}`. |
| `format.js`   | Pure helpers: labels, relative time, open/resolved split, and mapping an error message to a form field. |
| `dom.js`      | Small `el()` builder. It inserts text only as text nodes and refuses `on*` attributes. |
| `tests/`      | `node:test` unit tests for `format.js` and `api.js`. |

## Test

```bash
cd /workspace/frontend
node --test tests/*.test.js   # Node 22+, no dependencies
```

## Behaviour

- **Polling:** the UI fetches `/api/v1/summary` and `/api/v1/incidents` every 15 s. It also refreshes right
  after each declare or update (including failed ones, so a 409 shows the latest state) and when
  the tab becomes visible again. If an older response arrives after a newer one, the older one is dropped.
- **Stable re-rendering:** incident cards are keyed by id and updated in place. A poll does not
  collapse an expanded timeline, clear a half-typed update message, or move focus.
- **Errors:** API validation messages appear inline, in a `role="alert"` box under the form. When the
  message names a field (for example `title is required`), that field gets `aria-invalid` and focus. Network
  failures show a banner and the last known data stays on screen. Forms use `novalidate`, so the API is the
  single source of validation truth. `maxlength` still limits typing.
- **Security:** API data is rendered only through `textContent` and text nodes. Nothing uses `innerHTML`.
  `index.html` sets a strict CSP via `<meta http-equiv>`: `default-src 'self'` with no inline scripts or
  styles. Because a meta tag can't set `frame-ancestors`, that directive would need to be an HTTP header.
- **Accessibility:** the page uses semantic landmarks, a skip link, a label on every control, and visible `:focus-visible`
  outlines. A polite live region announces successful updates. Status always appears as text, with a
  differently shaped icon per status (circle, diamond, triangle, square) as a second cue besides colour.
- "Recently resolved" shows the 10 most recently created resolved incidents, in server order.

## `data-testid` hooks

| Hook | Element |
|------|---------|
| `status-banner`, `overall-status` | Banner and overall status label. Both carry `data-status`, which holds the raw API value. |
| `open-count`, `last-updated`, `connection-error`, `announcer` | Banner meta, error banner, and live region. |
| `services`, `service-card-<id>`, `service-status-<id>` | Services grid. Each card carries `data-status`. |
| `open-incidents`, `resolved-incidents`, `open-empty`, `resolved-empty` | Incident lists and their empty states. |
| `incident-<id>` | Incident card, with `data-status` and `data-severity`. |
| `incident-service-<id>`, `incident-severity-<id>`, `incident-status-<id>`, `incident-time-<id>` | Card details. |
| `timeline-toggle-<id>`, `timeline-<id>` | Expandable timeline (`<details>`). |
| `declare-form`, `declare-title`, `declare-service`, `declare-severity`, `declare-description`, `declare-submit`, `declare-error`, `declare-success` | Declare form. |
| `update-form-<id>`, `update-status-<id>`, `update-message-<id>`, `update-submit-<id>`, `update-error-<id>` | Per-incident update form (open incidents only). |
