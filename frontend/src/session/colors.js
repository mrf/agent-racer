export const MODEL_COLORS = {
  'claude-opus-4-5-20251101': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-opus-4-6': { main: '#a855f7', dark: '#7c3aed', light: '#c084fc', name: 'Opus' },
  'claude-sonnet-4-20250514': { main: '#3b82f6', dark: '#2563eb', light: '#60a5fa', name: 'Sonnet' },
  'claude-sonnet-4-5-20250929': { main: '#06b6d4', dark: '#0891b2', light: '#22d3ee', name: 'Sonnet' },
  'claude-sonnet-4-6': { main: '#0ea5e9', dark: '#0284c7', light: '#38bdf8', name: 'Sonnet' },
  'claude-haiku-3-5-20241022': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
  'claude-haiku-4-5-20251001': { main: '#22c55e', dark: '#16a34a', light: '#4ade80', name: 'Haiku' },
};

export const DEFAULT_COLOR = { main: '#6b7280', dark: '#4b5563', light: '#9ca3af', name: '?' };

export const SOURCE_COLORS = {
  claude: { bg: '#a855f7', label: 'C' },
  codex: { bg: '#10b981', label: 'X' },
  gemini: { bg: '#4285f4', label: 'G' },
};

export const DEFAULT_SOURCE = { bg: '#6b7280', label: '?' };

export function shortModelName(model) {
  if (!model) return '?';
  const parts = model.split(/[-_]/).filter(Boolean);
  if (parts.length === 0) return model.slice(0, 6).toUpperCase();
  if (parts[0] === 'claude') {
    if (!parts[1]) return 'CLAUDE';
    const family = parts[1].slice(0, 2).toUpperCase();
    if (parts.length >= 4) {
      return `${family}${parts[2]}.${parts[3]}`;
    }
    return parts[1].charAt(0).toUpperCase() + parts[1].slice(1);
  }
  if (parts[0] === 'gemini') {
    const version = parts[1] ? parts[1].replace(/[^0-9.]/g, '') : '';
    const tier = parts[2] ? parts[2][0].toUpperCase() : '';
    return `G${version}${tier}` || 'G';
  }
  if (parts[0].startsWith('o')) {
    return parts[0].toUpperCase();
  }
  if (parts[0] === 'gpt') {
    return `${parts[0].toUpperCase()}${parts[1] ? parts[1] : ''}`.slice(0, 6);
  }
  return parts[0].slice(0, 6).toUpperCase();
}

export function getModelColor(model, source) {
  if (MODEL_COLORS[model]) {
    return MODEL_COLORS[model];
  }

  if (model) {
    const lower = model.toLowerCase();
    if (lower.includes('opus')) {
      return { ...MODEL_COLORS['claude-opus-4-5-20251101'], name: 'Opus' };
    }
    if (lower.includes('sonnet')) {
      return { ...MODEL_COLORS['claude-sonnet-4-5-20250929'], name: 'Sonnet' };
    }
    if (lower.includes('haiku')) {
      return { ...MODEL_COLORS['claude-haiku-4-5-20251001'], name: 'Haiku' };
    }
    if (lower.startsWith('gemini')) {
      const GEMINI_COLOR = { main: '#4285f4', dark: '#2b5fc2', light: '#6aa6ff' };
      const tier = lower.includes('pro') ? 'Pro' : lower.includes('flash') ? 'Flash' : 'Gemini';
      return { ...GEMINI_COLOR, name: tier };
    }
    if (lower.includes('codex') || lower.startsWith('o') || lower.startsWith('gpt')) {
      const CODEX_COLOR = { main: '#10b981', dark: '#059669', light: '#34d399' };
      return { ...CODEX_COLOR, name: shortModelName(model) };
    }
    return { ...DEFAULT_COLOR, name: shortModelName(model) };
  }

  if (source) {
    return { ...DEFAULT_COLOR, name: source.toUpperCase() };
  }
  return DEFAULT_COLOR;
}

export function hexToRgb(hex) {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return { r, g, b };
}

export function lightenHex(hex, amount) {
  const { r, g, b } = hexToRgb(hex);
  return `rgb(${Math.min(255, r + amount)},${Math.min(255, g + amount)},${Math.min(255, b + amount)})`;
}
