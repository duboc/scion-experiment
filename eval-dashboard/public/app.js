// /workspace/eval-dashboard/public/app.js
// Reactive Client Logic for psearch-eval Dashboard

const STATUS_CLASSES = {
  idle: 'status-tag status-idle',
  active: 'status-tag status-active',
  blocked: 'status-tag status-gap',
  task_completed: 'status-tag status-pass'
};

async function fetchDashboardState() {
  try {
    const [infoRes, evalRes, agentsRes, backlogRes] = await Promise.all([
      fetch('/api/info').then(r => r.ok ? r.json() : {}).catch(() => ({})),
      fetch('/api/eval').then(r => r.ok ? r.json() : {}).catch(() => ({})),
      fetch('/api/agents').then(r => r.ok ? r.json() : { agents: [] }).catch(() => ({ agents: [] })),
      fetch('/api/backlog').then(r => r.ok ? r.json() : {}).catch(() => ({}))
    ]);

    renderHeaderInfo(infoRes);
    renderMetrics(evalRes);
    renderAgents(agentsRes.agents || []);
    renderRequirements(evalRes.requirements || []);
    renderDiagnostics(evalRes.diagnostics || []);
  } catch (err) {
    console.error('Failed to sync dashboard state:', err);
  }
}

function renderHeaderInfo(info) {
  const servingEl = document.getElementById('meta-serving-url');
  if (servingEl && info.psearch_serving_url) {
    servingEl.textContent = info.psearch_serving_url;
  }
}

function renderMetrics(report) {
  if (!report) return;
  const ndcgEl = document.getElementById('metric-ndcg');
  const mrrEl = document.getElementById('metric-mrr');
  const latEl = document.getElementById('metric-p95');
  const alphaEl = document.getElementById('metric-alpha');

  if (ndcgEl && report.overall_ndcg_at_10 !== undefined) {
    ndcgEl.textContent = Number(report.overall_ndcg_at_10).toFixed(3);
  }
  if (mrrEl && report.overall_mrr_at_10 !== undefined) {
    mrrEl.textContent = Number(report.overall_mrr_at_10).toFixed(3);
  }
  if (latEl && report.p95_latency_ms !== undefined) {
    latEl.textContent = `${Number(report.p95_latency_ms).toFixed(1)} ms`;
  }
  if (alphaEl && report.optimal_alpha !== undefined) {
    alphaEl.textContent = Number(report.optimal_alpha).toFixed(2);
  }
}

function renderAgents(agents) {
  const container = document.getElementById('agent-roster-body');
  if (!container || !agents.length) return;

  container.innerHTML = agents.map(a => `
    <tr class="agent-row">
      <td class="font-mono text-xs">${a.project || 'psearch'}</td>
      <td class="font-mono font-bold text-xs">@${a.name}</td>
      <td class="text-xs">${a.role}</td>
      <td><span class="${STATUS_CLASSES[a.activity] || 'status-tag'}">${a.activity}</span></td>
      <td class="font-mono text-xs tabular-nums">${a.current_turns || 0}</td>
      <td class="font-mono text-xs tabular-nums">${a.model_calls || 0}</td>
      <td class="text-xs text-muted truncate">${a.active_issue || 'Standby'}</td>
    </tr>
  `).join('');
}

function renderRequirements(reqs) {
  const container = document.getElementById('requirements-body');
  if (!container || !reqs.length) return;

  container.innerHTML = reqs.map(r => {
    let tagClass = 'status-tag status-warn';
    if (r.status === 'PASS') tagClass = 'status-tag status-pass';
    if (r.status === 'FAIL') tagClass = 'status-tag status-gap';

    return `
      <tr>
        <td class="font-mono text-xs font-bold">#${r.issue_number}</td>
        <td class="font-mono text-xs">${r.code}</td>
        <td class="text-xs font-medium">${r.title}</td>
        <td class="font-mono text-xs text-muted">${r.owner_agent}</td>
        <td><span class="${tagClass}">${r.status}</span></td>
        <td class="font-mono text-xs tabular-nums">${r.latency_ms ? `${r.latency_ms} ms` : '—'}</td>
        <td class="text-xs text-muted">${r.evidence || ''}</td>
      </tr>
    `;
  }).join('');
}

function renderDiagnostics(diags) {
  const container = document.getElementById('diagnostics-container');
  if (!container || !diags.length) return;

  container.innerHTML = diags.map(d => `
    <div class="diagnostic-card">
      <div class="diagnostic-header">
        <span class="diagnostic-severity ${d.severity.toLowerCase()}">${d.severity}</span>
        <span class="diagnostic-target">${d.target_agent} &middot; ${d.target_issue}</span>
      </div>
      <h4 class="diagnostic-title">${d.title}</h4>
      <p class="diagnostic-analysis">${d.analysis}</p>
      <div class="diagnostic-recommendation">
        <strong>Action:</strong> ${d.recommendation}
      </div>
    </div>
  `).join('');
}

// Auto-sync every 8 seconds
fetchDashboardState();
setInterval(fetchDashboardState, 8000);
