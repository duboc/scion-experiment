# Readability & Security Review: Incident Status Dashboard

**Reviewer:** readability-reviewer · **Date:** 2026-10-02
**Spec:** `/workspace/DESIGN.md` (§6, §7, §9)

---

## Backend (`/workspace/backend`)

**Verdict: LGTM** (updated 2026-10-02 16:40Z, after the R1 fix).

> **Re-check of the fixes.** eng-lead verified R1, and I spot-checked the code
> myself. `decodeJSON` now calls `mime.ParseMediaType` and accepts only
> `application/json`, so `; charset=utf-8` and mixed case still pass. Anything
> else, including a missing header or malformed parameters, gets 400
> `invalid_argument` with `Content-Type must be application/json`. A
> table-driven test covers `text/plain`, `text/plain;charset=UTF-8`, a form
> body, malformed parameters, and `application/json; charset=utf-8`.
> swe-backend also fixed **N3** (the trailing decode now reports the size
> limit), **N7** (`HEAD /healthz`) and **N8** (`/api` returns a JSON 404). Part
> of **N4** is done: static responses now get `nosniff`, but CSP is still open.
> **N5** is done: a directory without `index.html` returns 404. After the fixes,
> `gofmt`, `go vet` and `go test -race -count=1 ./...` are clean. Coverage is
> store 100%, api 97.1%, main 73.0%.
> Still open and non-blocking: N1 (control and bidi characters), N2 (validation
> outside the lock), N4-CSP (coordinate with the frontend), N6 (`-h` usage) and
> N9 (README sentence).
>
> *Original verdict:* Changes Requested, with one required change (R1). The
> text below is kept as it was written, as a record.

### What I checked

- I read every file: `main.go`, `internal/store/*`, `internal/api/*` and all the tests.
- `gofmt -l .` is clean. `go vet ./...` is clean. `go test -race -count=1 -cover ./...` passes with Go 1.26.1:
  - `store` 100.0%
  - `api` 96.6%
  - `main` 75.7%
- I built the binary and probed it live: cross-origin `text/plain` POST,
  path traversal (`/../`, `%2e%2e`), directory listing, oversized trailing data,
  `null` fields, an encoded `/` in `{id}`, control characters in input, `HEAD`,
  and `/api` without a trailing slash.

### Required changes

**R1. Require `Content-Type: application/json` on POST bodies (drive-by
cross-site writes).**
`decodeJSON` (`internal/api/api.go:186`) accepts any request content type.
A browser can send a "simple" cross-origin request (`Content-Type: text/plain`)
without a CORS preflight. So any web page that a user on the same network opens
can silently send requests to the dashboard. I checked this live:

```
curl -H 'Content-Type: text/plain' -H 'Origin: http://evil.example' \
     -d '{"title":"csrf","service_id":"api","severity":"sev1"}' .../api/v1/incidents
→ 201
```

AuthN/Z is a non-goal (§3), so this isn't about access control. The problem is
integrity. A malicious page can declare fake sev1 incidents, or send
`{"status":"resolved"}` to a real open incident. With choice (4), a resolved
incident is **permanently** locked (409), so an attacker can hide a live outage
from the dashboard, and the API offers no way to undo it.

*Fix:* in `decodeJSON` (or a small helper that both POST handlers call), parse
`r.Header.Get("Content-Type")` with `mime.ParseMediaType` and reject anything
other than `application/json`. `application/json; charset=utf-8` must still be
accepted. Return 400 `invalid_argument` with a message such as
`Content-Type must be application/json`. Reusing 400 keeps the §7 code set
closed; 415 would be the alternative. A non-simple content type forces a CORS
preflight, and the server doesn't answer preflights, so the browser blocks the
cross-site write. Add table cases to `TestCreateIncidentErrors` and
`TestAddUpdateErrors`:
- missing Content-Type
- `text/plain`
- `application/x-www-form-urlencoded`
- `application/json; charset=utf-8` (expected to succeed)

