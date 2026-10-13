package eval

import (
	"strings"
	"testing"
	"time"
)

func TestAnalyzeFactoryTelemetry(t *testing.T) {
	refTime, err := time.Parse(time.RFC3339, "2026-10-13T23:25:00Z")
	if err != nil {
		t.Fatalf("failed to parse reference time: %v", err)
	}

	reviewerTime, _ := time.Parse(time.RFC3339, "2026-10-09T17:57:21Z")
	backendTime, _ := time.Parse(time.RFC3339, "2026-10-13T20:22:04Z")
	frontendTime, _ := time.Parse(time.RFC3339, "2026-10-13T23:24:00Z")
	leadTime, _ := time.Parse(time.RFC3339, "2026-10-13T23:16:42Z")
	deployerTime, _ := time.Parse(time.RFC3339, "2026-10-13T21:47:39Z")

	agents := []AgentStatus{
		{
			ProjectSlug:    "psearch",
			AgentName:      "lead",
			AgentID:        "3f751e46-95d4-4641-aa9e-1217720d8a78",
			Template:       "lead",
			Harness:        "claude",
			Phase:          "running",
			Activity:       "blocked",
			LastSummary:    "Close psearch issues #1-#10 with live acceptance evidence",
			UpdatedAt:      refTime,
			LastActivityAt: &leadTime,
		},
		{
			ProjectSlug:    "psearch",
			AgentName:      "backend",
			AgentID:        "f6a7a024-6afe-486c-b7a0-50eba708ae6c",
			Template:       "backend",
			Harness:        "opencode",
			Phase:          "running",
			Activity:       "stalled",
			LastSummary:    "Ready",
			UpdatedAt:      refTime,
			LastActivityAt: &backendTime,
		},
		{
			ProjectSlug:    "psearch",
			AgentName:      "frontend",
			AgentID:        "41182c1a-752b-4251-ae25-931ce1a1735a",
			Template:       "frontend",
			Harness:        "antigravity",
			Phase:          "running",
			Activity:       "working",
			LastSummary:    "Issue #20 UI (session personalization)",
			UpdatedAt:      refTime,
			LastActivityAt: &frontendTime,
		},
		{
			ProjectSlug:    "psearch",
			AgentName:      "reviewer",
			AgentID:        "94d58b62-68a7-4a59-be2c-c6d16d11e102",
			Template:       "reviewer",
			Harness:        "hermes",
			Phase:          "running",
			Activity:       "stalled",
			LastSummary:    "Agent started",
			UpdatedAt:      refTime,
			LastActivityAt: &reviewerTime,
		},
		{
			ProjectSlug:    "psearch",
			AgentName:      "deployer",
			AgentID:        "cd251284-18c5-4b61-971b-b1a213947868",
			Template:       "deployer",
			Harness:        "claude",
			Phase:          "running",
			Activity:       "stalled",
			LastSummary:    "Acknowledged PO deploy rules: clean git-archive source and --to-latest after pinned rollback",
			UpdatedAt:      refTime,
			LastActivityAt: &deployerTime,
		},
	}

	report := AnalyzeFactoryTelemetry(agents, 384.3, refTime)

	if report.TotalAgents != 5 {
		t.Errorf("expected 5 agents, got %d", report.TotalAgents)
	}

	foundReviewer := false
	foundLead := false
	foundBackend := false
	foundFrontend := false
	foundTelemetry := false
	foundLatency := false
	foundProvisionFstring := false

	for _, d := range report.Diagnostics {
		switch d.ID {
		case "diag-reviewer-abandoned":
			foundReviewer = true
			if d.Severity != SeverityCritical {
				t.Errorf("expected critical severity for abandoned reviewer, got %s", d.Severity)
			}
		case "diag-lead-stale-objective":
			foundLead = true
			if d.Severity != SeverityCritical {
				t.Errorf("expected critical severity for lead stale objective, got %s", d.Severity)
			}
		case "diag-backend-idle-handoff":
			foundBackend = true
			if d.Severity != SeverityWarning {
				t.Errorf("expected warning severity for backend idle handoff, got %s", d.Severity)
			}
		case "diag-frontend-redundant-work":
			foundFrontend = true
		case "diag-telemetry-cloud-gap":
			foundTelemetry = true
		case "diag-spanner-latency-p95":
			foundLatency = true
		case "diag-antigravity-provision-fstring":
			foundProvisionFstring = true
			if d.Severity != SeverityCritical {
				t.Errorf("expected critical severity for antigravity provision f-string bug, got %s", d.Severity)
			}
		}
	}

	if !foundReviewer {
		t.Errorf("missing diagnostic for reviewer abandonment")
	}
	if !foundLead {
		t.Errorf("missing diagnostic for lead stale objective")
	}
	if !foundBackend {
		t.Errorf("missing diagnostic for backend idle handoff")
	}
	if !foundFrontend {
		t.Errorf("missing diagnostic for frontend redundant work")
	}
	if !foundTelemetry {
		t.Errorf("missing diagnostic for telemetry export gap")
	}
	if !foundLatency {
		t.Errorf("missing diagnostic for p95 latency spike")
	}
	if !foundProvisionFstring {
		t.Errorf("missing diagnostic for antigravity provision.py f-string NameError")
	}

	if len(report.ActionableRecommendations) == 0 {
		t.Errorf("expected actionable recommendations, got 0")
	}

	jsonStr, err := report.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize report to JSON: %v", err)
	}
	if !strings.Contains(jsonStr, "diag-reviewer-abandoned") {
		t.Errorf("expected json to contain diag-reviewer-abandoned")
	}
}
