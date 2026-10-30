package services

import (
	"context"
	"strings"
	"testing"

	"psearch/serving-go/internal/models"
)

func TestValidatePricing(t *testing.T) {
	for _, ok := range [][2]string{{"", ""}, {"b2b-corporate", "price_asc"}, {"vip-club", "relevance"}, {"", "popularity"}, {"", "rating_desc"}} {
		if err := ValidatePricing(ok[0], ok[1]); err != nil {
			t.Errorf("ValidatePricing(%v) = %v", ok, err)
		}
	}
	for _, bad := range [][2]string{{"B2B Corp", ""}, {"x'; DROP", ""}, {"", "cheapest"}, {"", "rating_asc"}} {
		if err := ValidatePricing(bad[0], bad[1]); err == nil {
			t.Errorf("ValidatePricing(%v) should fail", bad)
		}
	}
}

func TestDefaultPriceRows(t *testing.T) {
	rows := DefaultPriceRows(DefaultSeedProducts())
	if len(rows) != 3*15 {
		t.Fatalf("rows = %d, want 45", len(rows))
	}
	m := newMemoryPricing(rows)
	if got := m.effective("b2b-corporate", "prod-001", 189.90); got != 170.91 {
		t.Fatalf("b2b-corporate prod-001 = %v, want 170.91 (10%% off)", got)
	}
	if got := m.effective("", "prod-001", 189.90); got != 189.90 {
		t.Fatal("no table => list price")
	}
	if got := m.effective("unknown", "prod-001", 189.90); got != 189.90 {
		t.Fatal("unknown table => list price")
	}
}

func TestExclusiveAssortment(t *testing.T) {
	m := newMemoryPricing([]PriceRow{{ProductID: "p1", PriceTableID: "vip-club", Price: 1, Exclusive: true}})
	if m.visible("", "p1") || m.visible("b2b-gold", "p1") || !m.visible("vip-club", "p1") || !m.visible("", "p2") {
		t.Fatal("exclusive products are visible only to buyers on their table")
	}
}

// ADR-008 in memory mode: effective prices, price sort and price facets.
func TestMemoryContractPricing(t *testing.T) {
	ctx := context.Background()
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}
	out, _ := svc.HybridSearchDetailed(ctx, "laptop", 15, 0, 0.5, models.SearchFilters{PriceTable: "b2b-gold", SortBy: "price_asc"})
	prev := -1.0
	for _, r := range out.Results {
		if isUnavailable(r.Availability, r.AvailableQuantity) {
			continue
		}
		if r.PriceInfo.NumericPrice < prev {
			t.Fatalf("price_asc violated at %s: %v < %v", r.ID, r.PriceInfo.NumericPrice, prev)
		}
		prev = r.PriceInfo.NumericPrice
		if r.PriceInfo.PriceTableID != "b2b-gold" || r.PriceInfo.BasePrice <= r.PriceInfo.NumericPrice {
			t.Fatalf("%s must carry contract price below base: %+v", r.ID, r.PriceInfo)
		}
	}
	// prod-007 MacBook list 1399 -> b2b-gold Computers 18% off = 1147.18 (below the 1200 cap)
	capped, _ := svc.HybridSearchDetailed(ctx, "macbook", 15, 0, 0.5, models.SearchFilters{PriceTable: "b2b-gold", MaxPrice: 1200})
	found := false
	for _, r := range capped.Results {
		found = found || r.ID == "prod-007"
	}
	if !found {
		t.Fatal("max_price must apply to the effective contract price (prod-007 1147.18 <= 1200)")
	}
	f, _ := svc.Facets(ctx, []string{"prod-006"}, "b2b-gold", "default") // 1650 list -> 1353 contract
	if len(f.PriceRanges) != 1 || f.PriceRanges[0].Value != "500-1500" {
		t.Fatalf("price facet must use the contract price: %+v", f.PriceRanges)
	}
}

func TestPricingSQL(t *testing.T) {
	sql := buildHybridSQLWith(vectorColumnV2, false, true)
	for _, frag := range []string{
		"FROM product_prices AS pp",
		"@price_table",
		"is_exclusive",
		"effective_price",
		"IF(@sort_by = 'price_asc', effective_price, NULL) ASC",
		"IF(@sort_by = 'popularity', popularity_sort, NULL) DESC",
		"IF(@sort_by = 'rating_desc', rating_sort, NULL) DESC",
	} {
		if !strings.Contains(sql, frag) {
			t.Errorf("SQL missing %q", frag)
		}
	}
	if !strings.HasPrefix(optimizerHint, "@{OPTIMIZER_VERSION=") {
		t.Error("hybrid query must carry the optimizer-version statement hint (SCORE needs v6+)")
	}
	if !strings.Contains(facetPriceSQL, "product_prices") {
		t.Error("price facet must use the effective price")
	}
	cappedLex := buildCappedLexicalCountSQL(true)
	if !strings.Contains(cappedLex, "SELECT COUNT(*) FROM (SELECT 1") || !strings.Contains(cappedLex, "LIMIT 501)") {
		t.Errorf("buildCappedLexicalCountSQL malformed: %s", cappedLex)
	}
	cappedBrowse := buildCappedBrowseCountSQL()
	if !strings.Contains(cappedBrowse, "SELECT COUNT(*) FROM (SELECT 1") || !strings.Contains(cappedBrowse, "LIMIT 501)") {
		t.Errorf("buildCappedBrowseCountSQL malformed: %s", cappedBrowse)
	}
}

