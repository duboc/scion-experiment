// /workspace/eval-dashboard/server.js
// Node.js Express server serving the Impeccable evaluation dashboard

const express = require('express');
const path = require('path');
const http = require('http');

process.env.SPANNER_ENABLE_PROXIED_GCP_METRICS = 'false';

const app = express();
const port = process.env.PORT || 8080;
const projectId = process.env.GCP_PROJECT || 'riojucu-sandbox';
const servingUrl = process.env.PSEARCH_SERVING_URL || 'https://psearch-serving-mhjw244bgq-uc.a.run.app';
const spannerInstance = process.env.SPANNER_INSTANCE || 'psearch-instance';
const spannerDatabase = process.env.SPANNER_DATABASE || 'psearch-db';

let spannerDbClient = null;

function getSpannerDatabase() {
  if (!spannerDbClient) {
    try {
      const { Spanner } = require('@google-cloud/spanner');
      const spanner = new Spanner({
        projectId: projectId,
        clientOptions: { quotaProjectId: projectId }
      });
      spannerDbClient = spanner.instance(spannerInstance).database(spannerDatabase);
    } catch (err) {
      console.error('Failed to initialize Spanner client:', err.message);
    }
  }
  return spannerDbClient;
}

app.use(express.json());
app.use(express.static(path.join(__dirname, 'public')));

// Health check endpoints
app.get('/health', (req, res) => {
  res.json({ status: 'healthy', service: 'psearch-eval-dashboard', runtime: 'node' });
});

app.get('/api/health', (req, res) => {
  res.json({ status: 'healthy', service: 'psearch-eval-dashboard', runtime: 'node' });
});

app.get('/api/info', (req, res) => {
  res.json({
    service: 'psearch-eval-dashboard',
    project: 'psearch-eval',
    harness: 'antigravity (100%)',
    design_system: 'impeccable.style (Operate · Precision Telemetry Ledger)',
    sandbox: projectId,
    psearch_serving_url: servingUrl,
    spanner_instance: spannerInstance,
    spanner_database: spannerDatabase,
    orchestrator: 'eval-lead',
    agents: ['eval-lead', 'factory-analyst', 'search-evaluator', 'ui-craftsman', 'eval-deployer']
  });
});

// Cache for GitHub backlog API (60 seconds)
let backlogCache = null;
let backlogCacheTime = 0;
const BACKLOG_CACHE_TTL_MS = 60 * 1000;

app.get('/api/backlog', async (req, res) => {
  const now = Date.now();
  if (backlogCache && (now - backlogCacheTime < BACKLOG_CACHE_TTL_MS)) {
    return res.json(backlogCache);
  }

  const headers = {
    'User-Agent': 'psearch-eval-dashboard',
    'Accept': 'application/vnd.github.v3+json'
  };
  const token = process.env.GITHUB_TOKEN;
  if (token && token.trim()) {
    headers['Authorization'] = `Bearer ${token.trim()}`;
  }

  try {
    const fetchGithub = async (endpoint) => {
      const resp = await fetch(`https://api.github.com/repos/duboc/scion-experiment${endpoint}`, { headers });
      if (!resp.ok) {
        throw new Error(`GitHub API ${endpoint} returned HTTP ${resp.status}`);
      }
      return await resp.json();
    };

    const [issues, mainCommits, evalCommits, branches, openPrs] = await Promise.all([
      fetchGithub('/issues?state=all&per_page=30'),
      fetchGithub('/commits?sha=main&per_page=10'),
      fetchGithub('/commits?sha=eval-dashboard&per_page=10'),
      fetchGithub('/branches'),
      fetchGithub('/pulls?state=open')
    ]);

    const data = {
      issues: Array.isArray(issues) ? issues : [],
      main_commits: Array.isArray(mainCommits) ? mainCommits : [],
      eval_commits: Array.isArray(evalCommits) ? evalCommits : [],
      branches: Array.isArray(branches) ? branches : [],
      open_prs: Array.isArray(openPrs) ? openPrs : [],
      error: null,
      cached_at: new Date(now).toISOString()
    };

    backlogCache = data;
    backlogCacheTime = now;
    return res.json(data);
  } catch (err) {
    console.error('GitHub backlog fetch error:', err.message);
    return res.json({
      issues: [],
      main_commits: [],
      eval_commits: [],
      branches: [],
      open_prs: [],
      error: err.message,
      cached_at: new Date(now).toISOString()
    });
  }
});

