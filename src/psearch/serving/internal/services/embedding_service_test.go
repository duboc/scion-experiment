package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/models"
)

func testConfig(version string) *config.Config {
	return &config.Config{
		ProjectID:                "test-project",
		GenAILocation:            "global",
		EmbeddingModelName:       "gemini-embedding-2",
		GeminiEmbeddingDimension: 1024,
		ActiveEmbeddingVersion:   version,
		UseVectorIndex:           version == "v2",
		ANNNumLeavesToSearch:     20,
	}
}

func fakeEmbed(dim int, err error, captured *genai.EmbedContentConfig, model *string) embedContentFunc {
	return func(_ context.Context, m string, _ []*genai.Content, cfg *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
		if captured != nil && cfg != nil {
			*captured = *cfg
		}
		if model != nil {
			*model = m
		}
		if err != nil {
			return nil, err
		}
		return &genai.EmbedContentResponse{Embeddings: []*genai.ContentEmbedding{{Values: make([]float32, dim)}}}, nil
	}
}

func TestEmbedQuery_RequestsConfiguredModelDimensionAndTaskType(t *testing.T) {
	var got genai.EmbedContentConfig
	var model string
	svc := newEmbeddingServiceWith(testConfig("v2"), fakeEmbed(1024, nil, &got, &model))

	vec, err := svc.EmbedQuery(context.Background(), "tênis de corrida")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 1024 {
		t.Fatalf("want 1024 dims, got %d", len(vec))
	}
	if model != "gemini-embedding-2" {
		t.Errorf("want model gemini-embedding-2, got %q", model)
	}
	if got.TaskType != "RETRIEVAL_QUERY" {
		t.Errorf("want task type RETRIEVAL_QUERY, got %q", got.TaskType)
	}
	if got.OutputDimensionality == nil || *got.OutputDimensionality != 1024 {
		t.Errorf("want OutputDimensionality=1024, got %v", got.OutputDimensionality)
	}
}

func TestEmbedQuery_RejectsDimensionMismatchAndErrors(t *testing.T) {
	svc := newEmbeddingServiceWith(testConfig("v2"), fakeEmbed(3072, nil, nil, nil))
	if _, err := svc.EmbedQuery(context.Background(), "q"); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
	svc = newEmbeddingServiceWith(testConfig("v2"), fakeEmbed(0, errors.New("403 PERMISSION_DENIED"), nil, nil))
	if _, err := svc.EmbedQuery(context.Background(), "q"); err == nil {
		t.Fatal("expected upstream error to propagate")
	}
}

type stubEmbedder struct {
	vec []float32
	err error
}

func (s stubEmbedder) EmbedQuery(context.Context, string) ([]float32, error) { return s.vec, s.err }

func TestPlanVectorQuery_V1KeepsLegacyDeterministic768(t *testing.T) {
	svc := &SpannerService{config: testConfig("v1"), embeddings: stubEmbedder{err: errors.New("must not be called")}}
	plan := svc.planVectorQuery(context.Background(), "tenis")
	if plan.lexicalOnly || plan.column != vectorColumnV1 {
		t.Fatalf("v1 plan wrong: %+v", plan)
	}
	if v, ok := plan.embedding.([]float64); !ok || len(v) != config.LegacyEmbeddingDimension {
		t.Fatalf("v1 must use 768-d deterministic embedding, got %T", plan.embedding)
	}
}

func TestPlanVectorQuery_V2UsesGemini(t *testing.T) {
	svc := &SpannerService{config: testConfig("v2"), embeddings: stubEmbedder{vec: make([]float32, 1024)}}
	plan := svc.planVectorQuery(context.Background(), "tenis")
	if plan.lexicalOnly || plan.column != vectorColumnV2 {
		t.Fatalf("v2 plan wrong: %+v", plan)
	}
	if v, ok := plan.embedding.([]float32); !ok || len(v) != 1024 {
		t.Fatalf("v2 must use 1024-d Gemini embedding, got %T", plan.embedding)
	}
}

func TestPlanVectorQuery_V2DegradesToLexicalOnly(t *testing.T) {
	for name, emb := range map[string]QueryEmbedder{
		"nil service":   nil,
		"embed failure": stubEmbedder{err: errors.New("403 PERMISSION_DENIED")},
	} {
		svc := &SpannerService{config: testConfig("v2"), embeddings: emb}
		if plan := svc.planVectorQuery(context.Background(), "tenis"); !plan.lexicalOnly {
			t.Errorf("%s: expected lexical-only fallback, got %+v", name, plan)
		}
	}
}

