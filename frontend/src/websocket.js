export class RaceConnection {
  constructor({ onSnapshot, onDelta, onCompletion, onStatus, onSourceHealth }) {
    this.onSnapshot = onSnapshot;
    this.onDelta = onDelta;
    this.onCompletion = onCompletion;
    this.onStatus = onStatus;
    this.onSourceHealth = onSourceHealth || (() => {});
    this.ws = null;
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.reconnectAttempts = 0;
    this.reconnectTimeoutId = null;
  }

  connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const params = new URLSearchParams(location.search);
    const token = params.get('token');
    const tokenQuery = token ? `?token=${encodeURIComponent(token)}` : '';
    const url = `${protocol}//${location.host}/ws${tokenQuery}`;

    this.onStatus('connecting');
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      this.reconnectDelay = 1000;
      this.onStatus('connected');
    };

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        switch (msg.type) {
          case 'snapshot':
            this.onSnapshot(msg.payload);
            break;
          case 'delta':
            this.onDelta(msg.payload);
            break;
          case 'completion':
            this.onCompletion(msg.payload);
            break;
          case 'source_health':
            this.onSourceHealth(msg.payload);
            break;
        }
      } catch (err) {
        console.error('WS parse error:', err);
      }
    };

    this.ws.onclose = () => {
      this.onStatus('disconnected');
      this.scheduleReconnect();
    };

    this.ws.onerror = () => {
      this.onStatus('disconnected');
    };
  }

  scheduleReconnect() {
    this.reconnectAttempts++;
    const delay = Math.min(
      this.reconnectDelay * Math.pow(1.5, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    );
    this.reconnectTimeoutId = setTimeout(() => this.connect(), delay);
  }

  disconnect() {
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}
