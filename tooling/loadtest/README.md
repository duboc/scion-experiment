# Load test the PSearch Serving API

This directory contains Locust load tests for the PSearch Serving API (`psearch-serving`).
This document follows the Google Developer documentation style and the ASD-STE100 Simplified Technical English standard.

> [!IMPORTANT]
> Do not commit real Cloud Run URLs or project IDs to tracked files.
> Replace `SERVICE_URL` with your target endpoint (for example, `http://localhost:8080`).

## Overview

The Locust test suite (`locustfile.py`) simulates concurrent shoppers calling the PSearch Serving API:

- **Health probe task (`weight=1`):** Sends `GET /health` requests and verifies that `"status": "healthy"` is returned.
- **Product search task (`weight=5`):** Sends `POST /search` requests with realistic queries and limits from `search_queries.py` and validates that `results` and `total_found` exist in every `HTTP 200` response.

## Before you begin

1. Install Python 3.11 or later and `pip`.
2. Start a local `psearch-serving` instance (`USE_MEMORY_STORE=true go run ./cmd/server`) or obtain the URL of your deployed Cloud Run service.

## Quickstart

1. Open a terminal in `tooling/loadtest`:

   ```bash
   cd tooling/loadtest
   ```

2. Install the Python dependencies:

   ```bash
   python3 -m pip install -r requirements.txt
   ```

3. Run a 1-minute headless smoke load test against a local server:

   ```bash
   locust -f locustfile.py --headless -u 20 -r 5 --run-time 1m --host=http://localhost:8080
   ```

## Configure the load test

You can configure the target host and user wait time in a `.env` file or with environment variables (`config.py`).

1. Copy `.env.example` to `.env`:

   ```bash
   cp .env.example .env
   ```

2. Set the environment variables in `.env` or export them in your shell:

   | Variable | Default | Description |
   |---|---|---|
   | `API_HOST` | `https://product-search-api-1234567890.us-central1.run.app` | Default target URL when `--host` is not passed on the command line. |
   | `LOCAL_API_HOST` | `http://localhost:8080` | Local development target URL. |
   | `MIN_WAIT_TIME` | `1000` | Minimum wait time between requests per simulated user (milliseconds). |
   | `MAX_WAIT_TIME` | `5000` | Maximum wait time between requests per simulated user (milliseconds). |

## Run interactive and headless tests

### Use the Locust web UI

1. Start Locust from `tooling/loadtest`:

   ```bash
   locust -f locustfile.py --host="$SERVICE_URL"
   ```

2. Open `http://localhost:8089` in your browser.
3. Enter the number of concurrent users and the spawn rate (users per second), then click **Start swarming**.

### Export CSV and HTML reports in headless mode

Run Locust with `--headless`, `--csv`, and `--html` for automated CI benchmarks:

```bash
locust -f locustfile.py \
  --headless \
  -u 100 \
  -r 10 \
  --run-time 5m \
  --host="$SERVICE_URL" \
  --csv=results \
  --html=report.html
```

This command writes four output files:

- `results_stats.csv`: Request counts, latency percentiles (`p50`, `p95`, `p99`), and throughput.
- `results_failures.csv`: Failed HTTP requests and status codes.
- `results_exceptions.csv`: Unhandled Python exceptions during the run.
- `report.html`: Standalone HTML summary report.

## Customize queries and task weights

1. To add search payloads, append entries to `SEARCH_QUERIES` in `search_queries.py`:

   ```python
   SEARCH_QUERIES.append({"query": "running shoes", "limit": 12, "alpha": 0.5})
   ```

2. To adjust the ratio of search requests to health checks, change the `@task(N)` decorator weights in `locustfile.py`.

## Troubleshoot common issues

| Symptom | Cause | Solution |
|---|---|---|
| High `HTTP 429` or `HTTP 503` rate on Cloud Run | Concurrency exceeds the Cloud Run max-instances setting or Vertex AI quota on uncached queries. | Enable the two-tier `EmbeddingCache` on `psearch-serving` or increase Cloud Run max instances. |
| Connection refused on `localhost:8080` | The local `psearch-serving` process is not running. | Start the server in `src/psearch/serving` with `USE_MEMORY_STORE=true go run ./cmd/server`. |

## What's next

- Review the API endpoints and parameters in [`../../docs/USAGE.md`](../../docs/USAGE.md).
- Check the production deployment guide in [`../../DEPLOYMENT.md`](../../DEPLOYMENT.md).