func TestBuildHybridSQL(t *testing.T) {
	v2 := buildHybridSQL(vectorColumnV2, false)
	if !strings.Contains(v2, "COSINE_DISTANCE(embedding_v2, @query_embedding)") {
		t.Error("v2 SQL must search embedding_v2")
	}
	lex := buildHybridSQL(vectorColumnV2, true)
	if strings.Contains(lex, "@query_embedding") || strings.Contains(lex, "ann AS") {
		t.Error("lexical-only SQL must not reference the vector leg")
	}
	if !strings.Contains(lex, "fts AS") {
		t.Error("lexical-only SQL must keep the full-text leg")
	}
}

func TestConfigActiveDimension(t *testing.T) {
	if d := testConfig("v1").ActiveDimension(); d != 768 {
		t.Errorf("v1 active dimension want 768, got %d", d)
	}
	if d := testConfig("v2").ActiveDimension(); d != 1024 {
		t.Errorf("v2 active dimension want 1024, got %d", d)
	}
}

func TestHybridSearchDetailed_ReportsMemoryFallbackMode(t *testing.T) {
	svc := &SpannerService{config: testConfig("v2"), seedProducts: DefaultSeedProducts()}
	out, err := svc.HybridSearchDetailed(context.Background(), "tenis", 5, 0, 0.5, models.SearchFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RetrievalMode != RetrievalMemory {
		t.Fatalf("want retrieval mode %q without Spanner, got %q", RetrievalMemory, out.RetrievalMode)
	}
	if len(out.Results) == 0 {
		t.Fatal("memory fallback should return seed results")
	}
}

// Issue #3: catalog filters must be applied inside BOTH legs, and the row
// projection must stay at the 8 columns scanned by spannerHybridSearch.
func TestBuildHybridSQL_CatalogFiltersAndUnavailableLast(t *testing.T) {
	for _, lexical := range []bool{false, true} {
		sql := buildHybridSQL(vectorColumnV2, lexical)
		wantLegs := 3 // ann + fts + sponsored_matches
		if lexical {
			wantLegs = 2 // fts + sponsored_matches
		}
		if n := strings.Count(sql, "COALESCE(is_visible, TRUE)"); n != wantLegs {
			t.Errorf("lexical=%v: visibility filter appears %d times, want %d", lexical, n, wantLegs)
		}
		for _, frag := range []string{"@channel", "@only_available", "unavailable_rank", "ANY_VALUE(is_available) AS is_available", "ANY_VALUE(stock) AS stock"} {
			if !strings.Contains(sql, frag) {
				t.Errorf("lexical=%v: SQL missing %q", lexical, frag)
			}
		}
		// Issue #45: unavailable_rank must appear ONLY in the outer ORDER BY (after LIMIT @limit),
		// never before LIMIT @limit, so OOS lexical matches are not excluded from Stage-1 candidates.
		limitIdx := strings.LastIndex(sql, "LIMIT @limit")
		unavailIdx := strings.Index(sql, "unavailable_rank")
		if limitIdx < 0 || unavailIdx < limitIdx {
			t.Errorf("lexical=%v: unavailable_rank (idx=%d) must appear after Stage-1 LIMIT @limit (idx=%d)", lexical, unavailIdx, limitIdx)
		}
	}
}

func TestMemorySearchFilters(t *testing.T) {
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}
	out, _ := svc.HybridSearchDetailed(context.Background(), "refrigerator", 15, 0, 0.5, models.SearchFilters{Channel: "store", OnlyInStock: true})
	for _, r := range out.Results {
		if isUnavailable(r.Availability, r.AvailableQuantity) || !matchesChannel("store", r.Channels) {
			t.Fatalf("filtered result %s violates filters (avail=%s channels=%v)", r.ID, r.Availability, r.Channels)
		}
	}
}

func emptyFilters() models.SearchFilters { return models.SearchFilters{} }

