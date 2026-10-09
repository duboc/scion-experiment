# Deployment

## Cloud Run — incident-dashboard

| Field | Value |
|---|---|
| Live URL | https://incident-dashboard-249096448072.us-central1.run.app |
| Service | `incident-dashboard` |
| Revision | `incident-dashboard-00001-cc9` (100% traffic) |
| Project / region | `riojucu-sandbox` / `us-central1` |
| Deployed commit | `8d45577` |
| Date | 2026-10-09 |
| Access | Public (`--allow-unauthenticated`) |

Deployed with:

```sh
gcloud run deploy incident-dashboard --project=riojucu-sandbox --region=us-central1 \
  --source=/workspace --allow-unauthenticated --quiet
```

## Verification (curl)

| Request | Status | Result |
|---|---|---|
| `GET /` | 200 | Dashboard UI served |
| `GET /api/info` | 200 | `{"orchestrator":"lead","repo":"duboc/scion-experiment","sandbox":"riojucu-sandbox"}` (matches expected) |
| `GET /api/v1/summary` | 200 | `{"overall_status":"partial_outage","open_incidents":1,"services":[...5 services...]}` |

## Known issue

`GET /healthz` returns a **Google front-end 404**, not the app's response. The app registers
`/healthz` (`backend/internal/api/api.go`), but Cloud Run reserves some URL paths ending in `z`
(including `/healthz`), so these requests never reach the container.

Use `/api/info` or `/api/v1/summary` for health checks on Cloud Run. A follow-up could add a
non-reserved alias (for example `/api/v1/health`) and update `deploy/deploy.sh`, which still
suggests `curl $url/healthz` as the smoke test.
