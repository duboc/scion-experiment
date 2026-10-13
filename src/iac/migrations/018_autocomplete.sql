CREATE TABLE autocomplete_suggestions (
  tenant_id STRING(64) NOT NULL,
  locale STRING(16) NOT NULL,
  suggestion_text STRING(256) NOT NULL,
  search_count INT64,
  ctr_score FLOAT64,
  updated_at TIMESTAMP OPTIONS (allow_commit_timestamp = true),
  suggestion_folded STRING(256) AS (REGEXP_REPLACE(NORMALIZE(LOWER(suggestion_text), NFD), r'\p{M}', '')) STORED,
  suggestion_ngrams TOKENLIST AS (TOKENIZE_NGRAMS(REGEXP_REPLACE(NORMALIZE(LOWER(suggestion_text), NFD), r'\p{M}', ''), ngram_size_min=>2, ngram_size_max=>4)) HIDDEN
) PRIMARY KEY (tenant_id, locale, suggestion_text);
CREATE SEARCH INDEX autocomplete_ngram_idx ON autocomplete_suggestions(suggestion_ngrams)
