package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	_ "embed"
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

//go:embed ui.html
var impeccableWorkbenchHTML string
