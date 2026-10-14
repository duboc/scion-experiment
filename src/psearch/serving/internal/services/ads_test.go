package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"psearch/serving-go/internal/models"
)

func TestInjectSponsored_SlotsAndDedupe(t *testing.T) {
	organic := []models.SearchResult{
		{ID: "p1", Title: "Organic 1", Score: map[string]float64{"final_score": 0.9}},
		{ID: "p2", Title: "Organic 2", Score: map[string]float64{"final_score": 0.8}},
		{ID: "p3", Title: "Organic 3", Score: map[string]float64{"final_score": 0.7}},
		{ID: "p4", Title: "Organic 4", Score: map[string]float64{"final_score": 0.6}},
		{ID: "p5", Title: "Organic 5", Score: map[string]float64{"final_score": 0.5}},
		{ID: "p6", Title: "Organic 6", Score: map[string]float64{"final_score": 0.4}},
		{ID: "p7", Title: "Organic 7", Score: map[string]float64{"final_score": 0.3}},
		{ID: "p8", Title: "Organic 8", Score: map[string]float64{"final_score": 0.2}},
	}
	sponsored := []models.SearchResult{
		{ID: "p3", Title: "Organic 3", AdCampaignID: "camp-1", Score: map[string]float64{"cpc_micros": 500000}},
		{ID: "p9", Title: "Sponsored 9", AdCampaignID: "camp-2", Score: map[string]float64{"cpc_micros": 400000}},
		{ID: "p6", Title: "Organic 6", AdCampaignID: "camp-3", Score: map[string]float64{"cpc_micros": 300000}},
	}

	got := InjectSponsored(organic, sponsored, []int{1, 4, 8})
	// p3 and p6 were deduplicated from organic (8 - 2 = 6 organic + 3 ads = 9 total)
	if len(got) != 9 {
		t.Fatalf("len(got) = %d, want 9: %v", len(got), resultIDs(got))
	}
	// Check 1-based slots [1, 4, 8] => 0-based indices [0, 3, 7]
	wantSlots := map[int]string{0: "p3", 3: "p9", 7: "p6"}
	for idx, wantID := range wantSlots {
		if got[idx].ID != wantID || !got[idx].Sponsored || got[idx].AdCampaignID == "" || got[idx].AdClickID == "" {
			t.Fatalf("slot index %d = %+v, want sponsored %s", idx, got[idx], wantID)
		}
	}
	// Verify no duplicate product IDs exist anywhere in the output.
	seen := map[string]bool{}
	for _, r := range got {
		if seen[r.ID] {
			t.Fatalf("duplicate product_id %s in output: %v", r.ID, resultIDs(got))
		}
		seen[r.ID] = true
	}
	// Deduplicated p3 must preserve its organic final_score.
	if got[0].Score["final_score"] != 0.7 {
		t.Fatalf("deduplicated ad p3 lost organic score: %v", got[0].Score)
	}

	// Zero organic results => zero ads (ads never rescue a zero-result page).
	if empty := InjectSponsored(nil, sponsored, []int{1, 4, 8}); len(empty) != 0 {
		t.Fatalf("zero organic results must return zero ads, got %d", len(empty))
	}

	// Fewer organic results than slot index => ads appended in order, never exceeding K.
	short := InjectSponsored([]models.SearchResult{{ID: "p1"}}, sponsored[:2], []int{1, 4, 8})
	if len(short) != 3 || short[0].ID != "p3" || short[1].ID != "p1" || short[2].ID != "p9" {
		t.Fatalf("short organic placement = %v, want [p3 p1 p9]", resultIDs(short))
	}
}

