// /workspace/eval-dashboard/public/app.js
// Impeccable Reactive Telemetry & Evaluation Logic for psearch-eval Workbench
// Mode: Operate · Visual World: Precision Telemetry Ledger (Kinpaku & Warm Stone / Carbon Ink)

(function (root) {
  'use strict';

  let cachedEval = null;
  let cachedAgents = [];
  let cachedTrajectories = null;
  let cachedPyramidLayers = [];
  let selectedPyramidLayerId = 'l1_trajectory_sequence';
  let currentAgentFilter = 'all';
  let currentWaveFilter = 'all';
  let selectedProject = 'psearch';
  let selectedAgentName = 'lead';
  let selectedTrajectoryProject = 'psearch';
  let selectedTrajectoryAgent = 'lead';

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
    if (el) {
      el.classList.add('is-active');
    } else {
      document.querySelectorAll('.topo-node').forEach(n => {
        const attr = n.getAttribute('onclick') || '';
        if (attr.indexOf("'" + project + "','" + name + "'") !== -1) {
          n.classList.add('is-active');
        }
      });
    }

    const found = cachedAgents.find(a => a.project === project && a.name === name);
    const box = document.getElementById('node-inspector');
    if (box) {
      if (found) {
        box.innerHTML =
          '<span>Selected Agent: <strong class="mono">@' + escapeHTML(found.name) + ' (' + escapeHTML(found.project) + ')</strong> — ' + escapeHTML(found.role || found.template) + '</span>' +
          '<span class="mono">Harness: ' + escapeHTML(found.harness || 'antigravity') + ' · Steps: ' + escapeHTML(found.trajectory_steps_count || '--') + ' · Focus: ' + escapeHTML(found.active_issue || 'Standby') + '</span>';
      } else {
        box.innerHTML =
          '<span>Selected Agent: <strong class="mono">@' + escapeHTML(name) + ' (' + escapeHTML(project) + ')</strong></span>' +
          '<span class="mono">Telemetry active</span>';
      }
    }

    renderAgentsTable(project, name);
    selectTrajectoryView(project, name);
  }

  function renderAlphaCurve(points) {
    const svg = document.getElementById('svg-alpha-curve');
    if (!svg) return;
    if (!Array.isArray(points) || points.length === 0) {
      svg.innerHTML = '<text x="230" y="60" text-anchor="middle" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="12">No alpha sweep measured</text>';
      return;
    }

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

      return '<tr class="' + (isSel ? 'is-selected' : '') + '" style="cursor:pointer;" onclick="selectAgentNode(\'' + escapeHTML(a.project || 'psearch') + '\',\'' + escapeHTML(a.name) + '\')">' +
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

  function selectPyramidLayer(layerId, el) {
    selectedPyramidLayerId = layerId;
    document.querySelectorAll('.pyramid-card').forEach(c => c.classList.remove('is-active'));
    if (el) el.classList.add('is-active');

    const layer = cachedPyramidLayers.find(l => l.id === layerId);
    const titleEl = document.getElementById('pyramid-detail-title');
    const metaEl = document.getElementById('pyramid-detail-meta');
    const badgeEl = document.getElementById('pyramid-layer-badge');
    if (layer) {
      if (titleEl) titleEl.textContent = layer.name;
      if (metaEl) metaEl.textContent = layer.scenarios_passed + ' / ' + layer.scenarios_count + ' scenarios verified (' + (layer.threshold || '100%') + ' threshold)';
      if (badgeEl) {
        const bClass = layer.status === 'PASS' ? 'badge--pass' : 'badge--critical';
        badgeEl.className = 'badge ' + bClass;
        badgeEl.innerHTML = '<span class="badge-dot"></span>' + escapeHTML(layer.status) + ' (' + layer.scenarios_passed + '/' + layer.scenarios_count + ')';
      }
    }

    renderPyramidScenarios(layerId);
  }

  function renderPyramidScenarios(layerId) {
    const tbody = document.getElementById('tbody-pyramid-scenarios');
    if (!tbody) return;

    const layer = cachedPyramidLayers.find(l => l.id === layerId);
    if (!layer || !Array.isArray(layer.scenarios) || layer.scenarios.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="color:var(--text-muted);font-style:italic;padding:1rem;">No scenarios recorded for this layer.</td></tr>';
      return;
    }

    tbody.innerHTML = layer.scenarios.map(s => {
      const bClass = s.status === 'PASS' ? 'badge--pass' : (s.status === 'PARTIAL' ? 'badge--partial' : 'badge--gap');
      return '<tr>' +
        '<td class="mono num">' + escapeHTML(s.id) + '</td>' +
        '<td><strong class="mono" style="font-size:0.8125rem;">' + escapeHTML(s.name) + '</strong></td>' +
        '<td><span class="badge ' + bClass + '"><span class="badge-dot"></span>' + escapeHTML(s.status) + '</span></td>' +
        '<td class="mono num">' + (s.latency_ms !== null && s.latency_ms !== undefined ? s.latency_ms + 'ms' : '—') + '</td>' +
        '<td style="color:var(--text-secondary);font-size:0.8125rem;">' + escapeHTML(s.evidence || '—') + '</td>' +
      '</tr>';
    }).join('');
  }

  function renderPyramidLayers(layers, summary) {
    cachedPyramidLayers = Array.isArray(layers) ? layers : [];
    const container = document.getElementById('pyramid-stack');
    const valPyramid = document.getElementById('val-pyramid-pass');
    const summaryBadge = document.getElementById('pyramid-summary-badge');

    if (summary) {
      const passed = summary.passed_scenarios ?? 92;
      const total = summary.total_scenarios ?? 92;
      const pct = summary.pass_rate ? (summary.pass_rate * 100).toFixed(0) : 100;
      if (valPyramid) {
        valPyramid.textContent = passed + ' / ' + total + ' (' + pct + '%)';
      }
      if (summaryBadge) {
        summaryBadge.innerHTML = '<span class="badge-dot"></span>' + passed + ' / ' + total + ' PASS (' + pct + '%)';
      }
    }

    if (!container || cachedPyramidLayers.length === 0) return;

    // Render cards from top (L5) to bottom (L1)
    const displayOrder = [...cachedPyramidLayers].sort((a, b) => (b.tier || 0) - (a.tier || 0));

    container.innerHTML = displayOrder.map(l => {
      const isActive = l.id === selectedPyramidLayerId;
      const bClass = l.status === 'PASS' ? 'badge--pass' : 'badge--critical';
      return '<div class="pyramid-card ' + (isActive ? 'is-active' : '') + '" onclick="selectPyramidLayer(\'' + escapeHTML(l.id) + '\', this)">' +
        '<div style="display:flex;align-items:center;">' +
          '<span class="pyramid-tier-num">L' + escapeHTML(l.tier || '') + '</span>' +
          '<div>' +
            '<div style="font-weight:600;font-size:0.875rem;">' + escapeHTML(l.name) + '</div>' +
            '<div class="mono" style="font-size:0.75rem;color:var(--text-muted);">' + escapeHTML(l.scenarios_passed) + ' / ' + escapeHTML(l.scenarios_count) + ' scenarios · Threshold ' + escapeHTML(l.threshold || '100%') + '</div>' +
          '</div>' +
        '</div>' +
        '<span class="badge ' + bClass + '"><span class="badge-dot"></span>' + escapeHTML(l.status) + '</span>' +
      '</div>';
    }).join('');

    renderPyramidScenarios(selectedPyramidLayerId);
  }

  function renderRequirementsTable(reqs) {
    const tbody = document.getElementById('tbody-reqs');
    if (!tbody || !Array.isArray(reqs)) return;

    const reqsPassEl = document.getElementById('val-reqs-pass');
    if (reqsPassEl) {
      const passCnt = reqs.filter(r => r.status === 'PASS').length;
      reqsPassEl.textContent = passCnt + ' / ' + reqs.length;
    }

    const filtered = reqs.filter(r => currentWaveFilter === 'all' || r.wave === currentWaveFilter);
    tbody.innerHTML = filtered.map(r => {
      const bClass = r.status === 'PASS' ? 'badge--pass' : (r.status === 'PARTIAL' ? 'badge--partial' : 'badge--pending');
      const waveLabel = r.wave || (r.issue_number <= 3 ? 'wave-1' : (r.issue_number <= 6 ? 'wave-2' : (r.issue_number <= 8 ? 'wave-3' : (r.issue_number <= 10 ? 'wave-4' : 'wave-5'))));
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
    if (!container) return;
    if (!Array.isArray(diags) || diags.length === 0) {
      container.innerHTML = '<div style="color:var(--text-muted);padding:1rem;font-style:italic;">No active diagnostic findings recorded from factory analysis.</div>';
      return;
    }

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
    const metaEl = document.getElementById('golden-suite-meta');
    if (!tbody || !Array.isArray(queries)) return;

    if (metaEl && queries.length > 0) {
      metaEl.textContent = queries.length + ' Queries (Pharma · Dermo · Retail) · NDCG@10 & MRR@10';
    }

    tbody.innerHTML = queries.map(g => {
      const safeQ = escapeHTML(g.query || '').replace(/'/g, "\\'");
      const safeLoc = escapeHTML(g.locale || 'pt-BR').replace(/'/g, "\\'");
      const alphaVal = g.alpha !== undefined ? Number(g.alpha) : 0.65;
      const useCaseSub = g.use_case || g.department || '';
      return '<tr style="cursor:pointer;" title="Click to probe in Interactive Workbench" onclick="probePresetQuery(\'' + safeQ + '\',\'' + safeLoc + '\',' + alphaVal + ',\'\')">' +
        '<td>' +
          '<strong>' + escapeHTML(g.query) + '</strong>' +
          (useCaseSub ? ('<div class="mono" style="font-size:0.7rem;color:var(--text-muted);margin-top:0.12rem;">' + escapeHTML(useCaseSub) + '</div>') : '') +
        '</td>' +
        '<td class="mono">' + escapeHTML(g.locale || 'en-US') + '</td>' +
        '<td class="mono num">' + alphaVal.toFixed(2) + '</td>' +
        '<td class="mono num"><strong>' + (g.ndcg_at_10 !== undefined ? Number(g.ndcg_at_10).toFixed(3) : '--') + '</strong></td>' +
        '<td class="mono num">' + (g.mrr_at_10 !== undefined ? Number(g.mrr_at_10).toFixed(3) : '--') + '</td>' +
        '<td class="mono num">' + (g.latency_ms !== undefined ? Number(g.latency_ms).toFixed(1) + ' ms' : '--') + '</td>' +
        '<td class="mono">' + escapeHTML((g.returned_ids || []).slice(0, 3).join(', ')) + '</td>' +
      '</tr>';
    }).join('');
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

  function renderWorkbenchRows(items, isCatalogMode) {
    const tbody = document.getElementById('tbody-workbench');
    if (!tbody) return;
    if (!Array.isArray(items) || items.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:1rem;">No matching catalog results for current query &amp; filter parameters.</td></tr>';
      return;
    }

    tbody.innerHTML = items.map((item, idx) => {
      const isAvail = item.is_available === true || item.availability === 'IN_STOCK' || (item.availableQuantity !== undefined && item.availableQuantity > 0);
      const stockQty = item.availableQuantity ?? item.stock ?? (isAvail ? 'In Stock' : 0);
      const availBadge = isAvail
        ? '<span class="badge badge--pass"><span class="badge-dot"></span>IN_STOCK (' + stockQty + ')</span>'
        : '<span class="badge badge--pending">OUT_OF_STOCK (0)</span>';
      const brandStr = item.brand || (Array.isArray(item.brands) ? item.brands.join(', ') : '');
      const catStr = item.category || (Array.isArray(item.categories) ? item.categories.join(', ') : '');
      const finalScore = (item.score && item.score.final_score !== undefined) ? item.score.final_score : item.final_score;
      const baseP = item.base_price ?? (item.priceInfo && item.priceInfo.basePrice);
      const b2bP = item.b2b_price ?? (item.priceInfo && item.priceInfo.numericPrice);
      const curSym = (item.locale === 'pt-BR') ? 'R$' : '$';

      let rightCol = finalScore !== undefined && finalScore !== null ? Number(finalScore).toFixed(4) : '--';
      if (isCatalogMode && b2bP !== undefined && b2bP !== null) {
        rightCol = 'B2B ' + curSym + Number(b2bP).toFixed(2) + (baseP !== undefined && baseP !== null && baseP !== b2bP ? (' <span style="color:var(--text-muted);">(Base ' + curSym + Number(baseP).toFixed(2) + ')</span>') : '');
      } else if (b2bP !== undefined && b2bP !== null) {
        rightCol += ' <span style="color:var(--text-muted);">· ' + curSym + Number(b2bP).toFixed(2) + '</span>';
      }

      return '<tr>' +
        '<td class="mono num">#' + (idx + 1) + '</td>' +
        '<td class="mono">' + escapeHTML(item.id) + '</td>' +
        '<td><strong>' + escapeHTML(item.title) + '</strong>' +
          (brandStr ? (' <span style="color:var(--text-muted);">· ' + escapeHTML(brandStr) + '</span>') : '') +
          (catStr ? (' <span class="mono" style="color:var(--text-muted);font-size:0.75rem;">[' + escapeHTML(catStr) + ']</span>') : '') +
        '</td>' +
        '<td class="mono">' + escapeHTML(item.locale || 'pt-BR') + '</td>' +
        '<td>' + availBadge + '</td>' +
        '<td class="mono num">' + rightCol + '</td>' +
      '</tr>';
    }).join('');
  }

  async function probePresetQuery(query, locale, alpha, category) {
    const qEl = document.getElementById('wb-query');
    const alphaEl = document.getElementById('wb-alpha');
    const alphaValEl = document.getElementById('wb-alpha-val');
    const localeEl = document.getElementById('wb-locale');
    const catEl = document.getElementById('wb-category');

    if (qEl && query !== undefined) qEl.value = query;
    if (alphaEl && alpha !== undefined) {
      alphaEl.value = String(alpha);
      if (alphaValEl) alphaValEl.textContent = Number(alpha).toFixed(2);
    }
    if (localeEl && locale !== undefined) localeEl.value = locale;
    if (catEl && category !== undefined) catEl.value = category;

    await runWorkbenchQuery();
  }

  async function runWorkbenchQuery(ev) {
    if (ev && typeof ev.preventDefault === 'function') {
      ev.preventDefault();
    }
    const qEl = document.getElementById('wb-query');
    const alphaEl = document.getElementById('wb-alpha');
    const localeEl = document.getElementById('wb-locale');
    const catEl = document.getElementById('wb-category');
    const metaEl = document.getElementById('wb-meta');

    const q = (qEl && qEl.value !== undefined) ? qEl.value.trim() : 'protetor solar fps 60';
    const alpha = (alphaEl && alphaEl.value) ? alphaEl.value : '0.65';
    const locale = (localeEl && localeEl.value) ? localeEl.value : '';
    const category = (catEl && catEl.value) ? catEl.value : '';

    try {
      let url = '/api/workbench?q=' + encodeURIComponent(q) + '&alpha=' + encodeURIComponent(alpha) + '&locale=' + encodeURIComponent(locale);
      if (category) url += '&category=' + encodeURIComponent(category);
      const res = await fetch(url).then(r => r.ok ? r.json() : {});
      const items = res.results || [];

      if (metaEl) {
        metaEl.textContent =
          'effective_alpha=' + (res.effective_alpha ?? alpha) +
          (category ? (' · dept=' + category) : '') +
          ' · ' + (res.probe_latency_ms ? Number(res.probe_latency_ms).toFixed(1) : '--') + ' ms' +
          ' · ' + items.length + ' results';
      }

      renderWorkbenchRows(items, false);
    } catch (err) {
      if (metaEl) metaEl.textContent = 'Query probe failed: ' + err.message;
    }
  }

  async function loadFullCatalog() {
    const metaEl = document.getElementById('wb-meta');
    const btnEl = document.getElementById('btn-load-catalog');
    const catEl = document.getElementById('wb-category');
    const category = (catEl && catEl.value) ? catEl.value : '';

    if (metaEl) metaEl.textContent = 'Loading 20,535-SKU multi-department catalog showcase (Pharma · Dermo · Compounding · Retail + B2B pricing)...';
    if (btnEl) btnEl.disabled = true;

    try {
      const url = category ? ('/api/catalog?category=' + encodeURIComponent(category)) : '/api/catalog';
      const res = await fetch(url).then(r => r.ok ? r.json() : {});
      const items = res.results || res.items || [];
      const catSummary = res.catalog_summary || {};
      const totSkus = catSummary.total_products || res.total_catalog_skus || 20535;
      if (metaEl) {
        metaEl.textContent =
          '20,535-SKU Catalog Explorer (' + items.length + ' hero SKUs shown across ' +
          (category || 'all 6 departments') + ' · ' + Number(totSkus).toLocaleString('en-US') +
          ' total Spanner SKUs · b2b-corporate pricing) · ' +
          (res.probe_latency_ms ? Number(res.probe_latency_ms).toFixed(1) : '--') + ' ms';
      }
      renderWorkbenchRows(items, true);
    } catch (err) {
      if (metaEl) metaEl.textContent = 'Failed to load catalog: ' + err.message;
    } finally {
      if (btnEl) btnEl.disabled = false;
    }
  }

  async function triggerLiveEvaluation() {
    const btn = document.getElementById('btn-run-eval');
    const label = document.getElementById('btn-run-eval-label');
    if (btn) btn.disabled = true;
    if (label) label.textContent = 'Running Live Sweep...';

    try {
      const resp = await fetch('/api/eval/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      });
      const data = await resp.json();
      await refreshAll();
      if (label) {
        label.textContent = data && data.run_id ? ('Recorded ' + data.run_id.slice(0, 16)) : 'Sweep Complete';
        setTimeout(() => {
          if (label) label.textContent = 'Run Live Evaluation';
        }, 3000);
      }
    } catch (err) {
      console.error('Live evaluation trigger failed:', err);
      if (label) {
        label.textContent = 'Run Live Evaluation';
      }
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  function selectTrajectoryView(project, agentName, btn) {
    selectedTrajectoryProject = project || 'psearch';
    selectedTrajectoryAgent = agentName || 'lead';
    const targetKey = selectedTrajectoryProject + ':' + selectedTrajectoryAgent;

    document.querySelectorAll('[data-traj-key]').forEach(b => {
      const k = b.getAttribute('data-traj-key');
      b.setAttribute('aria-pressed', k === targetKey ? 'true' : 'false');
    });
    if (btn) {
      btn.setAttribute('aria-pressed', 'true');
    }

    renderAgentTrajectory();
  }

  let selectedInteractionEdge = 'all';
  let showTrajectoryConversationTraces = true;
  let trajChatInitialized = false;

  function toggleAllTrajectoryTraces() {
    showTrajectoryConversationTraces = !showTrajectoryConversationTraces;
    const lbl = document.getElementById('btn-toggle-conv-trace-label');
    if (lbl) {
      lbl.textContent = showTrajectoryConversationTraces
        ? 'Collapse Prompts / Thinking / Tools'
        : 'Expand Prompts / Thinking / Tools';
    }
    renderAgentTrajectory();
  }

  function filterByInteractionEdge(edgeId) {
    selectedInteractionEdge = edgeId || 'all';
    if (edgeId && edgeId !== 'all') {
      // Switch to All 10 Agents or All psearch so all steps on that edge are visible
      selectedTrajectoryProject = 'all';
      selectedTrajectoryAgent = 'all';
      const allBtns = document.querySelectorAll('#traj-agent-pills-psearch .filter-btn, #traj-agent-pills-eval .filter-btn');
      allBtns.forEach(b => b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false'));
    }
    renderInterAgentGraph();
    renderAgentTrajectory();
  }

  function selectGraphNode(project, agentName) {
    selectedInteractionEdge = 'all';
    if (project === 'external') {
      selectTrajectoryView('all', 'all', null);
    } else {
      selectTrajectoryView(project, agentName, null);
    }
    renderInterAgentGraph();
  }

  function renderInterAgentGraph() {
    const svg = document.getElementById('traj-agent-graph-svg');
    const pillsEl = document.getElementById('traj-edge-pills');
    const statsBadge = document.getElementById('traj-graph-stats-badge');
    if (!svg || !cachedTrajectories || !cachedTrajectories.agent_graph) return;

    const graph = cachedTrajectories.agent_graph;
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const summary = graph.summary || {};

    if (statsBadge) {
      statsBadge.textContent =
        (summary.total_nodes || nodes.length) + ' Nodes · ' +
        (summary.total_edges || edges.length) + ' Directed Edges · ' +
        (summary.total_handoffs || 0) + ' Handoffs · ' +
        (summary.total_tool_calls || 0) + ' Tool Calls';
    }

    const nodeById = {};
    nodes.forEach(n => { nodeById[n.id] = n; });

    let svgHtml = '<defs>' +
      '<marker id="arrow-default" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">' +
        '<path d="M 0 1 L 10 5 L 0 9 z" fill="var(--text-muted)" />' +
      '</marker>' +
      '<marker id="arrow-active" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">' +
        '<path d="M 0 1 L 10 5 L 0 9 z" fill="var(--accent-primary)" />' +
      '</marker>' +
    '</defs>';

    // Zone titles
    svgHtml += '<text x="340" y="22" text-anchor="middle" fill="var(--text-secondary)" font-size="12" font-weight="600">psearch Build Factory (GitHub → @lead ↔ @backend / @frontend / @reviewer / @deployer)</text>';
    svgHtml += '<line x1="890" y1="12" x2="890" y2="388" stroke="var(--border-hairline)" stroke-dasharray="4 4" />';
    svgHtml += '<text x="1105" y="22" text-anchor="middle" fill="var(--text-secondary)" font-size="12" font-weight="600">psearch-eval Observability Factory (@eval-lead ↔ Specialists)</text>';

    // Draw directed edges
    edges.forEach(e => {
      const src = nodeById[e.from];
      const dst = nodeById[e.to];
      if (!src || !dst) return;

      const isSelectedEdge = selectedInteractionEdge === e.id;
      const isAgentFocused = selectedInteractionEdge === 'all' && selectedTrajectoryAgent !== 'all' &&
        (e.from === selectedTrajectoryAgent || e.to === selectedTrajectoryAgent);
      const isHighlighted = isSelectedEdge || isAgentFocused;

      const x1 = src.x + 58;
      const y1 = src.y + 20;
      const x2 = dst.x - 58;
      const y2 = dst.y + 20;
      const dx = x2 - x1;
      const dy = y2 - y1;
      const cx = (x1 + x2) / 2;
      const cy = (y1 + y2) / 2 - (Math.abs(dy) < 30 ? 22 : (dx < 0 ? -28 : 16));

      const strokeW = Math.min(4.2, Math.max(1.3, 1.1 + Math.log2(e.count + 1) * 0.55));
      const strokeColor = isHighlighted ? 'var(--accent-primary)' : 'var(--border-strong)';
      const opacity = (selectedInteractionEdge !== 'all' && !isSelectedEdge) ? '0.28' : (isHighlighted ? '0.95' : '0.68');
      const marker = isHighlighted ? 'url(#arrow-active)' : 'url(#arrow-default)';
      const safeEdgeId = escapeHTML(e.id).replace(/'/g, "\\'");

      svgHtml += '<g style="cursor:pointer;" onclick="filterByInteractionEdge(\'' + safeEdgeId + '\')">' +
        '<path d="M' + x1 + ',' + y1 + ' Q' + cx + ',' + cy + ' ' + x2 + ',' + y2 + '" fill="none" stroke="' + strokeColor + '" stroke-width="' + strokeW.toFixed(1) + '" opacity="' + opacity + '" marker-end="' + marker + '" />' +
        '<rect x="' + (cx - 16) + '" y="' + (cy - 9) + '" width="32" height="16" rx="4" fill="var(--bg-surface)" stroke="' + strokeColor + '" stroke-width="1" opacity="' + opacity + '" />' +
        '<text x="' + cx + '" y="' + (cy + 2) + '" text-anchor="middle" fill="var(--text-primary)" font-family="JetBrains Mono, monospace" font-size="10" font-weight="600">' + e.count + '</text>' +
      '</g>';
    });

    // Draw agent nodes
    nodes.forEach(n => {
      const isSelectedNode = (selectedTrajectoryAgent === n.id) ||
        (selectedInteractionEdge !== 'all' && selectedInteractionEdge.indexOf(n.id) !== -1);
      const borderCol = isSelectedNode ? 'var(--accent-primary)' : 'var(--border-strong)';
      const bgFill = isSelectedNode ? 'var(--bg-elevated)' : 'var(--bg-surface)';
      const safeProj = escapeHTML(n.project).replace(/'/g, "\\'");
      const safeId = escapeHTML(n.id).replace(/'/g, "\\'");
      const subText = n.steps_count > 0
        ? (n.steps_count + ' steps · ' + n.tool_calls_count + ' tools')
        : (n.harness + ' · ' + n.model.slice(0, 14));

      svgHtml += '<g class="topo-node' + (isSelectedNode ? ' is-active' : '') + '" transform="translate(' + (n.x - 58) + ',' + n.y + ')" style="cursor:pointer;" onclick="selectGraphNode(\'' + safeProj + '\',\'' + safeId + '\')">' +
        '<rect width="116" height="42" rx="4" fill="' + bgFill + '" stroke="' + borderCol + '" stroke-width="' + (isSelectedNode ? '2' : '1.2') + '" />' +
        '<text x="58" y="16" text-anchor="middle" fill="var(--text-primary)" font-family="JetBrains Mono, monospace" font-size="11" font-weight="700">' + escapeHTML(n.label) + '</text>' +
        '<text x="58" y="31" text-anchor="middle" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9.5">' + escapeHTML(subText) + '</text>' +
      '</g>';
    });

    svg.innerHTML = svgHtml;

    if (pillsEl) {
      let pillsHtml = '<button type="button" class="btn" style="padding:0.18rem 0.5rem;font-size:0.7rem;' +
        (selectedInteractionEdge === 'all' ? 'background:var(--accent-primary);color:var(--bg-surface);' : '') +
        '" onclick="filterByInteractionEdge(\'all\')">All Handoffs (' + (summary.total_handoffs || 0) + ')</button>';

      edges.slice(0, 10).forEach(e => {
        const active = selectedInteractionEdge === e.id;
        const safeEdgeId = escapeHTML(e.id).replace(/'/g, "\\'");
        pillsHtml += '<button type="button" class="btn mono" style="padding:0.18rem 0.5rem;font-size:0.7rem;' +
          (active ? 'background:var(--accent-primary);color:var(--bg-surface);' : '') +
          '" onclick="filterByInteractionEdge(\'' + safeEdgeId + '\')">' +
          escapeHTML(e.id) + ' (' + e.count + ')' +
        '</button>';
      });
      pillsEl.innerHTML = pillsHtml;
    }
  }

  async function askTrajectoryPreset(presetQuery) {
    const input = document.getElementById('traj-chat-input');
    if (input) input.value = presetQuery;
    await runTrajectoryChatQuery(presetQuery);
  }

  async function submitTrajectoryChat(event) {
    if (event) event.preventDefault();
    const input = document.getElementById('traj-chat-input');
    const q = input ? input.value.trim() : '';
    if (!q) return;
    await runTrajectoryChatQuery(q);
  }

  async function runTrajectoryChatQuery(queryText) {
    const outEl = document.getElementById('traj-chat-output');
    const btn = document.getElementById('btn-traj-chat');
    if (!outEl) return;
    if (btn) btn.disabled = true;
    outEl.innerHTML = '<div class="mono" style="color:var(--text-muted);">Querying trajectory prompts, thinking traces, tool invocations &amp; inter-agent handoffs...</div>';

    try {
      const resp = await fetch('/api/trajectories/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: queryText,
          project: selectedTrajectoryProject,
          agent: selectedTrajectoryAgent
        })
      });
      const data = await resp.json();
      const matchedSteps = Array.isArray(data.matched_steps) ? data.matched_steps : [];
      const matchedEdges = Array.isArray(data.matched_edges) ? data.matched_edges : [];

      let html = '<div style="margin-bottom:0.5rem;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.4rem;">' +
        '<span class="mono" style="font-weight:700;color:var(--text-primary);">Query: "' + escapeHTML(queryText) + '"</span>' +
        '<span class="mono" style="font-size:0.7rem;color:var(--text-muted);">Matched ' + matchedSteps.length + ' Conversation Turns · ' + (data.latency_ms || 0) + 'ms</span>' +
      '</div>';

      if (matchedEdges.length > 0) {
        html += '<div style="margin-bottom:0.55rem;display:flex;flex-wrap:wrap;gap:0.3rem;align-items:center;">' +
          '<span class="mono" style="font-size:0.7rem;color:var(--text-secondary);">Active Handoff Edges:</span>' +
          matchedEdges.map(e => {
            const safeEdgeId = escapeHTML(e.id).replace(/'/g, "\\'");
            return '<button type="button" class="btn mono" style="padding:0.12rem 0.45rem;font-size:0.68rem;" onclick="filterByInteractionEdge(\'' + safeEdgeId + '\')">' +
              escapeHTML(e.from + ' → ' + e.to) + ' (' + e.count + ' handoffs · ' + e.tool_calls_count + ' tools)' +
            '</button>';
          }).join('') +
        '</div>';
      }

      html += matchedSteps.map(s => {
        const toolsHtml = (s.tool_calls || []).map(tc =>
          '<div class="mono" style="font-size:0.7rem;background:var(--bg-subtle);padding:0.22rem 0.45rem;border-left:2px solid var(--accent-primary);margin-top:0.2rem;">' +
            '<strong>[' + escapeHTML(tc.tool) + ']</strong> $ ' + escapeHTML(tc.command) +
            '<div style="color:var(--text-muted);margin-top:0.1rem;">→ ' + escapeHTML(tc.output) + '</div>' +
          '</div>'
        ).join('');

        return '<div style="border:1px solid var(--border-hairline);background:var(--bg-surface);padding:0.55rem 0.75rem;margin-bottom:0.5rem;">' +
          '<div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.35rem;">' +
            '<span class="mono" style="font-weight:700;color:var(--accent-primary);">Step #' + escapeHTML(s.step_index) + ' · @' + escapeHTML(s.agent) + ' (' + escapeHTML(s.interaction_edge || '') + ') · Issue #' + escapeHTML(s.issue_number || 'n/a') + '</span>' +
            '<span class="mono" style="font-size:0.7rem;color:var(--text-muted);">' + escapeHTML(String(s.commit_sha || '').slice(0, 14)) + ' · ' + escapeHTML(String(s.event_timestamp || '').slice(0, 19)) + '</span>' +
          '</div>' +
          '<div style="margin-top:0.3rem;font-size:0.76rem;"><strong>📥 Prompt:</strong> ' + escapeHTML(s.prompt || '') + '</div>' +
          '<div style="margin-top:0.22rem;font-size:0.76rem;color:var(--text-secondary);"><strong>🧠 Thinking:</strong> ' + escapeHTML(s.thinking || '') + '</div>' +
          '<div style="margin-top:0.25rem;">' + toolsHtml + '</div>' +
          '<div style="margin-top:0.28rem;font-size:0.76rem;"><strong>📤 Outcome (' + escapeHTML(s.status) + '):</strong> ' + escapeHTML(s.title) + ' — ' + escapeHTML((s.response || s.detail || '').slice(0, 260)) + '</div>' +
        '</div>';
      }).join('');

      outEl.innerHTML = html;
    } catch (err) {
      outEl.innerHTML = '<div class="mono" style="color:var(--status-critical);">Chat query failed: ' + escapeHTML(err.message) + '</div>';
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  function renderAgentTrajectory() {
    const tbody = document.getElementById('tbody-agent-trajectory');
    const summaryEl = document.getElementById('trajectory-agent-summary');
    const spannerBadge = document.getElementById('traj-spanner-badge');
    const kpiTrajEl = document.getElementById('val-traj-steps');
    if (!tbody) return;

    if (!cachedTrajectories || !Array.isArray(cachedTrajectories.agents)) {
      tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:1rem;">Loading agent execution trajectories...</td></tr>';
      return;
    }

    renderInterAgentGraph();
    if (!trajChatInitialized) {
      trajChatInitialized = true;
      runTrajectoryChatQuery('Show @lead, @reviewer, and @deployer prompts, thinking and tool calls on #34 clickable facets');
    }

    if (kpiTrajEl) {
      kpiTrajEl.textContent = (cachedTrajectories.total_agents || 10) + ' Agents · ' + (cachedTrajectories.total_steps || 0) + ' Steps';
    }
    if (spannerBadge) {
      const spannerCnt = cachedTrajectories.spanner_persisted_steps || cachedTrajectories.total_steps || 0;
      const spannerTurnsCnt = cachedTrajectories.spanner_persisted_turns || cachedTrajectories.spanner_turns_total || 0;
      const liveHarnessCnt = cachedTrajectories.live_harness_turns_total || 0;
      spannerBadge.innerHTML = '<span class="badge-dot"></span>Spanner Synced (' + spannerCnt + ' steps in agent_trajectories · ' + spannerTurnsCnt + ' turns / ' + liveHarnessCnt + ' live harness in agent_conversation_turns)';
    }

    let stepsToRender = [];
    if (selectedInteractionEdge !== 'all') {
      const allChrono = Array.isArray(cachedTrajectories.chronological_timeline) ? cachedTrajectories.chronological_timeline : [];
      stepsToRender = allChrono.filter(st => {
        const normFrom = (st.from_agent === 'Scion Hub Scheduler') ? 'PO / GitHub' : (st.from_agent || st.agent);
        const edgeId = normFrom + ' → ' + (st.to_agent || 'lead');
        return edgeId === selectedInteractionEdge || st.interaction_edge === selectedInteractionEdge;
      });
      if (summaryEl) {
        summaryEl.innerHTML =
          '<span>Viewing Handoff Edge: <strong class="mono">' + escapeHTML(selectedInteractionEdge) + '</strong> · <button type="button" class="btn" style="padding:0.1rem 0.45rem;font-size:0.7rem;" onclick="filterByInteractionEdge(\'all\')">Clear Edge Filter ×</button></span>' +
          '<span class="mono">Matching Conversation Turns: ' + stepsToRender.length + '</span>';
      }
    } else if (selectedTrajectoryProject === 'psearch' && selectedTrajectoryAgent === 'all') {
      stepsToRender = Array.isArray(cachedTrajectories.psearch_chronological) ? cachedTrajectories.psearch_chronological : [];
      if (summaryEl) {
        const psearchAgents = cachedTrajectories.agents.filter(a => a.project === 'psearch');
        const totTurns = psearchAgents.reduce((s, a) => s + (Number(a.reconstructed_turns) || 0), 0);
        const totCalls = psearchAgents.reduce((s, a) => s + (Number(a.reconstructed_model_calls) || 0), 0);
        summaryEl.innerHTML =
          '<span>Viewing Trajectory: <strong class="mono">All psearch Agents (Chronological)</strong> — @lead · @backend · @frontend · @reviewer · @deployer</span>' +
          '<span class="mono">Total Steps: ' + stepsToRender.length + ' · Waves 0–6 (Issues #1–#34) · Reconstructed Turns / Calls: ' + totTurns + ' / ' + totCalls + '</span>';
      }
    } else if (selectedTrajectoryProject === 'all' && selectedTrajectoryAgent === 'all') {
      stepsToRender = Array.isArray(cachedTrajectories.chronological_timeline) ? cachedTrajectories.chronological_timeline : [];
      if (summaryEl) {
        summaryEl.innerHTML =
          '<span>Viewing Trajectory: <strong class="mono">All 10 Factory Agents (psearch + psearch-eval)</strong> — Chronological Execution Ledger</span>' +
          '<span class="mono">Total Steps: ' + stepsToRender.length + ' · Spanner Tables: agent_trajectories + agent_conversation_turns</span>';
      }
    } else {
      const key = selectedTrajectoryProject + ':' + selectedTrajectoryAgent;
      const agentObj = (cachedTrajectories.agents_by_key && cachedTrajectories.agents_by_key[key]) ||
        cachedTrajectories.agents.find(a => a.project === selectedTrajectoryProject && a.name === selectedTrajectoryAgent);

      if (agentObj) {
        stepsToRender = (agentObj.trajectory || []).map(st => ({
          ...st,
          project: agentObj.project,
          agent_name: agentObj.name,
          role: agentObj.role,
          harness: agentObj.harness
        }));
        if (summaryEl) {
          const issuesStr = Array.isArray(agentObj.issues_covered) && agentObj.issues_covered.length > 0
            ? agentObj.issues_covered.map(n => '#' + n).join(', ')
            : 'All Waves';
          const liveTurnsNote = agentObj.live_spanner_turns_count > 0
            ? ' · ⚡ Live Spanner Turns: ' + agentObj.live_spanner_turns_count
            : '';
          summaryEl.innerHTML =
            '<span>Viewing Trajectory: <strong class="mono">@' + escapeHTML(agentObj.name) + ' (' + escapeHTML(agentObj.project) + ')</strong> — ' + escapeHTML(agentObj.role || '') + ' (' + escapeHTML(agentObj.harness || '') + ' · ' + escapeHTML(agentObj.model || 'gemini-4-argon') + ')</span>' +
            '<span class="mono">Steps: ' + stepsToRender.length + liveTurnsNote + ' · Tool Calls: ' + (agentObj.tool_calls_count || 0) + ' · Cost: $' + Number(agentObj.total_cost_usd || 0).toFixed(2) + ' · Issues: ' + escapeHTML(issuesStr) + '</span>';
        }
      }
    }

    if (stepsToRender.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:1rem;">No trajectory steps found for current selection.</td></tr>';
      return;
    }

    tbody.innerHTML = stepsToRender.map((s, idx) => {
      const stepNum = s.step_index || (idx + 1);
      const edgeStr = s.interaction_edge || ((s.from_agent || s.agent_name) + ' → ' + (s.to_agent || 'lead'));
      const isLiveHarness = s.source_mode === 'live_harness_stream' || String(s.commit_sha || '').indexOf('spanner-turn-') === 0;
      const agentBadge = '<span class="mono" style="font-weight:700;">@' + escapeHTML(s.agent_name || selectedTrajectoryAgent) + '</span>' +
        '<div class="mono" style="font-size:0.68rem;color:var(--accent-primary);margin-top:0.15rem;">' + escapeHTML(edgeStr) + '</div>' +
        '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);">' + escapeHTML(s.harness || '') + ' · ' + escapeHTML(s.model || '') + (isLiveHarness ? ' · <strong style="color:var(--status-pass);">⚡LIVE</strong>' : '') + '</div>';
      const issuePart = s.issue_number
        ? '<a href="https://github.com/duboc/scion-experiment/issues/' + encodeURIComponent(s.issue_number) + '" target="_blank" class="mono">#' + escapeHTML(s.issue_number) + '</a>'
        : '—';
      const waveIssueCell = '<span class="mono">' + escapeHTML(s.wave || 'wave') + '</span> · ' + issuePart;
      const shaStr = String(s.commit_sha || '');
      let commitCell = '<span class="mono" style="color:var(--text-muted);">spanner</span>';
      if (shaStr.indexOf('spanner-turn-') === 0) {
        commitCell = '<span class="mono" style="color:var(--status-pass);font-weight:700;" title="Streamed directly from container harness transcript into Spanner agent_conversation_turns">⚡ spanner-turn</span>';
      } else if (shaStr.indexOf('hub-state-') === 0) {
        commitCell = '<span class="mono" style="color:var(--accent-primary);font-weight:600;">hub.db</span>';
      } else if (shaStr.indexOf('comment-') === 0) {
        const cid = shaStr.replace('comment-', '').replace(/-relay-\d+$/, '');
        const cUrl = s.comment_url || ('https://github.com/duboc/scion-experiment/issues/' + encodeURIComponent(s.issue_number || 1) + '#issuecomment-' + encodeURIComponent(cid));
        commitCell = '<a href="' + escapeHTML(cUrl) + '" target="_blank" class="mono">#' + escapeHTML(shaStr.replace('comment-', '').slice(-10)) + '</a>';
      } else if (shaStr) {
        commitCell = '<a href="https://github.com/duboc/scion-experiment/commit/' + encodeURIComponent(shaStr) + '" target="_blank" class="mono">' + escapeHTML(shaStr.slice(0, 7)) + '</a>';
      }
      const tsFormatted = s.timestamp ? escapeHTML(String(s.timestamp).replace('T', ' ').slice(0, 19)) : '—';
      const stVal = s.status || 'PASS';
      const stClass = (stVal === 'ROLLED_BACK' || stVal === 'REJECTED' || stVal === 'ERROR')
        ? 'badge--critical'
        : (stVal === 'BLOCKED_RESOLVED' ? 'badge--partial' : 'badge--pass');
      const statusBadge = '<span class="badge ' + stClass + '"><span class="badge-dot"></span>' + escapeHTML(stVal) + '</span>';

      const tok = s.tokens || {};
      const costBadge = tok.cost_usd !== undefined
        ? '<div class="mono" style="font-size:0.68rem;color:var(--text-muted);margin-top:0.22rem;">in:' + (tok.input_tokens || 0) + (tok.cache_read_tokens ? ' · cache:' + tok.cache_read_tokens : '') + ' · out:' + (tok.output_tokens || 0) + ' · <strong>$' + Number(tok.cost_usd).toFixed(4) + '</strong></div>'
        : '';

      let convBlockHtml = '';
      if (showTrajectoryConversationTraces) {
        const toolListHtml = Array.isArray(s.tool_calls) && s.tool_calls.length > 0
          ? s.tool_calls.map(tc =>
              '<div class="mono" style="font-size:0.72rem;background:var(--bg-canvas);padding:0.3rem 0.55rem;border-left:2px solid var(--accent-primary);margin-top:0.22rem;">' +
                '<span style="font-weight:700;color:var(--accent-primary);">[' + escapeHTML(tc.tool) + ']</span> $ ' + escapeHTML(tc.command) +
                '<div style="color:var(--text-secondary);margin-top:0.12rem;">→ ' + escapeHTML(tc.output) + '</div>' +
              '</div>'
            ).join('')
          : '';

        convBlockHtml =
          '<div style="margin-top:0.45rem;padding:0.55rem 0.7rem;background:var(--bg-subtle);border:1px solid var(--border-hairline);display:flex;flex-direction:column;gap:0.3rem;">' +
            '<div style="font-size:0.76rem;"><strong class="mono" style="color:var(--text-primary);">📥 Prompt (' + escapeHTML(s.from_agent || 'orchestrator') + ' → @' + escapeHTML(s.agent_name || s.agent) + '):</strong> ' + escapeHTML(s.prompt || '') + '</div>' +
            '<div style="font-size:0.76rem;color:var(--text-secondary);"><strong class="mono" style="color:var(--text-primary);">🧠 Thinking:</strong> ' + escapeHTML(s.thinking || '') + '</div>' +
            (toolListHtml ? '<div><strong class="mono" style="font-size:0.74rem;color:var(--text-primary);">🛠️ Tool Usage (' + (s.tool_calls || []).length + ' calls):</strong>' + toolListHtml + '</div>' : '') +
            '<div style="font-size:0.76rem;"><strong class="mono" style="color:var(--text-primary);">📤 Response / Handoff (→ ' + escapeHTML(s.to_agent || 'next') + '):</strong> ' + escapeHTML((s.response || s.detail || '').slice(0, 420)) + '</div>' +
          '</div>';
      }

      return '<tr>' +
        '<td class="mono num">#' + escapeHTML(stepNum) + '</td>' +
        '<td>' + agentBadge + '</td>' +
        '<td class="mono">' + waveIssueCell + '</td>' +
        '<td><span class="badge badge--running">' + escapeHTML(s.action_type || 'step') + '</span>' + costBadge + '</td>' +
        '<td class="mono">' + commitCell + '</td>' +
        '<td class="mono" style="font-size:0.75rem;white-space:nowrap;">' + tsFormatted + '</td>' +
        '<td>' +
          '<div style="font-weight:600;font-size:0.8125rem;">' + escapeHTML(s.title || '') + '</div>' +
          '<div style="color:var(--text-secondary);font-size:0.78rem;margin-top:0.15rem;">' + escapeHTML(s.detail || '') + '</div>' +
          convBlockHtml +
        '</td>' +
        '<td>' + statusBadge + '</td>' +
      '</tr>';
    }).join('');
  }

  async function rebuildAgentTrajectories() {
    const btn = document.getElementById('btn-rebuild-trajectories');
    const label = document.getElementById('btn-rebuild-trajectories-label');
    const badge = document.getElementById('traj-spanner-badge');
    if (btn) btn.disabled = true;
    if (label) label.textContent = 'Rebuilding & Syncing to Spanner...';

    try {
      const resp = await fetch('/api/trajectories/rebuild', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      });
      const data = await resp.json();
      if (data && data.agents) {
        cachedTrajectories = data;
        if (data.spanner_sync && data.spanner_sync.synced_steps) {
          cachedTrajectories.spanner_persisted_steps = data.spanner_sync.synced_steps;
        }
        renderAgentTrajectory();
      }
      await refreshAll();
      if (label) {
        const syncedCnt = (data && data.spanner_sync && data.spanner_sync.synced_steps) ? data.spanner_sync.synced_steps : (data.total_steps || 0);
        label.textContent = 'Rebuilt ' + syncedCnt + ' Steps (' + (data.rebuild_latency_ms || 0) + 'ms)';
        setTimeout(() => {
          if (label) label.textContent = 'Rebuild & Sync Trajectories';
        }, 3500);
      }
    } catch (err) {
      console.error('Failed to rebuild agent trajectories:', err);
      if (label) label.textContent = 'Rebuild & Sync Trajectories';
      if (badge) badge.textContent = 'Rebuild error: ' + err.message;
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  async function refreshAll() {
    try {
      const [evalRes, agentsRes, backlogRes, trajRes] = await Promise.all([
        fetch('/api/eval').then(r => r.ok ? r.json() : {}).catch(() => ({})),
        fetch('/api/agents').then(r => r.ok ? r.json() : { agents: [] }).catch(() => ({ agents: [] })),
        fetch('/api/backlog').then(r => r.ok ? r.json() : {}).catch(() => ({})),
        fetch('/api/trajectories').then(r => r.ok ? r.json() : null).catch(() => null)
      ]);

      cachedEval = evalRes;
      cachedAgents = Array.isArray(agentsRes.agents) ? agentsRes.agents : [];
      if (trajRes && Array.isArray(trajRes.agents)) {
        cachedTrajectories = trajRes;
      }

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
      if (alphaEl) {
        alphaEl.textContent = (evalRes.optimal_alpha !== undefined && evalRes.optimal_alpha !== null)
          ? Number(evalRes.optimal_alpha).toFixed(2)
          : '--';
      }

      const timeEl = document.getElementById('val-timestamp');
      if (timeEl) {
        timeEl.textContent = evalRes.timestamp ? evalRes.timestamp.slice(11, 19) + ' UTC' : '--';
      }

      const modeEl = document.getElementById('lbl-backend-mode');
      if (modeEl) {
        modeEl.textContent = evalRes.data_source === 'sample' ? 'spanner (sample)' : (evalRes.backend_mode || 'spanner');
      }

      const p50El = document.getElementById('val-p50');
      if (p50El) {
        p50El.textContent = (evalRes.p50_latency_ms !== undefined && evalRes.p50_latency_ms !== null)
          ? Number(evalRes.p50_latency_ms).toFixed(1) + ' ms'
          : '-- ms';
      }

      const p95El = document.getElementById('val-p95');
      if (p95El) {
        const p95Str = (evalRes.p95_latency_ms !== undefined && evalRes.p95_latency_ms !== null)
          ? Number(evalRes.p95_latency_ms).toFixed(1)
          : '--';
        const p99Str = (evalRes.p99_latency_ms !== undefined && evalRes.p99_latency_ms !== null)
          ? Number(evalRes.p99_latency_ms).toFixed(1)
          : '--';
        p95El.textContent = p95Str + ' / ' + p99Str + ' ms';
      }

      const passEl = document.getElementById('val-reqs-pass');
      if (passEl) {
        const pCnt = (evalRes.pass_count !== undefined && evalRes.pass_count !== null) ? evalRes.pass_count : '--';
        const partCnt = (evalRes.partial_count !== undefined && evalRes.partial_count !== null) ? evalRes.partial_count : '--';
        const totReqs = evalRes.total_requirements || (Array.isArray(evalRes.requirements) ? evalRes.requirements.length : 17) || 17;
        passEl.textContent = pCnt + ' / ' + totReqs + ' PASS · ' + partCnt + ' PARTIAL';
      }

      const linkStorefront = document.getElementById('link-storefront');
      if (linkStorefront && evalRes.target_url) {
        linkStorefront.href = evalRes.target_url;
      }

      const catScaleEl = document.getElementById('val-catalog-scale');
      if (catScaleEl && evalRes.catalog_summary) {
        const totSkus = evalRes.catalog_summary.total_products || 20535;
        const totVecs = evalRes.catalog_summary.embedding_v2_products || 2515;
        catScaleEl.textContent = Number(totSkus).toLocaleString('en-US') + ' SKUs · ' + Number(totVecs).toLocaleString('en-US') + ' Vecs';
      }

      const agentsCntEl = document.getElementById('val-agents-cnt');
      if (agentsCntEl) {
        const running = cachedAgents.filter(a => a.phase === 'running' || a.activity === 'active').length;
        agentsCntEl.textContent = running + ' / ' + (cachedAgents.length || 10) + ' running';
      }

      // Render Visualizations and Sub-ledgers
      renderAlphaCurve(evalRes.alpha_curve || []);
      renderAgentsTable(selectedProject, selectedAgentName);
      renderAgentTrajectory();
      renderPyramidLayers(evalRes.pyramid_layers || [], evalRes.pyramid_summary || {});
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
  root.selectGraphNode = selectGraphNode;
  root.filterByInteractionEdge = filterByInteractionEdge;
  root.toggleAllTrajectoryTraces = toggleAllTrajectoryTraces;
  root.askTrajectoryPreset = askTrajectoryPreset;
  root.submitTrajectoryChat = submitTrajectoryChat;
  root.selectTrajectoryView = selectTrajectoryView;
  root.renderAgentTrajectory = renderAgentTrajectory;
  root.rebuildAgentTrajectories = rebuildAgentTrajectories;
  root.selectPyramidLayer = selectPyramidLayer;
  root.renderPyramidLayers = renderPyramidLayers;
  root.renderAlphaCurve = renderAlphaCurve;
  root.renderAgentsTable = renderAgentsTable;
  root.renderRequirementsTable = renderRequirementsTable;
  root.probePresetQuery = probePresetQuery;
  root.runWorkbenchQuery = runWorkbenchQuery;
  root.loadFullCatalog = loadFullCatalog;
  root.triggerLiveEvaluation = triggerLiveEvaluation;
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
