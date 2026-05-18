import { formatTokens, formatBurnRate, formatTime, formatElapsed, basename, esc } from './formatters.js';

function contextBarColor(utilization) {
  if (utilization > 0.8) return '#e94560';
  if (utilization > 0.5) return '#d97706';
  return '#22c55e';
}

function renderDetailContent(state) {
  const pct = (state.contextUtilization * 100).toFixed(1);

  return `
    <div class="detail-row" data-row="activity">
      <span class="label">Activity</span>
      <span class="value" data-field="activity"><span class="detail-activity ${esc(state.activity)}">${esc(state.activity)}</span></span>
    </div>
    ${state.isChurning ? `<div class="detail-row" data-section="churning">
      <span class="label">Process</span>
      <span class="value"><span class="detail-activity thinking">CPU Active</span></span>
    </div>` : ''}
    <div class="detail-progress">
      <div class="detail-progress-bar" data-field="progress-bar"></div>
      <span class="detail-progress-label" data-field="progress-label">${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)</span>
    </div>
    <div class="detail-row">
      <span class="label">Burn Rate</span>
      <span class="value burn-rate" data-field="burn-rate">${formatBurnRate(state.burnRatePerMinute)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Model</span>
      <span class="value">${esc(state.model) || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Source</span>
      <span class="value"><span class="source-badge${state.source ? ` source-${esc(state.source)}` : ''}">${esc(state.source) || 'unknown'}</span></span>
    </div>
    <div class="detail-row">
      <span class="label">Working Dir</span>
      <span class="value" title="${esc(state.workingDir) || ''}">${
        state.workingDir ? esc(basename(state.workingDir)) : '-'
      }</span>
    </div>
    <div class="detail-row">
      <span class="label">Branch</span>
      <span class="value">${esc(state.branch) || '-'}</span>
    </div>
    <div class="detail-row" data-row="tmux">
      <span class="label">Tmux</span>
      <span class="value">${state.tmuxTarget ? `${esc(state.tmuxTarget)} <span class="tmux-hint">(click car to jump)</span>` : 'not in tmux'}</span>
    </div>
    ${state.slug ? `<div class="detail-row" data-section="slug">
      <span class="label">Session Name</span>
      <span class="value">${esc(state.slug)}</span>
    </div>` : ''}
    <div class="detail-row">
      <span class="label">Session ID</span>
      <span class="value session-id-value">
        <span class="session-id-text" title="${esc(state.id)}">${esc(state.id).slice(0, 12)}</span>
        <button class="copy-btn" data-copy="${esc(state.id)}" title="Copy full ID">&#x2398;</button>
      </span>
    </div>
    <div class="detail-row">
      <span class="label">PID</span>
      <span class="value">${state.pid || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Messages</span>
      <span class="value" data-field="messages">${state.messageCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tool Calls</span>
      <span class="value" data-field="tool-calls">${state.toolCallCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Current Tool</span>
      <span class="value" data-field="current-tool">${esc(state.currentTool) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Started</span>
      <span class="value">${formatTime(state.startedAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Last Activity</span>
      <span class="value" data-field="last-activity">${formatTime(state.lastActivityAt)}</span>
    </div>
    <div class="detail-row" data-row="elapsed">
      <span class="label">Elapsed</span>
      <span class="value" data-field="elapsed">${formatElapsed(state.startedAt)}</span>
    </div>
    ${state.completedAt ? `
    <div class="detail-row" data-section="completed">
      <span class="label">Completed</span>
      <span class="value" data-field="completed">${formatTime(state.completedAt)}</span>
    </div>` : ''}
    <div class="detail-row">
      <span class="label">Input Tokens</span>
      <span class="value" data-field="input-tokens">${formatTokens(state.tokensUsed)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Max Tokens</span>
      <span class="value">${formatTokens(state.maxContextTokens)}</span>
    </div>
    <div class="detail-row" data-row="context-pct">
      <span class="label">Context %</span>
      <span class="value" data-field="context-pct">${pct}%</span>
    </div>
    ${(state.subagents && state.subagents.length > 0) ? `
    <div class="detail-row detail-section-divider" data-section="subagents-header">
      <span class="label detail-section-header" data-field="subagents-header">Subagents (${state.subagents.length})</span>
    </div>
    ${state.subagents.map((sub, i) => `
    <div class="detail-row" data-section="sub-${i}">
      <span class="label">${esc(sub.slug || sub.id)}</span>
      <span class="value" data-field="sub-${i}"><span class="detail-activity ${esc(sub.activity)}">${esc(sub.activity)}</span>${sub.currentTool ? ' · ' + esc(sub.currentTool) : ''}</span>
    </div>`).join('')}` : ''}
  `;
}