func TestAdServiceValidation(t *testing.T) {
	ctx := context.Background()
	svc := NewAdService(NewEmptyMemoryAdStore(), DefaultSeedProducts())

	future := "2020-01-01T00:00:00Z"
	past := "2019-01-01T00:00:00Z"
	bad := []models.AdCampaign{
		{ProductID: "", CPCMicros: 100000, BudgetMicros: 1000000},
		{ProductID: "prod-unknown", CPCMicros: 100000, BudgetMicros: 1000000},
		{ProductID: "prod-001", CPCMicros: 0, BudgetMicros: 1000000},
		{ProductID: "prod-001", CPCMicros: -10, BudgetMicros: 1000000},
		{ProductID: "prod-001", CPCMicros: 500000, BudgetMicros: 400000}, // budget < cpc
		{ProductID: "prod-001", CPCMicros: 100000, BudgetMicros: 1000000, Status: "invalid"},
		{ProductID: "prod-001", CPCMicros: 100000, BudgetMicros: 1000000, StartAt: &future, EndAt: &past},
	}
	for i, c := range bad {
		if _, err := svc.Save(ctx, c); err == nil {
			t.Errorf("bad campaign %d should fail validation: %+v", i, c)
		}
	}

	saved, err := svc.Save(ctx, models.AdCampaign{
		Advertiser:   "Adidas",
		ProductID:    "prod-001",
		CPCMicros:    200000,
		BudgetMicros: 1000000,
	})
	if err != nil || saved.CampaignID == "" || saved.TenantID != "default" || saved.Status != "active" {
		t.Fatalf("Save valid campaign = %+v, err=%v", saved, err)
	}
}

func TestMemoryAdStore_SelectionFiltersAndExhaustion(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig("v1")
	svc := &SpannerService{config: cfg, seedProducts: DefaultSeedProducts()}
	store := NewEmptyMemoryAdStore()
	ads := NewAdService(store, svc.seedProducts)
	svc.SetAds(ads)

	// Out-of-stock product (prod-004) must never be selected even with an active campaign.
	_, _ = ads.Save(ctx, models.AdCampaign{CampaignID: "c-oos", ProductID: "prod-004", CPCMicros: 900000, BudgetMicros: 5000000, Status: "active"})
	// Two active in-stock footwear campaigns with different CPC bids.
	_, _ = ads.Save(ctx, models.AdCampaign{CampaignID: "c-low", ProductID: "prod-001", CPCMicros: 200000, BudgetMicros: 400000, Status: "active"})
	_, _ = ads.Save(ctx, models.AdCampaign{CampaignID: "c-high", ProductID: "prod-003", CPCMicros: 500000, BudgetMicros: 500000, Status: "active"})

	matches := svc.memorySponsoredMatches(ctx, "running shoes", 0.5, models.SearchFilters{})
	if len(matches) != 2 || matches[0].ID != "prod-003" || matches[1].ID != "prod-001" {
		t.Fatalf("matches = %v, want [prod-003 prod-001] (higher CPC first, OOS excluded)", resultIDs(matches))
	}

	// Catalog filter: brand=Adidas excludes prod-003 (Nike).
	adidasOnly := svc.memorySponsoredMatches(ctx, "running shoes", 0.5, models.SearchFilters{Brand: "Adidas"})
	if len(adidasOnly) != 1 || adidasOnly[0].ID != "prod-001" {
		t.Fatalf("brand filter = %v, want [prod-001]", resultIDs(adidasOnly))
	}

	// Click c-high once (cpc=500k, budget=500k): must charge and transition status to exhausted.
	click1, err := ads.RecordClick(ctx, "c-high", "clk-001")
	if err != nil || !click1.Charged || click1.SpentMicros != 500000 || click1.Status != "exhausted" {
		t.Fatalf("click1 = %+v, err=%v", click1, err)
	}
	// Duplicate click_id must be idempotent (charged=false, no overspend).
	dup, err := ads.RecordClick(ctx, "c-high", "clk-001")
	if err != nil || dup.Charged || dup.SpentMicros != 500000 || dup.Reason != "duplicate_click" {
		t.Fatalf("duplicate click = %+v, err=%v", dup, err)
	}
	// Exhausted campaign must no longer be selected in search.
	afterExhaust := svc.memorySponsoredMatches(ctx, "running shoes", 0.5, models.SearchFilters{})
	if len(afterExhaust) != 1 || afterExhaust[0].ID != "prod-001" {
		t.Fatalf("exhausted campaign must not be selected, got %v", resultIDs(afterExhaust))
	}
}

