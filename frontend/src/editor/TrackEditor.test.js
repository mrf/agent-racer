// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  authFetch: vi.fn(),
}));

vi.mock('../auth.js', () => ({
  authFetch: mocks.authFetch,
}));

import { TrackEditor } from './TrackEditor.js';

let canvas;
let ctx;
let editor;

function flushAsyncWork() {
  return Promise.resolve();
}

function makeCtx() {
  return {
    fillRect: vi.fn(),
    strokeRect: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    bezierCurveTo: vi.fn(),
    fillText: vi.fn(),
    set fillStyle(value) {},
    set strokeStyle(value) {},
    set lineWidth(value) {},
    set lineCap(value) {},
    set font(value) {},
    set textAlign(value) {},
    set textBaseline(value) {},
  };
}

function makeCanvas(context, overrides = {}) {
  return {
    width: 1600,
    height: 1200,
    style: {},
    getContext: vi.fn(() => context),
    getBoundingClientRect: vi.fn(() => ({
      left: 100,
      top: 50,
      width: 800,
      height: 600,
      right: 900,
      bottom: 650,
      x: 100,
      y: 50,
    })),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  document.body.innerHTML = '';
  Object.defineProperty(window, 'devicePixelRatio', {
    value: 2,
    configurable: true,
    writable: true,
  });
  ctx = makeCtx();
  canvas = makeCanvas(ctx);
  document.body.appendChild(canvas);
  editor = new TrackEditor(canvas);
  vi.stubGlobal('alert', vi.fn());
  vi.stubGlobal('prompt', vi.fn());
  mocks.authFetch.mockReset();
});

afterEach(() => {
  editor?.deactivate();
  editor = null;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.innerHTML = '';
});

describe('TrackEditor', () => {
  it('draws against logical canvas size instead of physical backing pixels', () => {
    editor.active = true;
    editor.width = 1;
    editor.height = 1;
    editor.tiles = [['']];

    editor.draw();

    expect(ctx.fillRect.mock.calls[0]).toEqual([0, 0, 800, 600]);
  });

  it('maps pointer coordinates through the DPR-scaled canvas space before picking a cell', () => {
    canvas.getBoundingClientRect = vi.fn(() => ({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
    }));

    expect(editor._cellAt({ clientX: 116, clientY: 66 })).toEqual({ row: 1, col: 1 });
  });
});

describe('TrackEditor save form', () => {
  it('renders an inline toolbar save form and reports save success without dialogs', async () => {
    mocks.authFetch.mockResolvedValue({ ok: true });
    editor.activate();

    const toolbar = document.getElementById('track-editor-toolbar');
    const form = document.getElementById('track-editor-save-form');
    const input = form.querySelector('input');
    const status = document.getElementById('track-editor-save-status');

    expect(toolbar.contains(form)).toBe(true);

    input.value = 'My Test Track';
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await flushAsyncWork();
    await flushAsyncWork();

    expect(mocks.authFetch).toHaveBeenCalledWith('/api/tracks', expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    }));
    expect(JSON.parse(mocks.authFetch.mock.calls[0][1].body)).toMatchObject({
      id: 'my-test-track',
      name: 'My Test Track',
    });
    expect(status.textContent).toBe('Saved: My Test Track');
    expect(window.prompt).not.toHaveBeenCalled();
    expect(window.alert).not.toHaveBeenCalled();
  });

  it('prefills the loaded track name and shows save failures inline', async () => {
    mocks.authFetch.mockResolvedValue({ ok: false, status: 500 });
    editor.activate();
    editor._loadTrack({
      id: 'oval-track',
      name: 'Oval Track',
      width: 2,
      height: 2,
      tiles: [
        ['start-line', 'finish-line'],
        ['', ''],
      ],
    });

    const form = document.getElementById('track-editor-save-form');
    const input = form.querySelector('input');
    const status = document.getElementById('track-editor-save-status');

    expect(input.value).toBe('Oval Track');

    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await flushAsyncWork();
    await flushAsyncWork();

    expect(mocks.authFetch).toHaveBeenCalledWith('/api/tracks/oval-track', expect.objectContaining({
      method: 'PUT',
    }));
    expect(status.textContent).toBe('Save failed: 500');
    expect(window.alert).not.toHaveBeenCalled();
  });
});
