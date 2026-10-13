# ADR-007: Conversational Query Understanding & Session-Aware Personalization

- **Status:** Delivered & Live (`psearch-serving-00024-ltv`; #23 upgrades LLM fallback default to `gemini-3.8-flash`)
- **Covered Issues:** GitHub **Issue #19** (`[Req 4.1.11]`), **Issue #20** (`[Req 4.1.12]`), **Issue #23** (Model upgrade)
- **Migrations:** `src/iac/migrations/020_query_intent_cache.sql`, `src/iac/migrations/021_user_session_events.sql`

## Context (GitHub Issues #19 — Req 4.1.11, #20 — Req 4.1.12)
1. **#19 (Conversational Query Understanding):** Natural-language queries (`"tênis de corrida até 300 reais"`, `"running shoes nike under 200"`) previously reached hybrid search verbatim. Price/brand/color constraints polluted the lexical `SEARCH()` score and were not enforced as structured `WHERE` filters.
2. **#20 (Session-Aware Personalization):** Search is stateless; two shoppers with different browsing/purchase histories get identical rankings for broad queries.

## Decision & `psearch` Implementation
1. **Query Understanding (#19 — `internal/services/query_understanding.go`):**
   - `QueryUnderstandingService`: a **deterministic parser first** (price expressions in pt-BR/en/es, catalog brands, categories, and colors) producing `clean_query` + `extracted_filters` (`min_price`, `max_price`, `brand`, `category`, `color`).
   - **Gemini Flash Lite fallback** (`google.golang.org/genai`, Vertex `global`, structured JSON schema) **only** when the parser finds no entities in a query that still looks conversational ($\ge 4$ tokens). Results are cached in two tiers (in-memory LRU + Spanner `query_intent_cache` in `020_query_intent_cache.sql`). Failures degrade gracefully to the parser result.
   - Enabled automatically or via `parse_intent=true`; explicit request filters always override extracted ones.
   - `min_price` / `max_price` (and `color` when the catalog has it) are applied in **both** RRF legs via `catalogFilterSQL` (bound params) and mirrored in `matchesCatalog` for memory mode. The response carries `parsed_query {original_query, clean_query, extracted_filters, intent_source: parser|gemini|cache}`.
2. **Session Personalization (#20 — `internal/services/session.go`):**
   - Table `user_session_events (tenant_id, session_id, event_id, product_id, event_type, created_at) PRIMARY KEY (tenant_id, session_id, event_id)`; `POST /api/session/events` (`click` | `purchase`; validated, rate-limited per session size).
   - Session centroid = normalized weighted mean of the last $N$ ($\le 20$) products' active embeddings (`purchase` weight `3`, `click` weight `1`, exponential recency decay).
   - Personalization is a **bounded re-rank boost** (`score.session_boost = w · cos(centroid, product)`, `w` default `0.003`, comparable to the ADR-004 boost scale) applied in Go after Stage 1, keeping availability and pin/bury precedence. The response returns `session_id` and `personalized=true`.
   - Without a session (or on any failure) results are identical to non-personalized search.

## Consequences
- No PII: `session_id` is an opaque client token; events hold product IDs only.
- Gemini cost/latency is bounded by the parser-first strategy and two-tier Spanner caching.
