package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/models"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := &config.Config{
		Port:                   8080,
		Environment:            "test",
		ProjectID:              "riojucu-sandbox",
		Region:                 "us-central1",
		SpannerInstanceID:      "psearch-instance",
		SpannerDatabaseID:      "psearch-db",
		EmbeddingDimension:     768,
		ActiveEmbeddingVersion: "v1",
		DefaultAlpha:           0.5,
		DefaultLimit:           10,
		UseMemoryStore:         true,
	}
	SetupRouter(r, cfg)
	return r
}

func TestHealthAndInfoEndpoints(t *testing.T) {
	r := newTestRouter()

	for _, path := range []string{"/health", "/api/health", "/api/info"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, w.Code)
		}
	}
}

func TestHybridSearchAlphaParameterization(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=running+shoes&alpha=0.8&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/search status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp models.SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal SearchResponse: %v", err)
	}
	if resp.EffectiveAlpha != 0.8 {
		t.Errorf("effective_alpha = %v, want 0.8", resp.EffectiveAlpha)
	}
	if len(resp.Results) == 0 {
		t.Fatalf("expected non-empty search results")
	}
}

func doSearch(t *testing.T, r *gin.Engine, rawQuery string) models.SearchResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/search?"+rawQuery, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/search?%s status = %d, body = %s", rawQuery, w.Code, w.Body.String())
	}
	var resp models.SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal SearchResponse: %v", err)
	}
	return resp
}

func ids(results []models.SearchResult) string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.ID)
	}
	return strings.Join(out, ",")
}

// Issue #1: alpha=0 (pure text) and alpha=1 (pure vector) must produce different
// rankings; score.alpha/final_score echo the effective weight; out-of-range alpha is clamped.
func TestAlphaExtremesChangeRankingAndAreClamped(t *testing.T) {
	r := newTestRouter()
	text := doSearch(t, r, "q=running+shoes&alpha=0&limit=15")
	vec := doSearch(t, r, "q=running+shoes&alpha=1&limit=15")
	if ids(text.Results) == ids(vec.Results) {
		t.Fatalf("alpha=0 and alpha=1 returned identical rankings: %s", ids(text.Results))
	}
	for _, res := range text.Results {
		if res.Score["alpha"] != 0 {
			t.Fatalf("alpha=0: score.alpha = %v", res.Score["alpha"])
		}
		if _, ok := res.Score["final_score"]; !ok {
			t.Fatalf("result %s missing score.final_score", res.ID)
		}
	}
	if text.EffectiveAlpha != 0 || vec.EffectiveAlpha != 1 {
		t.Fatalf("effective_alpha = %v / %v, want 0 / 1", text.EffectiveAlpha, vec.EffectiveAlpha)
	}
	if got := doSearch(t, r, "q=running+shoes&alpha=7").EffectiveAlpha; got != 1 {
		t.Errorf("alpha=7 effective_alpha = %v, want clamped 1", got)
	}
	if got := doSearch(t, r, "q=running+shoes&alpha=-3").EffectiveAlpha; got != 0 {
		t.Errorf("alpha=-3 effective_alpha = %v, want clamped 0", got)
	}
}

func isOut(r models.SearchResult) bool {
	return r.Availability == "OUT_OF_STOCK" || (r.AvailableQuantity != nil && *r.AvailableQuantity <= 0)
}

// Issue #3: unavailable products are always ordered last; only_in_stock removes them.
func TestUnavailableLastAndOnlyInStock(t *testing.T) {
	r := newTestRouter()
	all := doSearch(t, r, "q=shoes&limit=15")
	seenOut := false
	for _, res := range all.Results {
		if isOut(res) {
			seenOut = true
		} else if seenOut {
			t.Fatalf("available product %s ranked after an unavailable one: %s", res.ID, ids(all.Results))
		}
	}
	if !seenOut {
		t.Fatalf("seed catalog should yield at least one OUT_OF_STOCK result: %s", ids(all.Results))
	}
	inStock := doSearch(t, r, "q=shoes&limit=15&only_in_stock=true")
	for _, res := range inStock.Results {
		if isOut(res) {
			t.Fatalf("only_in_stock=true returned unavailable product %s", res.ID)
		}
	}
	if len(inStock.Results) == 0 || len(inStock.Results) >= len(all.Results) {
		t.Fatalf("only_in_stock should drop unavailable items: %d vs %d", len(inStock.Results), len(all.Results))
	}
}

// Issue #3: channel filter via GET and POST (top-level JSON fields).
func TestChannelFilterGetAndPost(t *testing.T) {
	r := newTestRouter()
	store := doSearch(t, r, "q=shoes&limit=15&channel=store")
	if len(store.Results) == 0 {
		t.Fatal("expected store-channel results")
	}
	for _, res := range store.Results {
		found := false
		for _, c := range res.Channels {
			found = found || c == "store"
		}
		if !found {
			t.Fatalf("channel=store returned %s with channels %v", res.ID, res.Channels)
		}
	}
	body, _ := json.Marshal(map[string]interface{}{"query": "shoes", "limit": 15, "channel": "store", "only_in_stock": true})
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp models.SearchResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
		t.Fatalf("POST /api/search status=%d body=%s", w.Code, w.Body.String())
	}
	for _, res := range resp.Results {
		if isOut(res) {
			t.Fatalf("POST only_in_stock returned unavailable %s", res.ID)
		}
	}
	if ids(resp.Results) == "" {
		t.Fatal("POST with channel=store&only_in_stock should return results")
	}
}
