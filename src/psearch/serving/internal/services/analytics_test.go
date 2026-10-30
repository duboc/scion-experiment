package services

import (
	"context"
	"math"
	"testing"

	"psearch/serving-go/internal/models"
)

func TestAnalyticsIdempotentEventsSummaryMRRZeroResultAndRollup(t *testing.T) {
	ctx := context.Background()
	spannerSvc := &SpannerService{config: testConfig("v1"), seedProducts: DefaultSeedProducts()}
	sessionStore := NewMemorySessionStore()
	sessionSvc := NewSessionService(sessionStore, spannerSvc, DefaultSessionBoostW)
	analytics := NewAnalyticsService(NewMemoryAnalyticsStore(), spannerSvc, sessionSvc)

	// 1) Enqueue an impression for experiment_id=ltr_v2 with results [prod-001, prod-002, prod-003]
	// and a zero-result impression for q=xyznonexistent99.
	analytics.EnqueueImpression(SearchImpression{
		ImpressionID:  "imp-test-1",
		TenantID:      "default",
		QueryRaw:      "Running Shoes",
		ExperimentID:  "ltr_v2",
		RetrievalMode: "hybrid_v2",
		TotalFound:    3,
		LatencyMs:     12.5,
		ResultIDs:     []string{"prod-001", "prod-002", "prod-003"},
	})
	analytics.EnqueueImpression(SearchImpression{
		ImpressionID:  "imp-zero-1",
		TenantID:      "default",
		QueryRaw:      "xyznonexistent99",
		ExperimentID:  "control",
		RetrievalMode: "hybrid_v2",
		TotalFound:    0,
		LatencyMs:     8.0,
		ResultIDs:     nil,
	})

	// 2) Record a click at position=2 and a purchase for imp-test-1, then replay the click to test idempotency.
	clickReq := models.AnalyticsEventRequest{
		EventID:      "evt-click-pos2",
		QueryID:      "imp-test-1",
		SessionID:    "sess-analytics-01",
		EventType:    "click",
		ProductID:    "prod-002",
		Position:     2,
		ExperimentID: "ltr_v2",
		TenantID:     "default",
	}
	c1, err := analytics.RecordEvent(ctx, clickReq)
	if err != nil || c1.Duplicate || !c1.Recorded {
		t.Fatalf("first click event failed: %+v, err=%v", c1, err)
	}
	cDup, err := analytics.RecordEvent(ctx, clickReq)
	if err != nil || !cDup.Duplicate || cDup.Recorded {
		t.Fatalf("duplicate click replay must report duplicate=true: %+v, err=%v", cDup, err)
	}

	purchReq := models.AnalyticsEventRequest{
		EventID:       "evt-purch-1",
		QueryID:       "imp-test-1",
		SessionID:     "sess-analytics-01",
		EventType:     "purchase",
		ProductID:     "prod-002",
		Position:      2,
		ExperimentID:  "ltr_v2",
		RevenueMicros: 169900000,
		TenantID:      "default",
	}
	p1, err := analytics.RecordEvent(ctx, purchReq)
	if err != nil || p1.Duplicate || !p1.Recorded {
		t.Fatalf("purchase event failed: %+v, err=%v", p1, err)
	}

	// Verify session forwarding: prod-002 click and purchase were forwarded to SessionService.
	recentEvs, _ := sessionSvc.Recent(ctx, "default", "sess-analytics-01")
	if len(recentEvs) != 2 {
		t.Fatalf("expected 2 forwarded session events, got %d", len(recentEvs))
	}

	// 3) Summary reports MRR@10 = 0.5 (click at position 2), xyznonexistent99 in top_zero_result_queries, and ltr_v2 breakdown.
	sum, err := analytics.Summary(ctx, "default", 168)
	if err != nil {
		t.Fatalf("Summary error: %v", err)
	}
	if sum.TotalSearches != 2 || sum.ZeroResultSearches != 1 || sum.MRRAt10 != 0.5 || sum.TotalConversions != 1 {
		t.Fatalf("unexpected summary KPIs: %+v", sum)
	}
	if len(sum.TopZeroResultQueries) != 1 || sum.TopZeroResultQueries[0].Query != "xyznonexistent99" {
		t.Fatalf("expected xyznonexistent99 in top_zero_result_queries, got %+v", sum.TopZeroResultQueries)
	}
	foundLTRV2 := false
	for _, ex := range sum.Experiments {
		if ex.ExperimentID == "ltr_v2" && ex.MRRAt10 == 0.5 && ex.CTR == 1.0 && ex.Conversions == 1 {
			foundLTRV2 = true
		}
	}
	if !foundLTRV2 {
		t.Fatalf("missing or inaccurate ltr_v2 experiment breakdown: %+v", sum.Experiments)
	}

	// 4) RollupProductSignals updates product engagement signals with Laplace smoothing and zero NaNs.
	rollup, err := analytics.RollupProductSignals(ctx, "default", 30)
	if err != nil || rollup.ProductsUpdated < 3 {
		t.Fatalf("RollupProductSignals failed: %+v, err=%v", rollup, err)
	}
	for pid, sig := range rollup.UpdatedSignals {
		if math.IsNaN(sig.CTR) || math.IsNaN(sig.ConversionRate) || sig.CTR <= 0 || sig.CTR > 1 {
			t.Fatalf("invalid Laplace-smoothed signal for %s: %+v", pid, sig)
		}
	}
}
