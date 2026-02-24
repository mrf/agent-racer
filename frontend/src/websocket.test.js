import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RaceConnection } from './websocket.js';

class MockWebSocket {
  static instances = [];

  constructor(url) {
    this.url = url;
    this.onopen = null;
    this.onmessage = null;
    this.onclose = null;
    this.onerror = null;
    this.sentMessages = [];
    MockWebSocket.instances.push(this);
  }

  send(data) {
    this.sentMessages.push(data);
  }

  close() {
    this.closed = true;
  }

  simulateOpen() {
    this.onopen?.();
  }

  simulateMessage(data) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  simulateRawMessage(data) {
    this.onmessage?.({ data });
  }

  simulateClose() {
    this.onclose?.();
  }

  simulateError() {
    this.onerror?.();
  }
}

function createConnection(overrides = {}) {
  return new RaceConnection({
    onSnapshot: overrides.onSnapshot ?? vi.fn(),
    onDelta: overrides.onDelta ?? vi.fn(),
    onCompletion: overrides.onCompletion ?? vi.fn(),
    onStatus: overrides.onStatus ?? vi.fn(),
    authToken: overrides.authToken,
  });
}

function latestSocket() {
  const { instances } = MockWebSocket;
  return instances[instances.length - 1];
}

describe('RaceConnection', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
    vi.stubGlobal('location', {
      protocol: 'https:',
      host: 'example.com',
      search: '',
    });
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  describe('onStatus callbacks', () => {
    it('fires "connecting" immediately on connect()', () => {
      const onStatus = vi.fn();
      const conn = createConnection({ onStatus });

      conn.connect();

      expect(onStatus).toHaveBeenCalledWith('connecting');
    });

    it('fires "connected" on WebSocket open', () => {
      const onStatus = vi.fn();
      const conn = createConnection({ onStatus });

      conn.connect();
      latestSocket().simulateOpen();

      expect(onStatus).toHaveBeenCalledWith('connected');
    });

    it('fires "disconnected" on WebSocket close', () => {
      const onStatus = vi.fn();
      const conn = createConnection({ onStatus });

      conn.connect();
      latestSocket().simulateClose();

      expect(onStatus).toHaveBeenCalledWith('disconnected');
    });

    it('fires "disconnected" on WebSocket error', () => {
      const onStatus = vi.fn();
      const conn = createConnection({ onStatus });

      conn.connect();
      latestSocket().simulateError();

      expect(onStatus).toHaveBeenCalledWith('disconnected');
    });
  });

  describe('message parsing', () => {
    it('dispatches snapshot messages to onSnapshot', () => {
      const onSnapshot = vi.fn();
      const conn = createConnection({ onSnapshot });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateMessage({ type: 'snapshot', payload: { cars: [1, 2] } });

      expect(onSnapshot).toHaveBeenCalledWith({ cars: [1, 2] });
    });

    it('dispatches delta messages to onDelta', () => {
      const onDelta = vi.fn();
      const conn = createConnection({ onDelta });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateMessage({ type: 'delta', payload: { pos: 5 } });

      expect(onDelta).toHaveBeenCalledWith({ pos: 5 });
    });

    it('dispatches completion messages to onCompletion', () => {
      const onCompletion = vi.fn();
      const conn = createConnection({ onCompletion });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateMessage({ type: 'completion', payload: { winner: 'a' } });

      expect(onCompletion).toHaveBeenCalledWith({ winner: 'a' });
    });

    it('ignores unknown message types without error', () => {
      const onSnapshot = vi.fn();
      const onDelta = vi.fn();
      const onCompletion = vi.fn();
      const conn = createConnection({ onSnapshot, onDelta, onCompletion });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateMessage({ type: 'unknown', payload: {} });

      expect(onSnapshot).not.toHaveBeenCalled();
      expect(onDelta).not.toHaveBeenCalled();
      expect(onCompletion).not.toHaveBeenCalled();
    });
  });

  describe('malformed JSON handling', () => {
    it('logs error and does not throw on invalid JSON', () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const onSnapshot = vi.fn();
      const conn = createConnection({ onSnapshot });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateRawMessage('not valid json{{{');

      expect(consoleSpy).toHaveBeenCalledWith('WS parse error:', expect.any(SyntaxError));
      expect(onSnapshot).not.toHaveBeenCalled();
    });

    it('continues processing messages after a malformed one', () => {
      vi.spyOn(console, 'error').mockImplementation(() => {});
      const onSnapshot = vi.fn();
      const conn = createConnection({ onSnapshot });

      conn.connect();
      latestSocket().simulateOpen();
      latestSocket().simulateRawMessage('broken');
      latestSocket().simulateMessage({ type: 'snapshot', payload: { ok: true } });

      expect(onSnapshot).toHaveBeenCalledWith({ ok: true });
    });
  });

  describe('exponential backoff', () => {
    it('schedules first reconnect at 1s (base delay)', () => {
      const conn = createConnection();

      conn.connect();
      latestSocket().simulateClose();

      expect(MockWebSocket.instances).toHaveLength(1);
      vi.advanceTimersByTime(1000);
      expect(MockWebSocket.instances).toHaveLength(2);
    });

    it('increases delay with factor 1.5 on each attempt', () => {
      const conn = createConnection();

      // Attempt 1: connect and close → 1s delay
      conn.connect();
      MockWebSocket.instances[0].simulateClose();
      vi.advanceTimersByTime(999);
      expect(MockWebSocket.instances).toHaveLength(1);
      vi.advanceTimersByTime(1);
      expect(MockWebSocket.instances).toHaveLength(2);

      // Attempt 2: close → 1.5s delay (1000 * 1.5^1)
      MockWebSocket.instances[1].simulateClose();
      vi.advanceTimersByTime(1499);
      expect(MockWebSocket.instances).toHaveLength(2);
      vi.advanceTimersByTime(1);
      expect(MockWebSocket.instances).toHaveLength(3);

      // Attempt 3: close → 2.25s delay (1000 * 1.5^2)
      MockWebSocket.instances[2].simulateClose();
      vi.advanceTimersByTime(2249);
      expect(MockWebSocket.instances).toHaveLength(3);
      vi.advanceTimersByTime(1);
      expect(MockWebSocket.instances).toHaveLength(4);
    });

    it('caps delay at 30s', () => {
      const conn = createConnection();

      conn.connect();

      // Close repeatedly to push delay past 30s
      // 1000 * 1.5^(n-1) > 30000 when n >= ~9
      for (let i = 0; i < 20; i++) {
        latestSocket().simulateClose();
        vi.advanceTimersByTime(30000);
      }

      const countBefore = MockWebSocket.instances.length;
      // Next reconnect: close again, should fire at exactly 30s
      latestSocket().simulateClose();
      vi.advanceTimersByTime(29999);
      expect(MockWebSocket.instances).toHaveLength(countBefore);
      vi.advanceTimersByTime(1);
      expect(MockWebSocket.instances).toHaveLength(countBefore + 1);
    });

    it('resets backoff after successful connection', () => {
      const conn = createConnection();

      conn.connect();

      // Close a few times to increase delay
      MockWebSocket.instances[0].simulateClose();
      vi.advanceTimersByTime(1000);
      MockWebSocket.instances[1].simulateClose();
      vi.advanceTimersByTime(1500);

      // Successful open resets backoff
      MockWebSocket.instances[2].simulateOpen();
      MockWebSocket.instances[2].simulateClose();

      // Should be back to 1s delay
      vi.advanceTimersByTime(999);
      expect(MockWebSocket.instances).toHaveLength(3);
      vi.advanceTimersByTime(1);
      expect(MockWebSocket.instances).toHaveLength(4);
    });
  });

  describe('connect() URL construction', () => {
    it('uses wss: for https: protocol', () => {
      const conn = createConnection();
      conn.connect();

      expect(latestSocket().url).toBe('wss://example.com/ws');
    });

    it('uses ws: for http: protocol', () => {
      location.protocol = 'http:';
      const conn = createConnection();
      conn.connect();

      expect(latestSocket().url).toBe('ws://example.com/ws');
    });

    it('sends auth message when authToken is provided', () => {
      const conn = createConnection({ authToken: 'abc123' });
      conn.connect();

      expect(latestSocket().url).toBe('wss://example.com/ws');
      latestSocket().simulateOpen();
      expect(latestSocket().sentMessages[0]).toBe(
        JSON.stringify({ type: 'auth', token: 'abc123' })
      );
    });

    it('does not send auth message when no authToken', () => {
      const conn = createConnection();
      conn.connect();

      latestSocket().simulateOpen();
      expect(latestSocket().sentMessages).toHaveLength(0);
    });
  });

  describe('disconnect()', () => {
    it('closes the WebSocket and nulls it', () => {
      const conn = createConnection();
      conn.connect();
      const ws = latestSocket();

      conn.disconnect();

      expect(ws.closed).toBe(true);
      expect(conn.ws).toBeNull();
    });

    it('cancels pending reconnect timer', () => {
      const conn = createConnection();
      conn.connect();
      latestSocket().simulateClose();

      conn.disconnect();
      vi.advanceTimersByTime(30000);

      expect(MockWebSocket.instances).toHaveLength(1);
    });

    it('is safe to call when not connected', () => {
      const conn = createConnection();
      expect(() => conn.disconnect()).not.toThrow();
    });
  });
});
