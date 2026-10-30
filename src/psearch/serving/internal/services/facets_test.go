package services

import (
	"context"
	"strings"
	"testing"

	"psearch/serving-go/internal/models"
)

func TestPriceBucket(t *testing.T) {
	for _, tt := range []struct {
		price float64
		want  string
	}{{0, "0-100"}, {99.99, "0-100"}, {100, "100-500"}, {499.99, "100-500"}, {500, "500-1500"}, {1499.99, "500-1500"}, {1500, "1500+"}} {
		if got := PriceBucket(tt.price); got != tt.want {
			t.Errorf("PriceBucket(%v) = %s, want %s", tt.price, got, tt.want)
		}
	}
}

func TestMemoryFacets(t *testing.T) {
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}
	var ids []string
	for _, p := range svc.seedProducts {
		ids = append(ids, p.ID)
	}
	f, err := svc.Facets(context.Background(), ids, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sum := func(b []models.FacetBucket) (n int64) {
		for _, x := range b {
			n += x.Count
		}
		return
	}
	for name, b := range map[string][]models.FacetBucket{"categories": f.Categories, "brands": f.Brands, "price_ranges": f.PriceRanges, "availability": f.Availability} {
		if sum(b) != int64(len(ids)) {
			t.Errorf("%s counts sum to %d, want %d", name, sum(b), len(ids))
		}
		for i := 1; i < len(b); i++ {
			if b[i].Count > b[i-1].Count {
				t.Errorf("%s not sorted by count desc", name)
			}
		}
	}
	got := map[string]int64{}
	for _, b := range f.Availability {
		got[b.Value] = b.Count
	}
	if got["OUT_OF_STOCK"] != 2 || got["IN_STOCK"] != 13 {
		t.Errorf("availability facet = %v, want OUT_OF_STOCK=2 IN_STOCK=13", got)
	}
	if empty, _ := svc.Facets(context.Background(), nil, "", ""); len(empty.Categories) != 0 || empty.Categories == nil {
		t.Error("empty ids must give empty (non-nil) facet groups")
	}
}

func TestFacetSQL(t *testing.T) {
	for _, expr := range []string{facetCategorySQL, facetBrandSQL, facetPriceSQL, facetAvailabilitySQL} {
		sql := facetSQL(expr)
		if !strings.Contains(sql, "product_id IN UNNEST(@ids)") || !strings.Contains(sql, "GROUP BY value") {
			t.Errorf("facet SQL malformed: %s", sql)
		}
	}
}

func TestFoldAccents(t *testing.T) {
	for in, want := range map[string]string{"Tênis": "Tenis", "Calçado esportivo": "Calcado esportivo", "ação": "acao", "plain": "plain"} {
		if got := FoldAccents(in); got != want {
			t.Errorf("FoldAccents(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildHybridSQLWith_FoldedAndCatalogFilters(t *testing.T) {
	sql := buildHybridSQLWith(vectorColumnV2, false, true)
	for _, frag := range []string{"SEARCH(title_tokens_folded, @query_text)", "SCORE(description_tokens_folded, @query_text)", "@tenant_id", "@locale", "@category", "@brand"} {
		if !strings.Contains(sql, frag) {
			t.Errorf("folded SQL missing %q", frag)
		}
	}
	if strings.Contains(buildHybridSQL(vectorColumnV2, false), "_folded") {
		t.Error("default SQL must not use folded tokenlists")
	}
}

func TestBrowseSQL(t *testing.T) {
	sql := browseSQL()
	for _, frag := range []string{"@{OPTIMIZER_VERSION=", "FROM products", "COALESCE(is_visible, TRUE)", "products.clicks", "@sort_by", "LIMIT @limit"} {
		if !strings.Contains(sql, frag) {
			t.Errorf("browse SQL missing %q", frag)
		}
	}
	if strings.Contains(sql, " p.") {
		t.Error("browse SQL must not reference alias p")
	}
	if !IsBrowseQuery("  ") || !IsBrowseQuery("*") || IsBrowseQuery("tv") {
		t.Error("IsBrowseQuery")
	}
}

func TestDisjunctiveFacets_ExcludeOwnDimensionAndMultiSelect(t *testing.T) {
	ctx := context.Background()
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}

	// 1) Filtering by brands=Nike restricts results to Nike, while facets.brands retains
	// non-zero counts for sibling brands (Adidas, Asics, Reebok) via exclude-own-dimension.
	filters := models.SearchFilters{Brands: []string{"Nike"}}
	out, err := svc.HybridSearchDetailed(ctx, "shoes", 100, 0, 0.5, filters)
	if err != nil || len(out.Results) == 0 {
		t.Fatalf("search brands=Nike failed: %v (len=%d)", err, len(out.Results))
	}
	var filteredIDs []string
	for _, r := range out.Results {
		filteredIDs = append(filteredIDs, r.ID)
		if !strings.EqualFold(r.Brands[0], "Nike") {
			t.Fatalf("expected only Nike in results, got %s (%v)", r.ID, r.Brands)
		}
	}
	facets, err := svc.DisjunctiveFacets(ctx, "shoes", 0, 0.5, filters, filteredIDs)
	if err != nil {
		t.Fatalf("DisjunctiveFacets error: %v", err)
	}
	brandCounts := map[string]models.FacetBucket{}
	for _, b := range facets.Brands {
		brandCounts[strings.ToLower(b.Value)] = b
	}
	if !brandCounts["nike"].Selected || brandCounts["nike"].Count == 0 {
		t.Fatalf("expected Nike selected with count>0 in facets.brands: %+v", facets.Brands)
	}
	if brandCounts["adidas"].Count == 0 || brandCounts["asics"].Count == 0 {
		t.Fatalf("sibling brands Adidas/Asics must retain non-zero counts when brands=Nike: %+v", facets.Brands)
	}

	// 2) Multi-select OR within dimension + AND across dimensions: (Nike OR Asics) AND sizes=42.
	multiFilters := models.SearchFilters{Brands: []string{"Nike", "Asics"}, Sizes: []string{"42"}}
	multiOut, err := svc.HybridSearchDetailed(ctx, "shoes", 100, 0, 0.5, multiFilters)
	if err != nil || len(multiOut.Results) < 2 {
		t.Fatalf("expected both Nike and Asics footwear matching size 42, got len=%d err=%v", len(multiOut.Results), err)
	}
	seenBrands := map[string]bool{}
	for _, r := range multiOut.Results {
		b := strings.ToLower(r.Brands[0])
		if b != "nike" && b != "asics" {
			t.Fatalf("unexpected brand %s in (Nike OR Asics) AND size=42 results", r.Brands[0])
		}
		seenBrands[b] = true
	}
	if !seenBrands["nike"] || !seenBrands["asics"] {
		t.Fatalf("expected both nike and asics in multi-brand results, got %v", seenBrands)
	}
}