The test helper `do()` already sets the header, so existing tests are unaffected.
*Cross-team note:* the frontend must send `Content-Type: application/json` on
its `fetch` POSTs. I'll check this in the frontend review. sre-qa's curl
scripts also need `-H 'Content-Type: application/json'`.

### Decisions left open by the design (requested assessment)

| # | Choice | Assessment |
|---|--------|------------|
| 1 | Oversized body returns **400 `invalid_argument`** instead of 413 | **Reasonable, agree.** §7 lists a closed set of five codes, and 413 has no code in that set. The message names the limit (`must not exceed 65536 bytes`), and `http.MaxBytesReader` also closes the connection, which is right for abusive clients. One edge case is in N3. |
| 2 | Timestamps **truncated to whole seconds, UTC** | **Reasonable, agree.** The output is plain RFC3339 without fractional seconds, which is the easiest form for the UI and E2E tooling to parse and compare. Ordering stays deterministic because `ListIncidents` breaks ties on the sequence number (`seq`), and a timeline's order comes from append order, not timestamps. `TestListIncidentsNewestFirst` covers same-second creates. Clients must not rebuild ordering from timestamps alone. The README says so implicitly; consider one explicit sentence. |
| 3 | **Trim, then count runes** (not bytes) | **Reasonable, agree.** Trimming before the required check rejects whitespace-only titles and messages, which is the right behaviour. Counting runes matches the user's idea of "characters" better than bytes, and the multibyte at-limit tests (`é`, `ü`) pin it down. Frontend note: HTML `maxlength` counts UTF-16 code units, so emoji and other astral characters make the browser *stricter* than the server, never looser. That's acceptable, and server errors still show inline. See N1 for control characters. |
| 4 | Open incident can move to **any** status; updating a resolved one returns **409** | **Reasonable, agree. It matches §7 exactly.** Allowing backwards moves (monitoring → investigating, "fix did not hold") and same-status updates (progress notes) is how real incident timelines work. Note that because resolution is terminal, R1 matters more. A "reopen" flow is out of scope; flag it to eng-lead as possible future work, not a change to make now. |

### Nits (non-blocking, author's discretion)

- **N1. Control and bidi characters accepted in text fields.** `"a\u0000b‮"`
  was accepted as a title. `textContent` keeps this XSS-safe, but NUL and
  bidi-override characters (U+202E) can visually spoof titles on a status page.
  Consider rejecting `unicode.IsControl` runes, except `\n` and `\t` in
  `description` and `message`, plus the bidi override and isolate characters
  (U+202A–U+202E, U+2066–U+2069). Do it in one `validateText` helper.
- **N2. Validation placement is inconsistent.** `AddUpdate` validates *before*
  taking `s.mu`, but `CreateIncident` runs its pure checks (trim, lengths,
  severity) *under* the write lock. Move the pure checks above `s.mu.Lock()` and
  keep only the service-existence check inside. This shortens the critical
  section and makes the two methods read the same way.
- **N3. Misleading error for oversized trailing data.** A valid object followed
  by more than 64 KiB of whitespace returns `request body must contain a single
  JSON object` rather than the size message, because the second `Decode` hits
  `MaxBytesError` and the code only checks for `!= io.EOF`. Run the same
  `errors.As(err, &tooLarge)` check on the trailing decode as well.
- **N4. Static responses lack hardening headers.** `writeBody` sets
  `X-Content-Type-Options: nosniff` on JSON only. Wrap the `FileServer` and add
  `nosniff`, plus
  `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'; base-uri 'none'`
  and `Referrer-Policy: no-referrer`. The CSP is cheap defence in depth behind
  §8's `textContent` rule. Coordinate with swe-frontend first: it assumes no
  inline `<script>` or `style=` attributes.
- **N5. Directory listings are enabled.** `http.FileServer` lists any
  subdirectory under `-static` that has no `index.html` (I checked: `/sub/`
  returned a `<pre>` listing). This is harmless for the current flat frontend,
  but a small wrapper that returns 404 for directories without `index.html` is
  the conventional hardening.
