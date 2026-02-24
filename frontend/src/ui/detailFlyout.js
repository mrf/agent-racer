import { formatTokens, formatBurnRate, formatTime, formatElapsed, basename, esc } from './formatters.js';

function contextBarColor(utilization) {
  if (utilization > 0.8) return '#e94560';
  if (utilization > 0.5) return '#d97706';
  return '#22c55e';
}

function renderDetailContent(state) {
  const pct = (state.contextUtilization * 100).toFixed(1);
  const barColor = contextBarColor(state.contextUtilization);

  return `
    <div class="detail-row">
      <span class="label">Activity</span>
      <span class="value"><span class="detail-activity ${esc(state.activity)}">${esc(state.activity)}</span></span>
    </div>
    ${state.isChurning ? `<div class="detail-row">
      <span class="label">Process</span>
      <span class="value"><span class="detail-activity thinking">CPU Active</span></span>
    </div>` : ''}
    <div class="detail-progress">
      <div class="detail-progress-bar" style="width:${pct}%;background:${barColor}"></div>
      <span class="detail-progress-label">${formatTokens(state.tokensUsed)} / ${formatTokens(state.maxContextTokens)} (${pct}%)</span>
    </div>
    <div class="detail-row">
      <span class="label">Burn Rate</span>
      <span class="value burn-rate">${formatBurnRate(state.burnRatePerMinute)}</span>
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
    <div class="detail-row">
      <span class="label">Tmux</span>
      <span class="value">${state.tmuxTarget ? esc(state.tmuxTarget) : 'not in tmux'}</span>
    </div>
    ${state.slug ? `<div class="detail-row">
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
      <span class="value">${state.messageCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tool Calls</span>
      <span class="value">${state.toolCallCount}</span>
    </div>
    <div class="detail-row">
      <span class="label">Current Tool</span>
      <span class="value">${esc(state.currentTool) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Started</span>
      <span class="value">${formatTime(state.startedAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Last Activity</span>
      <span class="value">${formatTime(state.lastActivityAt)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Elapsed</span>
      <span class="value">${formatElapsed(state.startedAt)}</span>
    </div>
    ${state.completedAt ? `
    <div class="detail-row">
      <span class="label">Completed</span>
      <span class="value">${formatTime(state.completedAt)}</span>
    </div>` : ''}
    <div class="detail-row">
      <span class="label">Input Tokens</span>
      <span class="value">${formatTokens(state.tokensUsed)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Max Tokens</span>
      <span class="value">${formatTokens(state.maxContextTokens)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Context %</span>
      <span class="value">${pct}%</span>
    </div>
    ${(state.subagents && state.subagents.length > 0) ? `
    <div class="detail-row" style="margin-top:10px;padding-top:8px;border-top:1px solid #333">
      <span class="label" style="font-size:11px;font-weight:bold;color:#aaa">Subagents (${state.subagents.length})</span>
    </div>
    ${state.subagents.map(sub => `
    <div class="detail-row">
      <span class="label">${esc(sub.slug || sub.id)}</span>
      <span class="value"><span class="detail-activity ${esc(sub.activity)}">${esc(sub.activity)}</span>${sub.currentTool ? ' · ' + esc(sub.currentTool) : ''}</span>
    </div>`).join('')}` : ''}
  `;
}

function renderHamsterContent(hamsterState, parentState) {
  return `
    <div class="detail-row" style="margin-bottom:8px">
      <span class="label" style="font-size:13px;font-weight:bold;color:#ccc">Subagent: ${esc(hamsterState.slug || hamsterState.id)}</span>
    </div>
    <div class="detail-row">
      <span class="label">Activity</span>
      <span class="value"><span class="detail-activity ${esc(hamsterState.activity)}">${esc(hamsterState.activity)}</span></span>
    </div>
    <div class="detail-row">
      <span class="label">Model</span>
      <span class="value">${esc(hamsterState.model) || 'unknown'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Current Tool</span>
      <span class="value">${esc(hamsterState.currentTool) || '-'}</span>
    </div>
    <div class="detail-row">
      <span class="label">Messages</span>
      <span class="value">${hamsterState.messageCount || 0}</span>
    </div>
    <div class="detail-row">
      <span class="label">Tool Calls</span>
      <span class="value">${hamsterState.toolCallCount || 0}</span>
    </div>
    <div class="detail-row">
      <span class="label">Duration</span>
      <span class="value">${formatElapsed(hamsterState.startedAt)}</span>
    </div>
    <div class="detail-row" style="margin-top:10px;padding-top:8px;border-top:1px solid #333">
      <span class="label">Parent</span>
      <span class="value" title="${esc(parentState.workingDir) || ''}">${
        parentState.workingDir ? esc(basename(parentState.workingDir)) : esc(parentState.name) || '-'
      }</span>
    </div>
  `;
}

export function createFlyout({ detailFlyout, flyoutContent, canvas }) {
  let selectedSessionId = null;
  let selectedHamsterId = null;
  let flyoutAnchor = null;
  let flyoutCurrentX = null;
  let flyoutCurrentY = null;

  function positionFlyout(carX, carY) {
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

    const currentArrow = detailFlyout.className.match(/arrow-\w+/)?.[0];
    if (currentArrow !== arrowClass) {
      detailFlyout.className = detailFlyout.className.replace(/arrow-\w+/g, '').trim() + ` ${arrowClass}`;
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
    detailFlyout.classList.remove('hidden');
    positionFlyout(carX, carY);
  }

  function showHamster(hamsterState, parentState, hamsterX, hamsterY) {
    selectedSessionId = parentState.id;
    selectedHamsterId = hamsterState.id;
    resetPosition();
    flyoutContent.innerHTML = renderHamsterContent(hamsterState, parentState);
    detailFlyout.classList.remove('hidden');
    positionFlyout(hamsterX, hamsterY);
  }

  function hide() {
    detailFlyout.classList.add('hidden');
    selectedSessionId = null;
    selectedHamsterId = null;
    resetPosition();
  }

  function updateContent(sessions) {
    if (!selectedSessionId || !sessions.has(selectedSessionId)) return;
    const state = sessions.get(selectedSessionId);

    if (selectedHamsterId) {
      const sub = (state.subagents || []).find(s => s.id === selectedHamsterId);
      if (sub) {
        flyoutContent.innerHTML = renderHamsterContent(sub, state);
      } else {
        selectedHamsterId = null;
        flyoutContent.innerHTML = renderDetailContent(state);
      }
    } else {
      flyoutContent.innerHTML = renderDetailContent(state);
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
