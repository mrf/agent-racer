export function formatTokens(tokens) {
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

export function formatBurnRate(rate) {
  if (!rate || rate <= 0) return '-';
  if (rate >= 1000) return `${(rate / 1000).toFixed(1)}K/min`;
  return `${Math.round(rate)}/min`;
}

export function formatTime(dateStr) {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  return d.toLocaleTimeString();
}

export function formatElapsed(startStr) {
  if (!startStr) return '-';
  const start = new Date(startStr);
  const elapsed = Math.floor((Date.now() - start.getTime()) / 1000);
  const mins = Math.floor(elapsed / 60);
  const secs = elapsed % 60;
  return `${mins}m ${secs}s`;
}

export function basename(path) {
  return path.split('/').pop();
}

export function esc(s) {
  if (!s) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
