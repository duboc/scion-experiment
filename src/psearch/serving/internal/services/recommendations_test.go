package services

import (
	"context"
	"strings"
	"testing"

	"psearch/serving-go/internal/models"
)

func TestSimilarProducts_SimilarSubstitutesCoViewedAndNullEmbeddingFallback(t *testing.T) {
	ctx := context.Background()
	svc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}
	sessionStore := NewMemorySessionStore()
	sessionSvc := NewSessionService(sessionStore, svc, DefaultSessionBoostW)

	// 1) Out-of-stock seed item prod-004 (Reebok Footwear) with mode=substitutes:
	// all returned items must exclude prod-004, belong to Footwear, and be IN_STOCK.
	subs, err := svc.SimilarProducts(ctx, sessionSvc, SimilarOptions{
		SeedID: "prod-004",
		Mode:   SimilarModeSubstitutes,
		Limit:  4,
	})
	if err != nil || len(subs.Results) == 0 {
		t.Fatalf("substitutes for prod-004 failed: %v (len=%d)", err, len(subs.Results))
	}
	for _, r := range subs.Results {
		if r.ID == "prod-004" {
			t.Fatal("substitutes must never include seed product prod-004")
		}
		if len(r.Categories) == 0 || !strings.EqualFold(r.Categories[0], "Footwear") {
			t.Fatalf("substitute %s has category %v, want Footwear", r.ID, r.Categories)
		}
		if isUnavailable(r.Availability, r.AvailableQuantity) {
			t.Fatalf("substitute %s must be IN_STOCK, got %s", r.ID, r.Availability)
		}
		if r.SubstituteReason == "" || r.SimilarityScore <= 0 {
			t.Fatalf("substitute %s missing reason or similarity_score: %+v", r.ID, r)
		}
	}

	// 2) Record session co-visitation between prod-001 and prod-008, then query mode=co_viewed.
	_, _ = sessionSvc.Record(ctx, SessionEvent{TenantID: "default", SessionID: "sess-covis-1", ProductID: "prod-001", EventType: "click"})
	_, _ = sessionSvc.Record(ctx, SessionEvent{TenantID: "default", SessionID: "sess-covis-1", ProductID: "prod-008", EventType: "purchase"})
	_, _ = sessionSvc.Record(ctx, SessionEvent{TenantID: "default", SessionID: "sess-covis-2", ProductID: "prod-001", EventType: "click"})
	_, _ = sessionSvc.Record(ctx, SessionEvent{TenantID: "default", SessionID: "sess-covis-2", ProductID: "prod-008", EventType: "click"})

	coViewed, err := svc.SimilarProducts(ctx, sessionSvc, SimilarOptions{
		SeedID: "prod-001",
		Mode:   SimilarModeCoViewed,
		Limit:  6,
	})
	if err != nil || len(coViewed.Results) == 0 {
		t.Fatalf("co_viewed for prod-001 failed: %v", err)
	}
	if coViewed.Results[0].ID != "prod-008" || coViewed.Results[0].CoViewCount != 2 {
		t.Fatalf("expected prod-008 ranked #1 with co_view_count=2, got %+v", coViewed.Results[0])
	}

	// 3) NULL-embedding fallback: when seed product has NULL embedding, lexical token overlap still returns peers.
	svc.SetNullEmbeddingForTest("prod-005", true)
	defer svc.SetNullEmbeddingForTest("prod-005", false)
	nullFallback, err := svc.SimilarProducts(ctx, sessionSvc, SimilarOptions{
		SeedID:     "prod-005",
		Mode:       SimilarModeSimilar,
		Limit:      4,
		PriceTable: "b2b-gold",
		CEP:        "01310-100",
	})
	if err != nil || len(nullFallback.Results) == 0 {
		t.Fatalf("NULL-embedding lexical fallback failed: %v", err)
	}
	if nullFallback.Results[0].PriceInfo.PriceTableID != "b2b-gold" || nullFallback.Results[0].DeliveryPromise == nil {
		t.Fatalf("expected contract price and delivery promise on similar results: %+v", nullFallback.Results[0])
	}

	// 4) Unknown or wrong-tenant seed product returns ErrProductNotFound.
	if _, err := svc.SimilarProducts(ctx, sessionSvc, SimilarOptions{SeedID: "prod-unknown"}); err != ErrProductNotFound {
		t.Fatalf("expected ErrProductNotFound for unknown product, got %v", err)
	}
	if _, err := svc.SimilarProducts(ctx, sessionSvc, SimilarOptions{SeedID: "prod-001", TenantID: "other-tenant"}); err != ErrProductNotFound {
		t.Fatalf("expected ErrProductNotFound for wrong tenant, got %v", err)
	}
}

func TestBuildSimilarVectorSQL_StoringOnlyInANNSubquery(t *testing.T) {
	annSQL := buildSimilarVectorSQL(vectorColumnV2, true, 20)
	if !strings.Contains(annSQL, "APPROX_COSINE_DISTANCE") || !strings.Contains(annSQL, "products_embedding_v2_idx") {
		t.Fatalf("expected ScaNN ANN SQL in buildSimilarVectorSQL: %s", annSQL)
	}
	exactSQL := buildSimilarVectorSQL(vectorColumnV2, false, 20)
	if strings.Contains(exactSQL, "APPROX_COSINE_DISTANCE") || !strings.Contains(exactSQL, "COSINE_DISTANCE") {
		t.Fatalf("expected exact COSINE_DISTANCE SQL when useANN=false: %s", exactSQL)
	}
	var _ = models.SearchResult{}
}
