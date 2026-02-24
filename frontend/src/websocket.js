export class RaceConnection {
  constructor({ onSnapshot, onDelta, onCompletion, onStatus, authToken, onSourceHealth, onAchievementUnlocked }) {
    this.onSnapshot = onSnapshot;
    this.onDelta = onDelta;
    this.onCompletion = onCompletion;
    this.onStatus = onStatus;
    this.authToken = authToken || '';
    this.onSourceHealth = onSourceHealth || (() => {});
    this.onAchievementUnlocked = onAchievementUnlocked || (() => {});
    this.ws = null;
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.reconnectAttempts = 0;
    this.reconnectTimeoutId = null;
    this.lastSeq = 0;
  }

  connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${location.host}/ws`;

    this.onStatus('connecting');
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      if (this.authToken) {
        this.ws.send(JSON.stringify({ type: 'auth', token: this.authToken }));
      }
      this.reconnectAttempts = 0;
      this.reconnectDelay = 1000;
      this.lastSeq = 0;
      this.onStatus('connected');
    };

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        const seq = msg.seq || 0;

        // Snapshots always reset the sequence baseline.
        if (msg.type === 'snapshot') {
          this.lastSeq = seq;
        } else if (seq && this.lastSeq && seq !== this.lastSeq + 1) {
          this.requestResync();
          return;
        } else {
          this.lastSeq = seq;
        }

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
          case 'achievement_unlocked':
            this.onAchievementUnlocked(msg.payload);
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

  requestResync() {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'resync' }));
    }
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