// Live factory agent telemetry synced from Cloud Spanner factory_agent_status
app.get('/api/agents', async (req, res) => {
  try {
    const db = getSpannerDatabase();
    if (!db) {
      throw new Error('Spanner database client unavailable');
    }

    const [rows] = await db.run({
      sql: 'SELECT project_slug, agent_name, agent_id, template, harness, phase, activity, parent_agent, current_turns, model_calls, last_summary, updated_at, last_activity_at FROM factory_agent_status ORDER BY project_slug, agent_name',
      json: true
    });

    const agents = rows.map(r => ({
      project: r.project_slug,
      name: r.agent_name,
      agent_id: r.agent_id,
      template: r.template,
      role: r.template || r.agent_name,
      harness: r.harness,
      phase: r.phase,
      activity: r.activity,
      parent_agent: r.parent_agent,
      current_turns: (r.current_turns === null || r.current_turns === undefined) ? 'n/a' : r.current_turns,
      model_calls: (r.model_calls === null || r.model_calls === undefined) ? 'n/a' : r.model_calls,
      last_summary: r.last_summary,
      active_issue: r.last_summary || 'Standby',
      updated_at: r.updated_at,
      last_activity_at: r.last_activity_at
    }));

    const projectSet = new Set(agents.map(a => a.project));
    const projects = projectSet.size > 0 ? Array.from(projectSet) : ['psearch', 'psearch-eval'];

    res.json({
      projects: projects,
      agents: agents,
      source: 'spanner',
      updated_at: new Date().toISOString()
    });
  } catch (err) {
    console.error('Failed to query factory_agent_status from Spanner:', err.message);
    res.status(500).json({
      error: err.message,
      projects: ['psearch', 'psearch-eval'],
      agents: []
    });
  }
});

// Canonical 10 Biggy Search requirements template with unmeasured baseline
const defaultUnmeasuredRequirements = [
  { issue_number: 1, code: '4.1.1', title: 'Busca Híbrida (Dynamic RRF Alpha)', owner_agent: '@backend · @frontend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 2, code: '4.1.2', title: 'Busca Vetorial (1024-dim, Blue/Green, Cache)', owner_agent: '@backend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 3, code: '4.1.3', title: 'Disponibilidade (Estoque/Canal + Indisponíveis no Final)', owner_agent: '@backend · @frontend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 4, code: '4.1.4', title: 'Catálogo Multilanguage (pt-BR, en-US, es-MX)', owner_agent: '@backend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 5, code: '4.1.5', title: 'Facets Server-Side (Goroutines Concorrentes)', owner_agent: '@backend · @frontend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 6, code: '4.1.6', title: 'Relevance & Merchandising Rules + 8 Sinais de Boost', owner_agent: '@backend · @frontend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 7, code: '4.1.7', title: 'Low Relevance Synonyms (0.3x Score Weight)', owner_agent: '@backend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 8, code: '4.1.8', title: 'Delivery Promises (CEP/SLA via INTERLEAVE IN PARENT)', owner_agent: '@backend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 9, code: '4.1.9', title: 'Autocomplete (BQ Analytics + TOKENIZE_NGRAMS)', owner_agent: '@backend · @frontend', status: 'not measured', latency_ms: null, evidence: null },
  { issue_number: 10, code: '4.1.10', title: 'Learning to Rank (2-Stage In-Memory Re-Ranking)', owner_agent: '@backend · @reviewer', status: 'not measured', latency_ms: null, evidence: null }
];

