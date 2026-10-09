package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"psearch-eval/internal/eval"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getEnv("PORT", "8080")
	targetURL := getEnv("PSEARCH_SERVING_URL", "https://psearch-serving-249096448072.us-central1.run.app")
	ghRepo := getEnv("GITHUB_REPO", "duboc/scion-experiment")
	ghToken := getEnv("GITHUB_TOKEN", "")

	engine := eval.NewEngine(targetURL, ghRepo, ghToken)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "psearch-eval-dashboard", "design_skill": "impeccable-4.5.2"})
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "psearch-eval-dashboard", "design_skill": "impeccable-4.5.2"})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service":             "psearch-eval-dashboard",
			"project":             "psearch-eval",
			"harness":             "antigravity (100%)",
			"design_system":       "impeccable.style (Operate · Precision Telemetry Ledger)",
			"repo":                ghRepo,
			"branch":              "eval-dashboard",
			"sandbox":             "riojucu-sandbox",
			"psearch_serving_url": engine.TargetURL,
			"spanner_instance":    "psearch-instance",
			"spanner_database":    "psearch-db",
			"bigquery_dataset":    "psearch_dataset",
			"orchestrator":        "eval-lead",
			"agents":              []string{"eval-lead", "factory-analyst", "search-evaluator", "ui-craftsman", "eval-deployer"},
		})
	})

	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"projects": []string{"psearch", "psearch-eval"},
			"agents":   engine.GetAgents(),
		})
	})

	mux.HandleFunc("/api/agents/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Agents []eval.AgentTelemetry `json:"agents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		engine.UpdateAgents(payload.Agents)
		writeJSON(w, http.StatusOK, map[string]interface{}{"updated": len(payload.Agents)})
	})

	mux.HandleFunc("/api/eval", func(w http.ResponseWriter, r *http.Request) {
		report := engine.RunEvaluation()
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("/api/workbench", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			q = "running shoes"
		}
		alpha := r.URL.Query().Get("alpha")
		if alpha == "" {
			alpha = "0.65"
		}
		locale := r.URL.Query().Get("locale")
		u := fmt.Sprintf("%s/api/search?q=%s&alpha=%s&locale=%s&limit=8", engine.TargetURL, url.QueryEscape(q), url.QueryEscape(alpha), url.QueryEscape(locale))
		start := time.Now()
		resp, err := engine.Client.Get(u)
		latMs := float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error(), "latency_ms": latMs})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var out map[string]interface{}
		if err := json.Unmarshal(body, &out); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": "invalid JSON from psearch-serving", "latency_ms": latMs})
			return
		}
		out["probe_latency_ms"] = latMs
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("/api/backlog", func(w http.ResponseWriter, r *http.Request) {
		issues := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/issues?state=all&per_page=30&sort=created&direction=asc", ghRepo), ghToken)
		mainCommits := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/commits?sha=main&per_page=8", ghRepo), ghToken)
		evalCommits := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/commits?sha=eval-dashboard&per_page=8", ghRepo), ghToken)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"repo":         ghRepo,
			"issues":       issues,
			"main_commits": mainCommits,
			"eval_commits": evalCommits,
			"checked_at":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/api/sandbox", func(w http.ResponseWriter, r *http.Request) {
		var servingInfo map[string]interface{}
		if resp, err := engine.Client.Get(engine.TargetURL + "/api/info"); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			_ = json.Unmarshal(body, &servingInfo)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"gcp_project":      "riojucu-sandbox",
			"project_number":   "249096448072",
			"region":           "us-central1",
			"deployer_sa":      "agent-deployer@riojucu-sandbox.iam.gserviceaccount.com",
			"spanner_instance": "psearch-instance (ENTERPRISE, 100 PU)",
			"spanner_database": "psearch-db",
			"spanner_tables":   []string{"products", "factory_agent_status", "eval_runs"},
			"bigquery_dataset": "riojucu-sandbox:psearch_dataset",
			"psearch_serving":  servingInfo,
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(impeccableWorkbenchHTML))
	})

	log.Printf("psearch-eval Impeccable Workbench starting on :%s (probing %s)", port, targetURL)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func fetchGitHubJSON(client *http.Client, urlStr, token string) interface{} {
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return []interface{}{}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return []interface{}{}
	}
	defer resp.Body.Close()
	var out interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return []interface{}{}
	}
	return out
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