func TestCountCacheKey_DistinctAcrossEveryDimensionAndBoundedEviction(t *testing.T) {
	svc := &SpannerService{config: testConfig("v1")}
	baseF := models.SearchFilters{}
	baseExp := Expansion{}
	baseKey := svc.countCacheKey("shoes", baseExp, baseF)

	variants := map[string]struct {
		q      string
		exp    Expansion
		folded bool
		f      models.SearchFilters
	}{
		"sizes":          {"shoes", baseExp, false, models.SearchFilters{Sizes: []string{"40", "42"}}},
		"zip5":           {"shoes", baseExp, false, models.SearchFilters{Zip5: "01310"}},
		"attrs":          {"shoes", baseExp, false, models.SearchFilters{Attrs: map[string][]string{"ram": {"16GB"}}}},
		"price_ranges":   {"shoes", baseExp, false, models.SearchFilters{PriceRanges: []string{"100-500"}}},
		"categories":     {"shoes", baseExp, false, models.SearchFilters{Categories: []string{"Footwear", "Audio"}}},
		"brands":         {"shoes", baseExp, false, models.SearchFilters{Brands: []string{"Nike", "Asics"}}},
		"colors":         {"shoes", baseExp, false, models.SearchFilters{Colors: []string{"azul", "preto"}}},
		"tenant_id":      {"shoes", baseExp, false, models.SearchFilters{TenantID: "acme"}},
		"locale":         {"shoes", baseExp, false, models.SearchFilters{Locale: "pt-BR"}},
		"channel":        {"shoes", baseExp, false, models.SearchFilters{Channel: "mobile"}},
		"price_table_id": {"shoes", baseExp, false, models.SearchFilters{PriceTable: "b2b-gold"}},
		"only_in_stock":  {"shoes", baseExp, false, models.SearchFilters{OnlyInStock: true}},
		"min_price":      {"shoes", baseExp, false, models.SearchFilters{MinPrice: 50}},
		"max_price":      {"shoes", baseExp, false, models.SearchFilters{MaxPrice: 500}},
		"max_sla_hours":  {"shoes", baseExp, false, models.SearchFilters{Zip5: "01310", MaxSLAHours: 24}},
		"relax_text":     {"shoes", baseExp, false, models.SearchFilters{RelaxText: true}},
		"synonyms":       {"shoes", Expansion{Terms: []string{"sneakers"}, Weight: 0.4}, false, baseF},
		"text_folding":   {"shoes", baseExp, true, baseF},
	}

	seen := map[string]string{baseKey: "base"}
	for name, v := range variants {
		svc.config.TextFolding = v.folded
		k := svc.countCacheKey(v.q, v.exp, v.f)
		if prev, exists := seen[k]; exists {
			t.Fatalf("countCacheKey collision between %q and %q: %s", name, prev, k)
		}
		seen[k] = name
	}

	// Verify bounded cache size (maxCountCacheEntries = 512).
	for i := 0; i < maxCountCacheEntries+50; i++ {
		svc.setCachedCounts(strings.Repeat("k", 4)+string(rune(i)), 10, 10, true)
	}
	svc.countMu.RLock()
	sz := len(svc.countCache)
	svc.countMu.RUnlock()
	if sz > maxCountCacheEntries {
		t.Fatalf("countCache size = %d, want <= %d", sz, maxCountCacheEntries)
	}
}

func TestMemorySortByPopularityAndRatingDesc(t *testing.T) {
	ctx := context.Background()
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}

	pop, err := svc.HybridSearchDetailed(ctx, "shoes", 15, 0, 0.5, models.SearchFilters{SortBy: "popularity"})
	if err != nil || len(pop.Results) == 0 {
		t.Fatalf("popularity search failed: %v", err)
	}
	seenOOS := false
	prevPop := 1e18
	for _, r := range pop.Results {
		oos := isUnavailable(r.Availability, r.AvailableQuantity)
		if oos {
			seenOOS = true
			continue
		}
		if seenOOS {
			t.Fatalf("in-stock item %s appeared after OOS item in popularity sort", r.ID)
		}
		score := ProductPopularityScore(r)
		if score > prevPop {
			t.Fatalf("popularity order violated at %s: %v > %v", r.ID, score, prevPop)
		}
		prevPop = score
	}

	rat, err := svc.HybridSearchDetailed(ctx, "shoes", 15, 0, 0.5, models.SearchFilters{SortBy: "rating_desc"})
	if err != nil || len(rat.Results) == 0 {
		t.Fatalf("rating_desc search failed: %v", err)
	}
	seenOOS = false
	prevRating := 10.0
	for _, r := range rat.Results {
		oos := isUnavailable(r.Availability, r.AvailableQuantity)
		if oos {
			seenOOS = true
			continue
		}
		if seenOOS {
			t.Fatalf("in-stock item %s appeared after OOS item in rating_desc sort", r.ID)
		}
		rv, _ := ProductRating(r)
		if rv > prevRating {
			t.Fatalf("rating_desc order violated at %s: %v > %v", r.ID, rv, prevRating)
		}
		prevRating = rv
	}
}
