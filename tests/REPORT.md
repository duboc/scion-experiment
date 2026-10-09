# E2E Verification Report: Incident Status Dashboard

**Verifier:** eng-lead (stood in for sre-qa, which was unresponsive) · **Date:** 2026-10-02 · **Verdict: PASS**

## How to run
```bash
python3 /workspace/tests/e2e_test.py -v   # stdlib only; builds and starts a fresh server on a free port
```

## Results
| Suite | Result |
|-------|--------|
| E2E (`tests/e2e_test.py`) | 15/15 PASS (9.2 s) |
| Backend `go vet ./...` and `go test -race ./...` | clean / all ok |
| Frontend `node --test tests/*.test.js` | 15/15 PASS |

## Coverage of the DESIGN.md §7 contract
- `/healthz`, plus the `nosniff` header on API responses and on static files.
- Seed data: 5 services sorted by id; payments `partial_outage`; web `operational`; overall `partial_outage`; 1 open incident.
- Full flow: declare → monitoring → resolved. Checks the 201 response with a Location header, title trimming, the first update "Incident declared", and that `resolved_at` is set. Updating a resolved incident returns 409 `failed_precondition`.
- Derived status: sev4 or sev3 → degraded, sev2 → partial_outage, sev1 → major_outage; the worst open incident wins, and the service is operational again after resolve. The overall status follows the worst service.
- Lists are newest first, and both the `status` and `service_id` filters work.
- Errors, always in the JSON envelope:
  - 400 for blank or too-long titles (the 120-character boundary is accepted), too-long descriptions, an unknown service or severity, unknown fields, malformed JSON, and bodies over 64 KiB.
  - 400 for bad update messages or statuses, and for bad filter values.
  - 404 for an unknown incident or route; 405 (with an `Allow` header) for wrong methods.
- **CSRF guard:** `text/plain`, form-encoded, and missing Content-Type all return 400. `application/json; charset=utf-8` returns 201.
- **UI:**
  - `/` is served as HTML with the CSP meta tag and the §8 `data-testid` hooks, and its assets load.
  - No JS file uses `innerHTML`.
  - Headless Chromium runs the JS and renders the 5 service cards, the incident cards, and the update forms. These elements aren't in the static HTML, so the check proves the JS ran.

## Notes
- A deeper browser test (11 interactive steps: polling keeps state, inline errors, an XSS payload rendered as text) was run by swe-backend in its own scratch space. It isn't checked in.
