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

// Baseline requirements & alpha curve (used as reference and fallback)
const sampleAlphaCurve = [
  { alpha: 0.00, mode: 'Pure Text', ndcg: 0.860, mrr: 1.000, p50_ms: 38.5 },
  { alpha: 0.25, mode: 'Text-Heavy Hybrid', ndcg: 1.000, mrr: 1.000, p50_ms: 29.0 },
  { alpha: 0.50, mode: 'Balanced Hybrid', ndcg: 1.000, mrr: 1.000, p50_ms: 30.4 },
  { alpha: 0.65, mode: 'Optimal Hybrid', ndcg: 1.000, mrr: 1.000, p50_ms: 30.0 },
  { alpha: 0.80, mode: 'Vector-Heavy Hybrid', ndcg: 1.000, mrr: 1.000, p50_ms: 28.3 },
  { alpha: 1.00, mode: 'Pure Vector', ndcg: 1.000, mrr: 1.000, p50_ms: 30.2 }
];

const sampleRequirements = [
  { issue_number: 1, code: '4.1.1', title: 'Busca Híbrida (Dynamic RRF Alpha)', owner_agent: '@backend · @frontend', status: 'PASS', latency_ms: 74.5, evidence: 'dynamic ?alpha supported, effective_alpha verified' },
  { issue_number: 2, code: '4.1.2', title: 'Busca Vetorial (1024-dim, Blue/Green, Cache)', owner_agent: '@backend', status: 'PARTIAL', latency_ms: 30.2, evidence: '768-dim active in Spanner; pending 1024-dim v2 + embedding_cache' },
  { issue_number: 3, code: '4.1.3', title: 'Disponibilidade (Estoque/Canal + Indisponíveis no Final)', owner_agent: '@backend · @frontend', status: 'PASS', latency_ms: 39.9, evidence: 'Out-of-stock items (prod-004, prod-012) correctly demoted' },
  { issue_number: 4, code: '4.1.4', title: 'Catálogo Multilanguage (pt-BR, en-US, es-MX)', owner_agent: '@backend', status: 'PASS', latency_ms: 31.0, evidence: 'Verified locale=pt-BR and en-US catalog partitions' },
  { issue_number: 5, code: '4.1.5', title: 'Facets Server-Side (Goroutines Concorrentes)', owner_agent: '@backend · @frontend', status: 'PENDING', latency_ms: 0, evidence: 'Facets object absent in /api/search response' },
  { issue_number: 6, code: '4.1.6', title: 'Relevance & Merchandising Rules + 8 Sinais de Boost', owner_agent: '@backend · @frontend', status: 'PENDING', latency_ms: 0, evidence: 'GET /api/rules returns 404' },
  { issue_number: 7, code: '4.1.7', title: 'Low Relevance Synonyms (0.3x Score Weight)', owner_agent: '@backend', status: 'PENDING', latency_ms: 0, evidence: 'GET /api/synonyms returns 404' },
  { issue_number: 8, code: '4.1.8', title: 'Delivery Promises (CEP/SLA via INTERLEAVE IN PARENT)', owner_agent: '@backend', status: 'PENDING', latency_ms: 0, evidence: 'No CEP/SLA delivery estimates in payload' },
  { issue_number: 9, code: '4.1.9', title: 'Autocomplete (BQ Analytics + TOKENIZE_NGRAMS)', owner_agent: '@backend · @frontend', status: 'PENDING', latency_ms: 0, evidence: 'GET /api/autocomplete returns 404' },
  { issue_number: 10, code: '4.1.10', title: 'Learning to Rank (2-Stage In-Memory Re-Ranking)', owner_agent: '@backend · @reviewer', status: 'PARTIAL', latency_ms: 44.6, evidence: '?ltr=true accepted on endpoint' }
];