- **N6. `-h` prints nothing.** `fs.SetOutput(io.Discard)` hides usage, so
  `-h` and bad flags exit 1 with only `flag: help requested` in the log. Send
  output to `os.Stderr` from `main`, or treat `flag.ErrHelp` as exit 0 after
  printing `fs.PrintDefaults()`.
- **N7. `HEAD /healthz` returns 405.** Some probes use HEAD. Optional: allow
  `http.MethodHead` on `/healthz`.
- **N8. `GET /api` returns an HTML 307 to `/api/`** (from the `/api/` subtree
  pattern). Trivial, and it lands on the JSON 404, but registering `/api` to
  `handleAPINotFound` keeps "every API-ish response is JSON" true.
- **N9. README nit:** state explicitly that list order comes from `created_at`
  and then id, so clients don't need to sort by timestamp (see choice 2).

### Praise

- **Concurrency is done right.** A single `sync.RWMutex` guards every field.
  `*Locked` helpers make the lock contract explicit. `insert` and every getter
  return deep copies (`slices.Clone(Updates)`, a copied `*ResolvedAt`), and
  `TestReturnedIncidentsAreCopies` proves no aliasing escapes. ID allocation
  happens under the write lock. `TestConcurrentAccess` with `-race` checks
  uniqueness and the final counts.
- **Clean layering.** The store owns domain rules and returns a typed `*store.Error`
  with a `Code`. The API layer maps codes to HTTP in one place
  (`writeStoreError`). The custom `Is` makes `errors.Is(err, ErrNotFound)`
  idiomatic, and unknown errors safely become 500 with server-side logging only,
  so no internals leak.
- **Robust JSON decoding.** It has `MaxBytesReader`, `DisallowUnknownFields`,
  trailing-data rejection, and precise client-safe messages for each error class
  (syntax offset, type mismatch, non-object, unknown field). Every 4xx/5xx,
  including 405s with a correct `Allow` header, recovered panics and unknown
  `/api/` routes, uses the §7 envelope with `application/json`.
- **Status derivation is small and correct.** `Severity.impact()` and
  `ServiceStatus.rank()` with `worse()` read like the spec. Empty slices
  serialize as `[]`, and `resolved_at` is an explicit `null`. Both are tested.
- **Production-quality `main`.** `run()` is testable (it takes ctx, args and a
  ready channel). The server sets all four timeouts (`ReadHeaderTimeout` guards
  against Slowloris), shuts down gracefully on SIGINT/SIGTERM, and the shutdown
  path is tested end to end.
- **Tests are exemplary.**
  - Store tests use an injected fake clock and are table-driven.
  - Boundary tests cover at-limit and over-limit lengths with multibyte input.
  - The API tests decode into independent "wire" structs with
    `DisallowUnknownFields`, so a renamed JSON tag fails a test. That's a nice
    touch.
  - Every §7 endpoint and error path is covered, as §9 requires.
- Naming, doc comments and package docs follow Effective Go and the Google Go
  style guide throughout. There's nothing to flag on readability.

---

## Frontend (`/workspace/frontend`)

**Verdict: LGTM.** No required changes. FN1–FN3 are worth doing (in that
order) if there's time before sign-off, but none of them block it.
Built by swe-backend, standing in for swe-frontend.

### What I checked

- I read every file: `index.html`, `styles.css`, `app.js`, `api.js`, `dom.js`,
  `format.js`, `README.md` and `tests/*.test.js`.
- `node --test tests/*.test.js` passes 15/15 on Node 24.21. `node --check`
  is clean for every module.
- I grepped for dangerous sinks (`innerHTML`, `outerHTML`,
  `insertAdjacentHTML`, `document.write`, `eval`, `new Function`, `style=`,
  `.style.`, inline `<script>`, `on*=`) and found **none**.
