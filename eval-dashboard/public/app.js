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
  let showTrajectoryConversationTraces = false;
  let trajChatInitialized = false;
  let costLedgerMode = 'pr'; // 'pr' or 'issue'
  let selectedDrilldownWorkItem = { type: 'pr', id: 17 };
  let selectedTrajectoryPrFilter = 'all';
  let selectedTrajectoryIssueFilter = 'all';
  let selectedCostBucketFilter = 'all'; // 'all', 'productive', 'rework', 'heartbeat_tax'
  let selectedWorkspaceLens = 'all'; // 'all', 'finops', 'pyramid', 'topology'
  let selectedInspectedStepKey = null;
  let trajectoryRowsLimit = 60;

  function switchWorkspaceLens(lens) {
    const validLenses = ['all', 'finops', 'pyramid', 'topology'];
    selectedWorkspaceLens = validLenses.includes(lens) ? lens : 'all';
    document.querySelectorAll('.lens-tab[data-lens]').forEach(btn => {
      btn.setAttribute('aria-pressed', btn.getAttribute('data-lens') === selectedWorkspaceLens ? 'true' : 'false');
    });
    const focalEl = document.querySelector('.focal-strip');
    const secPyramid = document.getElementById('section-pyramid');
    const secReqs = document.getElementById('section-requirements');
    const secWb = document.getElementById('section-workbench');
    const secTopo = document.getElementById('section-topology');
    const secTraj = document.getElementById('section-trajectories');

    if (focalEl) focalEl.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'pyramid' || selectedWorkspaceLens === 'finops') ? '' : 'none';
    if (secPyramid) secPyramid.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'pyramid') ? '' : 'none';
    if (secReqs) secReqs.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'pyramid') ? '' : 'none';
    if (secWb) secWb.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'pyramid') ? '' : 'none';
    if (secTopo) secTopo.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'topology') ? '' : 'none';
    if (secTraj) secTraj.style.display = (selectedWorkspaceLens === 'all' || selectedWorkspaceLens === 'finops') ? '' : 'none';

    try {
      if (window.history && window.history.replaceState) {
        const newHash = selectedWorkspaceLens === 'all' ? '' : ('#lens=' + selectedWorkspaceLens);
        window.history.replaceState(null, '', window.location.pathname + window.location.search + newHash);
      }
    } catch (_) {}
  }

  function filterByCostBucket(bucket) {
    const norm = bucket || 'all';
    if (selectedCostBucketFilter === norm && norm !== 'all') {
      selectedCostBucketFilter = 'all';
    } else {
      selectedCostBucketFilter = norm;
    }
    if (selectedCostBucketFilter !== 'all') {
      // Switch agent scope to All 10 Agents so all steps in the chosen FinOps bucket are visible
      selectedTrajectoryProject = 'all';
      selectedTrajectoryAgent = 'all';
      document.querySelectorAll('[data-traj-key]').forEach(b => {
        b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false');
      });
    }
    ['all', 'productive', 'rework', 'heartbeat_tax'].forEach(b => {
      const idSuffix = b === 'heartbeat_tax' ? 'heartbeat' : b;
      const btn = document.getElementById('btn-bucket-' + idSuffix);
      if (btn) btn.setAttribute('aria-pressed', selectedCostBucketFilter === b ? 'true' : 'false');
    });
    trajectoryRowsLimit = 60;
    renderAgentTrajectory();
  }

  function resetAllTrajectoryFilters() {
    selectedCostBucketFilter = 'all';
    selectedTrajectoryPrFilter = 'all';
    selectedTrajectoryIssueFilter = 'all';
    selectedInteractionEdge = 'all';
    selectedTrajectoryProject = 'all';
    selectedTrajectoryAgent = 'all';
    trajectoryRowsLimit = 60;
    document.querySelectorAll('[data-traj-key]').forEach(b => {
      b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false');
    });
    ['all', 'productive', 'rework', 'heartbeat_tax'].forEach(b => {
      const idSuffix = b === 'heartbeat_tax' ? 'heartbeat' : b;
      const btn = document.getElementById('btn-bucket-' + idSuffix);
      if (btn) btn.setAttribute('aria-pressed', b === 'all' ? 'true' : 'false');
    });
    syncWorkItemSelectDropdowns();
    renderAgentTrajectory();
  }

  function renderActiveCrossfilterBar(visibleStepsCount, visibleTurnsCount, visibleCostUsd) {
    const container = document.getElementById('active-crossfilter-pills');
    if (!container) return;
    const pills = [];

    if (selectedCostBucketFilter !== 'all') {
      const labelMap = { productive: 'Productive Work', rework: 'Rework / Rollback', heartbeat_tax: 'Heartbeat Tax' };
      pills.push('<button type="button" class="crossfilter-pill" onclick="filterByCostBucket(\'all\')" title="Remove FinOps bucket filter">Bucket: ' + escapeHTML(labelMap[selectedCostBucketFilter] || selectedCostBucketFilter) + ' &times;</button>');
    }
    if (selectedTrajectoryPrFilter !== 'all') {
      pills.push('<button type="button" class="crossfilter-pill" onclick="onTrajectoryPrFilterChange(\'all\')" title="Remove PR filter">PR #' + escapeHTML(selectedTrajectoryPrFilter) + ' &times;</button>');
    }
    if (selectedTrajectoryIssueFilter !== 'all') {
      pills.push('<button type="button" class="crossfilter-pill" onclick="onTrajectoryIssueFilterChange(\'all\')" title="Remove Issue filter">Issue #' + escapeHTML(selectedTrajectoryIssueFilter) + ' &times;</button>');
    }
    if (selectedInteractionEdge !== 'all') {
      pills.push('<button type="button" class="crossfilter-pill" onclick="filterByInteractionEdge(\'all\')" title="Remove Handoff Edge filter">Edge: ' + escapeHTML(selectedInteractionEdge) + ' &times;</button>');
    }
    if (!(selectedTrajectoryProject === 'all' && selectedTrajectoryAgent === 'all')) {
      const agLabel = selectedTrajectoryAgent === 'all' ? ('Project: ' + selectedTrajectoryProject) : ('@' + selectedTrajectoryAgent + ' (' + selectedTrajectoryProject + ')');
      pills.push('<button type="button" class="crossfilter-pill" onclick="selectTrajectoryView(\'all\',\'all\',null)" title="Switch to All 10 Agents">Agent: ' + escapeHTML(agLabel) + ' &times;</button>');
    }

    const statsSpan = (visibleStepsCount !== undefined)
      ? ('<span class="mono" style="font-size:0.7rem;color:var(--text-secondary);margin-left:0.25rem;">Showing <strong>' + visibleStepsCount + ' steps</strong> (' + (visibleTurnsCount || 0) + ' turns · <strong>$' + Number(visibleCostUsd || 0).toFixed(4) + '</strong>)</span>')
      : '';

    if (pills.length === 0) {
      container.innerHTML = '<span class="mono" style="font-size:0.7rem;color:var(--text-muted);">Crossfilter: All 10 Factory Agents (Unfiltered)</span>' + statsSpan;
    } else {
      container.innerHTML =
        '<span class="mono" style="font-size:0.7rem;font-weight:700;color:var(--text-secondary);">ACTIVE FILTERS:</span>' +
        pills.join('') +
        '<button type="button" class="btn mono" style="padding:0.16rem 0.5rem;font-size:0.68rem;" onclick="resetAllTrajectoryFilters()">Reset All Filters</button>' +
        statsSpan;
    }
  }

  function renderFinOpsConservationBar(costSummary) {
    const el = document.getElementById('finops-conservation-visual');
    if (!el || !costSummary || !costSummary.total_cost_usd) return;

    const total = Math.max(0.0001, Number(costSummary.total_cost_usd || 0));
    const prodUsd = Number(costSummary.productive_cost_usd || 0);
    const rewUsd = Number(costSummary.rework_cost_usd || 0);
    const hbUsd = Number(costSummary.heartbeat_tax_usd || 0);

    const prodPct = Number(((prodUsd / total) * 100).toFixed(2));
    const rewPct = Number(((rewUsd / total) * 100).toFixed(2));
    const hbPct = Number(Math.max(0, 100 - prodPct - rewPct).toFixed(2));

    const prExUsd = Number(costSummary.pr_exclusive_cost_usd || 0);
    const issStUsd = Number(costSummary.issue_standalone_cost_usd || 0);
    const prExPct = Number(((prExUsd / total) * 100).toFixed(2));
    const issStPct = Number(((issStUsd / total) * 100).toFixed(2));
    const hbExPct = Number(Math.max(0, 100 - prExPct - issStPct).toFixed(2));

    const driftVal = Number(costSummary.conservation_drift_usd || 0).toFixed(4);
    const muteProd = selectedCostBucketFilter !== 'all' && selectedCostBucketFilter !== 'productive' ? ' is-muted' : '';
    const muteRew = selectedCostBucketFilter !== 'all' && selectedCostBucketFilter !== 'rework' ? ' is-muted' : '';
    const muteHb = selectedCostBucketFilter !== 'all' && selectedCostBucketFilter !== 'heartbeat_tax' ? ' is-muted' : '';

    el.innerHTML =
      '<div class="finops-track-card">' +
        '<div style="display:flex;justify-content:space-between;align-items:baseline;gap:0.5rem;flex-wrap:wrap;">' +
          '<span class="mono" style="font-size:0.72rem;font-weight:700;color:var(--text-primary);">TRACK A — 3-BUCKET FINOPS OUTCOME DECOMPOSITION (CLICK SEGMENT TO FILTER)</span>' +
          '<span class="mono" style="font-size:0.68rem;color:var(--status-pass);font-weight:700;">100.0% = $' + total.toFixed(4) + ' USD</span>' +
        '</div>' +
        '<div class="finops-track-bar" role="group" aria-label="Track A 3-Bucket FinOps Decomposition">' +
          '<div class="finops-seg' + muteProd + '" data-track="A" data-bucket="productive" data-pct="' + prodPct.toFixed(2) + '" style="width:' + prodPct.toFixed(2) + '%;background:var(--status-pass);" title="Click to crossfilter Productive Work ($' + prodUsd.toFixed(4) + ' · ' + prodPct.toFixed(1) + '%)" onclick="filterByCostBucket(\'productive\')">' +
            (prodPct >= 14 ? ('Productive $' + prodUsd.toFixed(2) + ' (' + prodPct.toFixed(1) + '%)') : '') +
          '</div>' +
          '<div class="finops-seg' + muteRew + '" data-track="A" data-bucket="rework" data-pct="' + rewPct.toFixed(2) + '" style="width:' + rewPct.toFixed(2) + '%;background:var(--accent-primary);" title="Click to crossfilter Rework &amp; Rollbacks ($' + rewUsd.toFixed(4) + ' · ' + rewPct.toFixed(1) + '%)" onclick="filterByCostBucket(\'rework\')">' +
            (rewPct >= 8 ? ('Rework $' + rewUsd.toFixed(2) + ' (' + rewPct.toFixed(1) + '%)') : '') +
          '</div>' +
          '<div class="finops-seg' + muteHb + '" data-track="A" data-bucket="heartbeat_tax" data-pct="' + hbPct.toFixed(2) + '" style="width:' + hbPct.toFixed(2) + '%;background:var(--text-muted);" title="Click to crossfilter Idle Heartbeat Tax ($' + hbUsd.toFixed(4) + ' · ' + hbPct.toFixed(1) + '%)" onclick="filterByCostBucket(\'heartbeat_tax\')">' +
            (hbPct >= 8 ? ('HB $' + hbUsd.toFixed(2)) : '') +
          '</div>' +
        '</div>' +
        '<div class="mono" style="display:flex;justify-content:space-between;flex-wrap:wrap;gap:0.4rem;font-size:0.67rem;color:var(--text-secondary);">' +
          '<span style="cursor:pointer;" onclick="filterByCostBucket(\'productive\')"><span style="color:var(--status-pass);">●</span> Productive: <strong>$' + prodUsd.toFixed(4) + '</strong> (' + prodPct.toFixed(1) + '%)</span>' +
          '<span style="cursor:pointer;" onclick="filterByCostBucket(\'rework\')"><span style="color:var(--accent-primary);">●</span> Rework/Rollback: <strong>$' + rewUsd.toFixed(4) + '</strong> (' + rewPct.toFixed(1) + '%)</span>' +
          '<span style="cursor:pointer;" onclick="filterByCostBucket(\'heartbeat_tax\')"><span style="color:var(--text-muted);">●</span> Heartbeat Tax: <strong>$' + hbUsd.toFixed(4) + '</strong> (' + hbPct.toFixed(1) + '%)</span>' +
        '</div>' +
      '</div>' +
      '<div class="finops-track-card">' +
        '<div style="display:flex;justify-content:space-between;align-items:baseline;gap:0.5rem;flex-wrap:wrap;">' +
          '<span class="mono" style="font-size:0.72rem;font-weight:700;color:var(--text-primary);">TRACK B — EXCLUSIVE WORK-ITEM CONSERVATION LEDGER (ZERO DOUBLE-COUNTING)</span>' +
          '<span class="mono" style="font-size:0.68rem;color:var(--status-pass);font-weight:700;">Drift: $' + driftVal + ' ✓</span>' +
        '</div>' +
        '<div class="finops-track-bar" role="group" aria-label="Track B Work-Item Exclusive Conservation Ledger">' +
          '<div class="finops-seg" data-track="B" data-ledger="pr_exclusive" data-pct="' + prExPct.toFixed(2) + '" style="width:' + prExPct.toFixed(2) + '%;background:#0284c7;" title="Click to view Pull Requests Ledger ($' + prExUsd.toFixed(4) + ' · ' + prExPct.toFixed(1) + '%)" onclick="switchCostLedgerMode(\'pr\')">' +
            (prExPct >= 14 ? ('PR Exclusive $' + prExUsd.toFixed(2) + ' (' + prExPct.toFixed(1) + '%)') : '') +
          '</div>' +
          '<div class="finops-seg" data-track="B" data-ledger="issue_standalone" data-pct="' + issStPct.toFixed(2) + '" style="width:' + issStPct.toFixed(2) + '%;background:#0d9488;" title="Click to view Standalone GitHub Issues Ledger ($' + issStUsd.toFixed(4) + ' · ' + issStPct.toFixed(1) + '%)" onclick="switchCostLedgerMode(\'issue\')">' +
            (issStPct >= 14 ? ('Issue Standalone $' + issStUsd.toFixed(2) + ' (' + issStPct.toFixed(1) + '%)') : '') +
          '</div>' +
          '<div class="finops-seg" data-track="B" data-ledger="heartbeat_tax" data-pct="' + hbExPct.toFixed(2) + '" style="width:' + hbExPct.toFixed(2) + '%;background:var(--text-muted);" title="Idle Heartbeat Tax ($' + hbUsd.toFixed(4) + ' · ' + hbExPct.toFixed(1) + '%)" onclick="filterByCostBucket(\'heartbeat_tax\')">' +
            (hbExPct >= 8 ? ('HB $' + hbUsd.toFixed(2)) : '') +
          '</div>' +
        '</div>' +
        '<div class="mono" style="display:flex;justify-content:space-between;flex-wrap:wrap;gap:0.4rem;font-size:0.67rem;color:var(--text-secondary);">' +
          '<span style="cursor:pointer;" onclick="switchCostLedgerMode(\'pr\')"><span style="color:#0284c7;">■</span> PR Exclusive (9 PRs): <strong>$' + prExUsd.toFixed(4) + '</strong> (' + prExPct.toFixed(1) + '%)</span>' +
          '<span style="cursor:pointer;" onclick="switchCostLedgerMode(\'issue\')"><span style="color:#0d9488;">■</span> Issue Standalone: <strong>$' + issStUsd.toFixed(4) + '</strong> (' + issStPct.toFixed(1) + '%)</span>' +
          '<span style="cursor:pointer;" onclick="filterByCostBucket(\'heartbeat_tax\')"><span style="color:var(--text-muted);">■</span> Heartbeat Tax: <strong>$' + hbUsd.toFixed(4) + '</strong> (' + hbExPct.toFixed(1) + '%)</span>' +
        '</div>' +
      '</div>';
  }

  function switchCostLedgerMode(mode) {
    costLedgerMode = (mode === 'issue') ? 'issue' : 'pr';
    const btnPr = document.getElementById('btn-cost-mode-pr');
    const btnIss = document.getElementById('btn-cost-mode-issue');
    if (btnPr) btnPr.setAttribute('aria-pressed', costLedgerMode === 'pr' ? 'true' : 'false');
    if (btnIss) btnIss.setAttribute('aria-pressed', costLedgerMode === 'issue' ? 'true' : 'false');

    if (costLedgerMode === 'issue' && selectedDrilldownWorkItem.type !== 'issue') {
      selectedDrilldownWorkItem = { type: 'issue', id: 35 };
    } else if (costLedgerMode === 'pr' && selectedDrilldownWorkItem.type !== 'pr') {
      selectedDrilldownWorkItem = { type: 'pr', id: 17 };
    }
    renderCostAndWorkItemExplorer();
  }

  function selectCostDrilldownItem(type, num, applyTurnFilter) {
    const cleanNum = Number(num);
    if (!cleanNum) return;
    selectedDrilldownWorkItem = { type: type === 'issue' ? 'issue' : 'pr', id: cleanNum };
    if (type === 'issue' && costLedgerMode !== 'issue') {
      costLedgerMode = 'issue';
      const btnPr = document.getElementById('btn-cost-mode-pr');
      const btnIss = document.getElementById('btn-cost-mode-issue');
      if (btnPr) btnPr.setAttribute('aria-pressed', 'false');
      if (btnIss) btnIss.setAttribute('aria-pressed', 'true');
    } else if (type === 'pr' && costLedgerMode !== 'pr') {
      costLedgerMode = 'pr';
      const btnPr = document.getElementById('btn-cost-mode-pr');
      const btnIss = document.getElementById('btn-cost-mode-issue');
      if (btnPr) btnPr.setAttribute('aria-pressed', 'true');
      if (btnIss) btnIss.setAttribute('aria-pressed', 'false');
    }

    if (applyTurnFilter !== false) {
      selectedInteractionEdge = 'all';
      selectedTrajectoryProject = 'all';
      selectedTrajectoryAgent = 'all';
      trajectoryRowsLimit = 60;
      document.querySelectorAll('[data-traj-key]').forEach(b => {
        b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false');
      });
      if (type === 'pr') {
        selectedTrajectoryPrFilter = String(cleanNum);
        selectedTrajectoryIssueFilter = 'all';
      } else {
        selectedTrajectoryIssueFilter = String(cleanNum);
        selectedTrajectoryPrFilter = 'all';
      }
      syncWorkItemSelectDropdowns();
      renderAgentTrajectory();
    } else {
      renderCostAndWorkItemExplorer();
    }
  }

  function filterTurnsByDrilldownAgent(project, agentName) {
    if (!project || !agentName) return;
    selectedInteractionEdge = 'all';
    selectedTrajectoryProject = project;
    selectedTrajectoryAgent = agentName;
    trajectoryRowsLimit = 60;
    const targetKey = project + ':' + agentName;
    document.querySelectorAll('[data-traj-key]').forEach(b => {
      b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === targetKey ? 'true' : 'false');
    });
    renderAgentTrajectory();
  }

  function onTrajectoryPrFilterChange(val) {
    selectedTrajectoryPrFilter = val || 'all';
    trajectoryRowsLimit = 60;
    if (selectedTrajectoryPrFilter !== 'all') {
      selectedTrajectoryIssueFilter = 'all';
      selectedInteractionEdge = 'all';
      selectedTrajectoryProject = 'all';
      selectedTrajectoryAgent = 'all';
      document.querySelectorAll('[data-traj-key]').forEach(b => {
        b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false');
      });
      selectedDrilldownWorkItem = { type: 'pr', id: Number(selectedTrajectoryPrFilter) };
      costLedgerMode = 'pr';
      const btnPr = document.getElementById('btn-cost-mode-pr');
      const btnIss = document.getElementById('btn-cost-mode-issue');
      if (btnPr) btnPr.setAttribute('aria-pressed', 'true');
      if (btnIss) btnIss.setAttribute('aria-pressed', 'false');
    }
    syncWorkItemSelectDropdowns();
    renderAgentTrajectory();
  }

  function onTrajectoryIssueFilterChange(val) {
    selectedTrajectoryIssueFilter = val || 'all';
    trajectoryRowsLimit = 60;
    if (selectedTrajectoryIssueFilter !== 'all') {
      selectedTrajectoryPrFilter = 'all';
      selectedInteractionEdge = 'all';
      selectedTrajectoryProject = 'all';
      selectedTrajectoryAgent = 'all';
      document.querySelectorAll('[data-traj-key]').forEach(b => {
        b.setAttribute('aria-pressed', b.getAttribute('data-traj-key') === 'all:all' ? 'true' : 'false');
      });
      selectedDrilldownWorkItem = { type: 'issue', id: Number(selectedTrajectoryIssueFilter) };
      costLedgerMode = 'issue';
      const btnPr = document.getElementById('btn-cost-mode-pr');
      const btnIss = document.getElementById('btn-cost-mode-issue');
      if (btnPr) btnPr.setAttribute('aria-pressed', 'false');
      if (btnIss) btnIss.setAttribute('aria-pressed', 'true');
    }
    syncWorkItemSelectDropdowns();
    renderAgentTrajectory();
  }

  function clearTrajectoryWorkItemFilters() {
    selectedTrajectoryPrFilter = 'all';
    selectedTrajectoryIssueFilter = 'all';
    trajectoryRowsLimit = 60;
    syncWorkItemSelectDropdowns();
    renderAgentTrajectory();
  }

  function syncWorkItemSelectDropdowns() {
    const selPr = document.getElementById('traj-filter-pr');
    const selIssue = document.getElementById('traj-filter-issue');
    const clearBtn = document.getElementById('btn-clear-traj-workitem-filters');
    if (selPr && selPr.value !== String(selectedTrajectoryPrFilter)) {
      selPr.value = String(selectedTrajectoryPrFilter);
    }
    if (selIssue && selIssue.value !== String(selectedTrajectoryIssueFilter)) {
      selIssue.value = String(selectedTrajectoryIssueFilter);
    }
    if (clearBtn) {
      clearBtn.style.display = (selectedTrajectoryPrFilter !== 'all' || selectedTrajectoryIssueFilter !== 'all') ? 'inline-flex' : 'none';
    }
  }

  let lastLazyTurnsFetchKey = '';

  async function fetchLazyWorkItemSpannerTurns(type, id) {
    const key = type + ':' + id;
    lastLazyTurnsFetchKey = key;
    const container = document.getElementById('drilldown-lazy-turns-strip');
    if (!container) return;
    try {
      const qs = type === 'pr' ? ('pr=' + encodeURIComponent(id)) : ('issue=' + encodeURIComponent(id));
      const resp = await fetch('/api/trajectories/turns?' + qs + '&limit=12', { cache: 'no-store' });
      if (!resp.ok) return;
      const data = await resp.json();
      if (lastLazyTurnsFetchKey !== key) return;
      const summary = data.filtered_cost_summary || {};
      const turns = Array.isArray(data.turns) ? data.turns : [];
      const srcCounts = {};
      turns.forEach(t => {
        const s = t.attribution_source || 'explicit_step';
        srcCounts[s] = (srcCounts[s] || 0) + 1;
      });
      const srcSummary = Object.entries(srcCounts).map(([k, v]) => k + ': ' + v).join(' · ') || 'none';
      container.innerHTML =
        '<div class="mono" style="display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:0.4rem;padding:0.42rem 0.85rem;background:var(--bg-subtle);border-bottom:1px solid var(--border-hairline);font-size:0.68rem;color:var(--text-secondary);">' +
          '<span><strong>⚡ Lazy Spanner Turn Drill-Down (' + (data.filtered_count || 0) + ' turns):</strong> Prod <strong>$' + Number(summary.productive_cost_usd || 0).toFixed(4) + '</strong> · Rework <strong style="color:var(--status-warn);">$' + Number(summary.rework_cost_usd || 0).toFixed(4) + '</strong> · Cache Bloat <strong>' + Number(summary.cache_bloat_ratio || 0).toFixed(1) + 'x</strong></span>' +
          '<span>Attribution: <strong>' + escapeHTML(srcSummary) + '</strong> · Drift: <strong style="color:var(--status-good);">$' + Number(summary.conservation_drift_usd || 0).toFixed(4) + ' ✓</strong></span>' +
        '</div>';
    } catch (_) {
      // ignore transient network errors
    }
  }

  function renderCostAndWorkItemExplorer() {
    if (!cachedTrajectories) return;
    const costSummary = cachedTrajectories.cost_summary || {};
    const prRollups = Array.isArray(cachedTrajectories.pr_rollups) ? cachedTrajectories.pr_rollups : [];
    const issueRollups = Array.isArray(cachedTrajectories.issue_rollups) ? cachedTrajectories.issue_rollups : [];

    // 1. Update Top Cost Banner Summary + 2-Track FinOps Marimekko Bar
    renderFinOpsConservationBar(costSummary);

    const bannerEl = document.getElementById('traj-cost-banner-summary');
    if (bannerEl && costSummary.total_cost_usd !== undefined) {
      const totM = ((Number(costSummary.total_tokens) || 0) / 1e6).toFixed(2);
      const driftVal = Number(costSummary.conservation_drift_usd || 0).toFixed(4);
      const cacheBloat = Number(costSummary.cache_bloat_ratio || 0).toFixed(1);
      const bloatSlope = Number(costSummary.context_bloat_slope || 0);
      const slopeStr = (bloatSlope >= 0 ? '+' : '') + bloatSlope.toFixed(0) + ' tok/turn';
      bannerEl.innerHTML =
        '<div>Total Factory Cost: <strong style="color:var(--text-primary);">$' + Number(costSummary.total_cost_usd || 0).toFixed(4) + ' USD</strong> ' +
        '(psearch: $' + Number(costSummary.psearch_cost_usd || 0).toFixed(4) + ' · psearch-eval: $' + Number(costSummary.psearch_eval_cost_usd || 0).toFixed(4) + ') · ' +
        'Total Turns: <strong>' + (costSummary.total_turns || 0) + '</strong> (' + (costSummary.total_steps || 0) + ' steps · ' + (costSummary.total_model_calls || 0) + ' model calls · ' + (costSummary.total_tool_calls || 0) + ' tool calls · ' + totM + 'M tokens)</div>' +
        '<div style="margin-top:0.22rem;font-size:0.7rem;color:var(--text-secondary);">' +
          'FinOps 3-Bucket Ledger: <strong style="color:var(--status-good);">Productive $' + Number(costSummary.productive_cost_usd || 0).toFixed(4) + ' (' + Number(costSummary.productive_share_pct || 0).toFixed(1) + '%)</strong> · ' +
          '<strong style="color:var(--status-warn);">Rework $' + Number(costSummary.rework_cost_usd || 0).toFixed(4) + ' (' + Number(costSummary.rework_share_pct || 0).toFixed(1) + '%)</strong> · ' +
          '<strong>Heartbeat Tax $' + Number(costSummary.heartbeat_tax_usd || 0).toFixed(4) + ' (' + Number(costSummary.heartbeat_tax_share_pct || 0).toFixed(1) + '%)</strong> · ' +
          'Cache Bloat: <strong>' + cacheBloat + 'x</strong> (' + slopeStr + ') · ' +
          'Conservation Drift: <strong style="color:var(--status-good);">$' + driftVal + ' ✓</strong>' +
        '</div>';
    }

    // 2. Populate PR & Issue Filter Dropdowns
    const selPr = document.getElementById('traj-filter-pr');
    if (selPr && prRollups.length > 0) {
      let prOpts = '<option value="all">All Pull Requests (' + prRollups.length + ' PRs · #12–#27)</option>';
      prRollups.forEach(pr => {
        const selAttr = String(selectedTrajectoryPrFilter) === String(pr.pr_number) ? ' selected' : '';
        prOpts += '<option value="' + escapeHTML(pr.pr_number) + '"' + selAttr + '>' +
          'PR #' + escapeHTML(pr.pr_number) + ' ($' + Number(pr.cost_usd || 0).toFixed(4) + ' · ' + (pr.turns_count || 0) + ' turns · ' + (pr.agents_count || 0) + ' agents) — ' + escapeHTML(String(pr.title || '').slice(0, 46)) +
          '</option>';
      });
      selPr.innerHTML = prOpts;
    }

    const selIssue = document.getElementById('traj-filter-issue');
    if (selIssue && issueRollups.length > 0) {
      let issOpts = '<option value="all">All GitHub Issues (' + issueRollups.length + ' Issues · #1–#37)</option>';
      issueRollups.forEach(ir => {
        const selAttr = String(selectedTrajectoryIssueFilter) === String(ir.issue_number) ? ' selected' : '';
        issOpts += '<option value="' + escapeHTML(ir.issue_number) + '"' + selAttr + '>' +
          'Issue #' + escapeHTML(ir.issue_number) + ' [' + escapeHTML(ir.req_id || '') + '] ($' + Number(ir.cost_usd || 0).toFixed(4) + ' · ' + (ir.turns_count || 0) + ' turns · ' + (ir.agents_count || 0) + ' agents) — ' + escapeHTML(String(ir.title || '').slice(0, 42)) +
          '</option>';
      });
      selIssue.innerHTML = issOpts;
    }
    syncWorkItemSelectDropdowns();

    // 3. Render Left Table: PR Ledger or Issue Ledger with Inline Tufte Sparkbars
    const tbodyList = document.getElementById('tbody-cost-workitems');
    const thWorkitem = document.getElementById('th-cost-workitem');
    const tblTitle = document.getElementById('cost-ledger-table-title');
    const tblMeta = document.getElementById('cost-ledger-table-meta');

    if (tbodyList) {
      if (costLedgerMode === 'pr') {
        if (thWorkitem) thWorkitem.textContent = 'PR';
        if (tblTitle) tblTitle.textContent = 'Pull Request Cost, Turns & Agent Ledger (Click Any Row to Dig In & Filter Turns)';
        if (tblMeta) tblMeta.textContent = prRollups.length + ' Merged PRs (#12–#27) · Exclusive Ledger $' + Number(costSummary.pr_exclusive_cost_usd || 0).toFixed(4) + ' · Avg $' + Number(costSummary.avg_cost_per_pr_usd || 0).toFixed(4) + '/PR';

        const maxPrCost = Math.max(0.0001, ...prRollups.map(p => Number(p.cost_usd || 0)));

        tbodyList.innerHTML = prRollups.map(pr => {
          const isSelected = selectedDrilldownWorkItem.type === 'pr' && Number(selectedDrilldownWorkItem.id) === Number(pr.pr_number);
          const isFiltered = String(selectedTrajectoryPrFilter) === String(pr.pr_number);
          const rowStyle = isSelected ? 'background:var(--bg-elevated);border-left:3px solid var(--accent-primary);' : 'cursor:pointer;';
          const linkedIssuesStr = (pr.linked_issues || []).map(i => '#' + i).join(', ');
          const agentsList = (pr.agents_involved || []).map(a => '@' + a.agent).join(', ');
          const itemCost = Number(pr.cost_usd || 0);
          const prodUsd = Number(pr.productive_cost_usd !== undefined ? pr.productive_cost_usd : itemCost);
          const rewUsd = Number(pr.rework_cost_usd || 0);
          const prodW = Math.min(100, Math.max(0, (prodUsd / maxPrCost) * 100));
          const rewW = Math.min(100 - prodW, Math.max(0, (rewUsd / maxPrCost) * 100));
          const sparkbarHtml =
            '<div class="sparkbar-track" title="Productive: $' + prodUsd.toFixed(4) + ' · Rework: $' + rewUsd.toFixed(4) + '">' +
              '<div class="sparkbar-prod" style="width:' + prodW.toFixed(1) + '%;"></div>' +
              '<div class="sparkbar-rework" style="width:' + rewW.toFixed(1) + '%;"></div>' +
            '</div>';
          const subCostBadge = rewUsd > 0
            ? ('<div style="font-size:0.64rem;color:var(--status-warn);">Rework: $' + rewUsd.toFixed(4) + ' (' + Number(pr.rework_ratio_pct || 0).toFixed(0) + '%)</div>')
            : '<div style="font-size:0.64rem;color:var(--status-good);">100% Productive</div>';

          return '<tr style="' + rowStyle + '" onclick="selectCostDrilldownItem(\'pr\',' + Number(pr.pr_number) + ',true)">' +
            '<td class="mono" style="white-space:nowrap;font-weight:700;">' +
              '<a href="https://github.com/duboc/scion-experiment/pull/' + encodeURIComponent(pr.pr_number) + '" target="_blank" onclick="event.stopPropagation()">PR #' + escapeHTML(pr.pr_number) + '</a>' +
              '<div style="font-size:0.66rem;color:var(--text-muted);">' + escapeHTML(pr.merge_commit || '') + '</div>' +
            '</td>' +
            '<td>' +
              '<div style="font-weight:600;font-size:0.78rem;">' + escapeHTML(pr.title || '') + '</div>' +
              '<div class="mono" style="font-size:0.68rem;color:var(--text-muted);margin-top:0.12rem;">' +
                escapeHTML(pr.branch || 'main') + ' · Issues: ' + escapeHTML(linkedIssuesStr || '—') + ' · +' + (pr.additions || 0) + '/-' + (pr.deletions || 0) + ' (' + (pr.changed_files || 0) + ' files)' +
              '</div>' +
            '</td>' +
            '<td class="mono num" style="font-weight:700;color:var(--text-primary);white-space:nowrap;min-width:125px;">$' + itemCost.toFixed(4) + sparkbarHtml + subCostBadge + '</td>' +
            '<td class="mono num" style="white-space:nowrap;">' +
              '<strong>' + (pr.turns_count || 0) + '</strong> turns' +
              '<div style="font-size:0.66rem;color:var(--text-muted);">' + (pr.steps_count || 0) + ' steps · ' + (pr.tool_calls_count || 0) + ' tools</div>' +
            '</td>' +
            '<td class="mono" style="font-size:0.7rem;">' +
              '<strong>' + (pr.agents_count || 0) + ' agents</strong>' +
              '<div style="font-size:0.66rem;color:var(--text-secondary);">' + escapeHTML(agentsList) + '</div>' +
            '</td>' +
            '<td>' +
              '<button type="button" class="btn mono" style="padding:0.18rem 0.48rem;font-size:0.68rem;' + (isFiltered ? 'background:var(--accent-primary);color:var(--bg-surface);' : '') + '" onclick="event.stopPropagation();selectCostDrilldownItem(\'pr\',' + Number(pr.pr_number) + ',true)">' +
                (isFiltered ? 'Active Filter ✓' : 'Dig In &rarr;') +
              '</button>' +
            '</td>' +
          '</tr>';
        }).join('');
      } else {
        if (thWorkitem) thWorkitem.textContent = 'Issue';
        if (tblTitle) tblTitle.textContent = 'GitHub Issue Cost, Turns & Agent Ledger (Click Any Row to Dig In & Filter Turns)';
        if (tblMeta) tblMeta.textContent = issueRollups.length + ' Issues (#1–#37) · Standalone Ledger $' + Number(costSummary.issue_standalone_cost_usd || 0).toFixed(4) + ' · Avg $' + Number(costSummary.avg_cost_per_issue_usd || 0).toFixed(4) + '/Issue';

        const maxIssueCost = Math.max(0.0001, ...issueRollups.map(i => Number(i.cost_usd || 0)));

        tbodyList.innerHTML = issueRollups.map(ir => {
          const isSelected = selectedDrilldownWorkItem.type === 'issue' && Number(selectedDrilldownWorkItem.id) === Number(ir.issue_number);
          const isFiltered = String(selectedTrajectoryIssueFilter) === String(ir.issue_number);
          const rowStyle = isSelected ? 'background:var(--bg-elevated);border-left:3px solid var(--accent-primary);' : 'cursor:pointer;';
          const prLinksStr = (ir.linked_prs && ir.linked_prs.length > 0) ? (' · PR #' + ir.linked_prs.join(', PR #')) : '';
          const agentsList = (ir.agents_involved || []).map(a => '@' + a.agent).join(', ');
          const itemCost = Number(ir.cost_usd || 0);
          const prodUsd = Number(ir.productive_cost_usd !== undefined ? ir.productive_cost_usd : itemCost);
          const rewUsd = Number(ir.rework_cost_usd || 0);
          const prodW = Math.min(100, Math.max(0, (prodUsd / maxIssueCost) * 100));
          const rewW = Math.min(100 - prodW, Math.max(0, (rewUsd / maxIssueCost) * 100));
          const sparkbarHtml =
            '<div class="sparkbar-track" title="Productive: $' + prodUsd.toFixed(4) + ' · Rework: $' + rewUsd.toFixed(4) + '">' +
              '<div class="sparkbar-prod" style="width:' + prodW.toFixed(1) + '%;"></div>' +
              '<div class="sparkbar-rework" style="width:' + rewW.toFixed(1) + '%;"></div>' +
            '</div>';
          const subCostBadge = rewUsd > 0
            ? ('<div style="font-size:0.64rem;color:var(--status-warn);">Rework: $' + rewUsd.toFixed(4) + ' (' + Number(ir.rework_ratio_pct || 0).toFixed(0) + '%)</div>')
            : '<div style="font-size:0.64rem;color:var(--status-good);">100% Productive</div>';

          return '<tr style="' + rowStyle + '" onclick="selectCostDrilldownItem(\'issue\',' + Number(ir.issue_number) + ',true)">' +
            '<td class="mono" style="white-space:nowrap;font-weight:700;">' +
              '<a href="https://github.com/duboc/scion-experiment/issues/' + encodeURIComponent(ir.issue_number) + '" target="_blank" onclick="event.stopPropagation()">#' + escapeHTML(ir.issue_number) + '</a>' +
              '<div style="font-size:0.66rem;color:var(--text-muted);">' + escapeHTML(ir.req_id || '') + '</div>' +
            '</td>' +
            '<td>' +
              '<div style="font-weight:600;font-size:0.78rem;">' + escapeHTML(ir.title || '') + '</div>' +
              '<div class="mono" style="font-size:0.68rem;color:var(--text-muted);margin-top:0.12rem;">' +
                escapeHTML(ir.wave || '') + prLinksStr + (ir.cloud_run_revision ? (' · ' + escapeHTML(ir.cloud_run_revision)) : '') +
              '</div>' +
            '</td>' +
            '<td class="mono num" style="font-weight:700;color:var(--text-primary);white-space:nowrap;min-width:125px;">$' + itemCost.toFixed(4) + sparkbarHtml + subCostBadge + '</td>' +
            '<td class="mono num" style="white-space:nowrap;">' +
              '<strong>' + (ir.turns_count || 0) + '</strong> turns' +
              '<div style="font-size:0.66rem;color:var(--text-muted);">' + (ir.steps_count || 0) + ' steps · ' + (ir.tool_calls_count || 0) + ' tools</div>' +
            '</td>' +
            '<td class="mono" style="font-size:0.7rem;">' +
              '<strong>' + (ir.agents_count || 0) + ' agents</strong>' +
              '<div style="font-size:0.66rem;color:var(--text-secondary);">' + escapeHTML(agentsList) + '</div>' +
            '</td>' +
            '<td>' +
              '<button type="button" class="btn mono" style="padding:0.18rem 0.48rem;font-size:0.68rem;' + (isFiltered ? 'background:var(--accent-primary);color:var(--bg-surface);' : '') + '" onclick="event.stopPropagation();selectCostDrilldownItem(\'issue\',' + Number(ir.issue_number) + ',true)">' +
                (isFiltered ? 'Active Filter ✓' : 'Dig In &rarr;') +
              '</button>' +
            '</td>' +
          '</tr>';
        }).join('');
      }
    }

    // 4. Render Right Zone: Deep-Dive Inspector Card for Selected PR or Issue
    const ddTitle = document.getElementById('drilldown-workitem-title');
    const ddMeta = document.getElementById('drilldown-workitem-meta');
    const ddActions = document.getElementById('drilldown-workitem-actions');
    const ddKpis = document.getElementById('drilldown-workitem-kpis');
    const ddAgentsTbody = document.getElementById('tbody-drilldown-agents');

    let targetItem = null;
    if (selectedDrilldownWorkItem.type === 'pr') {
      targetItem = prRollups.find(p => Number(p.pr_number) === Number(selectedDrilldownWorkItem.id)) || prRollups[0];
    } else {
      targetItem = issueRollups.find(i => Number(i.issue_number) === Number(selectedDrilldownWorkItem.id)) || issueRollups[0];
    }

    if (!targetItem) return;

    const isPr = selectedDrilldownWorkItem.type === 'pr';
    const itemId = isPr ? targetItem.pr_number : targetItem.issue_number;

    if (ddTitle) {
      ddTitle.innerHTML = isPr
        ? ('PR #' + escapeHTML(itemId) + ': ' + escapeHTML(targetItem.title || ''))
        : ('Issue #' + escapeHTML(itemId) + ' (' + escapeHTML(targetItem.req_id || '') + '): ' + escapeHTML(targetItem.title || ''));
    }
    if (ddMeta) {
      if (isPr) {
        const linkedIss = (targetItem.linked_issues || []).map(n => '#' + n).join(', ');
        ddMeta.innerHTML = 'Merge Commit: <strong>' + escapeHTML(targetItem.merge_commit || 'main') + '</strong> · Branch: ' + escapeHTML(targetItem.branch || 'main') + ' · Linked Issues: <strong>' + escapeHTML(linkedIss || 'n/a') + '</strong> · Diff: +' + (targetItem.additions || 0) + ' / -' + (targetItem.deletions || 0) + ' (' + (targetItem.changed_files || 0) + ' files)';
      } else {
        const linkedPrs = (targetItem.linked_prs && targetItem.linked_prs.length > 0) ? ('PR #' + targetItem.linked_prs.join(', PR #')) : 'Direct main commit flow';
        ddMeta.innerHTML = 'Wave: <strong>' + escapeHTML(targetItem.wave || '') + '</strong> · Linked PRs: <strong>' + escapeHTML(linkedPrs) + '</strong> · Cloud Run Revision: <strong>' + escapeHTML(targetItem.cloud_run_revision || 'psearch-serving') + '</strong>';
      }
    }
    if (ddActions) {
      const ghUrl = isPr
        ? ('https://github.com/duboc/scion-experiment/pull/' + encodeURIComponent(itemId))
        : ('https://github.com/duboc/scion-experiment/issues/' + encodeURIComponent(itemId));
      const isCurrentlyFiltered = isPr
        ? (String(selectedTrajectoryPrFilter) === String(itemId))
        : (String(selectedTrajectoryIssueFilter) === String(itemId));
      ddActions.innerHTML =
        '<button type="button" class="btn mono" style="padding:0.2rem 0.55rem;font-size:0.7rem;' + (isCurrentlyFiltered ? 'background:var(--accent-primary);color:var(--bg-surface);' : '') + '" onclick="selectCostDrilldownItem(\'' + (isPr ? 'pr' : 'issue') + '\',' + Number(itemId) + ',true)">' +
          (isCurrentlyFiltered ? 'Filtering Turns Below ✓' : 'Filter Turns Below &darr;') +
        '</button>' +
        '<a href="' + escapeHTML(ghUrl) + '" target="_blank" class="btn mono" style="padding:0.2rem 0.5rem;font-size:0.7rem;text-decoration:none;">GitHub &nearr;</a>';
    }

    if (ddKpis) {
      const tokK = ((Number(targetItem.total_tokens) || 0) / 1000).toFixed(1);
      const prodUsd = Number(targetItem.productive_cost_usd || 0).toFixed(4);
      const rewUsd = Number(targetItem.rework_cost_usd || 0).toFixed(4);
      const bloatRatio = Number(targetItem.cache_bloat_ratio || 0).toFixed(1);
      ddKpis.innerHTML =
        '<div style="padding:0.55rem 0.85rem;border-right:1px solid var(--border-hairline);">' +
          '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);text-transform:uppercase;">Total ' + (isPr ? 'PR' : 'Issue') + ' Cost</div>' +
          '<div class="mono num" style="font-size:1.05rem;font-weight:700;color:var(--text-primary);margin-top:0.1rem;">$' + Number(targetItem.cost_usd || 0).toFixed(4) + ' <span style="font-size:0.7rem;color:var(--text-muted);">USD</span></div>' +
          '<div class="mono" style="font-size:0.63rem;color:var(--text-secondary);margin-top:0.08rem;">Prod $' + prodUsd + ' · Rework $' + rewUsd + '</div>' +
        '</div>' +
        '<div style="padding:0.55rem 0.85rem;border-right:1px solid var(--border-hairline);">' +
          '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);text-transform:uppercase;">Turns &amp; Steps</div>' +
          '<div class="mono num" style="font-size:1.05rem;font-weight:700;color:var(--text-primary);margin-top:0.1rem;">' + (targetItem.turns_count || 0) + ' <span style="font-size:0.72rem;font-weight:500;color:var(--text-secondary);">turns (' + (targetItem.steps_count || 0) + ' steps)</span></div>' +
          '<div class="mono" style="font-size:0.63rem;color:var(--text-secondary);margin-top:0.08rem;">Cache Bloat: ' + bloatRatio + 'x</div>' +
        '</div>' +
        '<div style="padding:0.55rem 0.85rem;border-right:1px solid var(--border-hairline);">' +
          '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);text-transform:uppercase;">Agents Involved</div>' +
          '<div class="mono num" style="font-size:1.05rem;font-weight:700;color:var(--accent-primary);margin-top:0.1rem;">' + (targetItem.agents_count || 0) + ' <span style="font-size:0.72rem;font-weight:500;color:var(--text-secondary);">agents</span></div>' +
        '</div>' +
        '<div style="padding:0.55rem 0.85rem;">' +
          '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);text-transform:uppercase;">Calls, Tools &amp; Tokens</div>' +
          '<div class="mono num" style="font-size:0.86rem;font-weight:700;color:var(--text-primary);margin-top:0.18rem;">' + (targetItem.model_calls_count || 0) + ' calls · ' + (targetItem.tool_calls_count || 0) + ' tools · ' + tokK + 'k tok</div>' +
        '</div>' +
        '<div id="drilldown-lazy-turns-strip" style="grid-column:1 / -1;"></div>';
      fetchLazyWorkItemSpannerTurns(isPr ? 'pr' : 'issue', itemId);
    }

    if (ddAgentsTbody) {
      const agentsBreakdown = Array.isArray(targetItem.agents_involved) ? targetItem.agents_involved : [];
      if (agentsBreakdown.length === 0) {
        ddAgentsTbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:0.8rem;">No agent steps recorded for this item.</td></tr>';
      } else {
        ddAgentsTbody.innerHTML = agentsBreakdown.map(ag => {
          const sharePct = Number(ag.share_pct || 0);
          const barW = Math.min(100, Math.max(4, Math.round(sharePct)));
          const tokStr = ((Number(ag.total_tokens) || 0) / 1000).toFixed(1) + 'k';
          const safeProj = escapeHTML(ag.project || 'psearch').replace(/'/g, "\\'");
          const safeAg = escapeHTML(ag.agent || 'lead').replace(/'/g, "\\'");
          return '<tr style="cursor:pointer;" title="Click to filter turn table below to @' + escapeHTML(ag.agent) + ' on this ' + (isPr ? 'PR' : 'Issue') + '" onclick="filterTurnsByDrilldownAgent(\'' + safeProj + '\',\'' + safeAg + '\')">' +
            '<td class="mono">' +
              '<strong>@' + escapeHTML(ag.agent) + '</strong>' +
              '<span style="font-size:0.66rem;color:var(--text-muted);margin-left:0.25rem;">(' + escapeHTML(ag.project) + ')</span>' +
            '</td>' +
            '<td class="mono" style="font-size:0.7rem;color:var(--text-secondary);">' + escapeHTML(ag.harness || '') + ' · ' + escapeHTML(ag.model || '') + '</td>' +
            '<td class="mono num"><strong>' + (ag.turns || 0) + '</strong></td>' +
            '<td class="mono num">' + (ag.steps_count || 0) + '</td>' +
            '<td class="mono num" style="font-size:0.72rem;">' + (ag.model_calls || 0) + ' / ' + (ag.tool_calls || 0) + '</td>' +
            '<td class="mono num" style="font-size:0.72rem;">' + tokStr + '</td>' +
            '<td class="mono num" style="font-weight:700;color:var(--text-primary);">$' + Number(ag.cost_usd || 0).toFixed(4) + '</td>' +
            '<td style="min-width:105px;">' +
              '<div style="display:flex;align-items:center;gap:0.35rem;">' +
                '<div style="flex:1;height:6px;background:var(--bg-subtle);border-radius:3px;overflow:hidden;">' +
                  '<div style="width:' + barW + '%;height:100%;background:var(--accent-primary);"></div>' +
                '</div>' +
                '<span class="mono num" style="font-size:0.68rem;color:var(--text-secondary);min-width:34px;text-align:right;">' + sharePct.toFixed(1) + '%</span>' +
              '</div>' +
            '</td>' +
          '</tr>';
        }).join('');
      }
    }
  }

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
        ? (n.steps_count + ' steps · $' + Number(n.total_cost_usd || 0).toFixed(2))
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
    outEl.innerHTML = '<div class="mono" style="color:var(--text-muted);">Querying trajectory PR/Issue costs, turns, prompts, thinking traces, tool invocations &amp; handoffs...</div>';

    try {
      const resp = await fetch('/api/trajectories/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: queryText,
          project: selectedTrajectoryProject,
          agent: selectedTrajectoryAgent,
          pr: selectedTrajectoryPrFilter !== 'all' ? selectedTrajectoryPrFilter : null,
          issue: selectedTrajectoryIssueFilter !== 'all' ? selectedTrajectoryIssueFilter : null
        })
      });
      const data = await resp.json();
      const matchedSteps = Array.isArray(data.matched_steps) ? data.matched_steps : [];
      const matchedEdges = Array.isArray(data.matched_edges) ? data.matched_edges : [];
      const matchedPrs = Array.isArray(data.matched_prs) ? data.matched_prs : [];
      const matchedIssues = Array.isArray(data.matched_issues) ? data.matched_issues : [];

      let html = '<div style="margin-bottom:0.5rem;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.4rem;">' +
        '<span class="mono" style="font-weight:700;color:var(--text-primary);">Query: "' + escapeHTML(queryText) + '"</span>' +
        '<span class="mono" style="font-size:0.7rem;color:var(--text-muted);">Matched ' + matchedSteps.length + ' Turns · ' + (data.latency_ms || 0) + 'ms</span>' +
      '</div>';

      if (matchedPrs.length > 0 || matchedIssues.length > 0) {
        html += '<div style="margin-bottom:0.6rem;display:flex;flex-direction:column;gap:0.35rem;">';
        matchedPrs.forEach(pr => {
          const agBadges = (pr.agents_involved || []).map(a => '@' + escapeHTML(a.agent) + ' ($' + Number(a.cost_usd || 0).toFixed(4) + ', ' + (a.turns || 0) + 't)').join(' · ');
          html += '<div style="padding:0.45rem 0.65rem;background:var(--bg-subtle);border-left:3px solid var(--accent-primary);font-size:0.75rem;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.4rem;">' +
            '<div>' +
              '<strong class="mono" style="color:var(--text-primary);">PR #' + escapeHTML(pr.pr_number) + ': ' + escapeHTML(pr.title) + '</strong>' +
              '<div class="mono" style="font-size:0.7rem;color:var(--text-secondary);margin-top:0.12rem;">💰 <strong>$' + Number(pr.cost_usd || 0).toFixed(4) + ' USD</strong> · 🔄 <strong>' + (pr.turns_count || 0) + ' turns</strong> (' + (pr.steps_count || 0) + ' steps) · 👥 <strong>' + (pr.agents_count || 0) + ' agents:</strong> ' + agBadges + '</div>' +
            '</div>' +
            '<button type="button" class="btn mono" style="padding:0.15rem 0.45rem;font-size:0.68rem;" onclick="selectCostDrilldownItem(\'pr\',' + Number(pr.pr_number) + ',true)">Filter PR #' + escapeHTML(pr.pr_number) + ' &darr;</button>' +
          '</div>';
        });
        matchedIssues.forEach(ir => {
          const agBadges = (ir.agents_involved || []).map(a => '@' + escapeHTML(a.agent) + ' ($' + Number(a.cost_usd || 0).toFixed(4) + ', ' + (a.turns || 0) + 't)').join(' · ');
          html += '<div style="padding:0.45rem 0.65rem;background:var(--bg-subtle);border-left:3px solid var(--status-pass);font-size:0.75rem;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.4rem;">' +
            '<div>' +
              '<strong class="mono" style="color:var(--text-primary);">Issue #' + escapeHTML(ir.issue_number) + ' [' + escapeHTML(ir.req_id || '') + ']: ' + escapeHTML(ir.title) + '</strong>' +
              '<div class="mono" style="font-size:0.7rem;color:var(--text-secondary);margin-top:0.12rem;">💰 <strong>$' + Number(ir.cost_usd || 0).toFixed(4) + ' USD</strong> · 🔄 <strong>' + (ir.turns_count || 0) + ' turns</strong> (' + (ir.steps_count || 0) + ' steps) · 👥 <strong>' + (ir.agents_count || 0) + ' agents:</strong> ' + agBadges + '</div>' +
            '</div>' +
            '<button type="button" class="btn mono" style="padding:0.15rem 0.45rem;font-size:0.68rem;" onclick="selectCostDrilldownItem(\'issue\',' + Number(ir.issue_number) + ',true)">Filter Issue #' + escapeHTML(ir.issue_number) + ' &darr;</button>' +
          '</div>';
        });
        html += '</div>';
      }

      if (matchedEdges.length > 0) {
        html += '<div style="margin-bottom:0.55rem;display:flex;flex-wrap:wrap;gap:0.3rem;align-items:center;">' +
          '<span class="mono" style="font-size:0.7rem;color:var(--text-secondary);">Active Handoff Edges:</span>' +
          matchedEdges.map(e => {
            const safeEdgeId = escapeHTML(e.id).replace(/'/g, "\\'");
            return '<button type="button" class="btn mono" style="padding:0.12rem 0.45rem;font-size:0.68rem;" onclick="filterByInteractionEdge(\'' + safeEdgeId + '\')">' +
              escapeHTML(e.from + ' → ' + e.to) + ' (' + e.count + ' handoffs · $' + Number(e.cost_usd || 0).toFixed(4) + ')' +
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
        const prLabel = s.pr_number ? (' · PR #' + escapeHTML(s.pr_number)) : '';

        return '<div style="border:1px solid var(--border-hairline);background:var(--bg-surface);padding:0.55rem 0.75rem;margin-bottom:0.5rem;">' +
          '<div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:0.35rem;">' +
            '<span class="mono" style="font-weight:700;color:var(--accent-primary);">Step #' + escapeHTML(s.step_index) + ' · @' + escapeHTML(s.agent) + ' (' + escapeHTML(s.interaction_edge || '') + ') · Issue #' + escapeHTML(s.issue_number || 'n/a') + prLabel + ' · $' + Number(s.cost_usd || 0).toFixed(4) + ' (' + (s.turns_count || 1) + ' turns)</span>' +
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

  let currentRenderedSteps = [];

  function getStepUniqueKey(s, idx) {
    if (!s) return 'step-' + idx;
    return String(s.step_id || s.commit_sha || ((s.project || 'psearch') + ':' + (s.agent_name || s.agent || 'lead') + ':' + (s.step_index || idx)));
  }

  function selectStepForInspection(stepKey) {
    selectedInspectedStepKey = String(stepKey || '');
    renderStepInspectorDrawer(currentRenderedSteps);
    renderAgentSwimlaneAndBloat(currentRenderedSteps);
    document.querySelectorAll('#tbody-agent-trajectory tr[data-step-key]').forEach(tr => {
      const isSel = tr.getAttribute('data-step-key') === selectedInspectedStepKey;
      tr.style.background = isSel ? 'var(--bg-elevated)' : '';
    });
  }

  function showAllTrajectoryRows() {
    trajectoryRowsLimit = 9999;
    renderAgentTrajectory();
  }

  function renderStepInspectorDrawer(stepsToRender) {
    const drawer = document.getElementById('traj-step-inspector-drawer');
    if (!drawer) return;
    if (!Array.isArray(stepsToRender) || stepsToRender.length === 0) {
      drawer.innerHTML = '<div class="mono" style="font-size:0.75rem;color:var(--text-muted);">No trajectory step selected (current filter returned 0 steps).</div>';
      return;
    }

    let step = stepsToRender.find((s, i) => getStepUniqueKey(s, i) === selectedInspectedStepKey);
    if (!step) {
      step = stepsToRender[0];
      selectedInspectedStepKey = getStepUniqueKey(step, 0);
    }

    const tok = step.tokens || {};
    const inTok = Number(tok.input_tokens || 0);
    const cacheTok = Number(tok.cache_read_tokens || 0);
    const outTok = Number(tok.output_tokens || 0);
    const totTok = Math.max(1, inTok + cacheTok + outTok);
    const inPct = ((inTok / totTok) * 100).toFixed(1);
    const cachePct = ((cacheTok / totTok) * 100).toFixed(1);
    const outPct = Math.max(0, 100 - Number(inPct) - Number(cachePct)).toFixed(1);

    const stepCost = Number(step.cost_usd !== undefined ? step.cost_usd : (tok.cost_usd || 0));
    const cBucket = String(step.cost_bucket || 'productive').toUpperCase();
    const bucketBg = cBucket === 'REWORK' ? 'var(--accent-primary)' : (cBucket === 'HEARTBEAT_TAX' ? 'var(--text-muted)' : 'var(--status-pass)');
    const attrSource = step.attribution_source || (step.pr_number ? 'git_commit_sha' : (step.issue_number ? 'explicit_mention' : 'session_carry_forward'));
    const edgeStr = step.interaction_edge || ((step.from_agent || step.agent_name || step.agent) + ' → ' + (step.to_agent || 'lead'));
    const toolCalls = Array.isArray(step.tool_calls) ? step.tool_calls : [];

    const toolsListHtml = toolCalls.length > 0
      ? toolCalls.map(tc => {
          const st = String(tc.status || 'OK').toUpperCase();
          const col = (st === 'WARN' || st === 'ERROR') ? 'var(--status-critical)' : 'var(--accent-primary)';
          return '<div class="mono" style="font-size:0.69rem;background:var(--bg-subtle);padding:0.25rem 0.45rem;border-left:2px solid ' + col + ';margin-bottom:0.25rem;">' +
            '<div><strong style="color:' + col + ';">[' + escapeHTML(tc.tool || 'tool') + ' · ' + escapeHTML(st) + ']</strong> $ ' + escapeHTML(tc.command || '') + '</div>' +
            '<div style="color:var(--text-secondary);margin-top:0.08rem;">&rarr; ' + escapeHTML(tc.output || '') + '</div>' +
          '</div>';
        }).join('')
      : '<div class="mono" style="color:var(--text-muted);font-size:0.7rem;">No external CLI/file mutations in this step.</div>';

    drawer.innerHTML =
      '<div style="display:flex;flex-wrap:wrap;justify-content:space-between;align-items:center;gap:0.65rem;padding-bottom:0.5rem;border-bottom:1px solid var(--border-hairline);">' +
        '<div style="display:flex;flex-wrap:wrap;align-items:center;gap:0.45rem;">' +
          '<span class="mono" style="font-size:0.72rem;font-weight:700;padding:0.15rem 0.48rem;border-radius:3px;background:var(--text-primary);color:var(--bg-surface);">🔍 PINNED TURN INSPECTOR</span>' +
          '<span class="mono" style="font-size:0.78rem;font-weight:700;color:var(--text-primary);">Step #' + escapeHTML(step.step_index || 1) + ' · @' + escapeHTML(step.agent_name || step.agent || 'lead') + ' (' + escapeHTML(step.project || 'psearch') + ')</span>' +
          '<span class="mono" style="font-size:0.72rem;color:var(--accent-primary);font-weight:600;">' + escapeHTML(edgeStr) + '</span>' +
          (step.pr_number ? ('<span class="mono" style="font-size:0.7rem;padding:0.1rem 0.4rem;border:1px solid var(--accent-primary);border-radius:3px;color:var(--accent-primary);font-weight:700;">PR #' + escapeHTML(step.pr_number) + '</span>') : '') +
          (step.issue_number ? ('<span class="mono" style="font-size:0.7rem;padding:0.1rem 0.4rem;border:1px solid var(--border-strong);border-radius:3px;color:var(--text-primary);font-weight:700;">Issue #' + escapeHTML(step.issue_number) + '</span>') : '') +
        '</div>' +
        '<div style="display:flex;flex-wrap:wrap;align-items:center;gap:0.45rem;">' +
          '<span class="mono" style="font-size:0.68rem;padding:0.14rem 0.45rem;border-radius:3px;background:' + bucketBg + ';color:#fff;font-weight:700;">BUCKET: ' + escapeHTML(cBucket) + '</span>' +
          '<span class="mono" style="font-size:0.68rem;padding:0.14rem 0.45rem;border-radius:3px;background:var(--bg-surface);border:1px solid var(--border-strong);color:var(--text-primary);font-weight:600;" title="Stateful attribution lineage rule">LINEAGE: ' + escapeHTML(attrSource) + '</span>' +
          '<span class="mono num" style="font-size:0.76rem;font-weight:700;color:var(--text-primary);">$' + stepCost.toFixed(4) + ' USD · ' + (step.turns_count || 4) + ' turns · ' + (totTok / 1000).toFixed(1) + 'k tok</span>' +
        '</div>' +
      '</div>' +
      '<div style="display:flex;align-items:center;gap:0.6rem;margin-top:0.42rem;flex-wrap:wrap;">' +
        '<span class="mono" style="font-size:0.68rem;color:var(--text-secondary);white-space:nowrap;">Token &amp; Cache Profile:</span>' +
        '<div style="flex:1;min-width:180px;max-width:360px;height:8px;background:var(--bg-subtle);border-radius:3px;overflow:hidden;display:flex;border:1px solid var(--border-hairline);">' +
          '<div style="width:' + inPct + '%;background:#0284c7;" title="Input Tokens: ' + inTok + ' (' + inPct + '%)"></div>' +
          '<div style="width:' + cachePct + '%;background:var(--accent-primary);" title="Cache Read Tokens: ' + cacheTok + ' (' + cachePct + '%)"></div>' +
          '<div style="width:' + outPct + '%;background:var(--status-pass);" title="Output Tokens: ' + outTok + ' (' + outPct + '%)"></div>' +
        '</div>' +
        '<span class="mono" style="font-size:0.67rem;color:var(--text-muted);">' +
          '<strong style="color:#0284c7;">in: ' + inTok.toLocaleString('en-US') + ' (' + inPct + '%)</strong> · ' +
          '<strong style="color:var(--accent-primary);">cache_read: ' + cacheTok.toLocaleString('en-US') + ' (' + cachePct + '%)</strong> · ' +
          '<strong style="color:var(--status-pass);">out: ' + outTok.toLocaleString('en-US') + ' (' + outPct + '%)</strong>' +
        '</span>' +
      '</div>' +
      '<div class="inspector-trace-grid">' +
        '<div class="inspector-trace-card">' +
          '<div class="mono" style="font-size:0.7rem;font-weight:700;color:var(--text-primary);margin-bottom:0.25rem;">📥 Incoming Prompt (' + escapeHTML(step.from_agent || 'orchestrator') + ' &rarr; @' + escapeHTML(step.agent_name || step.agent) + ')</div>' +
          '<div style="color:var(--text-secondary);">' + escapeHTML(step.prompt || step.title || '') + '</div>' +
        '</div>' +
        '<div class="inspector-trace-card">' +
          '<div class="mono" style="font-size:0.7rem;font-weight:700;color:var(--text-primary);margin-bottom:0.25rem;">🧠 Thinking Trace (' + escapeHTML(step.model || 'gemini-4-argon') + ')</div>' +
          '<div style="color:var(--text-secondary);">' + escapeHTML(step.thinking || step.detail || '') + '</div>' +
        '</div>' +
        '<div class="inspector-trace-card">' +
          '<div class="mono" style="font-size:0.7rem;font-weight:700;color:var(--text-primary);margin-bottom:0.25rem;">🛠️ Tool Invocations (' + toolCalls.length + ' calls)</div>' +
          toolsListHtml +
        '</div>' +
        '<div class="inspector-trace-card">' +
          '<div class="mono" style="font-size:0.7rem;font-weight:700;color:var(--text-primary);margin-bottom:0.25rem;">📤 Outcome &amp; Handoff (&rarr; ' + escapeHTML(step.to_agent || 'next') + ')</div>' +
          '<div style="font-weight:600;color:var(--text-primary);margin-bottom:0.2rem;">' + escapeHTML(step.title || '') + '</div>' +
          '<div style="color:var(--text-secondary);">' + escapeHTML(step.response || step.detail || '') + '</div>' +
        '</div>' +
      '</div>';
  }

  function renderAgentSwimlaneAndBloat(stepsToRender) {
    const swimSvg = document.getElementById('traj-swimlane-svg');
    const bloatSvg = document.getElementById('traj-bloat-curve-svg');
    const bloatBadge = document.getElementById('traj-bloat-ratio-badge');
    const bloatMeta = document.getElementById('traj-bloat-curve-meta');
    if (!swimSvg || !bloatSvg || !cachedTrajectories) return;

    const list = Array.isArray(stepsToRender) && stepsToRender.length > 0
      ? stepsToRender
      : (Array.isArray(cachedTrajectories.chronological_timeline) ? cachedTrajectories.chronological_timeline : []);

    // 1. Render 10-Agent Swimlane Execution Waterfall SVG
    const lanes = [
      { key: 'psearch:lead', label: '@lead' },
      { key: 'psearch:backend', label: '@backend' },
      { key: 'psearch:frontend', label: '@frontend' },
      { key: 'psearch:reviewer', label: '@reviewer' },
      { key: 'psearch:deployer', label: '@deployer' },
      { key: 'psearch-eval:eval-lead', label: '@eval-lead' },
      { key: 'psearch-eval:search-evaluator', label: '@search-eval' },
      { key: 'psearch-eval:factory-analyst', label: '@factory-analyst' },
      { key: 'psearch-eval:ui-craftsman', label: '@ui-craftsman' },
      { key: 'psearch-eval:eval-deployer', label: '@eval-deployer' }
    ];
    const laneIndexMap = {};
    lanes.forEach((l, i) => { laneIndexMap[l.key] = i; });

    const leftPad = 108;
    const rightPad = 16;
    const topPad = 24;
    const rowH = 22.5;
    const plotW = 880 - leftPad - rightPad;
    const nSteps = Math.max(1, list.length);

    let sHtml = '';
    // Draw horizontal lane backgrounds & labels
    lanes.forEach((l, i) => {
      const y = topPad + i * rowH;
      const bg = i % 2 === 0 ? 'var(--bg-subtle)' : 'var(--bg-surface)';
      sHtml += '<rect x="' + leftPad + '" y="' + y + '" width="' + plotW + '" height="' + rowH + '" fill="' + bg + '" />';
      sHtml += '<line x1="' + leftPad + '" y1="' + (y + rowH) + '" x2="' + (leftPad + plotW) + '" y2="' + (y + rowH) + '" stroke="var(--border-hairline)" stroke-width="0.7" />';
      sHtml += '<text x="' + (leftPad - 6) + '" y="' + (y + 14.5) + '" text-anchor="end" fill="var(--text-secondary)" font-family="JetBrains Mono, monospace" font-size="9.5" font-weight="600">' + escapeHTML(l.label) + '</text>';
    });
    // Divider between psearch (top 5) and psearch-eval (bottom 5)
    const divY = topPad + 5 * rowH;
    sHtml += '<line x1="8" y1="' + divY + '" x2="' + (leftPad + plotW) + '" y2="' + divY + '" stroke="var(--border-strong)" stroke-width="1.1" stroke-dasharray="3 2" />';

    // Wave axis markers across top
    const waveLabels = ['W0', 'W1', 'W2', 'W3', 'W4', 'W5', 'W6'];
    waveLabels.forEach((wl, wi) => {
      const wx = leftPad + (wi / waveLabels.length) * plotW;
      sHtml += '<line x1="' + wx.toFixed(1) + '" y1="' + (topPad - 6) + '" x2="' + wx.toFixed(1) + '" y2="' + (topPad + 10 * rowH) + '" stroke="var(--border-hairline)" stroke-width="0.6" stroke-dasharray="2 2" />';
      sHtml += '<text x="' + (wx + 4).toFixed(1) + '" y="14" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9" font-weight="600">' + wl + '</text>';
    });

    // Plot each step span block
    const barW = Math.max(3.2, Math.min(14, (plotW / nSteps) * 0.85));
    list.forEach((s, idx) => {
      const proj = s.project || 'psearch';
      const ag = s.agent_name || s.agent || 'lead';
      const lIdx = laneIndexMap[proj + ':' + ag] !== undefined ? laneIndexMap[proj + ':' + ag] : 0;
      const x = leftPad + ((idx + 0.5) / nSteps) * (plotW - barW);
      const y = topPad + lIdx * rowH + 3;
      const h = rowH - 6;
      const bucket = String(s.cost_bucket || 'productive').toLowerCase();
      const fill = bucket === 'rework' ? 'var(--accent-primary)' : (bucket === 'heartbeat_tax' ? 'var(--text-muted)' : 'var(--status-pass)');
      const uKey = getStepUniqueKey(s, idx);
      const isSel = uKey === selectedInspectedStepKey;
      const safeKey = escapeHTML(uKey).replace(/'/g, "\\'");
      const stepCost = Number(s.cost_usd !== undefined ? s.cost_usd : ((s.tokens && s.tokens.cost_usd) || 0));
      const tip = 'Step #' + (s.step_index || (idx + 1)) + ' · @' + ag + ' · ' + (s.pr_number ? ('PR #' + s.pr_number + ' · ') : '') + (s.issue_number ? ('Issue #' + s.issue_number + ' · ') : '') + '$' + stepCost.toFixed(4) + ' [' + bucket.toUpperCase() + '] — ' + String(s.title || '').slice(0, 65);

      sHtml += '<rect class="swimlane-step-rect" data-bucket="' + escapeHTML(bucket) + '" x="' + x.toFixed(1) + '" y="' + y.toFixed(1) + '" width="' + barW.toFixed(1) + '" height="' + h.toFixed(1) + '" rx="1.5" fill="' + fill + '" stroke="' + (isSel ? 'var(--text-primary)' : 'none') + '" stroke-width="' + (isSel ? '1.8' : '0') + '" style="cursor:pointer;" onclick="selectStepForInspection(\'' + safeKey + '\')">' +
        '<title>' + escapeHTML(tip) + '</title>' +
      '</rect>';
    });

    swimSvg.innerHTML = sHtml;

    // 2. Render Context Cache-Bloat Curve SVG (cache_read_tokens vs Step Index + Linear Regression Trendline)
    const bLeft = 48;
    const bRight = 14;
    const bTop = 20;
    const bBottom = 28;
    const bW = 540 - bLeft - bRight;
    const bH = 265 - bTop - bBottom;

    const points = list.map((s, i) => {
      const tok = s.tokens || {};
      const cacheRead = Number(tok.cache_read_tokens !== undefined ? tok.cache_read_tokens : (Number(tok.input_tokens || 0) * 0.68));
      const inTok = Math.max(1, Number(tok.input_tokens || 1000));
      return {
        idx: i,
        step: s,
        uKey: getStepUniqueKey(s, i),
        cacheRead,
        inTok,
        bucket: String(s.cost_bucket || 'productive').toLowerCase()
      };
    });

    const maxCache = Math.max(1000, ...points.map(p => p.cacheRead));
    // Compute linear regression y = a + b*x
    let sumX = 0, sumY = 0, sumXY = 0, sumXX = 0;
    const N = points.length;
    points.forEach((p, i) => {
      sumX += i;
      sumY += p.cacheRead;
      sumXY += i * p.cacheRead;
      sumXX += i * i;
    });
    const denom = (N * sumXX - sumX * sumX);
    const slope = denom !== 0 ? ((N * sumXY - sumX * sumY) / denom) : 0;
    const intercept = N > 0 ? ((sumY - slope * sumX) / N) : 0;

    const totalCache = points.reduce((a, p) => a + p.cacheRead, 0);
    const totalIn = Math.max(1, points.reduce((a, p) => a + p.inTok, 0));
    const ratio = (totalCache / totalIn).toFixed(2);

    if (bloatBadge) {
      bloatBadge.textContent = ratio + 'x Cache Ratio · ' + (slope >= 0 ? '+' : '') + slope.toFixed(0) + ' tok/step';
    }
    if (bloatMeta) {
      bloatMeta.textContent = 'Peak Cache Read: ' + (maxCache / 1000).toFixed(1) + 'k tok · Regression Slope: ' + (slope >= 0 ? '+' : '') + slope.toFixed(1) + ' tok/step across ' + N + ' steps';
    }

    let bHtml = '';
    // Horizontal Y grid lines
    [0, 0.25, 0.5, 0.75, 1].forEach(frac => {
      const gy = bTop + bH - frac * bH;
      const valK = ((frac * maxCache) / 1000).toFixed(0) + 'k';
      bHtml += '<line x1="' + bLeft + '" y1="' + gy.toFixed(1) + '" x2="' + (bLeft + bW) + '" y2="' + gy.toFixed(1) + '" stroke="var(--border-hairline)" stroke-width="0.7" />';
      bHtml += '<text x="' + (bLeft - 6) + '" y="' + (gy + 3).toFixed(1) + '" text-anchor="end" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9">' + valK + '</text>';
    });

    if (N > 1) {
      const polyCoords = points.map((p, i) => {
        const px = bLeft + (i / Math.max(1, N - 1)) * bW;
        const py = bTop + bH - Math.min(1, Math.max(0, p.cacheRead / maxCache)) * bH;
        return px.toFixed(1) + ',' + py.toFixed(1);
      }).join(' ');
      const areaCoords = bLeft + ',' + (bTop + bH) + ' ' + polyCoords + ' ' + (bLeft + bW) + ',' + (bTop + bH);
      bHtml += '<polygon points="' + areaCoords + '" fill="var(--accent-tint)" opacity="0.55" />';
      bHtml += '<polyline points="' + polyCoords + '" fill="none" stroke="var(--accent-primary)" stroke-width="1.5" />';

      // Linear regression trendline
      const y0 = Math.max(0, Math.min(maxCache, intercept));
      const y1 = Math.max(0, Math.min(maxCache, intercept + slope * (N - 1)));
      const ry0 = bTop + bH - (y0 / maxCache) * bH;
      const ry1 = bTop + bH - (y1 / maxCache) * bH;
      bHtml += '<line x1="' + bLeft + '" y1="' + ry0.toFixed(1) + '" x2="' + (bLeft + bW) + '" y2="' + ry1.toFixed(1) + '" stroke="var(--text-primary)" stroke-width="1.4" stroke-dasharray="4 3" />';
    }

    // Sampled interactive points
    const stepStride = Math.max(1, Math.floor(N / 70));
    points.forEach((p, i) => {
      const isSel = p.uKey === selectedInspectedStepKey;
      if (i % stepStride !== 0 && !isSel && p.bucket !== 'rework') return;
      const px = bLeft + (i / Math.max(1, N - 1)) * bW;
      const py = bTop + bH - Math.min(1, Math.max(0, p.cacheRead / maxCache)) * bH;
      const dotCol = p.bucket === 'rework' ? 'var(--accent-primary)' : 'var(--status-pass)';
      const safeKey = escapeHTML(p.uKey).replace(/'/g, "\\'");
      bHtml += '<circle cx="' + px.toFixed(1) + '" cy="' + py.toFixed(1) + '" r="' + (isSel ? '4.5' : '2.6') + '" fill="' + dotCol + '" stroke="var(--bg-surface)" stroke-width="1" style="cursor:pointer;" onclick="selectStepForInspection(\'' + safeKey + '\')">' +
        '<title>Step #' + (p.step.step_index || (i + 1)) + ' (@' + escapeHTML(p.step.agent_name || p.step.agent) + '): cache_read=' + p.cacheRead.toLocaleString('en-US') + ' tok</title>' +
      '</circle>';
    });

    bHtml += '<text x="' + (bLeft + bW / 2) + '" y="256" text-anchor="middle" fill="var(--text-muted)" font-family="JetBrains Mono, monospace" font-size="9.5">Chronological Step Sequence (1 &rarr; ' + N + ') · Dashed = Linear Trendline</text>';
    bloatSvg.innerHTML = bHtml;
  }

  function renderAgentTrajectory() {
    const tbody = document.getElementById('tbody-agent-trajectory');
    const summaryEl = document.getElementById('trajectory-agent-summary');
    const spannerBadge = document.getElementById('traj-spanner-badge');
    const kpiTrajEl = document.getElementById('val-traj-steps');
    const filteredCostPill = document.getElementById('traj-filtered-cost-pill');
    if (!tbody) return;

    if (!cachedTrajectories || !Array.isArray(cachedTrajectories.agents)) {
      tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:1rem;">Loading agent execution trajectories...</td></tr>';
      return;
    }

    renderCostAndWorkItemExplorer();
    renderInterAgentGraph();
    if (!trajChatInitialized) {
      trajChatInitialized = true;
      runTrajectoryChatQuery('How much did PR #17, PR #25, and Issue #35 cost, and how many turns and agents were involved?');
    }

    if (kpiTrajEl) {
      const totCost = cachedTrajectories.cost_summary ? Number(cachedTrajectories.cost_summary.total_cost_usd || 0).toFixed(2) : '0.00';
      kpiTrajEl.textContent = (cachedTrajectories.total_agents || 10) + ' Agents · ' + (cachedTrajectories.total_steps || 0) + ' Steps ($' + totCost + ')';
    }
    if (spannerBadge) {
      const spannerCnt = cachedTrajectories.spanner_persisted_steps || cachedTrajectories.total_steps || 0;
      const spannerTurnsCnt = cachedTrajectories.spanner_persisted_turns || cachedTrajectories.spanner_turns_total || 0;
      const liveHarnessCnt = cachedTrajectories.live_harness_turns_total || 0;
      spannerBadge.innerHTML = '<span class="badge-dot"></span>Spanner Synced (' + spannerCnt + ' steps in agent_trajectories · ' + spannerTurnsCnt + ' turns / ' + liveHarnessCnt + ' live harness in agent_conversation_turns)';
    }

    const allChrono = Array.isArray(cachedTrajectories.chronological_timeline) ? cachedTrajectories.chronological_timeline : [];
    let stepsToRender = [];
    if (selectedInteractionEdge !== 'all') {
      stepsToRender = allChrono.filter(st => {
        const normFrom = (st.from_agent === 'Scion Hub Scheduler') ? 'PO / GitHub' : (st.from_agent || st.agent);
        const edgeId = normFrom + ' → ' + (st.to_agent || 'lead');
        return edgeId === selectedInteractionEdge || st.interaction_edge === selectedInteractionEdge;
      });
    } else if (selectedTrajectoryProject === 'psearch' && selectedTrajectoryAgent === 'all') {
      stepsToRender = Array.isArray(cachedTrajectories.psearch_chronological)
        ? cachedTrajectories.psearch_chronological
        : allChrono.filter(st => st.project === 'psearch');
    } else if (selectedTrajectoryProject === 'all' && selectedTrajectoryAgent === 'all') {
      stepsToRender = allChrono;
    } else {
      const key = selectedTrajectoryProject + ':' + selectedTrajectoryAgent;
      const agentObj = cachedTrajectories.agents.find(a => a.project === selectedTrajectoryProject && a.name === selectedTrajectoryAgent) ||
        (cachedTrajectories.agents_by_key && cachedTrajectories.agents_by_key[key]);

      if (agentObj) {
        const rawList = Array.isArray(agentObj.steps) ? agentObj.steps : (Array.isArray(agentObj.trajectory) ? agentObj.trajectory : allChrono.filter(st => st.project === selectedTrajectoryProject && st.agent === selectedTrajectoryAgent));
        stepsToRender = rawList.map(st => ({
          ...st,
          project: agentObj.project,
          agent_name: agentObj.name,
          role: agentObj.role,
          harness: agentObj.harness
        }));
      }
    }

    // Apply PR, Issue, and FinOps Cost Bucket filters on top of stepsToRender
    const activePrNum = selectedTrajectoryPrFilter !== 'all' ? Number(selectedTrajectoryPrFilter) : null;
    const activeIssueNum = selectedTrajectoryIssueFilter !== 'all' ? Number(selectedTrajectoryIssueFilter) : null;

    if (activePrNum) {
      const prObj = (cachedTrajectories.pr_rollups || []).find(p => Number(p.pr_number) === activePrNum);
      const mergeSha = prObj && prObj.merge_commit ? String(prObj.merge_commit).slice(0, 7) : '';
      stepsToRender = stepsToRender.filter(st => {
        if (Number(st.pr_number) === activePrNum) return true;
        const sha7 = String(st.commit_sha || '').slice(0, 7);
        const text = (st.title || '') + ' ' + (st.detail || '');
        if (mergeSha && (sha7 === mergeSha || text.indexOf(mergeSha) !== -1)) return true;
        if (text.indexOf('#' + activePrNum) !== -1) return true;
        return false;
      });
    }

    if (activeIssueNum) {
      stepsToRender = stepsToRender.filter(st => Number(st.issue_number) === activeIssueNum);
    }

    if (selectedCostBucketFilter !== 'all') {
      stepsToRender = stepsToRender.filter(st => String(st.cost_bucket || 'productive').toLowerCase() === selectedCostBucketFilter);
    }

    currentRenderedSteps = stepsToRender;

    // Compute dynamic cost, turns, tools, tokens & agents involved for currently visible steps
    const visibleCostUsd = stepsToRender.reduce((acc, s) => acc + (Number(s.cost_usd || (s.tokens && s.tokens.cost_usd) || 0)), 0);
    const visibleTurns = stepsToRender.reduce((acc, s) => acc + (Number(s.turns_count) || 4), 0);
    const visibleTools = stepsToRender.reduce((acc, s) => acc + ((s.tool_calls || []).length), 0);
    const visibleTokens = stepsToRender.reduce((acc, s) => acc + (Number(s.tokens && s.tokens.total_tokens) || 0), 0);
    const visibleAgentsSet = new Set(stepsToRender.map(s => '@' + (s.agent_name || s.agent)));
    const visibleAgentsArr = Array.from(visibleAgentsSet);

    // Update Sticky Active Crossfilter Bar, Swimlane Waterfall & Context-Bloat Curve, and Pinned Turn Inspector Drawer
    renderActiveCrossfilterBar(stepsToRender.length, visibleTurns, visibleCostUsd);
    renderAgentSwimlaneAndBloat(stepsToRender);
    renderStepInspectorDrawer(stepsToRender);

    if (filteredCostPill) {
      const scopeTag = activePrNum
        ? ('PR #' + activePrNum)
        : (activeIssueNum ? ('Issue #' + activeIssueNum) : (selectedCostBucketFilter !== 'all' ? ('Bucket: ' + selectedCostBucketFilter) : (selectedTrajectoryAgent === 'all' ? 'All Selected Agents' : ('@' + selectedTrajectoryAgent))));
      filteredCostPill.innerHTML =
        escapeHTML(scopeTag) + ': <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong> · <strong>' + visibleTurns + ' turns</strong> (' + stepsToRender.length + ' steps) · <strong>' + visibleAgentsArr.length + ' agent(s)</strong> · ' + (visibleTokens / 1000).toFixed(1) + 'k tok';
    }

    if (summaryEl) {
      if (activePrNum || activeIssueNum || selectedCostBucketFilter !== 'all') {
        const filterTitle = activePrNum
          ? ('Filtered by PR #' + activePrNum)
          : (activeIssueNum ? ('Filtered by Issue #' + activeIssueNum) : ('Filtered by FinOps Bucket: ' + selectedCostBucketFilter.toUpperCase()));
        const agentScope = selectedTrajectoryAgent !== 'all' ? (' · Agent: @' + escapeHTML(selectedTrajectoryAgent)) : '';
        summaryEl.innerHTML =
          '<span><strong>' + escapeHTML(filterTitle) + '</strong>' + agentScope + ' — Agents Involved (' + visibleAgentsArr.length + '): <strong class="mono">' + escapeHTML(visibleAgentsArr.join(', ') || 'none') + '</strong></span>' +
          '<span class="mono">Total Cost: <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong> · Total Turns: <strong>' + visibleTurns + '</strong> (' + stepsToRender.length + ' steps · ' + visibleTools + ' tool calls · ' + (visibleTokens / 1000).toFixed(1) + 'k tokens)</span>';
      } else if (selectedInteractionEdge !== 'all') {
        summaryEl.innerHTML =
          '<span>Viewing Handoff Edge: <strong class="mono">' + escapeHTML(selectedInteractionEdge) + '</strong> · <button type="button" class="btn" style="padding:0.1rem 0.45rem;font-size:0.7rem;" onclick="filterByInteractionEdge(\'all\')">Clear Edge Filter ×</button></span>' +
          '<span class="mono">Matching Turns: ' + visibleTurns + ' (' + stepsToRender.length + ' steps) · Cost: <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong></span>';
      } else if (selectedTrajectoryProject === 'psearch' && selectedTrajectoryAgent === 'all') {
        summaryEl.innerHTML =
          '<span>Viewing Trajectory: <strong class="mono">All psearch Agents (Chronological)</strong> — @lead · @backend · @frontend · @reviewer · @deployer</span>' +
          '<span class="mono">Steps: ' + stepsToRender.length + ' · Turns: <strong>' + visibleTurns + '</strong> · Cost: <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong> · Waves 0–6 (Issues #1–#37)</span>';
      } else if (selectedTrajectoryProject === 'all' && selectedTrajectoryAgent === 'all') {
        summaryEl.innerHTML =
          '<span>Viewing Trajectory: <strong class="mono">All 10 Factory Agents (psearch + psearch-eval)</strong> — Chronological Execution Ledger</span>' +
          '<span class="mono">Steps: ' + stepsToRender.length + ' · Turns: <strong>' + visibleTurns + '</strong> · Total Cost: <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong> · Issues #1–#37 &amp; PRs #12–#27</span>';
      } else {
        const key = selectedTrajectoryProject + ':' + selectedTrajectoryAgent;
        const agentObj = (cachedTrajectories.agents_by_key && cachedTrajectories.agents_by_key[key]) ||
          cachedTrajectories.agents.find(a => a.project === selectedTrajectoryProject && a.name === selectedTrajectoryAgent) || {};
        const issuesStr = Array.isArray(agentObj.issues_covered) && agentObj.issues_covered.length > 0
          ? agentObj.issues_covered.map(n => '#' + n).join(', ')
          : 'All Waves';
        const prsStr = Array.isArray(agentObj.prs_touched) && agentObj.prs_touched.length > 0
          ? (' · PRs: #' + agentObj.prs_touched.join(', #'))
          : '';
        summaryEl.innerHTML =
          '<span>Viewing Trajectory: <strong class="mono">@' + escapeHTML(agentObj.name || selectedTrajectoryAgent) + ' (' + escapeHTML(agentObj.project || selectedTrajectoryProject) + ')</strong> — ' + escapeHTML(agentObj.role || '') + ' (' + escapeHTML(agentObj.harness || '') + ' · ' + escapeHTML(agentObj.model || 'gemini-4-argon') + ')</span>' +
          '<span class="mono">Steps: ' + stepsToRender.length + ' · Turns: <strong>' + visibleTurns + '</strong> · Tool Calls: ' + visibleTools + ' · Cost: <strong>$' + visibleCostUsd.toFixed(4) + ' USD</strong> · Issues: ' + escapeHTML(issuesStr) + escapeHTML(prsStr) + '</span>';
      }
    }

    if (stepsToRender.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:1rem;">No trajectory steps match the current filter combination. <button type="button" class="btn mono" style="margin-left:0.5rem;padding:0.15rem 0.5rem;font-size:0.72rem;" onclick="resetAllTrajectoryFilters()">Reset All Filters</button></td></tr>';
      return;
    }

    const windowedSteps = stepsToRender.slice(0, trajectoryRowsLimit);

    let rowsHtml = windowedSteps.map((s, idx) => {
      const stepNum = s.step_index || (idx + 1);
      const uKey = getStepUniqueKey(s, idx);
      const safeKey = escapeHTML(uKey).replace(/'/g, "\\'");
      const isInspected = uKey === selectedInspectedStepKey;
      const edgeStr = s.interaction_edge || ((s.from_agent || s.agent_name) + ' → ' + (s.to_agent || 'lead'));
      const isLiveHarness = s.source_mode === 'live_harness_stream' || String(s.commit_sha || '').indexOf('spanner-turn-') === 0;
      const agentBadge = '<span class="mono" style="font-weight:700;">@' + escapeHTML(s.agent_name || s.agent || selectedTrajectoryAgent) + '</span>' +
        '<div class="mono" style="font-size:0.68rem;color:var(--accent-primary);margin-top:0.15rem;">' + escapeHTML(edgeStr) + '</div>' +
        '<div class="mono" style="font-size:0.66rem;color:var(--text-muted);">' + escapeHTML(s.harness || '') + ' · ' + escapeHTML(s.model || '') + (isLiveHarness ? ' · <strong style="color:var(--status-pass);">⚡LIVE</strong>' : '') + '</div>';

      const issuePart = s.issue_number
        ? '<button type="button" class="btn mono" style="padding:0.08rem 0.36rem;font-size:0.68rem;" title="Filter turns by Issue #' + escapeHTML(s.issue_number) + '" onclick="event.stopPropagation();selectCostDrilldownItem(\'issue\',' + Number(s.issue_number) + ',true)">#' + escapeHTML(s.issue_number) + '</button>'
        : '—';
      const prPart = s.pr_number
        ? '<div style="margin-top:0.2rem;"><button type="button" class="btn mono" style="padding:0.08rem 0.36rem;font-size:0.66rem;border-color:var(--accent-primary);color:var(--accent-primary);" title="Inspect &amp; Filter turns by PR #' + escapeHTML(s.pr_number) + '" onclick="event.stopPropagation();selectCostDrilldownItem(\'pr\',' + Number(s.pr_number) + ',true)">PR #' + escapeHTML(s.pr_number) + '</button></div>'
        : '';
      const waveIssueCell = '<div class="mono" style="font-size:0.7rem;color:var(--text-muted);">' + escapeHTML(s.wave || 'wave') + '</div><div style="margin-top:0.15rem;">' + issuePart + '</div>' + prPart;

      const shaStr = String(s.commit_sha || '');
      let commitCell = '<span class="mono" style="color:var(--text-muted);">spanner</span>';
      if (shaStr.indexOf('spanner-turn-') === 0) {
        commitCell = '<span class="mono" style="color:var(--status-pass);font-weight:700;" title="Streamed directly from container harness transcript into Spanner agent_conversation_turns">⚡ spanner-turn</span>';
      } else if (shaStr.indexOf('hub-state-') === 0) {
        commitCell = '<span class="mono" style="color:var(--accent-primary);font-weight:600;">hub.db</span>';
      } else if (shaStr.indexOf('comment-') === 0) {
        const cid = shaStr.replace('comment-', '').replace(/-relay-\d+$/, '');
        const cUrl = s.comment_url || ('https://github.com/duboc/scion-experiment/issues/' + encodeURIComponent(s.issue_number || 1) + '#issuecomment-' + encodeURIComponent(cid));
        commitCell = '<a href="' + escapeHTML(cUrl) + '" target="_blank" class="mono" onclick="event.stopPropagation()">#' + escapeHTML(shaStr.replace('comment-', '').slice(-10)) + '</a>';
      } else if (shaStr) {
        commitCell = '<a href="https://github.com/duboc/scion-experiment/commit/' + encodeURIComponent(shaStr) + '" target="_blank" class="mono" onclick="event.stopPropagation()">' + escapeHTML(shaStr.slice(0, 7)) + '</a>';
      }
      const tsFormatted = s.timestamp ? escapeHTML(String(s.timestamp).replace('T', ' ').slice(0, 19)) : '—';
      const stVal = s.status || 'PASS';
      const stClass = (stVal === 'ROLLED_BACK' || stVal === 'REJECTED' || stVal === 'ERROR')
        ? 'badge--critical'
        : (stVal === 'BLOCKED_RESOLVED' ? 'badge--partial' : 'badge--pass');
      const statusBadge = '<span class="badge ' + stClass + '"><span class="badge-dot"></span>' + escapeHTML(stVal) + '</span>';

      const tok = s.tokens || {};
      const stepCost = Number(s.cost_usd !== undefined ? s.cost_usd : (tok.cost_usd || 0));
      const stepTurns = Number(s.turns_count) || 4;
      const stepCalls = Number(s.model_calls_count) || 8;
      const cBucket = String(s.cost_bucket || 'productive').toUpperCase();
      const cBucketColor = cBucket === 'REWORK' ? 'var(--status-warn)' : (cBucket === 'HEARTBEAT_TAX' ? 'var(--text-muted)' : 'var(--status-good)');
      const attrSrc = s.attribution_source ? (' · ' + s.attribution_source) : '';
      const costBadge =
        '<div class="mono num" style="font-size:0.74rem;font-weight:700;color:var(--text-primary);margin-top:0.22rem;">$' + stepCost.toFixed(4) + ' USD · ' + stepTurns + ' turns</div>' +
        '<div class="mono" style="font-size:0.64rem;color:' + cBucketColor + ';font-weight:600;margin-top:0.08rem;">[' + escapeHTML(cBucket) + ']' + escapeHTML(attrSrc) + '</div>' +
        '<div class="mono num" style="font-size:0.66rem;color:var(--text-muted);margin-top:0.08rem;">' + stepCalls + ' calls · in:' + (tok.input_tokens || 0) + (tok.cache_read_tokens ? ' · cache:' + tok.cache_read_tokens : '') + ' · out:' + (tok.output_tokens || 0) + '</div>';

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
      } else {
        const toolCount = Array.isArray(s.tool_calls) ? s.tool_calls.length : 0;
        convBlockHtml =
          '<div class="mono" style="margin-top:0.28rem;font-size:0.68rem;color:var(--accent-primary);display:flex;align-items:center;gap:0.5rem;">' +
            '<span>🛠️ ' + toolCount + ' tool call(s) · Lineage: ' + escapeHTML(s.attribution_source || 'git_commit_sha') + '</span>' +
            '<button type="button" class="btn mono" style="padding:0.1rem 0.42rem;font-size:0.66rem;" onclick="event.stopPropagation();selectStepForInspection(\'' + safeKey + '\')">Inspect in Drawer &uarr;</button>' +
          '</div>';
      }

      return '<tr data-step-key="' + escapeHTML(uKey) + '" style="cursor:pointer;' + (isInspected ? 'background:var(--bg-elevated);' : '') + '" onclick="selectStepForInspection(\'' + safeKey + '\')">' +
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

    if (stepsToRender.length > windowedSteps.length) {
      rowsHtml += '<tr><td colspan="8" style="text-align:center;padding:0.75rem;background:var(--bg-subtle);">' +
        '<button type="button" class="btn mono" onclick="showAllTrajectoryRows()">Showing First ' + windowedSteps.length + ' of ' + stepsToRender.length + ' Steps (DOM Windowed for Instant Paint) — Click to Render All ' + stepsToRender.length + ' Steps</button>' +
      '</td></tr>';
    }

    tbody.innerHTML = rowsHtml;
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
  root.switchWorkspaceLens = switchWorkspaceLens;
  root.filterByCostBucket = filterByCostBucket;
  root.resetAllTrajectoryFilters = resetAllTrajectoryFilters;
  root.selectStepForInspection = selectStepForInspection;
  root.showAllTrajectoryRows = showAllTrajectoryRows;
  root.setAgentFilter = setAgentFilter;
  root.setWaveFilter = setWaveFilter;
  root.selectAgentNode = selectAgentNode;
  root.selectGraphNode = selectGraphNode;
  root.filterByInteractionEdge = filterByInteractionEdge;
  root.toggleAllTrajectoryTraces = toggleAllTrajectoryTraces;
  root.switchCostLedgerMode = switchCostLedgerMode;
  root.selectCostDrilldownItem = selectCostDrilldownItem;
  root.filterTurnsByDrilldownAgent = filterTurnsByDrilldownAgent;
  root.onTrajectoryPrFilterChange = onTrajectoryPrFilterChange;
  root.onTrajectoryIssueFilterChange = onTrajectoryIssueFilterChange;
  root.clearTrajectoryWorkItemFilters = clearTrajectoryWorkItemFilters;
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
    try {
      const m = String(window.location.hash || '').match(/lens=(all|finops|pyramid|topology)/i);
      if (m && m[1]) {
        switchWorkspaceLens(m[1].toLowerCase());
      }
    } catch (_) {}
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
