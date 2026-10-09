package eval

import "testing"

func TestComputeNDCGAndMRR(t *testing.T) {
	ndcg, mrr := ComputeNDCGAndMRR([]string{"prod-001", "prod-003", "prod-002"}, []string{"prod-001", "prod-002", "prod-003"})
	if ndcg < 0.99 {
		t.Fatalf("ndcg = %v, want >= 0.99", ndcg)
	}
	if mrr != 1.0 {
		t.Fatalf("mrr = %v, want 1.0", mrr)
	}
}