function renderHamsterContent(hamsterState, parentState) {
  return `
    <div class="detail-row detail-row-spaced">
      <span class="label detail-subagent-title">Subagent: ${esc(hamsterState.slug || hamsterState.id)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Activity</span>
      <span class="value" data-field="h-activity"><span class="detail-activity ${esc(hamsterState.activity)}">${esc(hamsterState.activity)}</span></span>
    </div>
    <div class="detail-row">
      <span class="label">Model</span>
      <span class="value">${esc(hamsterState.model) || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Current Tool</span>
      <span class="value" data-field="h-current-tool">${esc(hamsterState.currentTool) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Messages</span>
      <span class="value" data-field="h-messages">${hamsterState.messageCount || 0}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tool Calls</span>
      <span class="value" data-field="h-tool-calls">${hamsterState.toolCallCount || 0}</span>
    </div>
    <div class="detail-row">
      <span class="label">Duration</span>
      <span class="value" data-field="h-duration">${formatElapsed(hamsterState.startedAt)}</span>
    </div>
    <div class="detail-row detail-section-divider">
      <span class="label">Parent</span>
      <span class="value" title="${esc(parentState.workingDir) || ''}">${
        parentState.workingDir ? esc(basename(parentState.workingDir)) : esc(parentState.name) || '-'
      }</span>
    </div>
  `;
}

function patchText(container, field, value) {
  const el = container.querySelector(`[data-field="${field}"]`);
  if (el && el.textContent !== value) el.textContent = value;
}

function patchHtml(container, field, html) {
  const el = container.querySelector(`[data-field="${field}"]`);
  if (el && el.innerHTML !== html) el.innerHTML = html;
}

function createRow(section, labelText, valueHtml) {
  const row = document.createElement('div');
  row.className = 'detail-row';
  row.setAttribute('data-section', section);
  row.innerHTML = `<span class="label">${labelText}</span><span class="value">${valueHtml}</span>`;
  return row;
}

function toggleSection(container, section, shouldExist, anchorRow, labelText, valueHtml) {
  const existing = container.querySelector(`[data-section="${section}"]`);
  if (shouldExist && !existing) {
    const anchor = container.querySelector(`[data-row="${anchorRow}"]`);
    if (anchor) {
      const row = createRow(section, labelText, valueHtml);
      anchor.after(row);
    }
  } else if (!shouldExist && existing) {
    existing.remove();
  }
}

