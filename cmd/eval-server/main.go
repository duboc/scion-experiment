package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "psearch-eval-dashboard"})
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "psearch-eval-dashboard"})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service":             "psearch-eval-dashboard",
			"project":             "psearch-eval",
			"repo":                ghRepo,
			"branch":              "eval-dashboard",
			"sandbox":             "riojucu-sandbox",
			"psearch_serving_url": engine.TargetURL,
			"spanner_instance":    "psearch-instance",
			"spanner_database":    "psearch-db",
			"bigquery_dataset":    "psearch_dataset",
			"orchestrator":        "lead",
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

	mux.HandleFunc("/api/backlog", func(w http.ResponseWriter, r *http.Request) {
		issues := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/issues?state=all&per_page=30&sort=created&direction=asc", ghRepo), ghToken)
		mainCommits := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/commits?sha=main&per_page=8", ghRepo), ghToken)
		evalCommits := fetchGitHubJSON(engine.Client, fmt.Sprintf("https://api.github.com/repos/%s/commits?sha=eval-dashboard&per_page=8", ghRepo), ghToken)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"repo":          ghRepo,
			"issues":        issues,
			"main_commits":  mainCommits,
			"eval_commits":  evalCommits,
			"checked_at":    time.Now().UTC().Format(time.RFC3339),
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
		_, _ = w.Write([]byte(controlTowerHTML))
	})

	log.Printf("psearch-eval Control Tower starting on :%s (probing %s)", port, targetURL)
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

const controlTowerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>psearch Control Tower — Eval, Multi-Agent Factory & Backlog Monitor</title>
  <style>
    :root { --bg: #0b1120; --panel: #1e293b; --border: #334155; --text: #f8fafc; --muted: #94a3b8; --blue: #38bdf8; --green: #22c55e; --amber: #f59e0b; --purple: #a855f7; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: var(--bg); color: var(--text); }
    header { padding: 18px 28px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; background: #0f172a; }
    .pill { padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; background: #1e3a8a; color: #93c5fd; }
    .container { max-width: 1400px; margin: 0 auto; padding: 22px 28px; display: flex; flex-direction: column; gap: 22px; }
    .kpi-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
    .kpi { background: var(--panel); border: 1px solid var(--border); border-radius: 12px; padding: 16px; }
    .kpi .label { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; }
    .kpi .value { font-size: 28px; font-weight: 800; margin-top: 6px; color: var(--blue); }
    .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
    @media (max-width: 960px) { .grid-2 { grid-template-columns: 1fr; } }
    .panel { background: var(--panel); border: 1px solid var(--border); border-radius: 12px; padding: 18px; }
    .panel h2 { margin: 0 0 14px 0; font-size: 16px; display: flex; justify-content: space-between; align-items: center; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { text-align: left; padding: 9px 10px; border-bottom: 1px solid #263349; }
    th { color: var(--muted); font-weight: 600; font-size: 11px; text-transform: uppercase; }
    .status-PASS, .status-closed, .status-running { color: var(--green); font-weight: 700; }
    .status-PARTIAL, .status-open { color: var(--amber); font-weight: 700; }
    .status-PENDING { color: var(--muted); font-weight: 600; }
    button { background: var(--blue); color: #0b1120; border: none; padding: 8px 14px; border-radius: 8px; font-weight: 700; cursor: pointer; font-size: 13px; }
    a { color: var(--blue); text-decoration: none; }
    a:hover { text-decoration: underline; }
    .mono { font-family: monospace; font-size: 12px; }
  </style>
</head>
<body>
  <header>
    <div>
      <h1 style="margin:0;font-size:20px;">psearch Control Tower — Multi-Agent Factory, Backlog & Live Search Eval</h1>
      <div style="color:var(--muted);font-size:13px;">Tracking Scion Projects <strong>psearch</strong> (main) & <strong>psearch-eval</strong> (eval-dashboard) · Sandbox: <strong>riojucu-sandbox</strong></div>
    </div>
    <div style="display:flex;gap:10px;align-items:center;">
      <span class="pill">Repo: duboc/scion-experiment</span>
      <span class="pill">Spanner: psearch-instance/psearch-db (100 PU)</span>
      <button onclick="refreshAll()">Run Live Eval Now</button>
    </div>
  </header>
  <div class="container">
    <div class="kpi-row">
      <div class="kpi"><div class="label">Search Quality (NDCG@10)</div><div class="value" id="kpi-ndcg">--</div></div>
      <div class="kpi"><div class="label">Mean Reciprocal Rank (MRR@10)</div><div class="value" id="kpi-mrr">--</div></div>
      <div class="kpi"><div class="label">Search Latency (p50 / p95)</div><div class="value" id="kpi-lat">--</div></div>
      <div class="kpi"><div class="label">Requirements Readiness (10 Items)</div><div class="value" id="kpi-reqs">--</div></div>
      <div class="kpi"><div class="label">Active Scion Factory Agents</div><div class="value" id="kpi-agents">9</div></div>
    </div>

    <div class="grid-2">
      <div class="panel">
        <h2><span>1. Functional Requirements Eval (Section 4.1 · Issues #1–#10)</span><span class="mono" id="eval-ts"></span></h2>
        <table>
          <thead><tr><th>Issue</th><th>Requirement</th><th>Wave</th><th>Eval Status</th><th>Evidence</th></tr></thead>
          <tbody id="reqs-table"></tbody>
        </table>
      </div>
      <div class="panel">
        <h2><span>2. Scion Software Factory — Live Multi-Agent Roster</span><span class="pill">psearch + psearch-eval</span></h2>
        <table>
          <thead><tr><th>Project</th><th>Agent</th><th>Parent</th><th>Harness</th><th>Phase</th><th>Activity</th></tr></thead>
          <tbody id="agents-table"></tbody>
        </table>
      </div>
    </div>

    <div class="grid-2">
      <div class="panel">
        <h2><span>3. Golden Query Benchmark (Spanner Hybrid RRF)</span><span id="serving-link"></span></h2>
        <table>
          <thead><tr><th>Query</th><th>Alpha</th><th>NDCG@10</th><th>MRR@10</th><th>Latency</th><th>Top Returned IDs</th></tr></thead>
          <tbody id="golden-table"></tbody>
        </table>
      </div>
      <div class="panel">
        <h2><span>4. GitHub Backlog & Recent Commits (duboc/scion-experiment)</span><a href="https://github.com/duboc/scion-experiment/issues" target="_blank">Open GitHub ↗</a></h2>
        <table>
          <thead><tr><th>#</th><th>GitHub Issue Title</th><th>State</th><th>Recent Commits (main)</th></tr></thead>
          <tbody id="backlog-table"></tbody>
        </table>
      </div>
    </div>
  </div>

  <script>
    async function refreshAll() {
      const [evalRes, agentsRes, backlogRes] = await Promise.all([
        fetch('/api/eval').then(r => r.json()),
        fetch('/api/agents').then(r => r.json()),
        fetch('/api/backlog').then(r => r.json())
      ]);

      document.getElementById('kpi-ndcg').textContent = (evalRes.overall_ndcg_at_10 || 0).toFixed(3);
      document.getElementById('kpi-mrr').textContent = (evalRes.overall_mrr_at_10 || 0).toFixed(3);
      document.getElementById('kpi-lat').textContent = (evalRes.p50_latency_ms || 0) + ' / ' + (evalRes.p95_latency_ms || 0) + ' ms';
      document.getElementById('kpi-reqs').textContent = evalRes.pass_count + ' PASS · ' + evalRes.partial_count + ' PARTIAL';
      document.getElementById('eval-ts').textContent = evalRes.backend_mode + ' · ' + (evalRes.timestamp || '');
      document.getElementById('serving-link').innerHTML = '<a href="' + evalRes.target_url + '" target="_blank">Open psearch-serving ↗</a>';

      document.getElementById('reqs-table').innerHTML = (evalRes.requirements || []).map(r =>
        '<tr><td class="mono"><a href="https://github.com/duboc/scion-experiment/issues/' + r.issue_number + '" target="_blank">#' + r.issue_number + '</a></td>' +
        '<td><strong>' + r.code + '</strong> ' + r.title + '</td>' +
        '<td class="mono">' + r.wave + '</td>' +
        '<td class="status-' + r.status + '">' + r.status + '</td>' +
        '<td style="color:#94a3b8;font-size:12px;">' + r.evidence + '</td></tr>'
      ).join('');

      const agents = agentsRes.agents || [];
      document.getElementById('kpi-agents').textContent = agents.length + ' (' + agents.filter(a => a.phase === 'running').length + ' running)';
      document.getElementById('agents-table').innerHTML = agents.map(a =>
        '<tr><td><strong>' + a.project + '</strong></td>' +
        '<td class="mono">@' + a.name + '</td>' +
        '<td class="mono">' + (a.parent_agent || 'root') + '</td>' +
        '<td>' + a.harness + '</td>' +
        '<td class="status-' + a.phase + '">' + a.phase + '</td>' +
        '<td class="mono">' + (a.activity || 'idle') + '</td></tr>'
      ).join('');

      document.getElementById('golden-table').innerHTML = (evalRes.golden_queries || []).map(g =>
        '<tr><td><strong>' + g.query + '</strong></td>' +
        '<td class="mono">' + g.alpha + '</td>' +
        '<td class="status-PASS">' + g.ndcg_at_10.toFixed(3) + '</td>' +
        '<td class="mono">' + g.mrr_at_10.toFixed(3) + '</td>' +
        '<td class="mono">' + g.latency_ms + ' ms</td>' +
        '<td class="mono">' + (g.returned_ids || []).slice(0, 3).join(', ') + '</td></tr>'
      ).join('');

      const issues = Array.isArray(backlogRes.issues) ? backlogRes.issues.filter(i => !i.pull_request) : [];
      const commits = Array.isArray(backlogRes.main_commits) ? backlogRes.main_commits : [];
      document.getElementById('backlog-table').innerHTML = issues.slice(0, 10).map((iss, idx) => {
        const c = commits[idx];
        const cText = c ? ('<span class="mono">' + c.sha.slice(0, 7) + '</span> ' + (c.commit && c.commit.message ? c.commit.message.split('\n')[0].slice(0, 42) : '')) : '';
        return '<tr><td class="mono"><a href="' + iss.html_url + '" target="_blank">#' + iss.number + '</a></td>' +
          '<td>' + iss.title.slice(0, 52) + '</td>' +
          '<td class="status-' + iss.state + '">' + iss.state.toUpperCase() + '</td>' +
          '<td style="color:#94a3b8;font-size:12px;">' + cText + '</td></tr>';
      }).join('');
    }
    refreshAll();
    setInterval(refreshAll, 15000);
  </script>
</body>
</html>`
