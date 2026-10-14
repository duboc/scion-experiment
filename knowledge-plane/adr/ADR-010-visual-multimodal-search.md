# ADR-010: Visual & Multimodal Search (photo + text refinement)

- **Status:** Accepted (Issue #33)
- **Covered Issue:** GitHub **Issue #33**, "Visual & Multimodal Search" (busca por foto + refinamento textual na mesma query)
- **Migrations:** none. The query photo is matched against the existing `products.embedding_v2` column (migration 011).

## Context
Query embedding was text-only (`EmbeddingService.EmbedQuery`), and `/api/search` could not accept an image.

The issue suggests adding `image_embedding VECTOR(1408)` (Vertex AI `multimodalembedding@001`) with a ScaNN index, then combining the photo vector with the text-modifier vector in the hybrid query.

PSearch already uses **`gemini-embedding-2`** (1024-d, location `global`) for product vectors in `embedding_v2`. That model is natively multimodal: text and images share one embedding space.

A live probe on the sandbox (deployer, 2026-10-14, google-genai 2.30.1) confirmed this:
- A running-shoe photo embedded at `output_dimensionality=1024` ranked all 4 footwear products first against the live `embedding_v2` catalog vectors.
- A refrigerator photo ranked both refrigerators first.
- Cross-modal cosines are low (~0.25–0.35, versus ~0.7+ for text-to-text), so ranking must be rank-based (RRF), not threshold-based.
- The model accepts one content per call, so a batch of contents is rejected. A single content may still mix image and text parts.
- Warm image calls take ~350–550 ms; a cold call takes ~1 s.

## Decision
1. **Model and space.** Embed the query photo with `gemini-embedding-2` at the configured dimension (1024).
   - Image calls take no task type.
   - The photo vector searches `embedding_v2` directly, so there is no new column, index or catalog backfill.
2. **API.** `POST /api/search` (and `/search`) accepts `image_base64` (raw base64 or a `data:` URL), `image_mime_type` and the optional `image_weight` in `(0,1]`, alongside the optional text `query`.
   - Accepted types are `image/jpeg`, `image/png` and `image/webp`, verified by magic bytes. The decoded image is capped at 4 MB.
   - The request body is capped at 8 MB (HTTP 413).
   - **Image URLs are never fetched server-side (SSRF).**
3. **Fusion.**
   - Image + text: query vector = `normalize(w·unit(v_img) + (1−w)·unit(v_txt))`, with `w = image_weight` (default **0.6**). The text also drives the full-text leg, so a modifier like "verde" still matches lexically.
   - Synonyms, catalog filters, merchandising, variants (ADR-009), facets, LTR, pricing and zero-results recovery apply unchanged. The image vector rides in `SearchFilters.ImageVector`, so recovery re-runs keep it.
   - Default `alpha` is **0.7** when an image is present and the caller did not set `alpha`.
   - Image only: a **vector-only** SQL mode (`buildHybridSQLMode(..., vectorOnly)`) runs the ann leg alone with weight 1.0. It is never treated as a catalog browse.
4. **Observability.** `retrieval_mode = multimodal_v2`, plus `image_query`, `image_weight` and `image_cache_hit` in the response.
5. **Caching.** An in-process LRU of 256 entries keyed by `sha256(mime ‖ bytes)`. Errors are not cached.
6. **Degradation.**

   | Situation | Result |
   |---|---|
   | Invalid image or weight | 400 |
   | `ACTIVE_EMBEDDING_VERSION=v1` or no embedder | 503 |
   | Image embed fails, text present | text-only hybrid |
   | Image embed fails, image only | 503 |
   | Text embed fails, image present | image vector plus full-text leg |
   | No Spanner (memory engine), image only | empty result (the memory engine has no vectors in this space) |
   | No Spanner (memory engine), image + text | text refinement only |

7. **UI.** Storefront and React have a **Photo** button that resizes client-side to ≤512 px JPEG and sends the image inline. The text box acts as the refinement, and the mode badge shows the photo weight.
8. **Gate.** `cmd/sqlcheck` embeds a live image (`SQLCHECK_IMAGE` or a generated JPEG) and runs the vector-only, fused and fused + variant-filter SQL against Spanner.

## Deviations from the issue's suggestion
- **No `image_embedding VECTOR(1408)` column yet.** The demo catalog's product images are random placeholders (picsum), so per-product image vectors would add noise. When real catalog photos exist, add an `image_embedding ARRAY<FLOAT32>(vector_length=>1024)` column embedded with `gemini-embedding-2`. It stays in the same space, so it can join the RRF as a third leg.
- **No ScaNN index yet.** With a small catalog, exact `COSINE_DISTANCE` is exact and cheap. Revisit together with a switch to `APPROX_COSINE_DISTANCE` when the catalog grows. This is already on the follow-up list.

## Consequences
- Photo search works over the existing catalog vectors with zero DDL and zero backfill cost.
- Each new photo costs one extra Vertex AI embedding call (~0.35–0.55 s warm). Repeats are served from the LRU.
- Cross-modal scores are not comparable to text-only scores. `min_score` thresholds tuned for text should not be reused for photo queries.