const sampleDiagnostics = [
  {
    id: 'diag-wave1',
    severity: 'HEALTHY',
    target_agent: 'psearch/@lead',
    target_issue: '#1 + #3 + #4',
    title: 'Wave 1 Hybrid RRF, Availability Demotion & Locale Partitions Verified',
    analysis: 'Ground truth Golden Query suite achieved 1.000 NDCG@10 and 1.000 MRR@10 with p95 of 74.5ms against Cloud Spanner.',
    recommendation: 'Close Wave 1 Issue #1 and proceed to Wave 2 (#5 Facets Server-Side).'
  },
  {
    id: 'diag-wave2',
    severity: 'ACTION',
    target_agent: 'psearch/@backend',
    target_issue: '#5 Facets Server-Side',
    title: 'Implement Server-Side Facet Aggregation in /api/search',
    analysis: 'Search API currently lacks facet counts for category and brand filtering.',
    recommendation: 'Have @backend add concurrent Spanner GROUP BY goroutines in internal/api/routes.go.'
  },
  {
    id: 'diag-bottleneck-iam',
    severity: 'RESOLVED',
    target_agent: 'psearch/@deployer',
    target_issue: '#11 Gemini 1024-dim',
    title: 'IAM 403 on Vertex AI predict for Cloud Run Compute SA',
    analysis: 'Encountered 403 Forbidden on Vertex AI predict call during 1024-dim rollout, resulting in v1 rollback and 6e4c0fb unmasking fix.',
    recommendation: 'Ensure roles/aiplatform.user is granted to Cloud Run default compute service account.'
  },
  {
    id: 'diag-bottleneck-gating',
    severity: 'INSIGHT',
    target_agent: 'psearch/@lead',
    target_issue: 'Factory Throughput',
    title: 'Single Commit Slot Gating on main Branch',
    analysis: 'Serial commit slot model causes idle latency for frontend and reviewer while awaiting backend PR merges.',
    recommendation: 'Enable parallel feature worktrees for non-conflicting components (routes.go vs application/ui).'
  }
];

// Requirements evaluation summary from Spanner eval_runs (or explicitly labeled sample)
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
      return res.json({
        timestamp: run.created_at ? run.created_at.toISOString() : new Date().toISOString(),
        data_source: 'spanner',
        is_sample: false,
        run_id: run.run_id,
        git_commit: run.git_commit,
        target_url: servingUrl,
        backend_mode: 'spanner',
        overall_ndcg_at_10: run.ndcg_at_10,
        overall_mrr_at_10: run.mrr_at_10,
        optimal_alpha: reqData.optimal_alpha ?? 0.65,
        p50_latency_ms: run.p50_ms,
        p95_latency_ms: run.p95_ms,
        p99_latency_ms: run.p99_ms,
        pass_count: reqData.pass_count ?? 3,
        partial_count: reqData.partial_count ?? 2,
        pending_count: reqData.pending_count ?? 5,
        alpha_curve: reqData.alpha_curve || sampleAlphaCurve,
        requirements: reqData.requirements || sampleRequirements,
        diagnostics: reqData.diagnostics || sampleDiagnostics,
        golden_queries: reqData.golden_queries || []
      });
    }

    // eval_runs is empty: explicitly label as 'sample'
    return res.json({
      timestamp: new Date().toISOString(),
      data_source: 'sample',
      is_sample: true,
      target_url: servingUrl,
      backend_mode: 'spanner',
      overall_ndcg_at_10: 1.000,
      overall_mrr_at_10: 1.000,
      optimal_alpha: 0.65,
      p50_latency_ms: 44.6,
      p95_latency_ms: 74.5,
      p99_latency_ms: 74.5,
      pass_count: 3,
      partial_count: 2,
      pending_count: 5,
      alpha_curve: sampleAlphaCurve,
      requirements: sampleRequirements,
      diagnostics: sampleDiagnostics,
      sample_note: 'Spanner eval_runs table currently has no rows; metrics explicitly labeled as sample'
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
    res.json({
      ...data,
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
