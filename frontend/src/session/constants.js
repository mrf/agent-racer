export const TERMINAL_ACTIVITIES = new Set(['complete', 'errored', 'lost']);
export const PIT_ACTIVITIES = new Set(['idle', 'waiting', 'starting']);
export const DEFAULT_CONTEXT_WINDOW = 200000;
export const DATA_FRESHNESS_MS = 30_000;

export function isTerminalActivity(activity) {
  return TERMINAL_ACTIVITIES.has(activity);
}
