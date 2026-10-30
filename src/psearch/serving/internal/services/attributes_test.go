package services

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"psearch/serving-go/internal/models"
)

func TestParseAndValidateDynamicAttrs(t *testing.T) {
	q := url.Values{
		"attr.ram":     []string{"16GB,32GB"},
		"attr.storage": []string{"1TB"},
		"q":            []string{"notebook"},
	}
	got, err := ParseAndValidateDynamicAttrs(q, map[string][]string{"RAM": {"32GB", "64GB"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got["ram"]) != 3 || len(got["storage"]) != 1 {
		t.Fatalf("merged attrs = %+v", got)
	}

	// Invalid key syntax (SQL injection attempt) must be rejected.
	badQ := url.Values{"attr.ram;DROP TABLE": []string{"16GB"}}
	if _, err := ParseAndValidateDynamicAttrs(badQ, nil); err == nil {
		t.Fatal("expected invalid attribute key error")
	}
}

func TestDynamicAttrFilterSQL_UsesBoundParamsWithoutInterpolatingKeys(t *testing.T) {
	sql, params := dynamicAttrFilterSQL("products", map[string][]string{
		"ram":     {"16GB", "32GB"},
		"voltage": {"220V"},
	})
	if strings.Contains(sql, "'ram'") || strings.Contains(sql, "'voltage'") {
		t.Fatalf("SQL must bind attribute names via @attr_k_name, got: %s", sql)
	}
	for _, want := range []string{
		"FROM product_attributes AS pa",
		"LOWER(pa.attr_name) = @attr_0_name",
		"pa.attr_value_lower IN UNNEST(@attr_0_vals)",
		"LOWER(pa.attr_name) = @attr_1_name",
		"pa.attr_value_lower IN UNNEST(@attr_1_vals)",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("dynamicAttrFilterSQL missing %q in:\n%s", want, sql)
		}
	}
	if params["attr_0_name"] != "ram" || params["attr_1_name"] != "voltage" {
		t.Fatalf("unexpected bound params: %+v", params)
	}
}

func TestDefaultProductAttributes_SunscreenAttrsOnlyOnDermocosmetics(t *testing.T) {
	products := DefaultSeedProducts()
	attrsByProd := DefaultProductAttributes()
	allowedByCat := map[string]map[string]bool{
		"Footwear":       {"surface": true, "cushioning": true, "drop": true},
		"Computers":      {"ram": true, "storage": true, "processor": true, "screen_size": true},
		"Audio":          {"connectivity": true, "battery_life": true, "noise_cancelling": true},
		"Smartphones":    {"ram": true, "storage": true, "connectivity": true},
		"Appliances":     {"voltage": true, "energy_rating": true, "capacity": true, "pressure": true},
		"TV & Video":     {"screen_size": true, "resolution": true, "refresh_rate": true},
		"Accessories":    {"connectivity": true, "switch_type": true},
		"Dermocosmetics": {"fps": true, "skin_type": true, "form": true},
	}

	for _, p := range products {
		cat := ""
		if len(p.Categories) > 0 {
			cat = p.Categories[0]
		}
		attrs := attrsByProd[p.ID]
		if len(attrs) == 0 {
			t.Fatalf("seed product %s (%s) has no default attributes", p.ID, cat)
		}
		isDermo := strings.EqualFold(cat, "Dermocosmetics") || strings.EqualFold(cat, "Health & Beauty")
		for k := range attrs {
			if (k == "fps" || k == "skin_type" || k == "form") && !isDermo {
				t.Fatalf("sunscreen attribute %q found on non-dermocosmetic product %s (category=%q)", k, p.ID, cat)
			}
			if allowed := allowedByCat[cat]; allowed != nil && !allowed[k] {
				t.Fatalf("attribute %q on product %s does not match category %q", k, p.ID, cat)
			}
		}
	}
}

func TestDynamicAttributesSearchFacetsAndUpsertInMemory(t *testing.T) {
	ctx := context.Background()
	seed := DefaultSeedProducts()
	qty := 20
	dermo1 := models.SearchResult{
		ID:                "prod-dermo-1",
		Title:             "Protetor Solar Facial Anthelios FPS 50",
		Description:       "Protetor solar toque seco para pele oleosa",
		Brands:            []string{"LaRoche"},
		Categories:        []string{"Dermocosmetics"},
		PriceInfo:         models.PriceInfo{CurrencyCode: "BRL", Price: "89.90", NumericPrice: 89.90},
		Availability:      "IN_STOCK",
		AvailableQuantity: &qty,
		TenantID:          "default",
		DynamicAttributes: map[string]interface{}{"fps": "FPS 50", "skin_type": "Pele Oleosa", "form": "Gel-Creme"},
	}
	dermo2 := models.SearchResult{
		ID:                "prod-dermo-2",
		Title:             "Protetor Solar Corporal FPS 70",
		Description:       "Fluido solar resistente à água",
		Brands:            []string{"Isdin"},
		Categories:        []string{"Dermocosmetics"},
		PriceInfo:         models.PriceInfo{CurrencyCode: "BRL", Price: "109.90", NumericPrice: 109.90},
		Availability:      "IN_STOCK",
		AvailableQuantity: &qty,
		TenantID:          "default",
		DynamicAttributes: map[string]interface{}{"fps": "FPS 70", "skin_type": "Todos os Tipos", "form": "Fluido"},
	}
	seed = append(seed, dermo1, dermo2)
	svc := &SpannerService{config: testConfig("v1"), seedProducts: seed}

	// 1) Searching q=notebook automatically surfaces notebook attribute facets (ram, storage, processor).
	nbOut, err := svc.HybridSearchDetailed(ctx, "notebook", 100, 0, 0.5, models.SearchFilters{})
	if err != nil || len(nbOut.Results) == 0 {
		t.Fatalf("notebook search failed: %v", err)
	}
	var nbIDs []string
	for _, r := range nbOut.Results {
		nbIDs = append(nbIDs, r.ID)
	}
	nbFacets, err := svc.DisjunctiveFacets(ctx, "notebook", 0, 0.5, models.SearchFilters{}, nbIDs)
	if err != nil || len(nbFacets.Attributes) == 0 {
		t.Fatalf("expected auto-discovered dynamic facets for notebook, got %+v (err=%v)", nbFacets, err)
	}
	topNames := map[string]bool{}
	for i := 0; i < len(nbFacets.Attributes) && i < 3; i++ {
		topNames[nbFacets.Attributes[i].Name] = true
	}
	if !topNames["ram"] || !topNames["storage"] {
		t.Fatalf("expected ram and storage in top notebook dynamic facets, got %+v", nbFacets.Attributes)
	}

	// 2) Searching q=protetor automatically surfaces fps and skin_type at the top of facets.attributes.
	prOut, err := svc.HybridSearchDetailed(ctx, "protetor", 100, 0, 0.5, models.SearchFilters{})
	if err != nil || len(prOut.Results) == 0 {
		t.Fatalf("protetor search failed: %v", err)
	}
	var prIDs []string
	for _, r := range prOut.Results {
		prIDs = append(prIDs, r.ID)
	}
	prFacets, err := svc.DisjunctiveFacets(ctx, "protetor", 0, 0.5, models.SearchFilters{}, prIDs)
	if err != nil || len(prFacets.Attributes) < 2 {
		t.Fatalf("expected dynamic facets for protetor, got %+v (err=%v)", prFacets, err)
	}
	if prFacets.Attributes[0].Name != "fps" || prFacets.Attributes[1].Name != "skin_type" {
		t.Fatalf("expected fps and skin_type first for q=protetor, got %+v", prFacets.Attributes)
	}

	// 3) Filtering by attr.ram=16GB,32GB returns only products with 16GB or 32GB RAM, and preserves sibling ram counts.
	ramFilter := models.SearchFilters{Attrs: map[string][]string{"ram": {"32GB"}}}
	ram32Out, err := svc.HybridSearchDetailed(ctx, "notebook", 100, 0, 0.5, ramFilter)
	if err != nil || len(ram32Out.Results) != 2 {
		t.Fatalf("expected 2 notebooks with 32GB RAM (prod-005, prod-006), got %d (err=%v)", len(ram32Out.Results), err)
	}
	var ram32IDs []string
	for _, r := range ram32Out.Results {
		ram32IDs = append(ram32IDs, r.ID)
	}
	ramFacets, err := svc.DisjunctiveFacets(ctx, "notebook", 0, 0.5, ramFilter, ram32IDs)
	if err != nil {
		t.Fatalf("DisjunctiveFacets with ram=32GB failed: %v", err)
	}
	found16Sibling := false
	for _, df := range ramFacets.Attributes {
		if df.Name == "ram" {
			for _, b := range df.Buckets {
				if b.Value == "16GB" && b.Count > 0 {
					found16Sibling = true
				}
			}
		}
	}
	if !found16Sibling {
		t.Fatalf("expected sibling 16GB RAM count > 0 when ram=32GB is selected: %+v", ramFacets.Attributes)
	}

	// 4) UpsertProductAttributes updates in-memory attributes and rejects unknown products with ErrProductNotFound.
	updated, err := svc.UpsertProductAttributes(ctx, "default", "prod-007", map[string]interface{}{
		"ram":     []string{"24GB"},
		"storage": "2TB",
	})
	if err != nil || len(updated["ram"]) != 1 || updated["ram"][0] != "24GB" {
		t.Fatalf("UpsertProductAttributes failed: %+v, err=%v", updated, err)
	}
	if _, err := svc.UpsertProductAttributes(ctx, "default", "prod-nonexistent", map[string]interface{}{"ram": "16GB"}); err != ErrProductNotFound {
		t.Fatalf("expected ErrProductNotFound, got %v", err)
	}
}