function patchDetailContent(container, state) {
  const pct = (state.contextUtilization * 100).toFixed(1);
  const barColor = contextBarColor(state.contextUtilization);

  patchHtml(container, 'activity',
    `<span class="detail-activity ${esc(state.activity)}">${esc(state.activity)}</span>`);

  // Structural: churning row (inserted after activity row)
  toggleSection(container, 'churning', state.isChurning, 'activity',
    'Process', '<span class="detail-activity thinking">CPU Active</span>');

  const bar = container.querySelector('[data-field="progress-bar"]');
  if (bar) {
    const w = `${pct}%`;
    if (bar.style.width !== w) bar.style.width = w;
    if (bar.style.background !== barColor) bar.style.background = barColor;
  }
  patchText(container, 'progress-label',
    `${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)`);

  patchText(container, 'burn-rate', formatBurnRate(state.burnRatePerMinute));
  patchText(container, 'messages', String(state.messageCount));
  patchText(container, 'tool-calls', String(state.toolCallCount));
  patchText(container, 'current-tool', state.currentTool || '-');
  patchText(container, 'last-activity', formatTime(state.lastActivityAt));
  patchText(container, 'elapsed', formatElapsed(state.startedAt));
  patchText(container, 'input-tokens', formatTokens(state.tokensUsed));
  patchText(container, 'context-pct', `${pct}%`);

  // Structural: completed row (inserted after elapsed row)
  const completedEl = container.querySelector('[data-section="completed"]');
  if (state.completedAt && !completedEl) {
    const anchor = container.querySelector('[data-row="elapsed"]');
    if (anchor) {
      const row = createRow('completed', 'Completed', `<span data-field="completed">${formatTime(state.completedAt)}</span>`);
      anchor.after(row);
    }
  } else if (!state.completedAt && completedEl) {
    completedEl.remove();
  } else if (state.completedAt && completedEl) {
    patchText(container, 'completed', formatTime(state.completedAt));
  }

  // Structural: slug row (inserted after tmux row)
  const slugEl = container.querySelector('[data-section="slug"]');
  if (state.slug && !slugEl) {
    const anchor = container.querySelector('[data-row="tmux"]');
    if (anchor) {
      const row = createRow('slug', 'Session Name', esc(state.slug));
      anchor.after(row);
    }
  } else if (!state.slug && slugEl) {
    slugEl.remove();
  } else if (state.slug && slugEl) {
    const valueEl = slugEl.querySelector('.value');
    if (valueEl && valueEl.textContent !== state.slug) valueEl.textContent = state.slug;
  }

  // Structural: subagent rows
  const subagents = state.subagents || [];
  const headerEl = container.querySelector('[data-section="subagents-header"]');

  if (subagents.length > 0) {
    if (!headerEl) {
      // Create header and all subagent rows
      const anchor = container.querySelector('[data-row="context-pct"]');
      if (anchor) {
        const header = document.createElement('div');
        header.className = 'detail-row detail-section-divider';
        header.setAttribute('data-section', 'subagents-header');
        header.innerHTML = `<span class="label detail-section-header" data-field="subagents-header">Subagents (${subagents.length})</span>`;
        anchor.after(header);
        let prev = header;
        for (let i = 0; i < subagents.length; i++) {
          const sub = subagents[i];
          const row = document.createElement('div');
          row.className = 'detail-row';
          row.setAttribute('data-section', `sub-${i}`);
          row.innerHTML = `<span class="label">${esc(sub.slug || sub.id)}</span><span class="value" data-field="sub-${i}"><span class="detail-activity ${esc(sub.activity)}">${esc(sub.activity)}</span>${sub.currentTool ? ' \u00b7 ' + esc(sub.currentTool) : ''}</span>`;
          prev.after(row);
          prev = row;
        }
      }
    } else {
      // Update header count
      patchText(container, 'subagents-header', `Subagents (${subagents.length})`);
      // Reconcile subagent rows
      for (let i = 0; i < subagents.length; i++) {
        const sub = subagents[i];
        const existing = container.querySelector(`[data-section="sub-${i}"]`);
        if (existing) {
          // Update label (slug may change) and value
          const label = existing.querySelector('.label');
          const newLabel = sub.slug || sub.id;
          if (label && label.textContent !== newLabel) label.textContent = newLabel;
          patchHtml(container, `sub-${i}`,
            `<span class="detail-activity ${esc(sub.activity)}">${esc(sub.activity)}</span>${sub.currentTool ? ' \u00b7 ' + esc(sub.currentTool) : ''}`);
        } else {
          // Insert new subagent row after the last existing one or header
          const prev = container.querySelector(`[data-section="sub-${i - 1}"]`) || headerEl;
          const row = document.createElement('div');
          row.className = 'detail-row';
          row.setAttribute('data-section', `sub-${i}`);
          row.innerHTML = `<span class="label">${esc(sub.slug || sub.id)}</span><span class="value" data-field="sub-${i}"><span class="detail-activity ${esc(sub.activity)}">${esc(sub.activity)}</span>${sub.currentTool ? ' \u00b7 ' + esc(sub.currentTool) : ''}</span>`;
          prev.after(row);
        }
      }
      // Remove excess subagent rows
      let idx = subagents.length;
      let excess = container.querySelector(`[data-section="sub-${idx}"]`);
      while (excess) {
        excess.remove();
        idx++;
        excess = container.querySelector(`[data-section="sub-${idx}"]`);
      }
    }
  } else if (headerEl) {
    // Remove all subagent elements
    headerEl.remove();
    let idx = 0;
    let row = container.querySelector(`[data-section="sub-${idx}"]`);
    while (row) {
      row.remove();
      idx++;
      row = container.querySelector(`[data-section="sub-${idx}"]`);
    }
  }
}

function patchHamsterContent(container, hamsterState) {
  patchHtml(container, 'h-activity',
    `<span class="detail-activity ${esc(hamsterState.activity)}">${esc(hamsterState.activity)}</span>`);
  patchText(container, 'h-current-tool', hamsterState.currentTool || '-');
  patchText(container, 'h-messages', String(hamsterState.messageCount || 0));
  patchText(container, 'h-tool-calls', String(hamsterState.toolCallCount || 0));
  patchText(container, 'h-duration', formatElapsed(hamsterState.startedAt));
}

