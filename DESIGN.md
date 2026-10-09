# Incident Status Dashboard: Design Doc

**Author:** eng-lead · **Status:** Approved for implementation · **Date:** 2026-10-02

## 1. Context

On-call engineers and stakeholders need one page that answers two questions:
"Is anything broken right now?" and "What is being done about it?" This
project is a small, self-contained Incident Status Dashboard. It has a JSON API
and a browser UI that can list, declare, update, and resolve incidents across a
fixed set of services.

## 2. Goals

- A Go JSON API (standard library only, no third-party modules) with
  in-memory, concurrency-safe storage and unit tests.
- A dashboard UI with no build step (plain HTML, CSS, and ES-module JavaScript,
  no npm dependencies). It shows overall status, per-service status, and open
  and resolved incidents. It also lets users declare an incident and post
  status updates.
- A single binary serves both the API and the static UI, so local runs and E2E
  verification take one command.
- Clear input validation and a consistent error envelope.

## 3. Non-Goals

- Persistence across restarts (in-memory only; seeded on startup).
- Authentication and authorization, multi-tenancy, and notifications
  (email, pager).
- Production deployment or containerization.
- Live push updates (the UI polls instead; WebSockets and SSE are out of scope).

## 4. Directory Layout and Ownership

| Path                    | Owner                 | Contents                                         |
|-------------------------|-----------------------|--------------------------------------------------|
| `/workspace/DESIGN.md`  | eng-lead              | This document                                    |
| `/workspace/backend/`   | swe-backend           | Go module `incidentdash`, `main.go`, `internal/...`, `*_test.go`, `README.md` |
| `/workspace/frontend/`  | swe-frontend          | `index.html`, `styles.css`, `app.js` (+ optional ES modules), `README.md` |
| `/workspace/tests/`     | sre-qa                | E2E/smoke scripts (e.g. `e2e.sh` or `e2e_test.py`, stdlib only) and `REPORT.md` |
| `/workspace/REVIEW.md`  | readability-reviewer  | Review findings                                  |

The workspace is **shared-plain** (shared files, and no git). Edit only the
paths you own. Raise changes to someone else's files with that owner by message.

## 5. Running

```bash
cd /workspace/backend
go test ./...                                   # unit tests (also run with -race)
go run . -addr :8080 -static ../frontend        # API + UI on http://localhost:8080
```

Flags: `-addr` (default `:8080`) and `-static` (default `../frontend`; this is
the directory served at `/`).

## 6. Data Model

```text
Service  { id: string, name: string, status: ServiceStatus }
Incident {
  id: string,                 // "inc-<n>", server-assigned, monotonically increasing
  title: string,              // required, 1..120 chars after trim
  description: string,        // optional, <= 2000 chars
  service_id: string,         // required, must reference a known service
  severity: "sev1"|"sev2"|"sev3"|"sev4",
  status: "investigating"|"identified"|"monitoring"|"resolved",
  created_at: RFC3339 UTC,
  updated_at: RFC3339 UTC,
  resolved_at: RFC3339 UTC | null,
  updates: [ { status, message, created_at } ]   // chronological, first entry = creation
}
```

**Service status is derived** from that service's open (non-resolved)
incidents, using the worst severity among them:
`sev1 → major_outage`, `sev2 → partial_outage`, `sev3|sev4 → degraded`, none →
`operational`.

**Overall status** is the worst status across all services. The order is
`operational < degraded < partial_outage < major_outage`.

**Seed data** (loaded at startup): services `api` (Public API), `web` (Web App),
`db` (Database), `auth` (Authentication), and `payments` (Payments). There are
also 2 seed incidents: one open `sev2` on `payments` and one `resolved` `sev3`
on `web`.

## 7. API Contract (v1)

All responses are `Content-Type: application/json`. All timestamps are RFC3339
UTC. Request bodies are JSON, and the API rejects unknown fields and bodies
larger than 64 KiB.

**Error envelope** (every 4xx/5xx response):

```json
{ "error": { "code": "invalid_argument", "message": "title is required" } }
```

The codes are `invalid_argument` (400), `not_found` (404),
`method_not_allowed` (405), `failed_precondition` (409), and `internal` (500).

| Method | Path                               | Description |
|--------|------------------------------------|-------------|
| GET    | `/healthz`                         | `200 {"status":"ok"}` |
| GET    | `/api/v1/summary`                  | `{ "overall_status", "open_incidents": int, "services": [Service] }` |
| GET    | `/api/v1/services`                 | `{ "services": [Service] }`, sorted by id |
| GET    | `/api/v1/incidents`                | `{ "incidents": [Incident] }`, newest first. Optional `?status=open\|resolved` and `?service_id=<id>`. Invalid filter values → 400 |
| POST   | `/api/v1/incidents`                | Body `{title, description?, service_id, severity}` → `201` + Incident (status `investigating`, 1 update with message `"Incident declared"`) |
| GET    | `/api/v1/incidents/{id}`           | Incident, or 404 |
| POST   | `/api/v1/incidents/{id}/updates`   | Body `{status, message}`. `message` is required (1..1000 chars). Appends an update and sets the incident status. If `resolved`, sets `resolved_at`. Updating an already-resolved incident → 409 `failed_precondition`. Returns `200` + Incident |

## 8. Frontend Requirements

- **Header banner:** the overall status, colour coded (green, yellow, orange,
  red), and the count of open incidents.
- **Services grid:** one card per service with a status badge.
- **Incidents:** "Open" and "Recently resolved" sections. Each incident shows
  its title, service, severity, status, and relative time. It also has an
  expandable timeline of updates.
- **Declare incident form:** title, service (a dropdown filled from
  `/api/v1/services`), severity, and description. Show the API's validation
  errors inline.
- **Post update form** on each open incident: a status select and a message.
- The UI polls `/api/v1/summary` and `/api/v1/incidents` every 15 seconds and
  refreshes after each mutation.
- **Accessibility:** semantic HTML, labelled form controls, and visible focus
  states. Status is never conveyed by colour alone (always include a text
  label).
- **Security:** render all user content with `textContent` (never `innerHTML`
  with API data).
- Call the API with same-origin relative URLs (`/api/v1/...`).
- Add stable `data-testid` attributes for E2E, for example `overall-status`,
  `service-card-<id>`, `incident-<id>`, `declare-form`, and `update-form-<id>`.

## 9. Testing and Verification

- **Backend unit tests:** store logic (status derivation, ordering, filters,
  and state transitions, including the 409), and handler tests with
  `httptest` covering every endpoint and error path. Run `go test -race ./...`
  and `go vet ./...`, both of which must be clean.
- **Readability review:** Go and JS idioms, naming, error handling,
  concurrency safety, input validation, and XSS safety. Findings go to
  `/workspace/REVIEW.md` with an LGTM or a list of required changes.
- **SRE/QA:** build and run the server, then run an E2E smoke test with stdlib
  tooling (curl and Python). It covers every endpoint in §7, including error
  paths, the declare → update → resolve flow, the derived service status, and a
  check that the UI is served at `/` with the expected `data-testid` hooks.
  Report in `/workspace/tests/REPORT.md` with a PASS/FAIL verdict.

## 10. Workflow

1. swe-backend and swe-frontend implement in parallel against the contract in
   §7. If the contract needs to change, message eng-lead first.
2. Each implementer messages @eng-lead when ready.
3. eng-lead dispatches readability-reviewer and sre-qa. Fixes go back to the
   owner.
4. Sign-off once tests pass and both reviewers give LGTM.
