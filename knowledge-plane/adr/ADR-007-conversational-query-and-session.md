# ADR-007: Conversational Query Understanding & Session-Aware Personalization

## Context (GitHub Issues #19 — Req 4.1.11, #20 — Req 4.1.12)
1. **#19:** Natural-language queries ("tênis de corrida até 300 reais", "running shoes nike under 200") reach hybrid search verbatim. Price/brand/color constraints pollute the lexical score and are not enforced.
2. **#20:** Search is stateless; two shoppers with different histories get identical rankings.

## Decision
1. **Query understanding (#19):**
   - `QueryUnderstandingService` (`internal/services/query_understanding.go`): a **deterministic parser first** (price expressions in pt-BR/en/es, catalog brands, categories and colors) producing `clean_query` + `extracted_filters` (`min_price`, `max_price`, `brand`, `category`, `color`).
   - **Gemini Flash Lite fallback** (`google.golang.org/genai`, Vertex `global`, structured JSON schema) **only** when the parser finds no entities in a query that still looks conversational (≥ 4 tokens). Results are cached in two tiers (LRU + Spanner `query_intent_cache`). Failures degrade to the parser result.
   - Opt-in via `parse_intent=true`; explicit request filters always override extracted ones.
   - `min_price` / `max_price` (and `color` when the catalog has it) are applied in **both** RRF legs via `catalogFilterSQL` (bound params). The response carries `parsed_query {original_query, clean_query, extracted_filters, intent_source: parser|gemini|cache}`.
2. **Session personalization (#20):**
   - Table `user_session_events (tenant_id, session_id, event_id, product_id, event_type, created_at) PRIMARY KEY (tenant_id, session_id, event_id)`; `POST /api/session/events` (click | purchase; validated, rate-limited per session size).
   - Session centroid = normalized weighted mean of the last N (≤ 20) products' active embeddings (purchase weight 3, click 1, exponential recency decay).
   - Personalization is a **bounded re-rank boost** (`score.session_boost = w · cos(centroid, product)`, `w` default 0.003, comparable to the ADR-004 boost scale) applied in Go after stage 1, keeping availability/pins precedence. The response returns `session_id` and `personalized=true`.
   - Without a session (or on any failure) results are identical to non-personalized search.

## Consequences
- No PII: `session_id` is an opaque client token; events hold product IDs only.
- Gemini cost/latency is bounded by the parser-first strategy and caching.
