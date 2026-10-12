// /workspace/eval-dashboard/public/app.js
// Impeccable Reactive Telemetry & Evaluation Logic for psearch-eval Workbench
// Mode: Operate · Visual World: Precision Telemetry Ledger (Kinpaku & Warm Stone / Carbon Ink)

(function (root) {
  'use strict';

  let cachedEval = null;
  let cachedAgents = [];
  let currentAgentFilter = 'all';
  let currentWaveFilter = 'all';
  let selectedProject = 'psearch';
  let selectedAgentName = 'lead';

  function escapeHTML(str) {
    return String(str ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function toggleTheme() {
    const docEl = document.documentElement;
    const current = docEl.getAttribute('data-theme') || 'daylight';
    const next = current === 'carbon' ? 'daylight' : 'carbon';
    docEl.setAttribute('data-theme', next);

    const labelEl = document.getElementById('theme-label');
    if (labelEl) {
      labelEl.textContent = next === 'carbon' ? 'Daylight Mode' : 'Carbon Mode';
    }

    try {
      localStorage.setItem('psearch-theme', next);
    } catch (_) {
      // Ignore storage restrictions
    }

    if (cachedEval && Array.isArray(cachedEval.alpha_curve)) {
      renderAlphaCurve(cachedEval.alpha_curve);
    }
  }

  function setAgentFilter(filter, btn) {
    currentAgentFilter = filter;
    if (btn && btn.parentElement) {
      btn.parentElement.querySelectorAll('.filter-btn').forEach(b => b.setAttribute('aria-pressed', 'false'));
      btn.setAttribute('aria-pressed', 'true');
    }
    renderAgentsTable(selectedProject, selectedAgentName);
  }

  function setWaveFilter(wave, btn) {
    currentWaveFilter = wave;
    if (btn && btn.parentElement) {
      btn.parentElement.querySelectorAll('.filter-btn').forEach(b => b.setAttribute('aria-pressed', 'false'));
      btn.setAttribute('aria-pressed', 'true');
    }
    if (cachedEval && Array.isArray(cachedEval.requirements)) {
      renderRequirementsTable(cachedEval.requirements);
    }
  }

  function selectAgentNode(project, name, el) {
    selectedProject = project;
    selectedAgentName = name;

    document.querySelectorAll('.topo-node').forEach(n => n.classList.remove('is-active'));
    if (el) el.classList.add('is-active');

    const found = cachedAgents.find(a => a.project === project && a.name === name);
    const box = document.getElementById('node-inspector');
    if (box) {
      if (found) {
        box.innerHTML =
          '<span>Selected Agent: <strong class="mono">@' + escapeHTML(found.name) + ' (' + escapeHTML(found.project) + ')</strong> — ' + escapeHTML(found.role || found.template) + '</span>' +
          '<span class="mono">Harness: ' + escapeHTML(found.harness || 'antigravity') + ' · Parent: ' + escapeHTML(found.parent_agent || 'orchestrator') + ' · Focus: ' + escapeHTML(found.active_issue || 'Standby') + '</span>';
      } else {
        box.innerHTML =
          '<span>Selected Agent: <strong class="mono">@' + escapeHTML(name) + ' (' + escapeHTML(project) + ')</strong></span>' +
          '<span class="mono">Telemetry active</span>';
      }
    }

    renderAgentsTable(project, name);
  }

  function renderAlphaCurve(points) {
    const svg = document.getElementById('svg-alpha-curve');
    if (!svg || !Array.isArray(points) || points.length === 0) return;

    const w = 460;
    const h = 118;
    const padL = 34;
    const padR = 16;
    const padT = 14;
    const padB = 24;
    const plotW = w - padL - padR;
    const plotH = h - padT - padB;
    const minY = 0.50;
    const maxY = 1.00;

    const xFor = a => padL + a * plotW;
    const yFor = v => padT + plotH - ((Math.max(minY, Math.min(maxY, v)) - minY) / (maxY - minY)) * plotH;

    let dNDCG = '';
    let dMRR = '';
    let dots = '';

    points.forEach((pt, idx) => {
      const x = xFor(pt.alpha);
      const yn = yFor(pt.ndcg);
      const ym = yFor(pt.mrr);
      dNDCG += (idx === 0 ? 'M' : 'L') + x.toFixed(1) + ',' + yn.toFixed(1) + ' ';
      dMRR += (idx === 0 ? 'M' : 'L') + x.toFixed(1) + ',' + ym.toFixed(1) + ' ';
      dots +=
        '<circle cx="' + x.toFixed(1) + '" cy="' + yn.toFixed(1) + '" r="3.5" fill="var(--accent-primary)" />' +
        '<text x="' + x.toFixed(1) + '" y="' + (yn - 7).toFixed(1) + '" text-anchor="middle" fill="var(--text-primary)" font-family="JetBrains Mono, monospace" font-size="12" font-weight="600">' + Number(pt.ndcg).toFixed(2) + '</text>' +
        '<text x="' + x.toFixed(1) + '" y="' + (h - 6) + '" text-anchor="middle" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="12">&alpha;=' + Number(pt.alpha).toFixed(2) + '</text>';
    });

    svg.innerHTML =
      '<line x1="' + padL + '" y1="' + padT + '" x2="' + (w - padR) + '" y2="' + padT + '" stroke="var(--border-hairline)" stroke-dasharray="2 2" />' +
      '<line x1="' + padL + '" y1="' + (padT + plotH / 2) + '" x2="' + (w - padR) + '" y2="' + (padT + plotH / 2) + '" stroke="var(--border-hairline)" stroke-dasharray="2 2" />' +
      '<line x1="' + padL + '" y1="' + (padT + plotH) + '" x2="' + (w - padR) + '" y2="' + (padT + plotH) + '" stroke="var(--border-strong)" />' +
      '<text x="2" y="' + (padT + 4) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="12">1.00</text>' +
      '<text x="2" y="' + (padT + plotH / 2 + 3) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="12">0.75</text>' +
      '<text x="2" y="' + (padT + plotH + 3) + '" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="12">0.50</text>' +
      '<path d="' + dMRR + '" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-dasharray="4 3" />' +
      '<path d="' + dNDCG + '" fill="none" stroke="var(--accent-primary)" stroke-width="2.25" />' +
      dots;
  }

  function renderAgentsTable(selProj, selName) {
    const tbody = document.getElementById('tbody-agents');
    if (!tbody) return;

    const proj = selProj || selectedProject;
    const name = selName || selectedAgentName;

    const rows = cachedAgents.filter(a => currentAgentFilter === 'all' || a.project === currentAgentFilter);
    tbody.innerHTML = rows.map(a => {
      const isSel = a.project === proj && a.name === name;
      const isRunning = (a.phase === 'running' || a.activity === 'active' || a.activity === 'working' || a.activity === 'executing' || a.activity === 'thinking');
      const badgeClass = isRunning ? 'badge--running' : 'badge--pending';
      const statusText = a.phase ? (a.phase + ' (' + (a.activity || 'idle') + ')') : (a.activity || 'idle');
      const turnsDisplay = (a.current_turns !== null && a.current_turns !== undefined && a.current_turns !== 'n/a') ? a.current_turns : 'n/a';
      const callsDisplay = (a.model_calls !== null && a.model_calls !== undefined && a.model_calls !== 'n/a') ? a.model_calls : 'n/a';
      const turnsCallsText = (turnsDisplay === 'n/a' && callsDisplay === 'n/a') ? 'n/a' : (turnsDisplay + ' / ' + callsDisplay);

      return '<tr class="' + (isSel ? 'is-selected' : '') + '">' +
        '<td><strong>' + escapeHTML(a.project || 'psearch') + '</strong></td>' +
        '<td class="mono">@' + escapeHTML(a.name) + '</td>' +
        '<td>' + escapeHTML(a.role || a.template || 'Agent') + '</td>' +
        '<td class="mono">' + escapeHTML(a.harness || 'antigravity') + '</td>' +
        '<td class="mono">' + escapeHTML(a.active_issue || a.last_summary || 'Standby') + '</td>' +
        '<td class="mono num">' + escapeHTML(turnsCallsText) + '</td>' +
        '<td><span class="badge ' + badgeClass + '"><span class="badge-dot"></span>' + escapeHTML(statusText) + '</span></td>' +
      '</tr>';
    }).join('');
  }

  function renderRequirementsTable(reqs) {
    const tbody = document.getElementById('tbody-reqs');
    if (!tbody || !Array.isArray(reqs)) return;

    const filtered = reqs.filter(r => currentWaveFilter === 'all' || r.wave === currentWaveFilter);
    tbody.innerHTML = filtered.map(r => {
      const bClass = r.status === 'PASS' ? 'badge--pass' : (r.status === 'PARTIAL' ? 'badge--partial' : 'badge--pending');
      const waveLabel = r.wave || (r.issue_number <= 3 ? 'wave-1' : (r.issue_number <= 6 ? 'wave-2' : (r.issue_number <= 8 ? 'wave-3' : 'wave-4')));
      return '<tr>' +
        '<td class="mono num"><a href="https://github.com/duboc/scion-experiment/issues/' + r.issue_number + '" target="_blank" rel="noopener noreferrer">#' + r.issue_number + '</a></td>' +
        '<td><strong>' + escapeHTML(r.code) + '</strong> ' + escapeHTML(r.title) + '</td>' +
        '<td class="mono">' + escapeHTML(waveLabel) + '</td>' +
        '<td class="mono">' + escapeHTML(r.owner_agent || '@backend') + '</td>' +
        '<td><span class="badge ' + bClass + '"><span class="badge-dot"></span>' + escapeHTML(r.status) + '</span></td>' +
        '<td style="color:var(--text-secondary);">' + escapeHTML(r.evidence || '—') + '</td>' +
      '</tr>';
    }).join('');
  }

  function renderDiagnostics(diags) {
    const container = document.getElementById('diag-container');
    if (!container || !Array.isArray(diags)) return;

    container.innerHTML = diags.map(d => {
      const sev = (d.severity || 'HEALTHY').toUpperCase();
      return '<div class="diag-row">' +
        '<span class="badge badge--' + escapeHTML(sev) + '"><span class="badge-dot"></span>' + escapeHTML(sev) + '</span>' +
        '<div>' +
          '<div class="diag-title">' + escapeHTML(d.title) + ' <span class="mono" style="font-weight:400;color:var(--text-muted);">[' + escapeHTML(d.target_agent || '') + ' · ' + escapeHTML(d.target_issue || '') + ']</span></div>' +
          '<div class="diag-desc">' + escapeHTML(d.analysis || '') + '</div>' +
          '<div class="diag-rec"><strong>Directive (' + escapeHTML(d.analyst_agent || '@factory-analyst') + '):</strong> ' + escapeHTML(d.recommendation || '') + '</div>' +
        '</div>' +
      '</div>';
    }).join('');
  }

  function renderGoldenQueries(queries) {
    const tbody = document.getElementById('tbody-golden');
    if (!tbody || !Array.isArray(queries)) return;

    tbody.innerHTML = queries.map(g =>
      '<tr>' +
        '<td><strong>' + escapeHTML(g.query) + '</strong></td>' +
        '<td class="mono">' + escapeHTML(g.locale || 'en-US') + '</td>' +
        '<td class="mono num">' + (g.alpha !== undefined ? Number(g.alpha).toFixed(2) : '0.65') + '</td>' +
        '<td class="mono num"><strong>' + (g.ndcg_at_10 !== undefined ? Number(g.ndcg_at_10).toFixed(3) : '--') + '</strong></td>' +
        '<td class="mono num">' + (g.mrr_at_10 !== undefined ? Number(g.mrr_at_10).toFixed(3) : '--') + '</td>' +
        '<td class="mono num">' + (g.latency_ms !== undefined ? Number(g.latency_ms).toFixed(1) + ' ms' : '--') + '</td>' +
        '<td class="mono">' + escapeHTML((g.returned_ids || []).slice(0, 3).join(', ')) + '</td>' +
      '</tr>'
    ).join('');
  }

  function renderCommits(backlog) {
    const tbody = document.getElementById('tbody-commits');
    const errEl = document.getElementById('backlog-error');
    if (!tbody || !backlog) return;

    if (errEl) {
      if (backlog.error) {
        errEl.textContent = 'GitHub Notice: ' + backlog.error;
        errEl.style.display = 'inline-flex';
      } else {
        errEl.style.display = 'none';
      }
    }

    const mainC = (Array.isArray(backlog.main_commits) ? backlog.main_commits : []).slice(0, 5).map(c => ({ branch: 'main', c }));
    const evalC = (Array.isArray(backlog.eval_commits) ? backlog.eval_commits : []).slice(0, 5).map(c => ({ branch: 'eval-dashboard', c }));
    const allCommits = mainC.concat(evalC);

    if (allCommits.length === 0) {
      if (backlog.error) {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:1rem;">GitHub telemetry unavailable: ' + escapeHTML(backlog.error) + '</td></tr>';
      } else {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:1rem;">No commits recorded.</td></tr>';
      }
      return;
    }

    tbody.innerHTML = allCommits.map(item => {
      const sha = (item.c && item.c.sha) ? item.c.sha.slice(0, 7) : '';
      const msg = (item.c && item.c.commit && item.c.commit.message) ? item.c.commit.message.split('\n')[0] : '';
      const author = (item.c && item.c.commit && item.c.commit.author) ? item.c.commit.author.name : '';
      return '<tr>' +
        '<td class="mono">' + escapeHTML(item.branch) + '</td>' +
        '<td class="mono"><a href="https://github.com/duboc/scion-experiment/commit/' + escapeHTML(item.c.sha || '') + '" target="_blank" rel="noopener noreferrer">' + escapeHTML(sha) + '</a></td>' +
        '<td>' + escapeHTML(msg) + '</td>' +
        '<td style="color:var(--text-muted);">' + escapeHTML(author) + '</td>' +
      '</tr>';
    }).join('');
  }

  async function runWorkbenchQuery(ev) {
    if (ev && typeof ev.preventDefault === 'function') {
      ev.preventDefault();
    }
    const qEl = document.getElementById('wb-query');
    const alphaEl = document.getElementById('wb-alpha');
    const localeEl = document.getElementById('wb-locale');
    const metaEl = document.getElementById('wb-meta');
    const tbody = document.getElementById('tbody-workbench');

    const q = (qEl && qEl.value.trim()) ? qEl.value.trim() : 'running shoes';
    const alpha = (alphaEl && alphaEl.value) ? alphaEl.value : '0.65';
    const locale = (localeEl && localeEl.value) ? localeEl.value : '';

    try {
      const url = '/api/workbench?q=' + encodeURIComponent(q) + '&alpha=' + encodeURIComponent(alpha) + '&locale=' + encodeURIComponent(locale);
      const res = await fetch(url).then(r => r.ok ? r.json() : {});
      const items = res.results || [];

      if (metaEl) {
        metaEl.textContent =
          'effective_alpha=' + (res.effective_alpha ?? alpha) +
          ' · ' + (res.probe_latency_ms ? Number(res.probe_latency_ms).toFixed(1) : '--') + ' ms' +
          ' · ' + items.length + ' results';
      }

      if (tbody) {
        if (items.length === 0) {
          tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:1rem;">No matching catalog results for current query &amp; filter parameters.</td></tr>';
        } else {
          tbody.innerHTML = items.map((item, idx) => {
            const availBadge = item.is_available
              ? '<span class="badge badge--pass"><span class="badge-dot"></span>IN_STOCK (' + (item.stock ?? 0) + ')</span>'
              : '<span class="badge badge--pending">OUT_OF_STOCK (0)</span>';
            return '<tr>' +
              '<td class="mono num">#' + (idx + 1) + '</td>' +
              '<td class="mono">' + escapeHTML(item.id) + '</td>' +
              '<td><strong>' + escapeHTML(item.title) + '</strong> <span style="color:var(--text-muted);">· ' + escapeHTML(item.brand || '') + '</span></td>' +
              '<td class="mono">' + escapeHTML(item.locale || 'en-US') + '</td>' +
              '<td>' + availBadge + '</td>' +
              '<td class="mono num">' + (item.final_score !== undefined ? Number(item.final_score).toFixed(4) : '--') + '</td>' +
            '</tr>';
          }).join('');
        }
      }
    } catch (err) {
      if (metaEl) metaEl.textContent = 'Query probe failed: ' + err.message;
    }
  }

  async function refreshAll() {
    try {
      const [evalRes, agentsRes, backlogRes] = await Promise.all([
        fetch('/api/eval').then(r => r.ok ? r.json() : {}).catch(() => ({})),
        fetch('/api/agents').then(r => r.ok ? r.json() : { agents: [] }).catch(() => ({ agents: [] })),
        fetch('/api/backlog').then(r => r.ok ? r.json() : {}).catch(() => ({}))
      ]);

      cachedEval = evalRes;
      cachedAgents = Array.isArray(agentsRes.agents) ? agentsRes.agents : [];

      // Primary Asymmetric Focal Readouts
      const ndcgEl = document.getElementById('val-ndcg');
      if (ndcgEl) {
        const val = evalRes.overall_ndcg_at_10 !== undefined ? Number(evalRes.overall_ndcg_at_10).toFixed(3) : '--';
        ndcgEl.textContent = val + (evalRes.is_sample ? ' (sample)' : '');
      }

      const mrrEl = document.getElementById('val-mrr');
      if (mrrEl) {
        const val = evalRes.overall_mrr_at_10 !== undefined ? Number(evalRes.overall_mrr_at_10).toFixed(3) : '--';
        mrrEl.textContent = val + (evalRes.is_sample ? ' (sample)' : '');
      }

      const alphaEl = document.getElementById('val-opt-alpha');
      if (alphaEl) alphaEl.textContent = evalRes.optimal_alpha !== undefined ? Number(evalRes.optimal_alpha).toFixed(2) : '0.65';

      const timeEl = document.getElementById('val-timestamp');
      if (timeEl) {
        timeEl.textContent = evalRes.timestamp ? evalRes.timestamp.slice(11, 19) + ' UTC' : '--';
      }

      const modeEl = document.getElementById('lbl-backend-mode');
      if (modeEl) {
        modeEl.textContent = evalRes.data_source === 'sample' ? 'spanner (sample)' : (evalRes.backend_mode || 'spanner');
      }

      const p50El = document.getElementById('val-p50');
      if (p50El) p50El.textContent = evalRes.p50_latency_ms !== undefined ? Number(evalRes.p50_latency_ms).toFixed(1) + ' ms' : '-- ms';

      const p95El = document.getElementById('val-p95');
      if (p95El) {
        const p95Str = evalRes.p95_latency_ms !== undefined ? Number(evalRes.p95_latency_ms).toFixed(1) : '--';
        const p99Str = evalRes.p99_latency_ms !== undefined ? Number(evalRes.p99_latency_ms).toFixed(1) : '--';
        p95El.textContent = p95Str + ' / ' + p99Str + ' ms';
      }

      const passEl = document.getElementById('val-reqs-pass');
      if (passEl) {
        passEl.textContent = (evalRes.pass_count ?? 8) + ' PASS · ' + (evalRes.partial_count ?? 2) + ' PARTIAL';
      }

      const linkStorefront = document.getElementById('link-storefront');
      if (linkStorefront && evalRes.target_url) {
        linkStorefront.href = evalRes.target_url;
      }

      const agentsCntEl = document.getElementById('val-agents-cnt');
      if (agentsCntEl) {
        const running = cachedAgents.filter(a => a.phase === 'running' || a.activity === 'active').length;
        agentsCntEl.textContent = running + ' / ' + (cachedAgents.length || 10) + ' running';
      }

      // Render Visualizations and Sub-ledgers
      renderAlphaCurve(evalRes.alpha_curve || []);
      renderAgentsTable(selectedProject, selectedAgentName);
      renderRequirementsTable(evalRes.requirements || []);
      renderDiagnostics(evalRes.diagnostics || []);
      renderGoldenQueries(evalRes.golden_queries || []);
      renderCommits(backlogRes);
    } catch (err) {
      console.error('Failed to sync dashboard telemetry:', err);
    }
  }

  // Restore saved theme if any
  try {
    const savedTheme = localStorage.getItem('psearch-theme');
    if (savedTheme === 'carbon' || savedTheme === 'daylight') {
      document.documentElement.setAttribute('data-theme', savedTheme);
      const labelEl = document.getElementById('theme-label');
      if (labelEl) {
        labelEl.textContent = savedTheme === 'carbon' ? 'Daylight Mode' : 'Carbon Mode';
      }
    }
  } catch (_) {
    // Ignore storage restrictions
  }

  // Export to window for inline HTML event attributes
  root.toggleTheme = toggleTheme;
  root.setAgentFilter = setAgentFilter;
  root.setWaveFilter = setWaveFilter;
  root.selectAgentNode = selectAgentNode;
  root.renderAlphaCurve = renderAlphaCurve;
  root.renderAgentsTable = renderAgentsTable;
  root.renderRequirementsTable = renderRequirementsTable;
  root.runWorkbenchQuery = runWorkbenchQuery;
  root.refreshAll = refreshAll;

  // Initialize on load
  function init() {
    refreshAll();
    runWorkbenchQuery();
    setInterval(refreshAll, 15000);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})(typeof window !== 'undefined' ? window : this);
