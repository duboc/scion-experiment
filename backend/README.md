# Incident Status Dashboard — Backend

Go JSON API (standard library only) for the Incident Status Dashboard.
Contract: [`/workspace/DESIGN.md`](../DESIGN.md) §6 (data model) and §7 (API).

## Run

```bash
cd /workspace/backend
go run . -addr :8080 -static ../frontend   # API + UI on http://localhost:8080
```

| Flag / env | Default       | Meaning                    |
|------------|---------------|----------------------------|
| `-addr`    | `:8080`       | Listen address. If given explicitly, it always wins, even when set to `:8080`. |
| `PORT`     | unset         | If `-addr` is not given and `PORT` is non-empty, listen on `:$PORT`. Cloud Run injects this variable. It must be a decimal number from 1 to 65535, otherwise the server exits with an error. |
| `-static`  | `../frontend` | Directory served at `/`    |

```bash
PORT=9090 go run . -static ../frontend              # listens on :9090
PORT=9090 go run . -addr :7000 -static ../frontend  # listens on :7000 (explicit -addr wins)
```

The store is in memory and re-seeded on every start (5 services, an open sev2
on `payments` as `inc-2`, a resolved sev3 on `web` as `inc-1`). The server
shuts down gracefully on SIGINT/SIGTERM. If the static directory is missing it
logs a warning and `/` returns 404; the API still works.

## Test

```bash
go vet ./...
go test -race ./...
go test -cover ./...
```

## Layout

| Path                       | Purpose |
|----------------------------|---------|
| `main.go`                  | Flags, HTTP server timeouts, graceful shutdown |
| `internal/store/`          | Domain types, validation, derived status, `sync.RWMutex`-guarded in-memory store, seed data |
| `internal/api/`            | Routing (`net/http.ServeMux`), JSON decoding, error envelope, static file serving |

## Behaviour notes

- Every API response, including errors, is `Content-Type: application/json`.
  Errors use `{"error":{"code","message"}}` with codes `invalid_argument`
  (400), `not_found` (404), `method_not_allowed` (405, with an `Allow` header),
  `failed_precondition` (409) and `internal` (500, also used for recovered
  panics). Unknown `/api` and `/api/...` routes return a JSON 404.
- `/healthz` accepts GET and HEAD.
- Every response (API, errors, recovered panics and static files) carries these
  hardening headers, set by one middleware:
  - `X-Content-Type-Options: nosniff`
  - `Content-Security-Policy: frame-ancestors 'none'` and `X-Frame-Options: DENY`,
    which stop any site from framing the dashboard (clickjacking). The second
    one is for older browsers.

  The CSP header contains only `frame-ancestors`. Browsers enforce every policy
  they receive, so this header adds to the frontend's `<meta http-equiv>` CSP
  and never loosens it. The frontend's CSP controls scripts and styles. A meta
  tag can't express `frame-ancestors`, which is why it is sent as a header.
- The static handler returns 404 for directories without an `index.html`
  instead of listing them.
- POST requests must send `Content-Type: application/json` (parameters such as
  `; charset=utf-8` are fine). Any other or missing content type gets 400
  `invalid_argument`. This makes browsers send a CORS preflight before any
  cross-origin write. The server never approves preflights, so other web pages
  cannot declare or resolve incidents. With curl, pass
  `-H 'Content-Type: application/json'`.
- Request bodies must be a single JSON object of at most 64 KiB with no unknown
  fields. Oversized bodies are rejected with 400 `invalid_argument`.
- `title`, `description` and update `message` are trimmed of surrounding
  whitespace before validation; lengths are counted in characters (runes).
- Timestamps are UTC, second precision, RFC3339 (e.g. `2026-10-02T12:00:00Z`).
  `resolved_at` is `null` until an incident is resolved.
- Incidents are listed newest first by `created_at`, ties broken by higher id.
  Clients should keep the server's order rather than re-sorting by timestamp,
  because timestamps have only second precision.
- `POST /api/v1/incidents` returns `201` with a `Location` header.
- Status updates may move an open incident to any status (including back to
  `investigating`); updating a resolved incident returns 409.
