import { pickTemplate, fillTemplate } from './templates.js';
import { TERMINAL_ACTIVITIES } from '../session/constants.js';

function sessionName(session) {
  return session.name || session.id;
}

/**
 * Priority levels for commentary triggers.
 * Higher priority items can interrupt the cooldown for lower ones.
 */
const PRIORITY = {
  completion: 10,
  error: 10,
  overtake: 8,
  context_90: 7,
  subagent_spawn: 6,
  session_start: 5,
  compaction: 5,
  context_50: 4,
  high_burn: 3,
  tool_use: 2,
  idle_long: 1,
};

const COOLDOWN_MS = 5000;
const IDLE_THRESHOLD_MS = 60000;     // 60s before "long idle" commentary
const HIGH_BURN_TOKENS_PER_SEC = 500; // tokens/sec threshold for "high burn"

export class CommentaryEngine {
  constructor() {
    /** @type {Array<{text: string, priority: number, time: number}>} */
    this._queue = [];
    this._lastEmitTime = 0;
    this._currentMessage = null;

    // Per-session tracking state
    this._prevPositions = new Map();   // id -> rank position
    this._crossedContext50 = new Set(); // session ids that hit 50%
    this._crossedContext90 = new Set(); // session ids that hit 90%
    this._knownSessions = new Set();    // session ids we've seen
    this._prevTokens = new Map();       // id -> { tokens, time }
    this._idleCommented = new Set();    // session ids we've commented as idle
    this._prevSubagentCounts = new Map(); // id -> subagent count

    /** Callback: (message: string) => void */
    this.onMessage = null;
  }

  /**
   * Called every frame or on data update with the current sessions map.
   * Detects events and queues commentary.
   */
  processUpdate(sessions) {
    const now = Date.now();
    const active = [];

    for (const session of sessions.values()) {
      if (TERMINAL_ACTIVITIES.has(session.activity)) continue;

      const name = sessionName(session);

      // Track new sessions
      if (!this._knownSessions.has(session.id)) {
        this._knownSessions.add(session.id);
        this._enqueue('session_start', { name });
      }

      // Context utilization milestones
      const util = session.contextUtilization || 0;
      if (util >= 0.5 && !this._crossedContext50.has(session.id)) {
        this._crossedContext50.add(session.id);
        this._enqueue('context_50', { name });
      }
      if (util >= 0.9 && !this._crossedContext90.has(session.id)) {
        this._crossedContext90.add(session.id);
        this._enqueue('context_90', { name });
      }

      // High burn rate detection
      const prevTokenData = this._prevTokens.get(session.id);
      const currentTokens = session.tokensUsed || 0;
      if (prevTokenData) {
        const dtSec = (now - prevTokenData.time) / 1000;
        if (dtSec > 0.5) {
          const rate = (currentTokens - prevTokenData.tokens) / dtSec;
          if (rate > HIGH_BURN_TOKENS_PER_SEC) {
            this._enqueue('high_burn', { name });
          }
        }
      }
      this._prevTokens.set(session.id, { tokens: currentTokens, time: now });

      // Long idle detection
      if (session.lastDataReceivedAt) {
        const idleMs = now - new Date(session.lastDataReceivedAt).getTime();
        if (idleMs > IDLE_THRESHOLD_MS && !this._idleCommented.has(session.id)) {
          this._idleCommented.add(session.id);
          this._enqueue('idle_long', { name });
        } else if (idleMs < IDLE_THRESHOLD_MS) {
          this._idleCommented.delete(session.id);
        }
      }

      // Subagent spawn detection
      const subagentCount = session.subAgents ? session.subAgents.length : 0;
      const prevCount = this._prevSubagentCounts.get(session.id) || 0;
      if (subagentCount > prevCount) {
        this._enqueue('subagent_spawn', { name });
      }
      this._prevSubagentCounts.set(session.id, subagentCount);

      active.push(session);
    }

    // Overtake detection: compare position rankings by contextUtilization
    const ranked = active
      .slice()
      .sort((a, b) => (b.contextUtilization || 0) - (a.contextUtilization || 0));

    for (let i = 0; i < ranked.length; i++) {
      const session = ranked[i];
      const prevRank = this._prevPositions.get(session.id);
      if (prevRank !== undefined && prevRank > i && i < ranked.length - 1) {
        const passed = ranked[i + 1];
        this._enqueue('overtake', {
          name: sessionName(session),
          other: sessionName(passed),
        });
      }
      this._prevPositions.set(session.id, i);
    }

    // Clean up tracking for removed sessions
    for (const id of this._knownSessions) {
      if (!sessions.has(id)) {
        this._cleanup(id);
      }
    }

    // Flush queue
    this._flush(now);
  }

  /**
   * Called when a session completes or errors (from WebSocket completion event).
   */
  onCompletion(sessionId, name, activity) {
    const trigger = activity === 'complete' ? 'completion' : 'error';
    this._enqueue(trigger, { name: name || sessionId });
  }

  /**
   * Called when a tool_use activity starts for a session.
   */
  onToolUse(name) {
    this._enqueue('tool_use', { name });
  }

  /**
   * Called on compaction detection (e.g., token count drops significantly).
   */
  onCompaction(name) {
    this._enqueue('compaction', { name });
  }

  /**
   * Returns the current message to display, or null if none.
   */
  getCurrentMessage() {
    return this._currentMessage;
  }

  /**
   * Clear the current message (e.g., after display timeout).
   */
  clearMessage() {
    this._currentMessage = null;
  }

  _enqueue(trigger, vars) {
    const template = pickTemplate(trigger);
    if (!template) return;
    const text = fillTemplate(template, vars);
    const priority = PRIORITY[trigger] || 0;
    this._queue.push({ text, priority, time: Date.now() });
    // Sort by priority descending, then by time ascending
    this._queue.sort((a, b) => b.priority - a.priority || a.time - b.time);
    if (this._queue.length > 10) {
      this._queue.length = 10;
    }
  }

  _flush(now) {
    if (this._queue.length === 0) return;

    const top = this._queue[0];
    const timeSinceLastEmit = now - this._lastEmitTime;

    // Allow high-priority events to bypass cooldown
    const canEmit = timeSinceLastEmit >= COOLDOWN_MS || top.priority >= 8;

    if (canEmit) {
      this._queue.shift();
      this._currentMessage = top.text;
      this._lastEmitTime = now;
      if (this.onMessage) this.onMessage(top.text);
    }
  }

  _cleanup(id) {
    this._knownSessions.delete(id);
    this._crossedContext50.delete(id);
    this._crossedContext90.delete(id);
    this._prevPositions.delete(id);
    this._prevTokens.delete(id);
    this._idleCommented.delete(id);
    this._prevSubagentCounts.delete(id);
  }
}