// Requirements evaluation summary: built ONLY from real probe results in Spanner eval_runs
app.get('/api/eval', async (req, res) => {
  try {
    const db = getSpannerDatabase();
    let runs = [];
    if (db) {
      try {
        const [queryRows] = await db.run({
          sql: 'SELECT run_id, git_commit, overall_score, ndcg_at_10, mrr_at_10, p50_ms, p95_ms, p99_ms, requirements_json, created_at FROM eval_runs ORDER BY created_at DESC LIMIT 1',
          json: true
        });
        runs = queryRows;
      } catch (qErr) {
        console.warn('Could not read from eval_runs table:', qErr.message);
      }
    }

    if (runs && runs.length > 0) {
      const run = runs[0];
      const reqData = run.requirements_json || {};

      const hasRealReqs = Array.isArray(reqData.requirements) && reqData.requirements.length > 0;
      const hasRealAlpha = Array.isArray(reqData.alpha_curve) && reqData.alpha_curve.length > 0;
      const hasRealMetrics = run.ndcg_at_10 !== null && run.mrr_at_10 !== null && run.p50_ms !== null;

      // is_sample=true whenever anything is not from a real measured source
      const isSample = Boolean(!hasRealReqs || !hasRealAlpha || !hasRealMetrics || reqData.is_sample);

      const rawRequirements = hasRealReqs ? reqData.requirements : defaultUnmeasuredRequirements;
      const requirements = rawRequirements.map(r => ({
        ...r,
        id: r.code || r.id,
        name: r.title || r.name
      }));
      const alphaCurve = hasRealAlpha ? reqData.alpha_curve : [];
      const diagnostics = Array.isArray(reqData.diagnostics) ? reqData.diagnostics : [];
      const goldenQueries = Array.isArray(reqData.golden_queries) ? reqData.golden_queries : [];

      const passCount = requirements.filter(r => r.status === 'PASS').length;
      const partialCount = requirements.filter(r => r.status === 'PARTIAL').length;
      const pendingCount = requirements.filter(r => r.status === 'PENDING').length;
      const notMeasuredCount = requirements.filter(r => r.status === 'not measured').length;

      return res.json({
        timestamp: run.created_at ? run.created_at.toISOString() : new Date().toISOString(),
        data_source: isSample ? 'sample' : 'spanner',
        is_sample: isSample,
        run_id: run.run_id,
        git_commit: run.git_commit,
        target_url: servingUrl,
        backend_mode: 'spanner',
        overall_ndcg_at_10: run.ndcg_at_10,
        ndcg_at_10: run.ndcg_at_10,
        overall_mrr_at_10: run.mrr_at_10,
        mrr_at_10: run.mrr_at_10,
        optimal_alpha: reqData.optimal_alpha ?? null,
        p50_latency_ms: run.p50_ms,
        p50_ms: run.p50_ms,
        p95_latency_ms: run.p95_ms,
        p95_ms: run.p95_ms,
        p99_latency_ms: run.p99_ms,
        p99_ms: run.p99_ms,
        pass_count: passCount,
        partial_count: partialCount,
        pending_count: pendingCount,
        not_measured_count: notMeasuredCount,
        alpha_curve: alphaCurve,
        alphaCurve: alphaCurve,
        requirements: requirements,
        diagnostics: diagnostics,
        golden_queries: goldenQueries
      });
    }

    // When eval_runs is completely empty: return unmeasured baseline with null values, never fallback numbers
    return res.json({
      timestamp: new Date().toISOString(),
      data_source: 'sample',
      is_sample: true,
      target_url: servingUrl,
      backend_mode: 'spanner',
      overall_ndcg_at_10: null,
      ndcg_at_10: null,
      overall_mrr_at_10: null,
      mrr_at_10: null,
      optimal_alpha: null,
      p50_latency_ms: null,
      p50_ms: null,
      p95_latency_ms: null,
      p95_ms: null,
      p99_latency_ms: null,
      p99_ms: null,
      pass_count: 0,
      partial_count: 0,
      pending_count: 0,
      not_measured_count: 10,
      alpha_curve: [],
      alphaCurve: [],
      requirements: defaultUnmeasuredRequirements.map(r => ({
        ...r,
        id: r.code || r.id,
        name: r.title || r.name
      })),
      diagnostics: [],
      note: 'Spanner eval_runs table currently has no rows; metrics are unmeasured'
    });
  } catch (err) {
    console.error('Eval endpoint error:', err.message);
    return res.status(500).json({ error: err.message, data_source: 'error', is_sample: true });
  }
});

// Interactive probe workbench against psearch-serving
app.get('/api/workbench', async (req, res) => {
  const q = req.query.q || 'running shoes';
  const alpha = req.query.alpha || '0.65';
  const locale = req.query.locale || '';

  const params = new URLSearchParams({ q, alpha });
  if (locale) params.set('locale', locale);

  const start = Date.now();
  try {
    const target = `${servingUrl}/api/search?${params.toString()}`;
    const resp = await fetch(target);
    const data = await resp.json();
    const elapsed = Date.now() - start;
    const rawResults = Array.isArray(data.results) ? data.results : [];
    const normalizedResults = rawResults.map(r => {
      const isAvailable = r.is_available === true || r.availability === 'IN_STOCK' || (r.availableQuantity !== undefined && r.availableQuantity > 0);
      const stock = r.availableQuantity ?? r.stock ?? 0;
      const brand = r.brand || (Array.isArray(r.brands) ? r.brands.join(', ') : '');
      const finalScore = (r.score && r.score.final_score !== undefined) ? r.score.final_score : r.final_score;
      return {
        ...r,
        brand,
        is_available: isAvailable,
        stock,
        final_score: finalScore
      };
    });
    res.json({
      ...data,
      results: normalizedResults,
      probe_latency_ms: elapsed
    });
  } catch (err) {
    res.status(502).json({
      error: err.message,
      results: [],
      probe_latency_ms: Date.now() - start
    });
  }
});

app.get('*', (req, res) => {
  res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

app.listen(port, () => {
  console.log(`psearch-eval dashboard running on port ${port}`);
});