export function createFlyout({ detailFlyout, flyoutContent, canvas }) {
  let selectedSessionId = null;
  let selectedHamsterId = null;
  let flyoutAnchor = null;
  let flyoutCurrentX = null;
  let flyoutCurrentY = null;
  let lastMode = null;       // 'detail' | 'hamster'

  function positionFlyout(carX, carY) {
    if (!isVisible()) return;
    const canvasRect = canvas.getBoundingClientRect();
    const margin = 50;
    const padding = 10;

    const flyoutWidth = detailFlyout.offsetWidth || 380;
    const flyoutHeight = detailFlyout.offsetHeight || 200;

    const carVX = canvasRect.left + carX;
    const carVY = canvasRect.top + carY;

    const canFit = (anchor) => {
      switch (anchor) {
        case 'right': return carVX + margin + flyoutWidth + padding < window.innerWidth;
        case 'left':  return carVX - margin - flyoutWidth > padding;
        case 'below': return carVY + margin + flyoutHeight + padding < window.innerHeight;
        case 'above': return carVY - margin - flyoutHeight > padding;
        default: return false;
      }
    };

    const preferredOrder = ['right', 'left', 'below', 'above'];
    if (!flyoutAnchor || !canFit(flyoutAnchor)) {
      flyoutAnchor = preferredOrder.find(canFit) || 'right';
    }

    let targetX, targetY, arrowClass;

    switch (flyoutAnchor) {
      case 'right':
        targetX = carVX + margin;
        targetY = carVY - flyoutHeight / 2;
        arrowClass = 'arrow-left';
        break;
      case 'left':
        targetX = carVX - margin - flyoutWidth;
        targetY = carVY - flyoutHeight / 2;
        arrowClass = 'arrow-right';
        break;
      case 'below':
        targetX = carVX - flyoutWidth / 2;
        targetY = carVY + margin;
        arrowClass = 'arrow-up';
        break;
      case 'above':
        targetX = carVX - flyoutWidth / 2;
        targetY = carVY - margin - flyoutHeight;
        arrowClass = 'arrow-down';
        break;
    }

    targetX = Math.max(padding, Math.min(window.innerWidth - flyoutWidth - padding, targetX));
    targetY = Math.max(padding, Math.min(window.innerHeight - flyoutHeight - padding, targetY));

    if (flyoutCurrentX === null) {
      flyoutCurrentX = targetX;
      flyoutCurrentY = targetY;
    } else {
      const smoothing = 0.25;
      flyoutCurrentX += (targetX - flyoutCurrentX) * smoothing;
      flyoutCurrentY += (targetY - flyoutCurrentY) * smoothing;
      if (Math.abs(flyoutCurrentX - targetX) < 1) flyoutCurrentX = targetX;
      if (Math.abs(flyoutCurrentY - targetY) < 1) flyoutCurrentY = targetY;
    }

    detailFlyout.style.left = `${Math.round(flyoutCurrentX)}px`;
    detailFlyout.style.top = `${Math.round(flyoutCurrentY)}px`;

    if (!detailFlyout.classList.contains(arrowClass)) {
      detailFlyout.classList.remove('arrow-left', 'arrow-right', 'arrow-up', 'arrow-down');
      detailFlyout.classList.add(arrowClass);
    }
  }

  function resetPosition() {
    flyoutAnchor = null;
    flyoutCurrentX = null;
    flyoutCurrentY = null;
  }

  function show(state, carX, carY) {
    selectedSessionId = state.id;
    selectedHamsterId = null;
    resetPosition();
    flyoutContent.innerHTML = renderDetailContent(state);
    patchDetailContent(flyoutContent, state);
    lastMode = 'detail';
    detailFlyout.classList.remove('hidden');
    positionFlyout(carX, carY);
  }

  function showHamster(hamsterState, parentState, hamsterX, hamsterY) {
    selectedSessionId = parentState.id;
    selectedHamsterId = hamsterState.id;
    resetPosition();
    flyoutContent.innerHTML = renderHamsterContent(hamsterState, parentState);
    lastMode = 'hamster';
    detailFlyout.classList.remove('hidden');
    positionFlyout(hamsterX, hamsterY);
  }

  function hide() {
    detailFlyout.classList.add('hidden');
    selectedSessionId = null;
    selectedHamsterId = null;
    lastMode = null;
    resetPosition();
  }

  function updateContent(sessions) {
    if (!selectedSessionId || !sessions.has(selectedSessionId)) return;
    const state = sessions.get(selectedSessionId);

    if (selectedHamsterId) {
      const sub = (state.subagents || []).find(s => s.id === selectedHamsterId);
      if (sub) {
        if (lastMode !== 'hamster') {
          flyoutContent.innerHTML = renderHamsterContent(sub, state);
          lastMode = 'hamster';
        }
        patchHamsterContent(flyoutContent, sub);
      } else {
        selectedHamsterId = null;
        if (lastMode !== 'detail') {
          flyoutContent.innerHTML = renderDetailContent(state);
          lastMode = 'detail';
        }
        patchDetailContent(flyoutContent, state);
      }
    } else {
      if (lastMode !== 'detail') {
        flyoutContent.innerHTML = renderDetailContent(state);
        lastMode = 'detail';
      }
      patchDetailContent(flyoutContent, state);
    }
  }

  function isVisible() {
    return !detailFlyout.classList.contains('hidden');
  }

  function getSelectedSessionId() {
    return selectedSessionId;
  }

  function getSelectedHamsterId() {
    return selectedHamsterId;
  }

  return {
    show,
    showHamster,
    hide,
    updatePosition: positionFlyout,
    updateContent,
    isVisible,
    getSelectedSessionId,
    getSelectedHamsterId,
  };
}