func TestBuildHybridSQLANN_UsesScaNNVectorIndexAndApproxCosineDistance(t *testing.T) {
	annSQL := buildHybridSQLANN(vectorColumnV2, false, false, true, true, 25)
	for _, want := range []string{
		"APPROX_COSINE_DISTANCE(embedding_v2, @query_embedding, options => JSON '{\"num_leaves_to_search\": 25}') AS distance",
		"FROM products@{FORCE_INDEX=products_embedding_v2_idx}",
		"WHERE embedding_v2 IS NOT NULL",
		"ORDER BY distance ASC",
		"LIMIT @ann_limit",
		"JOIN products AS p ON p.product_id = a.product_id\n\t\t\t\t\tWHERE COALESCE(p.is_visible, TRUE)",
	} {
		if !strings.Contains(annSQL, want) {
			t.Errorf("ANN SQL missing %q", want)
		}
	}
	// Defect 1 regression test: extract the ann CTE before fts AS and assert it
	// contains ONLY STORING-column predicates and ZERO non-STORING predicates
	// (no channels IN UNNEST, product_prices, skus, or regional_inventory).
	annIdx := strings.Index(annSQL, "ann AS (")
	ftsIdx := strings.Index(annSQL, "fts AS (")
	if annIdx < 0 || ftsIdx <= annIdx {
		t.Fatalf("could not isolate ann CTE from hybrid SQL")
	}
	annCTE := annSQL[annIdx:ftsIdx]
	for _, forbidden := range []string{
		"channels",
		"UNNEST(IFNULL(channels",
		"product_prices",
		"is_exclusive",
		"FROM skus",
		"regional_inventory",
		"@zip5",
		"@price_table",
		"@color",
		"@size",
	} {
		if strings.Contains(annCTE, forbidden) {
			t.Errorf("ann CTE must not contain non-STORING predicate %q; got:\n%s", forbidden, annCTE)
		}
	}

	// Exact mode (useANN=false) or v1 column must keep exact COSINE_DISTANCE.
	exactSQL := buildHybridSQLANN(vectorColumnV2, false, false, true, false, 20)
	if strings.Contains(exactSQL, "APPROX_COSINE_DISTANCE") || !strings.Contains(exactSQL, "COSINE_DISTANCE(embedding_v2, @query_embedding)") {
		t.Error("exact SQL must use COSINE_DISTANCE without APPROX_COSINE_DISTANCE")
	}
	v1SQL := buildHybridSQLANN(vectorColumnV1, false, false, false, true, 20)
	if strings.Contains(v1SQL, "APPROX_COSINE_DISTANCE") {
		t.Error("v1 column must never use products_embedding_v2_idx")
	}
}

func TestSpannerHybridSearch_ScaNNANNFallbackAndCooldown(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig("v2")
	var sqlCalls []string
	var failANN atomic.Bool
	failANN.Store(true)

	svc := &SpannerService{
		config:       cfg,
		embeddings:   stubEmbedder{vec: make([]float32, 1024)},
		seedProducts: DefaultSeedProducts(),
		hybridQueryRunner: func(_ context.Context, sql string, _ map[string]interface{}, _, _ float64, _ vectorPlan, _ models.SearchFilters) ([]models.SearchResult, []models.SearchResult, error) {
			sqlCalls = append(sqlCalls, sql)
			if strings.Contains(sql, "APPROX_COSINE_DISTANCE") && failANN.Load() {
				return nil, nil, errors.New("spanner: code = InvalidArgument, desc = Index products_embedding_v2_idx not found")
			}
			return []models.SearchResult{{ID: "prod-001", Title: "Shoe"}}, nil, nil
		},
	}

	// 1) When index is missing, request succeeds via automatic exact fallback (vector_index_used=false) and sets cooldown.
	out, err := svc.HybridSearchDetailed(ctx, "running shoes", 10, 0, 0.5, models.SearchFilters{})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got err: %v", err)
	}
	if out.VectorIndexUsed || svc.VectorIndexUsed() || len(out.Results) != 1 || !svc.IsANNCooledDown() {
		t.Fatalf("after ANN fallback: VectorIndexUsed=%v svc.VectorIndexUsed=%v len=%d cooledDown=%v", out.VectorIndexUsed, svc.VectorIndexUsed(), len(out.Results), svc.IsANNCooledDown())
	}
	if len(sqlCalls) != 2 || !strings.Contains(sqlCalls[0], "APPROX_COSINE_DISTANCE") || !strings.Contains(sqlCalls[1], "ORDER BY COSINE_DISTANCE") {
		t.Fatalf("expected 1st call ANN and 2nd call exact fallback, got %d calls", len(sqlCalls))
	}

	// 2) During cooldown, subsequent queries go straight to exact COSINE_DISTANCE (1 call).
	sqlCalls = nil
	out2, err := svc.HybridSearchDetailed(ctx, "running shoes", 10, 0, 0.5, models.SearchFilters{})
	if err != nil || out2.VectorIndexUsed || svc.VectorIndexUsed() || len(sqlCalls) != 1 || strings.Contains(sqlCalls[0], "APPROX_COSINE_DISTANCE") {
		t.Fatalf("during cooldown expected single exact SQL call, got calls=%d vecUsed=%v err=%v", len(sqlCalls), out2.VectorIndexUsed, err)
	}

	// 3) After cooldown expires and index is healthy, ANN succeeds and reports vector_index_used=true.
	svc.annCooldownUntil.Store(0)
	failANN.Store(false)
	sqlCalls = nil
	out3, err := svc.HybridSearchDetailed(ctx, "running shoes", 10, 0, 0.5, models.SearchFilters{})
	if err != nil || !out3.VectorIndexUsed || !svc.VectorIndexUsed() || len(sqlCalls) != 1 || !strings.Contains(sqlCalls[0], "APPROX_COSINE_DISTANCE") {
		t.Fatalf("expected ANN success with VectorIndexUsed=true, got vecUsed=%v svcVecUsed=%v calls=%d err=%v", out3.VectorIndexUsed, svc.VectorIndexUsed(), len(sqlCalls), err)
	}
}