- **Live browser check:** I ran the backend with `-static ../frontend`. I
  declared an incident with title `<img src=x onerror=alert(1)><script>alert(2)</script>`
  and description `<b>bold</b>`, then posted an update with message
  `<svg onload=alert(3)>`. I rendered the page in headless Chromium
  (`--dump-dom`, 5 s virtual time). Results:
  - All payloads appear as **escaped text**. The DOM has no injected
    `img`, `script`, `svg` or `b` elements.
  - The console shows **no CSP violations and no JS errors**.
  - The banner shows `data-status="major_outage"` with the text "Major outage"
    and "2 open incidents".
  - The service dropdown has all 5 services from `/api/v1/services`.
  - These hooks are present: `overall-status`, `service-card-{api,auth,db,payments,web}`,
    `incident-inc-{1,2,3}`, `declare-form`, and `update-form-inc-{2,3}`
    (open incidents only).
  - Every asset is served with the right MIME type and `nosniff`.

### §8 requirement checklist

| Requirement | Status |
|---|---|
| Banner: overall status, colour coded, open count | ✅ `data-status` drives four colours. A text label is always shown, plus the count. |
| Services grid, one card per service with a badge | ✅ |
| Open / Recently resolved; title, service, severity, status, relative time; expandable timeline | ✅ Uses `<details>`, so the timeline is keyboard-accessible natively. |
| Declare form with service dropdown from `/api/v1/services`, inline API errors | ✅ Errors appear in a `role="alert"` box, and the named field gets `aria-invalid` and focus. |
| Post-update form on each open incident | ✅ Removed when the incident resolves. |
| Poll summary + incidents every 15 s; refresh after every mutation | ✅ Also refreshes when the tab becomes visible again. |
| Accessibility: semantic HTML, labelled controls, visible focus, not colour-only | ✅ Skip link, landmarks, `<label for>` on every control, `:focus-visible` outline, and a **different icon shape** per status on top of the text. |
| Security: `textContent` only | ✅ Enforced structurally by `dom.js` (see Praise). |
| Same-origin relative URLs | ✅ Every call goes through `api.js` and uses `/api/v1/...`. |
| CSP meta, no inline script/style | ✅ `default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'` and more. Verified with zero violations. |
| Every POST sends `Content-Type: application/json` | ✅ Set centrally in `request()` whenever there's a body, and unit-tested for both POST methods. |
| `data-testid` hooks | ✅ All the required ones, plus a well-documented set of extras (README table). |

### Should fix (non-blocking, highest value first)

- **FN1. A stale default in the update status select can silently revert
  status.** `createUpdateForm` sets `statusSelect.value = inc.status` once,
  when the card is created. Cards are reused across polls, so if another user
  moves the incident from `identified` to `monitoring`, this user's select still
  shows `identified`. Posting a plain progress note then moves the incident
  *back* to `identified` without anyone noticing.
  *Fix:* in `updateIncidentView`, when the status changed since the last render
  and the select isn't focused and hasn't been edited, set
  `view.form.elements.namedItem('status').value = inc.status`. Tracking
  `view.lastStatus` is enough.
- **FN2. A 409 error disappears along with its form.** If the incident was
  resolved elsewhere, `submitForm` shows "already resolved" in the form's alert
  box, then `refreshNow()` sees it resolved and `updateIncidentView` removes the
  form, error included. A sighted user sees the card jump to "Recently resolved"
  with no explanation. A screen-reader user may hear nothing.
  *Fix:* on `ApiError` with `code === 'failed_precondition'`, also
  `announce(err.message)`, or show it in a page-level message that persists.
- **FN3. Focus goes to `<body>` after resolving.** Resolving from the update
  form removes the form while its submit button has focus, so keyboard users
  are sent back to the top of the page.
  *Fix:* after a successful `resolved` update, focus the incident's `<h3>` with
  `tabindex="-1"` in its new place in the resolved list. The live-region
  announcement already exists, which helps.

### Nits

- **FN4. `fieldForError` can highlight the wrong field.** The server echoes
  the user's value back (`unknown service_id "title"`), and `find` walks the
  field list in order, so the `title` field gets flagged. Anchor the match to
  the backend's message shapes: the first word, or `unknown <field>`. Add a test
  case.
- **FN5. Polling runs while the tab is hidden.** Skip `refresh()` when
  `document.hidden` (the `visibilitychange` handler already catches up on
  return). This saves a request every 15 to 60 s per background tab.