func TestMemoryAdStore_ConcurrentClicksNoOverspend(t *testing.T) {
	ctx := context.Background()
	store := NewEmptyMemoryAdStore()
	ads := NewAdService(store, DefaultSeedProducts())

	// Budget allows at most 3 clicks (3 * 300,000 = 900,000 <= 1,000,000; 4th would be 1,200,000).
	_, err := ads.Save(ctx, models.AdCampaign{
		CampaignID:   "camp-race",
		ProductID:    "prod-001",
		CPCMicros:    300000,
		BudgetMicros: 1000000,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	const goroutines = 60
	var wg sync.WaitGroup
	var chargedCount atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Mix unique click IDs and duplicate click IDs across concurrent workers.
			clickID := fmt.Sprintf("clk-%d", idx%25)
			res, err := ads.RecordClick(ctx, "camp-race", clickID)
			if err != nil {
				t.Errorf("RecordClick(%s): %v", clickID, err)
				return
			}
			if res.Charged {
				chargedCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := chargedCount.Load(); got != 3 {
		t.Fatalf("charged clicks = %d, want exactly 3", got)
	}
	list, err := ads.List(ctx, "default")
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %+v, err=%v", list, err)
	}
	camp := list[0]
	if camp.SpentMicros != 900000 || camp.SpentMicros > camp.BudgetMicros || camp.Status != "exhausted" {
		t.Fatalf("campaign after concurrent clicks = %+v (want spent=900000, status=exhausted)", camp)
	}
}

func TestBuildHybridSQLMode_SponsoredMatchesCTE(t *testing.T) {
	hybrid := buildHybridSQLMode(vectorColumnV2, false, false, true)
	for _, want := range []string{
		"sponsored_matches AS",
		"FROM ad_campaigns AS ac",
		"ac.status = 'active'",
		"ac.spent_micros + ac.cpc_micros <= ac.budget_micros",
		"@ad_min_similarity",
		"LIMIT @ad_k",
		"TRUE AS is_sponsored",
		"FALSE AS is_sponsored",
	} {
		if !strings.Contains(hybrid, want) {
			t.Errorf("hybrid SQL missing %q", want)
		}
	}

	lex := buildHybridSQLMode(vectorColumnV2, true, false, false)
	if strings.Contains(lex, "@query_embedding") || strings.Contains(lex, "@ad_min_similarity") {
		t.Error("lexical-only SQL must not reference @query_embedding or @ad_min_similarity")
	}
	if !strings.Contains(lex, "sponsored_matches AS") {
		t.Error("lexical-only SQL must include sponsored_matches CTE")
	}

	vecOnly := buildHybridSQLMode(vectorColumnV2, false, true, false)
	if strings.Contains(vecOnly, "@query_text") || strings.Contains(vecOnly, "SEARCH(") {
		t.Error("vector-only SQL must not reference @query_text or SEARCH()")
	}
	if !strings.Contains(vecOnly, "sponsored_matches AS") || !strings.Contains(vecOnly, "@ad_min_similarity") {
		t.Error("vector-only SQL must include sponsored_matches CTE with @ad_min_similarity")
	}
}

func TestCampaignWindowFiltering(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	past := "2026-01-01T00:00:00Z"
	future := "2027-01-01T00:00:00Z"
	lateStart := "2026-12-01T00:00:00Z"
	earlyEnd := "2026-05-01T00:00:00Z"

	if !campaignWithinWindow(models.AdCampaign{StartAt: &past, EndAt: &future}, now) {
		t.Fatal("campaign within [past, future] should be active")
	}
	if campaignWithinWindow(models.AdCampaign{StartAt: &lateStart}, now) {
		t.Fatal("campaign starting in the future should be inactive")
	}
	if campaignWithinWindow(models.AdCampaign{EndAt: &earlyEnd}, now) {
		t.Fatal("campaign that already ended should be inactive")
	}
}