func TestEmbedDocumentsBatch_RetriesOn429And503AndSingleContentConstraint(t *testing.T) {
	var attempts atomic.Int32
	svc := newEmbeddingServiceWith(testConfig("v2"), func(_ context.Context, _ string, contents []*genai.Content, cfg *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
		// Defect 2 regression guard: gemini-embedding-2 rejects len(contents) > 1.
		if len(contents) != 1 {
			return nil, errors.New("InvalidArgument: embedContent API for this model only supports one content at a time")
		}
		if cfg.TaskType != "RETRIEVAL_DOCUMENT" {
			return nil, fmt.Errorf("unexpected TaskType %q", cfg.TaskType)
		}
		n := attempts.Add(1)
		if n == 1 {
			return nil, errors.New("429 RESOURCE_EXHAUSTED: quota exceeded")
		}
		if n == 2 {
			return nil, errors.New("503 UNAVAILABLE: backend overloaded")
		}
		v := make([]float32, 1024)
		v[0] = 1.0
		return &genai.EmbedContentResponse{Embeddings: []*genai.ContentEmbedding{{Values: v}}}, nil
	})
	svc.retryBaseDelay = time.Millisecond

	docs := make([]string, 32)
	for i := range docs {
		docs[i] = fmt.Sprintf("doc-%d", i+1)
	}
	vecs, err := svc.EmbedDocumentsBatch(context.Background(), docs)
	if err != nil {
		t.Fatalf("EmbedDocumentsBatch unexpected error: %v", err)
	}
	if len(vecs) != 32 || attempts.Load() < 34 {
		t.Fatalf("len(vecs)=%d attempts=%d, want 32 vecs and >=34 RPC attempts", len(vecs), attempts.Load())
	}
}

type fakeBatchEmbedder struct {
	maxBatchSeen atomic.Int32
}

func (f *fakeBatchEmbedder) EmbedDocumentsBatch(_ context.Context, texts []string) ([][]float32, error) {
	for {
		cur := f.maxBatchSeen.Load()
		if int32(len(texts)) <= cur || f.maxBatchSeen.CompareAndSwap(cur, int32(len(texts))) {
			break
		}
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, 1024)
		vec[0] = 1.0
		out[i] = vec
	}
	return out, nil
}

func TestRunEmbeddingBackfill_ConcurrentWorkersBatchesAnd200RowChunks(t *testing.T) {
	rows := make([]BackfillRow, 1050)
	for i := range rows {
		rows[i] = BackfillRow{ID: fmt.Sprintf("prod-%04d", i+1), Text: fmt.Sprintf("Product %d", i+1)}
	}
	emb := &fakeBatchEmbedder{}
	var mu sync.Mutex
	var chunkSizes []int
	seenIDs := map[string]bool{}

	writer := func(_ context.Context, chunk []BackfillUpdate) error {
		mu.Lock()
		defer mu.Unlock()
		chunkSizes = append(chunkSizes, len(chunk))
		for _, u := range chunk {
			if len(u.Vector) != 1024 {
				return fmt.Errorf("bad vector len %d", len(u.Vector))
			}
			seenIDs[u.ID] = true
		}
		return nil
	}

	stats, err := RunEmbeddingBackfill(context.Background(), rows, emb, writer, BackfillOptions{
		Workers:    8,
		EmbedBatch: 32,
		WriteChunk: 200,
	})
	if err != nil {
		t.Fatalf("RunEmbeddingBackfill error: %v", err)
	}
	if stats.TotalEmbedded != 1050 || stats.TotalWritten != 1050 || stats.ChunksCommitted != 6 || len(seenIDs) != 1050 {
		t.Fatalf("unexpected stats: %+v seenIDs=%d", stats, len(seenIDs))
	}
	if emb.maxBatchSeen.Load() != 32 {
		t.Fatalf("maxBatchSeen = %d, want 32", emb.maxBatchSeen.Load())
	}
	for i, sz := range chunkSizes {
		if sz > 200 {
			t.Fatalf("chunk %d size %d exceeds WriteChunk=200", i, sz)
		}
	}
}