- **FN6. Clickjacking.** The README is correct that `frame-ancestors` can't go
  in a meta tag. Because the page has a form that can irreversibly resolve
  incidents, ask the backend to send
  `Content-Security-Policy: frame-ancestors 'none'` (or `X-Frame-Options: DENY`)
  on static responses. That is the remaining half of backend N4.
- **FN7. `/tests/*.test.js` and `/README.md` are publicly served** from the
  static root. They're harmless (no secrets), but the backend's static handler
  could limit what it serves to the files the page needs, or the tests could
  live outside the served directory. Owner's call.
- **FN8. The "Recently resolved (10)" heading shows the capped count.** Say
  "showing 10 of N", or drop the count when the list is truncated, so the
  heading isn't misleading.
- **FN9. Errors in `refresh()` are always reported as connectivity
  problems.** A rendering bug (non-`ApiError`) becomes "Could not refresh
  status: Unexpected error… retrying". That's acceptable because it's logged to
  the console, but a different message for non-`ApiError` cases would make
  bugs easier to spot.

### Praise

- **XSS safety is enforced by construction, not by convention.** `dom.js`'s
  `el()` and `append()` only create text nodes for strings, and `el()`
  *throws* on any `on*` attribute. Every render path goes through them, so
  using `innerHTML` would mean deliberately bypassing the module. Together
  with the strict CSP, this gives defence in depth, and I confirmed it live
  with real payloads.
- **Rendering is keyed and works in place.** Cards are reused by id, `setText`
  skips no-op writes, and nodes move only when out of order ("moving a node
  blurs focus"). As a result, polling never collapses an open timeline, wipes a
  half-typed message, or steals focus. That's careful, user-centred
  engineering.
- **Polling is correct.** `schedulePoll` always clears the previous timer, so
  timers never stack, and the `refreshSeq` sequence number drops stale or
  superseded responses. Failed refreshes keep the last known data and show a
  `role="alert"` banner. Service loading retries until it succeeds.
- **The API client is small and robust.** It handles network failure, non-JSON
  error bodies and invalid success bodies, all as one `ApiError`. Ids are
  `encodeURIComponent`-escaped, which is tested with `inc/../1`. It sets
  `cache: 'no-store'`. `fetchFn` is injectable, which keeps the tests
  hermetic.
- **The pure helpers are separated** in `format.js` and run under
  `node --test`. `labelFor` uses `Object.hasOwn`, so it's safe against
  prototype keys (tested with `toString`). `splitIncidents` keeps server order
  on purpose, which shows the author read backend choice (2).
- **Accessibility goes beyond the spec:**
  - A skip link.
  - A per-status icon *shape* (circle, diamond, triangle, square) as well as
    the text label.
  - Status pairs meet WCAG AA contrast, as documented.
  - `prefers-reduced-motion`.
  - A polite live region that clears and re-sets its text, so repeated
    messages are announced.
  - `aria-describedby` linking invalid fields to the error text.
- **Idiomatic modern JS:** ES modules, `const` throughout, `??` and `?.`,
  frozen label maps, JSDoc types, and no globals besides one `state` object.
  The README is clear and documents every hook.

---

## Overall

| Component | Verdict |
|---|---|
|| Backend  | **LGTM** (R1 fixed and verified) |
|| Frontend | **LGTM** (FN1–FN3 recommended, not blocking) |

---

## Final Verification

- `/workspace/backend/status.json`: Exists and is valid JSON.
- `/workspace/frontend/status.html`: Exists and is valid HTML.

---

## Verification: /api/info + repo badge

**Date:** 2026-10-09

**Backend commit:** a911d1a
**Frontend commit:** 2245e66

**Verdict: APPROVED**

### Backend

- The new endpoint `GET /api/info` correctly returns the repository, sandbox, and orchestrator.
- A new test `TestInfo` was added and passes.

```
ok  	incidentdash	0.047s
ok  	incidentdash/internal/api	0.105s
ok  	incidentdash/internal/store	0.022s
```

### Frontend

- A footer with a badge showing the repository and sandbox information has been added to `index.html`.
- The new styles for the footer and badge are correctly implemented in `styles.css`.