const impeccableWorkbenchHTML = `<!DOCTYPE html>
<html lang="en" data-theme="daylight">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>psearch-eval — Factory Intelligence & Search Quality Workbench</title>
  <meta name="description" content="Impeccable-crafted observability, multi-agent diagnostics, and Cloud Spanner hybrid search evaluation workbench for the Scion psearch software factory." />
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=Albert+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
  <style>
    :root, [data-theme="daylight"] {
      --bg-canvas: oklch(97.8% 0.007 80);
      --bg-surface: oklch(99.3% 0.004 80);
      --bg-subtle: oklch(95.2% 0.010 80);
      --bg-elevated: oklch(92.8% 0.014 80);
      --border-hairline: oklch(87.5% 0.014 80);
      --border-strong: oklch(76.0% 0.018 80);
      --text-primary: oklch(19.5% 0.016 255);
      --text-secondary: oklch(40.0% 0.018 255);
      --text-muted: oklch(52.0% 0.016 255);
      --accent-primary: oklch(48.0% 0.165 248);
      --accent-hover: oklch(42.0% 0.165 248);
      --accent-tint: oklch(94.0% 0.030 248);
      --status-pass: oklch(45.0% 0.140 155);
      --status-pass-bg: oklch(94.5% 0.035 155);
      --status-warn: oklch(50.0% 0.150 68);
      --status-warn-bg: oklch(95.0% 0.045 75);
      --status-gap: oklch(48.0% 0.175 28);
      --status-gap-bg: oklch(95.0% 0.035 28);
      --shadow-elevated: 0 6px 20px -4px oklch(20% 0.02 255 / 0.08);
    }

    [data-theme="carbon"] {
      --bg-canvas: oklch(14.5% 0.012 250);
      --bg-surface: oklch(17.5% 0.014 250);
      --bg-subtle: oklch(20.5% 0.016 250);
      --bg-elevated: oklch(24.5% 0.020 250);
      --border-hairline: oklch(27.5% 0.016 250);
      --border-strong: oklch(38.0% 0.020 250);
      --text-primary: oklch(95.5% 0.008 80);
      --text-secondary: oklch(77.0% 0.012 80);
      --text-muted: oklch(63.0% 0.014 80);
      --accent-primary: oklch(68.0% 0.155 245);
      --accent-hover: oklch(74.0% 0.155 245);
      --accent-tint: oklch(25.0% 0.045 245);
      --status-pass: oklch(74.0% 0.150 155);
      --status-pass-bg: oklch(24.0% 0.045 155);
      --status-warn: oklch(78.0% 0.155 75);
      --status-warn-bg: oklch(25.0% 0.050 75);
      --status-gap: oklch(72.0% 0.170 28);
      --status-gap-bg: oklch(25.0% 0.050 28);
      --shadow-elevated: 0 8px 24px -4px oklch(8% 0.01 250 / 0.45);
    }

    /* Impeccable Craft Floor: Themed Browser Surfaces */
    ::selection {
      background: var(--accent-primary);
      color: var(--bg-surface);
    }
    html {
      font-family: 'Albert Sans', -apple-system, BlinkMacSystemFont, sans-serif;
      font-size: 16px;
      background: var(--bg-canvas);
      color: var(--text-primary);
      caret-color: var(--accent-primary);
      scrollbar-color: var(--border-strong) var(--bg-subtle);
      scrollbar-width: thin;
      -webkit-font-smoothing: antialiased;
    }
    ::-webkit-scrollbar { width: 8px; height: 8px; }
    ::-webkit-scrollbar-track { background: var(--bg-subtle); }
    ::-webkit-scrollbar-thumb { background: var(--border-strong); border-radius: 0; }
    :focus-visible {
      outline: 2px solid var(--accent-primary);
      outline-offset: 2px;
    }

    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; display: flex; flex-direction: column; }

    /* Typography & Tabular Numerals */
    h1, h2, h3 { margin: 0; text-wrap: balance; font-weight: 600; color: var(--text-primary); }
    h1 { font-size: 1.25rem; letter-spacing: -0.025em; }
    h2 { font-size: 1.0625rem; letter-spacing: -0.015em; }
    h3 { font-size: 0.9375rem; letter-spacing: -0.01em; }
    p { margin: 0; max-width: 70ch; line-height: 1.55; color: var(--text-secondary); font-size: 0.9375rem; }
    .mono, code, .num {
      font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, monospace;
      font-variant-numeric: tabular-nums;
    }
    a {
      color: var(--accent-primary);
      text-decoration: underline;
      text-underline-offset: 3px;
      text-decoration-thickness: 1px;
      transition: color 160ms cubic-bezier(0.22, 1, 0.36, 1);
    }
    a:hover { color: var(--accent-hover); }

    /* Top Operational Bar */
    .topbar {
      background: var(--bg-surface);
      border-bottom: 1px solid var(--border-hairline);
      padding: 0.75rem 1.5rem;
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 1rem;
      flex-wrap: wrap;
      position: sticky;
      top: 0;
      z-index: 30;
    }
    .brand-cluster {
      display: flex;
      align-items: baseline;
      gap: 0.875rem;
      flex-wrap: wrap;
    }
    .brand-meta {
      font-size: 0.8125rem;
      color: var(--text-muted);
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    .toolbar-actions {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      flex-wrap: wrap;
    }
    .btn {
      font-family: inherit;
      font-size: 0.8125rem;
      font-weight: 600;
      padding: 0.4375rem 0.8125rem;
      border: 1px solid var(--border-strong);
      background: var(--bg-surface);
      color: var(--text-primary);
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 0.4375rem;
      transition: background 160ms ease, border-color 160ms ease, transform 120ms ease;
    }
    .btn:hover { background: var(--bg-subtle); border-color: var(--text-secondary); }
    .btn:active { transform: translateY(1px); }
    .btn-primary {
      background: var(--accent-primary);
      color: var(--bg-surface);
      border-color: var(--accent-primary);
    }
    .btn-primary:hover {
      background: var(--accent-hover);
      border-color: var(--accent-hover);
    }

    /* Main Structural Hairline Grid (No Nested Cards) */
    .workbench {
      max-width: 1520px;
      width: 100%;
      margin: 0 auto;
      padding: 1.25rem 1.5rem 2.5rem;
      display: flex;
      flex-direction: column;
      gap: 1.5rem;
    }

    .ledger-frame {
      background: var(--bg-surface);
      border: 1px solid var(--border-hairline);
    }

    /* Asymmetric Focal Strip: Primary Search Quality Readout + Alpha Curve + Compact Ledger */
    .focal-strip {
      display: grid;
      grid-template-columns: 1.15fr 1.35fr 1fr;
    }
    @media (max-width: 1120px) {
      .focal-strip { grid-template-columns: 1fr; }
    }
    .focal-cell {
      padding: 1.25rem 1.375rem;
      border-right: 1px solid var(--border-hairline);
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      gap: 0.875rem;
    }
    .focal-cell:last-child { border-right: none; }
    @media (max-width: 1120px) {
      .focal-cell { border-right: none; border-bottom: 1px solid var(--border-hairline); }
    }

    .primary-readout {
      display: flex;
      align-items: baseline;
      gap: 1.5rem;
      flex-wrap: wrap;
    }
    .big-figure {
      font-size: 2.375rem;
      font-weight: 700;
      letter-spacing: -0.035em;
      line-height: 1;
      color: var(--text-primary);
    }
    .figure-unit {
      font-size: 0.8125rem;
      color: var(--text-secondary);
      margin-top: 0.25rem;
    }

    .kv-list {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 0.625rem 1rem;
      padding-top: 0.625rem;
      border-top: 1px solid var(--border-hairline);
      font-size: 0.8125rem;
    }
    .kv-item {
      display: flex;
      justify-content: space-between;
      align-items: baseline;
      gap: 0.5rem;
    }
    .kv-key { color: var(--text-muted); }
    .kv-val { font-weight: 600; color: var(--text-primary); }

    /* Two-Column Structural Split */
    .split-2 {
      display: grid;
      grid-template-columns: 1.15fr 0.85fr;
    }
    @media (max-width: 1120px) {
      .split-2 { grid-template-columns: 1fr; }
    }
    .zone {
      border-right: 1px solid var(--border-hairline);
      display: flex;
      flex-direction: column;
    }
    .zone:last-child { border-right: none; }
    .zone-header {
      padding: 0.9375rem 1.25rem;
      background: var(--bg-subtle);
      border-bottom: 1px solid var(--border-hairline);
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 0.75rem;
      flex-wrap: wrap;
    }
    .zone-body {
      padding: 1.125rem 1.25rem;
    }
    .zone-body--flush {
      padding: 0;
    }

    /* Tables */
    .table-wrap { width: 100%; overflow-x: auto; }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.8125rem;
    }
    th, td {
      text-align: left;
      padding: 0.625rem 0.875rem;
      border-bottom: 1px solid var(--border-hairline);
      vertical-align: middle;
    }
    th {
      background: var(--bg-subtle);
      color: var(--text-secondary);
      font-weight: 600;
      font-size: 0.75rem;
      white-space: nowrap;
    }
    tbody tr { transition: background 120ms ease; }
    tbody tr:hover { background: var(--bg-subtle); }
    tbody tr.is-selected { background: var(--bg-elevated); }

    /* Semantic Status Badges (1px border, tinted surface, never >1px left-border) */
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 0.35rem;
      padding: 0.15rem 0.5rem;
      font-size: 0.75rem;
      font-weight: 600;
      border: 1px solid transparent;
    }
    .badge-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: currentColor;
    }
    .badge--pass, .badge--running, .badge--closed, .badge--HEALTHY {
      color: var(--status-pass);
      background: var(--status-pass-bg);
      border-color: var(--status-pass);
    }
    .badge--partial, .badge--open, .badge--ACTION, .badge--INSIGHT {
      color: var(--status-warn);
      background: var(--status-warn-bg);
      border-color: var(--status-warn);
    }
    .badge--pending {
      color: var(--text-muted);
      background: var(--bg-subtle);
      border-color: var(--border-hairline);
    }

    /* Filter Bar & Inputs */
    .filter-group {
      display: inline-flex;
      border: 1px solid var(--border-strong);
      background: var(--bg-surface);
    }
    .filter-btn {
      font-family: inherit;
      font-size: 0.75rem;
      font-weight: 600;
      padding: 0.3125rem 0.625rem;
      border: none;
      border-right: 1px solid var(--border-hairline);
      background: transparent;
      color: var(--text-secondary);
      cursor: pointer;
    }
    .filter-btn:last-child { border-right: none; }
    .filter-btn:hover { background: var(--bg-subtle); color: var(--text-primary); }
    .filter-btn[aria-pressed="true"] {
      background: var(--accent-primary);
      color: var(--bg-surface);
    }

    /* Interactive Hybrid RRF Query Workbench */
    .workbench-bar {
      display: grid;
      grid-template-columns: 1.5fr 1fr 0.7fr auto;
      gap: 0.75rem;
      padding: 0.875rem 1.25rem;
      background: var(--bg-subtle);
      border-bottom: 1px solid var(--border-hairline);
      align-items: end;
    }
    @media (max-width: 900px) {
      .workbench-bar { grid-template-columns: 1fr; }
    }
    .field {
      display: flex;
      flex-direction: column;
      gap: 0.3125rem;
    }
    .field label {
      font-size: 0.75rem;
      font-weight: 600;
      color: var(--text-secondary);
      display: flex;
      justify-content: space-between;
    }
    .input, select {
      font-family: inherit;
      font-size: 0.8125rem;
      padding: 0.4375rem 0.625rem;
      background: var(--bg-surface);
      color: var(--text-primary);
      border: 1px solid var(--border-strong);
    }
    input[type="range"] {
      width: 100%;
      accent-color: var(--accent-primary);
      cursor: pointer;
    }

    /* Diagnostic Stream Rows (Hairline separated, no nested cards, no thick side-border) */
    .diag-list {
      display: flex;
      flex-direction: column;
    }
    .diag-row {
      padding: 0.875rem 1.25rem;
      border-bottom: 1px solid var(--border-hairline);
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 0.875rem;
      align-items: start;
    }
    .diag-row:last-child { border-bottom: none; }
    .diag-title {
      font-size: 0.875rem;
      font-weight: 600;
      color: var(--text-primary);
      margin-bottom: 0.25rem;
    }
    .diag-desc {
      font-size: 0.8125rem;
      color: var(--text-secondary);
      margin-bottom: 0.375rem;
    }
    .diag-rec {
      font-size: 0.8125rem;
      color: var(--text-primary);
      background: var(--bg-subtle);
      padding: 0.375rem 0.625rem;
      border: 1px solid var(--border-hairline);
    }

    /* Topology SVG & Inspector */
    .topo-canvas {
      width: 100%;
      height: 250px;
      display: block;
      background: var(--bg-surface);
      border-bottom: 1px solid var(--border-hairline);
    }
    .topo-node { cursor: pointer; }
    .topo-node rect {
      fill: var(--bg-subtle);
      stroke: var(--border-strong);
      stroke-width: 1.25;
      transition: fill 150ms ease, stroke 150ms ease;
    }
    .topo-node:hover rect, .topo-node.is-active rect {
      fill: var(--accent-tint);
      stroke: var(--accent-primary);
      stroke-width: 2;
    }
    .topo-node text {
      fill: var(--text-primary);
      font-family: 'JetBrains Mono', monospace;
      font-size: 11px;
      font-weight: 600;
    }
    .topo-node .sub {
      fill: var(--text-muted);
      font-family: 'Albert Sans', sans-serif;
      font-size: 10px;
      font-weight: 500;
    }
    .topo-edge {
      stroke: var(--border-strong);
      stroke-width: 1.25;
      fill: none;
    }
    .inspector-strip {
      padding: 0.75rem 1.25rem;
      background: var(--bg-subtle);
      border-bottom: 1px solid var(--border-hairline);
      font-size: 0.8125rem;
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 1rem;
      flex-wrap: wrap;
    }
  </style>
</head>
<body>
  <header class="topbar">
    <div class="brand-cluster">
      <h1>psearch-eval Workbench</h1>
      <div class="brand-meta">
        <span>Scion Software Factory Intelligence &amp; Biggy Search Parity Evaluation</span>
        <span>·</span>
        <span class="mono">duboc/scion-experiment</span>
        <span>·</span>
        <span class="mono">Cloud Spanner 100 PU (Enterprise)</span>
      </div>
    </div>
    <div class="toolbar-actions">
      <a id="link-storefront" href="https://psearch-serving-249096448072.us-central1.run.app" target="_blank" class="btn">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><path d="M7 17L17 7M17 7H8M17 7V16"/></svg>
        <span>Open psearch-serving</span>
      </a>
      <a href="https://github.com/duboc/scion-experiment/issues" target="_blank" class="btn">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="12"/><circle cx="12" cy="16" r="0.8" fill="currentColor"/></svg>
        <span>Backlog (#1–#10)</span>
      </a>
      <button type="button" id="btn-theme" class="btn" onclick="toggleTheme()">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2"/></svg>
        <span id="theme-label">Carbon Mode</span>
      </button>
      <button type="button" id="btn-run-eval" class="btn btn-primary" onclick="refreshAll()">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        <span>Run Live Evaluation</span>
      </button>
    </div>
  </header>

  <main class="workbench">
    <!-- Asymmetric Focal Strip: Relevance Readout + Alpha Curve SVG + Operational Ledger -->
    <section class="ledger-frame focal-strip" aria-label="Search Quality and Factory Readiness Summary">
      <div class="focal-cell">
        <div>
          <h2>Hybrid Relevance &amp; Ranking Quality</h2>
          <p style="font-size:0.8125rem;margin-top:0.2rem;">Evaluated live against <code id="lbl-backend-mode">spanner</code> across 6 golden e-commerce queries (EN &amp; PT-BR).</p>
        </div>
        <div class="primary-readout">
          <div>
            <div class="big-figure num" id="val-ndcg">--</div>
            <div class="figure-unit">NDCG@10 (Target &ge; 0.850)</div>
          </div>
          <div>
            <div class="big-figure num" id="val-mrr">--</div>
            <div class="figure-unit">MRR@10 (Reciprocal Rank)</div>
          </div>
        </div>
        <div class="kv-list">
          <div class="kv-item"><span class="kv-key">Optimal RRF &alpha;</span><span class="kv-val mono" id="val-opt-alpha">0.65</span></div>
          <div class="kv-item"><span class="kv-key">Last Probe</span><span class="kv-val mono" id="val-timestamp">--</span></div>
        </div>
      </div>

      <div class="focal-cell">
        <div style="display:flex;justify-content:space-between;align-items:baseline;">
          <h2>RRF &alpha; Sensitivity Curve (Lexical vs. Vector Fusion)</h2>
          <span class="mono" style="font-size:0.75rem;color:var(--text-muted);">&alpha;=0.0 FTS &rarr; &alpha;=1.0 Vector</span>
        </div>
        <svg id="svg-alpha-curve" viewBox="0 0 460 118" style="width:100%;height:118px;display:block;overflow:visible;" aria-label="Alpha sensitivity curve"></svg>
      </div>

      <div class="focal-cell">
        <div>
          <h2>Factory &amp; Spanner Operational Ledger</h2>
          <p style="font-size:0.8125rem;margin-top:0.2rem;">Latency percentiles and Section 4.1 backlog readiness across Waves 1–4.</p>
        </div>
        <div class="kv-list" style="border-top:none;padding-top:0;">
          <div class="kv-item"><span class="kv-key">Latency p50</span><span class="kv-val mono" id="val-p50">-- ms</span></div>
          <div class="kv-item"><span class="kv-key">Latency p95 / p99</span><span class="kv-val mono" id="val-p95">-- ms</span></div>
          <div class="kv-item"><span class="kv-key">Requirements Passing</span><span class="kv-val mono" id="val-reqs-pass">-- / 10</span></div>
          <div class="kv-item"><span class="kv-key">Active Scion Agents</span><span class="kv-val mono" id="val-agents-cnt">10 / 10</span></div>
          <div class="kv-item"><span class="kv-key">Spanner Instance</span><span class="kv-val mono">psearch-instance (100 PU)</span></div>
          <div class="kv-item"><span class="kv-key">Eval Harness</span><span class="kv-val mono">antigravity (100%)</span></div>
        </div>
      </div>
    </section>

    <!-- Multi-Agent Software Factory Topology + Antigravity Analyst Diagnostics -->
    <section class="ledger-frame split-2" aria-label="Scion Multi-Agent Software Factory and Diagnostics">
      <div class="zone">
        <div class="zone-header">
          <h2>Scion Multi-Agent Topology &amp; Live Roster</h2>
          <div class="filter-group" role="group" aria-label="Filter agents by Scion project">
            <button type="button" class="filter-btn" aria-pressed="true" onclick="setAgentFilter('all', this)">All (10)</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setAgentFilter('psearch', this)">psearch (main)</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setAgentFilter('psearch-eval', this)">psearch-eval (antigravity)</button>
          </div>
        </div>

        <!-- Interactive Topology SVG -->
        <svg class="topo-canvas" viewBox="0 0 760 240" aria-label="Agent delegation graph">
          <!-- psearch tree (left) -->
          <text x="195" y="20" text-anchor="middle" fill="var(--text-secondary)" font-size="11" font-weight="600">Project: psearch (main branch · Storefront &amp; Spanner Engine)</text>
          <path class="topo-edge" d="M195,62 L65,112 M195,62 L152,112 M195,62 L238,112 M195,62 L325,112" />
          <g class="topo-node is-active" onclick="selectAgentNode('psearch','lead',this)" transform="translate(135,28)">
            <rect width="120" height="34" />
            <text x="60" y="15" text-anchor="middle">@lead</text>
            <text x="60" y="27" text-anchor="middle" class="sub">claude · orchestrator</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch','backend',this)" transform="translate(22,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@backend</text>
            <text x="42" y="28" text-anchor="middle" class="sub">opencode · Go</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch','frontend',this)" transform="translate(110,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@frontend</text>
            <text x="42" y="28" text-anchor="middle" class="sub">antigravity</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch','reviewer',this)" transform="translate(198,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@reviewer</text>
            <text x="42" y="28" text-anchor="middle" class="sub">hermes · TDD</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch','deployer',this)" transform="translate(286,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@deployer</text>
            <text x="42" y="28" text-anchor="middle" class="sub">claude · GCP</text>
          </g>

          <!-- Divider -->
          <line x1="382" y1="12" x2="382" y2="228" stroke="var(--border-hairline)" stroke-dasharray="3 3" />

          <!-- psearch-eval tree (right — 100% Antigravity) -->
          <text x="570" y="20" text-anchor="middle" fill="var(--text-secondary)" font-size="11" font-weight="600">Project: psearch-eval (eval-dashboard · 100% Antigravity Harness)</text>
          <path class="topo-edge" d="M570,62 L435,112 M570,62 L525,112 M570,62 L615,112 M570,62 L705,112" />
          <g class="topo-node" onclick="selectAgentNode('psearch-eval','eval-lead',this)" transform="translate(505,28)">
            <rect width="130" height="34" />
            <text x="65" y="15" text-anchor="middle">@eval-lead</text>
            <text x="65" y="27" text-anchor="middle" class="sub">antigravity · director</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch-eval','factory-analyst',this)" transform="translate(392,112)">
            <rect width="88" height="36" />
            <text x="44" y="15" text-anchor="middle">@factory-analyst</text>
            <text x="44" y="28" text-anchor="middle" class="sub">antigravity</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch-eval','search-evaluator',this)" transform="translate(484,112)">
            <rect width="88" height="36" />
            <text x="44" y="15" text-anchor="middle">@search-eval</text>
            <text x="44" y="28" text-anchor="middle" class="sub">antigravity</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch-eval','ui-craftsman',this)" transform="translate(576,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@ui-craftsman</text>
            <text x="42" y="28" text-anchor="middle" class="sub">impeccable</text>
          </g>
          <g class="topo-node" onclick="selectAgentNode('psearch-eval','eval-deployer',this)" transform="translate(664,112)">
            <rect width="84" height="36" />
            <text x="42" y="15" text-anchor="middle">@eval-deployer</text>
            <text x="42" y="28" text-anchor="middle" class="sub">antigravity</text>
          </g>

          <!-- Shared Sandbox Target -->
          <rect x="60" y="176" width="640" height="44" fill="var(--bg-subtle)" stroke="var(--border-hairline)" />
          <text x="380" y="195" text-anchor="middle" fill="var(--text-primary)" font-family="JetBrains Mono, monospace" font-size="11" font-weight="600">GCP Sandbox: riojucu-sandbox · Cloud Spanner (psearch-instance / psearch-db) · BigQuery (psearch_dataset)</text>
          <text x="380" y="211" text-anchor="middle" fill="var(--text-muted)" font-size="10.5">psearch-eval observes psearch agents in hub.db, benchmarks psearch-serving, and recommends factory optimizations</text>
        </svg>

        <div class="inspector-strip" id="node-inspector">
          <span>Selected Agent: <strong class="mono">@lead (psearch)</strong> — Factory Tech Lead &amp; Backlog Orchestrator</span>
          <span class="mono">Parent: root (PO) · Focus: Wave 1 (#1–#3)</span>
        </div>

        <div class="zone-body--flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Project</th>
                <th>Agent</th>
                <th>Role &amp; Responsibility</th>
                <th>Harness</th>
                <th>Focus</th>
                <th>Turns / Calls</th>
                <th>Phase</th>
              </tr>
            </thead>
            <tbody id="tbody-agents"></tbody>
          </table>
        </div>
      </div>

      <div class="zone">
        <div class="zone-header">
          <h2>Antigravity Factory Analyst — Actionable Diagnostics</h2>
          <span class="mono" style="font-size:0.75rem;color:var(--text-muted);">By @factory-analyst &amp; @search-evaluator</span>
        </div>
        <div class="diag-list" id="diag-container"></div>
      </div>
    </section>

    <!-- Interactive Hybrid RRF Query Workbench + Golden Benchmark Table -->
    <section class="ledger-frame split-2" aria-label="Interactive Hybrid Search Workbench and Golden Queries">
      <div class="zone">
        <div class="zone-header">
          <h2>Interactive Hybrid RRF Query Workbench</h2>
          <span class="mono" style="font-size:0.75rem;color:var(--text-muted);" id="wb-meta">Live probe against psearch-serving</span>
        </div>
        <form class="workbench-bar" onsubmit="runWorkbenchQuery(event)">
          <div class="field">
            <label for="wb-query"><span>Search Query</span><span class="mono">FTS + Vector</span></label>
            <input id="wb-query" class="input" type="text" value="running shoes" placeholder="e.g. running shoes, tenis corrida, geladeira inox" />
          </div>
          <div class="field">
            <label for="wb-alpha"><span>RRF Weight (&alpha;)</span><span class="mono" id="wb-alpha-val">0.65</span></label>
            <input id="wb-alpha" type="range" min="0" max="1" step="0.05" value="0.65" oninput="document.getElementById('wb-alpha-val').textContent=parseFloat(this.value).toFixed(2)" />
          </div>
          <div class="field">
            <label for="wb-locale"><span>Catalog Locale</span></label>
            <select id="wb-locale">
              <option value="">All Locales</option>
              <option value="en-US">en-US</option>
              <option value="pt-BR">pt-BR</option>
            </select>
          </div>
          <button type="submit" class="btn btn-primary">Probe Query</button>
        </form>
        <div class="zone-body--flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Rank</th>
                <th>Product ID</th>
                <th>Title &amp; Brand</th>
                <th>Locale</th>
                <th>Stock</th>
                <th>RRF Score</th>
              </tr>
            </thead>
            <tbody id="tbody-workbench"></tbody>
          </table>
        </div>
      </div>

      <div class="zone">
        <div class="zone-header">
          <h2>Golden Query Benchmark Suite (Cloud Spanner)</h2>
          <span class="mono" style="font-size:0.75rem;color:var(--text-muted);">6 Queries · NDCG@10 &amp; MRR@10</span>
        </div>
        <div class="zone-body--flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Query</th>
                <th>Locale</th>
                <th>&alpha;</th>
                <th>NDCG@10</th>
                <th>MRR@10</th>
                <th>Latency</th>
                <th>Top Returned IDs</th>
              </tr>
            </thead>
            <tbody id="tbody-golden"></tbody>
          </table>
        </div>
      </div>
    </section>

    <!-- Section 4.1 Functional Requirements Matrix (Issues #1-#10) + GitHub Commits -->
    <section class="ledger-frame split-2" aria-label="Section 4.1 Functional Requirements and GitHub Activity">
      <div class="zone">
        <div class="zone-header">
          <h2>Section 4.1 Requirements Matrix (GitHub Issues #1–#10)</h2>
          <div class="filter-group" role="group" aria-label="Filter requirements by wave">
            <button type="button" class="filter-btn" aria-pressed="true" onclick="setWaveFilter('all', this)">All (10)</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setWaveFilter('wave-1', this)">Wave 1</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setWaveFilter('wave-2', this)">Wave 2</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setWaveFilter('wave-3', this)">Wave 3</button>
            <button type="button" class="filter-btn" aria-pressed="false" onclick="setWaveFilter('wave-4', this)">Wave 4</button>
          </div>
        </div>
        <div class="zone-body--flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Issue</th>
                <th>Requirement (Biggy Search / IS 2.0)</th>
                <th>Wave</th>
                <th>Assigned</th>
                <th>Status</th>
                <th>Live Verification Evidence</th>
              </tr>
            </thead>
            <tbody id="tbody-reqs"></tbody>
          </table>
        </div>
      </div>

      <div class="zone">
        <div class="zone-header">
          <h2>GitHub Repository &amp; Branch Telemetry</h2>
          <a href="https://github.com/duboc/scion-experiment" target="_blank" class="mono" style="font-size:0.75rem;">duboc/scion-experiment &rarr;</a>
        </div>
        <div class="zone-body--flush table-wrap">
          <table>
            <thead>
              <tr>
                <th>Branch</th>
                <th>Commit</th>
                <th>Commit Summary</th>
                <th>Author</th>
              </tr>
            </thead>
            <tbody id="tbody-commits"></tbody>
          </table>
        </div>
      </div>
    </section>
  </main>

  <script>
    let cachedEval = null;
    let cachedAgents = [];
    let currentAgentFilter = 'all';
    let currentWaveFilter = 'all';

    function escapeHTML(str) {
      return String(str ?? '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
    }

    function toggleTheme() {
      const root = document.documentElement;
      const next = root.getAttribute('data-theme') === 'carbon' ? 'daylight' : 'carbon';
      root.setAttribute('data-theme', next);
      document.getElementById('theme-label').textContent = next === 'carbon' ? 'Daylight Mode' : 'Carbon Mode';
      if (cachedEval) renderAlphaCurve(cachedEval.alpha_curve || []);
    }

    function setAgentFilter(filter, btn) {
      currentAgentFilter = filter;
      btn.parentElement.querySelectorAll('.filter-btn').forEach(b => b.setAttribute('aria-pressed', 'false'));
      btn.setAttribute('aria-pressed', 'true');
      renderAgentsTable();
    }

    function setWaveFilter(wave, btn) {
      currentWaveFilter = wave;
      btn.parentElement.querySelectorAll('.filter-btn').forEach(b => b.setAttribute('aria-pressed', 'false'));
      btn.setAttribute('aria-pressed', 'true');
      if (cachedEval) renderRequirementsTable(cachedEval.requirements || []);
    }

    function selectAgentNode(project, name, el) {
      document.querySelectorAll('.topo-node').forEach(n => n.classList.remove('is-active'));
      if (el) el.classList.add('is-active');
      const found = cachedAgents.find(a => a.project === project && a.name === name);
      const box = document.getElementById('node-inspector');
      if (found) {
        box.innerHTML = '<span>Selected Agent: <strong class="mono">@' + escapeHTML(found.name) + ' (' + escapeHTML(found.project) + ')</strong> — ' + escapeHTML(found.role || found.template) + '</span>' +
          '<span class="mono">Harness: ' + escapeHTML(found.harness) + ' · Parent: ' + escapeHTML(found.parent_agent) + ' · Focus: ' + escapeHTML(found.active_issue || 'Standby') + '</span>';
      }
      renderAgentsTable(project, name);
    }

    function renderAlphaCurve(points) {
      const svg = document.getElementById('svg-alpha-curve');
      if (!points || points.length === 0) return;
      const w = 460, h = 118, padL = 34, padR = 16, padT = 14, padB = 24;
      const plotW = w - padL - padR;
      const plotH = h - padT - padB;
      const minY = 0.50, maxY = 1.00;

      const xFor = a => padL + a * plotW;
      const yFor = v => padT + plotH - ((Math.max(minY, Math.min(maxY, v)) - minY) / (maxY - minY)) * plotH;

      let dNDCG = '', dMRR = '', dots = '', labels = '';
      points.forEach((pt, idx) => {
        const x = xFor(pt.alpha);
        const yn = yFor(pt.ndcg);
        const ym = yFor(pt.mrr);
        dNDCG += (idx === 0 ? 'M' : 'L') + x.toFixed(1) + ',' + yn.toFixed(1) + ' ';
        dMRR += (idx === 0 ? 'M' : 'L') + x.toFixed(1) + ',' + ym.toFixed(1) + ' ';
        dots += '<circle cx="' + x.toFixed(1) + '" cy="' + yn.toFixed(1) + '" r="3.5" fill="var(--accent-primary)" />' +
                '<text x="' + x.toFixed(1) + '" y="' + (yn - 7).toFixed(1) + '" text-anchor="middle" fill="var(--text-primary)" font-family="JetBrains Mono, monospace" font-size="10" font-weight="600">' + pt.ndcg.toFixed(2) + '</text>' +
                '<text x="' + x.toFixed(1) + '" y="' + (h - 6) + '" text-anchor="middle" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="10">&alpha;=' + pt.alpha.toFixed(2) + '</text>';
      });

      svg.innerHTML =
        '<line x1="' + padL + '" y1="' + padT + '" x2="' + (w - padR) + '" y2="' + padT + '" stroke="var(--border-hairline)" stroke-dasharray="2 2" />' +
        '<line x1="' + padL + '" y1="' + (padT + plotH / 2) + '" x2="' + (w - padR) + '" y2="' + (padT + plotH / 2) + '" stroke="var(--border-hairline)" stroke-dasharray="2 2" />' +
        '<line x1="' + padL + '" y1="' + (padT + plotH) + '" x2="' + (w - padR) + '" y2="' + (padT + plotH) + '" stroke="var(--border-strong)" />' +
        '<text x="2" y="' + (padT + 4) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9.5">1.00</text>' +
        '<text x="2" y="' + (padT + plotH / 2 + 3) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9.5">0.75</text>' +
        '<text x="2" y="' + (padT + plotH + 3) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9.5">0.50</text>' +
        '<path d="' + dMRR + '" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-dasharray="4 3" />' +
        '<path d="' + dNDCG + '" fill="none" stroke="var(--accent-primary)" stroke-width="2.25" />' +
        dots;
    }

    function renderAgentsTable(selProj, selName) {
      const rows = cachedAgents.filter(a => currentAgentFilter === 'all' || a.project === currentAgentFilter);
      document.getElementById('tbody-agents').innerHTML = rows.map(a => {
        const isSel = a.project === selProj && a.name === selName;
        const badgeClass = a.phase === 'running' ? 'badge--running' : 'badge--pending';
        return '<tr class="' + (isSel ? 'is-selected' : '') + '">' +
          '<td><strong>' + escapeHTML(a.project) + '</strong></td>' +
          '<td class="mono">@' + escapeHTML(a.name) + '</td>' +
          '<td>' + escapeHTML(a.role || a.template) + '</td>' +
          '<td class="mono">' + escapeHTML(a.harness) + '</td>' +
          '<td class="mono">' + escapeHTML(a.active_issue || 'Standby') + '</td>' +
          '<td class="mono num">' + (a.current_turns || 0) + ' / ' + (a.model_calls || 0) + '</td>' +
          '<td><span class="badge ' + badgeClass + '"><span class="badge-dot"></span>' + escapeHTML(a.phase) + ' (' + escapeHTML(a.activity || 'idle') + ')</span></td>' +
        '</tr>';
      }).join('');
    }

    function renderRequirementsTable(reqs) {
      const filtered = reqs.filter(r => currentWaveFilter === 'all' || r.wave === currentWaveFilter);
      document.getElementById('tbody-reqs').innerHTML = filtered.map(r => {
        const bClass = r.status === 'PASS' ? 'badge--pass' : (r.status === 'PARTIAL' ? 'badge--partial' : 'badge--pending');
        return '<tr>' +
          '<td class="mono num"><a href="https://github.com/duboc/scion-experiment/issues/' + r.issue_number + '" target="_blank">#' + r.issue_number + '</a></td>' +
          '<td><strong>' + escapeHTML(r.code) + '</strong> ' + escapeHTML(r.title) + '</td>' +
          '<td class="mono">' + escapeHTML(r.wave) + '</td>' +
          '<td class="mono">' + escapeHTML(r.owner_agent || '@backend') + '</td>' +
          '<td><span class="badge ' + bClass + '"><span class="badge-dot"></span>' + escapeHTML(r.status) + '</span></td>' +
          '<td style="color:var(--text-secondary);">' + escapeHTML(r.evidence) + '</td>' +
        '</tr>';
      }).join('');
    }

    async function runWorkbenchQuery(ev) {
      if (ev) ev.preventDefault();
      const q = document.getElementById('wb-query').value || 'running shoes';
      const alpha = document.getElementById('wb-alpha').value || '0.65';
      const locale = document.getElementById('wb-locale').value || '';
      const res = await fetch('/api/workbench?q=' + encodeURIComponent(q) + '&alpha=' + encodeURIComponent(alpha) + '&locale=' + encodeURIComponent(locale)).then(r => r.json());
      const items = res.results || [];
      document.getElementById('wb-meta').textContent = 'effective_alpha=' + (res.effective_alpha ?? alpha) + ' · ' + (res.probe_latency_ms ? res.probe_latency_ms.toFixed(1) : '--') + ' ms · ' + items.length + ' results';
      document.getElementById('tbody-workbench').innerHTML = items.map((item, idx) => {
        const availBadge = item.is_available
          ? '<span class="badge badge--pass"><span class="badge-dot"></span>IN_STOCK (' + (item.stock ?? 0) + ')</span>'
          : '<span class="badge badge--pending">OUT_OF_STOCK (0)</span>';
        return '<tr>' +
          '<td class="mono num">#' + (idx + 1) + '</td>' +
          '<td class="mono">' + escapeHTML(item.id) + '</td>' +
          '<td><strong>' + escapeHTML(item.title) + '</strong> <span style="color:var(--text-muted);">· ' + escapeHTML(item.brand || '') + '</span></td>' +
          '<td class="mono">' + escapeHTML(item.locale || 'en-US') + '</td>' +
          '<td>' + availBadge + '</td>' +
          '<td class="mono num">' + (item.final_score ? Number(item.final_score).toFixed(4) : '--') + '</td>' +
        '</tr>';
      }).join('');
    }

    async function refreshAll() {
      const [evalRes, agentsRes, backlogRes] = await Promise.all([
        fetch('/api/eval').then(r => r.json()),
        fetch('/api/agents').then(r => r.json()),
        fetch('/api/backlog').then(r => r.json())
      ]);
      cachedEval = evalRes;
      cachedAgents = agentsRes.agents || [];

      document.getElementById('val-ndcg').textContent = (evalRes.overall_ndcg_at_10 || 0).toFixed(3);
      document.getElementById('val-mrr').textContent = (evalRes.overall_mrr_at_10 || 0).toFixed(3);
      document.getElementById('val-opt-alpha').textContent = (evalRes.optimal_alpha || 0.65).toFixed(2);
      document.getElementById('val-timestamp').textContent = (evalRes.timestamp || '').slice(11, 19) + ' UTC';
      document.getElementById('lbl-backend-mode').textContent = evalRes.backend_mode || 'spanner';
      document.getElementById('val-p50').textContent = (evalRes.p50_latency_ms || 0).toFixed(1) + ' ms';
      document.getElementById('val-p95').textContent = (evalRes.p95_latency_ms || 0).toFixed(1) + ' / ' + (evalRes.p99_latency_ms || 0).toFixed(1) + ' ms';
      document.getElementById('val-reqs-pass').textContent = evalRes.pass_count + ' PASS · ' + evalRes.partial_count + ' PARTIAL';
      document.getElementById('link-storefront').href = evalRes.target_url || 'https://psearch-serving-249096448072.us-central1.run.app';

      const runningCnt = cachedAgents.filter(a => a.phase === 'running').length;
      document.getElementById('val-agents-cnt').textContent = runningCnt + ' / ' + cachedAgents.length + ' running';

      renderAlphaCurve(evalRes.alpha_curve || []);
      renderAgentsTable();
      renderRequirementsTable(evalRes.requirements || []);

      document.getElementById('diag-container').innerHTML = (evalRes.diagnostics || []).map(d =>
        '<div class="diag-row">' +
          '<span class="badge badge--' + escapeHTML(d.severity) + '"><span class="badge-dot"></span>' + escapeHTML(d.severity) + '</span>' +
          '<div>' +
            '<div class="diag-title">' + escapeHTML(d.title) + ' <span class="mono" style="font-weight:400;color:var(--text-muted);">[' + escapeHTML(d.target_agent) + ' · ' + escapeHTML(d.target_issue) + ']</span></div>' +
            '<div class="diag-desc">' + escapeHTML(d.analysis) + '</div>' +
            '<div class="diag-rec"><strong>Directive (' + escapeHTML(d.analyst_agent) + '):</strong> ' + escapeHTML(d.recommendation) + '</div>' +
          '</div>' +
        '</div>'
      ).join('');

      document.getElementById('tbody-golden').innerHTML = (evalRes.golden_queries || []).map(g =>
        '<tr>' +
          '<td><strong>' + escapeHTML(g.query) + '</strong></td>' +
          '<td class="mono">' + escapeHTML(g.locale || 'en-US') + '</td>' +
          '<td class="mono num">' + g.alpha.toFixed(2) + '</td>' +
          '<td class="mono num"><strong>' + g.ndcg_at_10.toFixed(3) + '</strong></td>' +
          '<td class="mono num">' + g.mrr_at_10.toFixed(3) + '</td>' +
          '<td class="mono num">' + g.latency_ms.toFixed(1) + ' ms</td>' +
          '<td class="mono">' + escapeHTML((g.returned_ids || []).slice(0, 3).join(', ')) + '</td>' +
        '</tr>'
      ).join('');

      const mainC = (Array.isArray(backlogRes.main_commits) ? backlogRes.main_commits : []).slice(0, 4).map(c => ({ branch: 'main', c }));
      const evalC = (Array.isArray(backlogRes.eval_commits) ? backlogRes.eval_commits : []).slice(0, 4).map(c => ({ branch: 'eval-dashboard', c }));
      const allCommits = mainC.concat(evalC);
      document.getElementById('tbody-commits').innerHTML = allCommits.map(item => {
        const sha = (item.c && item.c.sha) ? item.c.sha.slice(0, 7) : '';
        const msg = (item.c && item.c.commit && item.c.commit.message) ? item.c.commit.message.split('\n')[0] : '';
        const author = (item.c && item.c.commit && item.c.commit.author) ? item.c.commit.author.name : '';
        return '<tr>' +
          '<td class="mono">' + escapeHTML(item.branch) + '</td>' +
          '<td class="mono"><a href="https://github.com/duboc/scion-experiment/commit/' + escapeHTML(item.c.sha || '') + '" target="_blank">' + escapeHTML(sha) + '</a></td>' +
          '<td>' + escapeHTML(msg) + '</td>' +
          '<td style="color:var(--text-muted);">' + escapeHTML(author) + '</td>' +
        '</tr>';
      }).join('');
    }

    refreshAll();
    runWorkbenchQuery();
    setInterval(refreshAll, 20000);
  </script>
</body>
</html>`
