# Claude Racing Dashboard - Technical Implementation Specification

## Overview

A real-time monitoring interface for concurrent Claude Code sessions running in tmux, visualized as a racing game. Sessions are represented as racers competing to reach a finish line.

**Key insight:** This is fundamentally a process monitor with game aesthetics, not a game with monitoring features. Design decisions should prioritize accurate session state representation over gamification.

---

## Core Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              System Overview                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐     poll      ┌──────────────┐    WS     ┌─────────┐ │
│  │    tmux      │ ──────────>   │  Go Backend  │ ──────>   │ Browser │ │
│  │  (sessions)  │    500ms      │              │  deltas   │ (Canvas)│ │
│  └──────────────┘               └──────────────┘           └─────────┘ │
│         │                              │                        │      │
│         │                              │                        │      │
│         v                              v                        v      │
│  ┌──────────────┐               ┌──────────────┐        ┌───────────┐  │
│  │ Pane Content │               │ Session State│        │ Animation │  │
│  │   Buffer     │               │    Store     │        │   State   │  │
│  └──────────────┘               └──────────────┘        └───────────┘  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Session State Model

### The Activity Model (Not Progress)

**Problem:** Claude sessions have no predictable progress. A session might spend 90% of wall-clock time in the first tool call, then complete rapidly. Progress estimation is meaningless and misleading.

**Solution:** Model sessions as state machines with observable activity, not progress bars.

```go
type SessionActivity int

const (
    ActivityUnknown   SessionActivity = iota
    ActivityStarting                  // Just spawned, no output yet
    ActivityThinking                  // Claude is generating (streaming output)
    ActivityToolUse                   // Executing a tool (file write, bash, etc.)
    ActivityWaiting                   // Waiting for user input/permission
    ActivityIdle                      // No output for >N seconds, process alive
    ActivityComplete                  // Detected completion signal
    ActivityErrored                   // Detected error state
    ActivityLost                      // Process/pane disappeared
)
```

### Visual Mapping

Instead of horizontal position = progress, use visual states:

| Activity | Visual Representation |
|----------|----------------------|
| Starting | Car at starting line, engine revving animation |
| Thinking | Car moving forward with thought bubbles, variable speed |
| ToolUse | Car with sparks/tool icon, rapid movement burst |
| Waiting | Car stopped, hazard lights blinking |
| Idle | Car slowing down, exhaust fading |
| Complete | Car crosses finish, victory animation |
| Errored | Car spins out, smoke/fire effects |
| Lost | Car fades out with "?" indicator |

### Position Calculation

Position is derived from **cumulative activity**, not estimated progress:

```go
type SessionState struct {
    ID              string          `json:"id"`
    Name            string          `json:"name"`
    Activity        SessionActivity `json:"activity"`
    
    // Position is a function of completed work, not estimated remaining
    CompletedChunks int             `json:"completedChunks"` // Tool calls, file writes, etc.
    TotalOutputLen  int             `json:"totalOutputLen"`  // Cumulative output bytes
    
    // Derived position: normalized against other active sessions
    // Leader = rightmost, others scaled relative to leader
    RelativePosition float64        `json:"relativePosition"` // 0.0 to 1.0
    
    // Timing
    StartedAt       time.Time       `json:"startedAt"`
    LastActivityAt  time.Time       `json:"lastActivityAt"`
    CompletedAt     *time.Time      `json:"completedAt,omitempty"`
    
    // Metadata
    WorkingDir      string          `json:"workingDir,omitempty"`
    CurrentTool     string          `json:"currentTool,omitempty"` // "bash", "write", "read", etc.
}
```

**Position algorithm:**

```go
func (s *SessionStore) CalculatePositions() {
    // Find the "leader" - most cumulative work among active sessions
    var maxChunks int
    for _, sess := range s.sessions {
        if sess.Activity != ActivityComplete && sess.Activity != ActivityLost {
            if sess.CompletedChunks > maxChunks {
                maxChunks = sess.CompletedChunks
            }
        }
    }
    
    // Scale all sessions relative to leader
    for id, sess := range s.sessions {
        if sess.Activity == ActivityComplete {
            sess.RelativePosition = 1.0 // At finish line
        } else if maxChunks == 0 {
            sess.RelativePosition = 0.0 // No work done yet
        } else {
            // Logarithmic scaling prevents early chunks from dominating
            sess.RelativePosition = math.Log1p(float64(sess.CompletedChunks)) / 
                                    math.Log1p(float64(maxChunks)) * 0.9 // Cap at 90% until complete
        }
        s.sessions[id] = sess
    }
}
```

---

## Completion Detection

### Strategy Hierarchy

Completion detection uses a priority-ordered strategy chain. First match wins.

```go
type CompletionResult struct {
    Detected   bool
    Confidence float64 // 0.0 to 1.0
    Reason     string
}

type DetectorChain struct {
    detectors []Detector
}

func (c *DetectorChain) Check(session *SessionState, paneContent string) CompletionResult {
    for _, d := range c.detectors {
        result := d.Detect(session, paneContent)
        if result.Detected && result.Confidence >= d.MinConfidence() {
            return result
        }
    }
    return CompletionResult{Detected: false}
}
```

### Detector Implementations

#### 1. Explicit Marker Detector (Primary - Confidence: 1.0)

Sessions that want reliable tracking emit markers. This is the **recommended** approach.

```go
type MarkerDetector struct {
    Markers []string
}

var DefaultMarkers = []string{
    "⚡ RACE:DONE",      // Our custom marker
    "# CLAUDE_COMPLETE", // Alternative
}

func (d *MarkerDetector) Detect(sess *SessionState, content string) CompletionResult {
    for _, marker := range d.Markers {
        if strings.Contains(content, marker) {
            return CompletionResult{
                Detected:   true,
                Confidence: 1.0,
                Reason:     fmt.Sprintf("marker found: %s", marker),
            }
        }
    }
    return CompletionResult{Detected: false}
}

func (d *MarkerDetector) MinConfidence() float64 { return 1.0 }
```

**Usage:** Wrap Claude invocations to emit marker on exit:

```bash
#!/bin/bash
# claude-race-wrapper.sh
claude "$@"
EXIT_CODE=$?
if [ $EXIT_CODE -eq 0 ]; then
    echo "⚡ RACE:DONE"
else
    echo "⚡ RACE:ERROR:$EXIT_CODE"
fi
exit $EXIT_CODE
```

#### 2. Claude Code State Detector (Confidence: 0.95)

Parse Claude Code's specific output patterns.

```go
type ClaudeCodeDetector struct {
    // Claude Code emits these patterns
    exitPatterns []string
}

var claudeCodePatterns = []string{
    `>\s*$`,                           // Clean prompt waiting for input (ambiguous)
    `Completed in \d+`,                // Explicit completion message
    `Total cost:.*\$[\d.]+`,           // Cost summary = session end
    `Session complete`,                // Direct completion
}

var claudeCodeErrorPatterns = []string{
    `Error:`,
    `Failed to`,
    `APIError`,
    `rate limit`,
}

func (d *ClaudeCodeDetector) Detect(sess *SessionState, content string) CompletionResult {
    lines := getLastNLines(content, 20)
    
    // Check for explicit completion indicators
    for _, pattern := range claudeCodePatterns {
        if matched, _ := regexp.MatchString(pattern, lines); matched {
            // "Cost summary" is highest confidence
            if strings.Contains(lines, "Total cost:") {
                return CompletionResult{true, 0.95, "cost summary detected"}
            }
            // Clean prompt is lower confidence (could be waiting for input)
            if strings.HasSuffix(strings.TrimSpace(lines), ">") {
                // Check if idle for sufficient time
                if time.Since(sess.LastActivityAt) > 5*time.Second {
                    return CompletionResult{true, 0.7, "idle at prompt"}
                }
            }
        }
    }
    return CompletionResult{Detected: false}
}
```

#### 3. Shell Prompt Detector (Confidence: 0.6)

Fallback for non-Claude-Code processes.

```go
type ShellPromptDetector struct {
    // Common prompt patterns - intentionally conservative
    patterns []*regexp.Regexp
}

func NewShellPromptDetector() *ShellPromptDetector {
    return &ShellPromptDetector{
        patterns: []*regexp.Regexp{
            // Standard bash/zsh prompts
            regexp.MustCompile(`\n\$\s*$`),
            regexp.MustCompile(`\n%\s*$`),
            // Common custom prompts (conservative)
            regexp.MustCompile(`\n❯\s*$`),
            regexp.MustCompile(`\n➜\s*$`),
            // User-configurable via config
        },
    }
}

func (d *ShellPromptDetector) Detect(sess *SessionState, content string) CompletionResult {
    lines := getLastNLines(content, 5)
    
    for _, pattern := range d.patterns {
        if pattern.MatchString(lines) {
            // Only trigger if we've seen substantial activity first
            if sess.CompletedChunks < 3 {
                continue // Probably just started, not finished
            }
            // And if idle for a bit
            if time.Since(sess.LastActivityAt) < 3*time.Second {
                continue
            }
            return CompletionResult{true, 0.6, "shell prompt detected"}
        }
    }
    return CompletionResult{Detected: false}
}
```

#### 4. Idle Timeout Detector (Confidence: 0.4)

Last resort - no output for extended period.

```go
type IdleDetector struct {
    Threshold time.Duration // e.g., 30 seconds
}

func (d *IdleDetector) Detect(sess *SessionState, content string) CompletionResult {
    if sess.Activity == ActivityIdle && 
       time.Since(sess.LastActivityAt) > d.Threshold &&
       sess.CompletedChunks > 0 {
        return CompletionResult{
            Detected:   true,
            Confidence: 0.4,
            Reason:     fmt.Sprintf("idle for %v", time.Since(sess.LastActivityAt)),
        }
    }
    return CompletionResult{Detected: false}
}
```

### Error Detection

```go
type ErrorDetector struct {
    patterns []*regexp.Regexp
}

var defaultErrorPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)error:`),
    regexp.MustCompile(`(?i)fatal:`),
    regexp.MustCompile(`(?i)panic:`),
    regexp.MustCompile(`Traceback \(most recent call last\)`),
    regexp.MustCompile(`APIError`),
    regexp.MustCompile(`rate limit exceeded`),
    regexp.MustCompile(`CLAUDE_RACE:ERROR`),
}

func (d *ErrorDetector) Detect(sess *SessionState, content string) (bool, string) {
    lines := getLastNLines(content, 30)
    for _, pattern := range d.patterns {
        if match := pattern.FindString(lines); match != "" {
            return true, match
        }
    }
    return false, ""
}
```

---

## Tmux Integration

### Session Discovery

```go
type TmuxMonitor struct {
    pollInterval time.Duration
    cmdFilter    string // Process name to look for
    sessions     map[string]*TmuxSession
    mu           sync.RWMutex
}

type TmuxSession struct {
    SessionName  string
    WindowID     string
    WindowName   string
    PaneID       string
    PanePID      int
    PaneCommand  string
    PanePath     string // Working directory
    LastOutput   string
    OutputHash   uint64 // For change detection
}

func (m *TmuxMonitor) discoverSessions() ([]TmuxSession, error) {
    // Get all panes with their details
    // Format: session_name|window_id|window_name|pane_id|pane_pid|pane_current_command|pane_current_path
    cmd := exec.Command("tmux", "list-panes", "-a", "-F",
        "#{session_name}|#{window_id}|#{window_name}|#{pane_id}|#{pane_pid}|#{pane_current_command}|#{pane_current_path}")
    
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("tmux list-panes failed: %w", err)
    }
    
    var sessions []TmuxSession
    scanner := bufio.NewScanner(bytes.NewReader(output))
    
    for scanner.Scan() {
        parts := strings.Split(scanner.Text(), "|")
        if len(parts) != 7 {
            continue
        }
        
        sess := TmuxSession{
            SessionName: parts[0],
            WindowID:    parts[1],
            WindowName:  parts[2],
            PaneID:      parts[3],
            PaneCommand: parts[5],
            PanePath:    parts[6],
        }
        sess.PanePID, _ = strconv.Atoi(parts[4])
        
        // Filter: only track panes running our target command
        if m.matchesFilter(sess) {
            sessions = append(sessions, sess)
        }
    }
    
    return sessions, nil
}

func (m *TmuxMonitor) matchesFilter(sess TmuxSession) bool {
    // Match "claude" command or our wrapper
    return strings.Contains(sess.PaneCommand, "claude") ||
           strings.Contains(sess.WindowName, "claude") ||
           strings.HasPrefix(sess.WindowName, "race-") // Convention for tracked sessions
}
```

### Pane Content Capture

```go
func (m *TmuxMonitor) capturePaneContent(paneID string, lines int) (string, error) {
    // Capture visible content + scrollback
    cmd := exec.Command("tmux", "capture-pane", "-p", "-t", paneID, "-S", fmt.Sprintf("-%d", lines))
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    return string(output), nil
}

func (m *TmuxMonitor) captureRecentOutput(paneID string) (string, bool) {
    content, err := m.capturePaneContent(paneID, 100) // Last 100 lines
    if err != nil {
        return "", false
    }
    
    // Hash for change detection
    hash := xxhash.Sum64String(content)
    
    m.mu.Lock()
    sess, exists := m.sessions[paneID]
    changed := !exists || sess.OutputHash != hash
    if exists {
        sess.OutputHash = hash
        sess.LastOutput = content
    }
    m.mu.Unlock()
    
    return content, changed
}
```

### Activity Classification

```go
func (m *TmuxMonitor) classifyActivity(sess *TmuxSession, content string, changed bool) SessionActivity {
    if !changed {
        // No new output
        if sess.LastActivityAt.IsZero() {
            return ActivityStarting
        }
        idle := time.Since(sess.LastActivityAt)
        if idle > 10*time.Second {
            return ActivityIdle
        }
        return sess.CurrentActivity // Maintain previous state
    }
    
    // New output detected - classify it
    recentLines := getLastNLines(content, 10)
    
    // Tool use patterns (Claude Code specific)
    toolPatterns := map[string]*regexp.Regexp{
        "bash":  regexp.MustCompile(`Running: |Executing: |bash_tool`),
        "write": regexp.MustCompile(`Writing to |create_file|str_replace`),
        "read":  regexp.MustCompile(`Reading |view_file|Viewing:`),
        "web":   regexp.MustCompile(`web_search|web_fetch|Searching:`),
    }
    
    for tool, pattern := range toolPatterns {
        if pattern.MatchString(recentLines) {
            sess.CurrentTool = tool
            return ActivityToolUse
        }
    }
    
    // Waiting for input
    if strings.Contains(recentLines, "Allow?") ||
       strings.Contains(recentLines, "[y/n]") ||
       strings.Contains(recentLines, "Press Enter") {
        return ActivityWaiting
    }
    
    // Default: thinking/generating
    return ActivityThinking
}
```

### Chunk Counting

"Chunks" represent completed units of work for position calculation:

```go
type ChunkCounter struct {
    patterns map[string]*regexp.Regexp
}

func NewChunkCounter() *ChunkCounter {
    return &ChunkCounter{
        patterns: map[string]*regexp.Regexp{
            "tool_complete": regexp.MustCompile(`(?m)^✓|^Done|^Completed|^Created|^Updated`),
            "file_written":  regexp.MustCompile(`(?m)Wrote \d+ bytes|Created file|Updated file`),
            "command_done":  regexp.MustCompile(`(?m)^Process exited|exit code \d+`),
            "section":       regexp.MustCompile(`(?m)^#{1,3} `), // Markdown headers as progress
        },
    }
}

func (c *ChunkCounter) CountInContent(content string) int {
    seen := make(map[string]bool) // Dedupe
    count := 0
    
    for name, pattern := range c.patterns {
        matches := pattern.FindAllString(content, -1)
        for _, m := range matches {
            key := name + ":" + m
            if !seen[key] {
                seen[key] = true
                count++
            }
        }
    }
    
    return count
}
```

---

## WebSocket Protocol

### Delta Updates

Full state broadcast is wasteful. Send only changes.

```go
type WSMessage struct {
    Type      string    `json:"type"`
    Timestamp time.Time `json:"ts"`
    Payload   any       `json:"payload"`
}

// Message types
const (
    MsgTypeSnapshot = "snapshot" // Full state (on connect, periodic sync)
    MsgTypeDelta    = "delta"    // Incremental update
    MsgTypeComplete = "complete" // Session finished
    MsgTypeError    = "error"    // Session errored
    MsgTypeLost     = "lost"     // Session disappeared
)

type SnapshotPayload struct {
    Sessions []SessionState `json:"sessions"`
}

type DeltaPayload struct {
    SessionID string         `json:"id"`
    Changes   map[string]any `json:"changes"` // Only changed fields
}

type CompletionPayload struct {
    SessionID   string        `json:"id"`
    FinalState  SessionState  `json:"state"`
    Duration    time.Duration `json:"duration"`
    Confidence  float64       `json:"confidence"`
}
```

### Broadcast Logic

```go
type Broadcaster struct {
    clients    map[*websocket.Conn]bool
    register   chan *websocket.Conn
    unregister chan *websocket.Conn
    broadcast  chan WSMessage
    mu         sync.RWMutex
    
    // State tracking for delta computation
    lastState map[string]SessionState
}

func (b *Broadcaster) computeDelta(sessionID string, newState SessionState) *DeltaPayload {
    old, exists := b.lastState[sessionID]
    if !exists {
        return nil // Will send in next snapshot
    }
    
    changes := make(map[string]any)
    
    if old.Activity != newState.Activity {
        changes["activity"] = newState.Activity
    }
    if old.RelativePosition != newState.RelativePosition {
        changes["relativePosition"] = newState.RelativePosition
    }
    if old.CompletedChunks != newState.CompletedChunks {
        changes["completedChunks"] = newState.CompletedChunks
    }
    if old.CurrentTool != newState.CurrentTool {
        changes["currentTool"] = newState.CurrentTool
    }
    if !old.LastActivityAt.Equal(newState.LastActivityAt) {
        changes["lastActivityAt"] = newState.LastActivityAt
    }
    
    if len(changes) == 0 {
        return nil
    }
    
    b.lastState[sessionID] = newState
    return &DeltaPayload{SessionID: sessionID, Changes: changes}
}
```

---

## Frontend Architecture

### Why Canvas, Not Phaser

Phaser is overkill for this use case. We need:
- Smooth sprite movement
- Particle effects (exhaust, confetti)
- State-based animations

A lightweight Canvas approach with requestAnimationFrame provides this without 500KB+ of game engine:

```javascript
// frontend/src/main.js

class RaceCanvas {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.canvas = document.createElement('canvas');
        this.ctx = this.canvas.getContext('2d');
        this.container.appendChild(this.canvas);
        
        this.racers = new Map();
        this.particles = [];
        this.lastFrame = 0;
        
        this.resize();
        window.addEventListener('resize', () => this.resize());
    }
    
    resize() {
        const rect = this.container.getBoundingClientRect();
        this.canvas.width = rect.width * devicePixelRatio;
        this.canvas.height = rect.height * devicePixelRatio;
        this.canvas.style.width = rect.width + 'px';
        this.canvas.style.height = rect.height + 'px';
        this.ctx.scale(devicePixelRatio, devicePixelRatio);
        
        this.trackWidth = rect.width;
        this.laneHeight = Math.min(80, rect.height / 10);
    }
    
    start() {
        const loop = (timestamp) => {
            const dt = timestamp - this.lastFrame;
            this.lastFrame = timestamp;
            
            this.update(dt);
            this.render();
            
            requestAnimationFrame(loop);
        };
        requestAnimationFrame(loop);
    }
}
```

### Racer Entity

```javascript
// frontend/src/entities/Racer.js

class Racer {
    constructor(canvas, session) {
        this.canvas = canvas;
        this.id = session.id;
        this.name = session.name;
        
        // Visual state
        this.x = 50; // Starting position
        this.targetX = 50;
        this.y = 0;
        this.lane = 0;
        
        // Animation state
        this.activity = 'starting';
        this.wheelRotation = 0;
        this.exhaustParticles = [];
        this.shake = 0;
        
        // Colors by activity
        this.colors = {
            starting: '#888888',
            thinking: '#4CAF50',
            toolUse: '#FF9800',
            waiting: '#FFC107',
            idle: '#9E9E9E',
            complete: '#2196F3',
            errored: '#F44336',
            lost: '#424242'
        };
    }
    
    update(dt, state) {
        // Smooth position interpolation
        const targetX = this.calculateTargetX(state.relativePosition);
        this.x += (targetX - this.x) * Math.min(1, dt / 200);
        
        // Activity-based animations
        this.activity = state.activity;
        
        switch (this.activity) {
            case 'thinking':
                this.wheelRotation += dt * 0.01;
                this.addExhaust();
                break;
            case 'toolUse':
                this.wheelRotation += dt * 0.02;
                this.shake = Math.sin(Date.now() / 50) * 2;
                this.addSparks();
                break;
            case 'waiting':
                // Hazard light blink
                this.hazardOn = Math.floor(Date.now() / 500) % 2 === 0;
                break;
            case 'errored':
                this.shake = Math.sin(Date.now() / 30) * 5;
                this.addSmoke();
                break;
            case 'complete':
                this.playVictory();
                break;
        }
        
        // Update particles
        this.exhaustParticles = this.exhaustParticles.filter(p => {
            p.x -= dt * 0.05;
            p.alpha -= dt * 0.002;
            return p.alpha > 0;
        });
    }
    
    calculateTargetX(relativePosition) {
        const startX = 50;
        const finishX = this.canvas.trackWidth - 100;
        return startX + (finishX - startX) * relativePosition;
    }
    
    render(ctx) {
        ctx.save();
        ctx.translate(this.x + this.shake, this.y);
        
        // Draw exhaust particles
        this.exhaustParticles.forEach(p => {
            ctx.globalAlpha = p.alpha;
            ctx.fillStyle = '#888';
            ctx.beginPath();
            ctx.arc(p.x - 30, p.y, p.size, 0, Math.PI * 2);
            ctx.fill();
        });
        ctx.globalAlpha = 1;
        
        // Draw car body
        const color = this.colors[this.activity] || this.colors.thinking;
        this.drawCar(ctx, color);
        
        // Draw activity indicator
        this.drawActivityIndicator(ctx);
        
        // Draw label
        ctx.fillStyle = '#fff';
        ctx.font = '12px monospace';
        ctx.fillText(this.name, -20, -25);
        
        ctx.restore();
    }
    
    drawCar(ctx, color) {
        // Simple car shape
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.roundRect(-25, -10, 50, 20, 5);
        ctx.fill();
        
        // Windshield
        ctx.fillStyle = '#333';
        ctx.beginPath();
        ctx.roundRect(5, -8, 15, 10, 2);
        ctx.fill();
        
        // Wheels
        ctx.fillStyle = '#222';
        ctx.save();
        ctx.translate(-15, 10);
        ctx.rotate(this.wheelRotation);
        ctx.beginPath();
        ctx.arc(0, 0, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
        
        ctx.save();
        ctx.translate(15, 10);
        ctx.rotate(this.wheelRotation);
        ctx.beginPath();
        ctx.arc(0, 0, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
    }
    
    drawActivityIndicator(ctx) {
        switch (this.activity) {
            case 'thinking':
                // Thought bubble
                ctx.fillStyle = '#fff';
                ctx.font = '16px sans-serif';
                ctx.fillText('💭', 20, -15);
                break;
            case 'toolUse':
                ctx.fillText('⚡', 20, -15);
                break;
            case 'waiting':
                if (this.hazardOn) {
                    ctx.fillStyle = '#FFC107';
                    ctx.beginPath();
                    ctx.arc(25, 0, 5, 0, Math.PI * 2);
                    ctx.fill();
                }
                break;
            case 'complete':
                ctx.fillText('🏆', 30, -15);
                break;
            case 'errored':
                ctx.fillText('💥', 20, -15);
                break;
        }
    }
    
    addExhaust() {
        if (Math.random() > 0.7) {
            this.exhaustParticles.push({
                x: 0,
                y: 5 + Math.random() * 5,
                size: 2 + Math.random() * 3,
                alpha: 0.5
            });
        }
    }
    
    addSparks() {
        // Tool use sparks
        if (Math.random() > 0.8) {
            this.exhaustParticles.push({
                x: 20,
                y: Math.random() * 20 - 10,
                size: 1 + Math.random() * 2,
                alpha: 1,
                color: '#FFD700'
            });
        }
    }
    
    playVictory() {
        // Confetti burst (one-time trigger)
        if (!this.victoryPlayed) {
            this.victoryPlayed = true;
            for (let i = 0; i < 50; i++) {
                this.canvas.addConfetti(this.x, this.y);
            }
        }
    }
}
```

### WebSocket Client with Reconnection

```javascript
// frontend/src/websocket.js

class RaceConnection {
    constructor(raceCanvas, url = 'ws://localhost:8080/ws') {
        this.canvas = raceCanvas;
        this.url = url;
        this.reconnectAttempts = 0;
        this.maxReconnectDelay = 30000;
        
        this.connect();
    }
    
    connect() {
        this.ws = new WebSocket(this.url);
        
        this.ws.onopen = () => {
            console.log('Connected to race server');
            this.reconnectAttempts = 0;
            this.canvas.setConnectionStatus('connected');
        };
        
        this.ws.onclose = () => {
            console.log('Disconnected, reconnecting...');
            this.canvas.setConnectionStatus('disconnected');
            this.scheduleReconnect();
        };
        
        this.ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
        
        this.ws.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            this.handleMessage(msg);
        };
    }
    
    scheduleReconnect() {
        const delay = Math.min(
            1000 * Math.pow(2, this.reconnectAttempts),
            this.maxReconnectDelay
        );
        this.reconnectAttempts++;
        setTimeout(() => this.connect(), delay);
    }
    
    handleMessage(msg) {
        switch (msg.type) {
            case 'snapshot':
                this.canvas.setAllRacers(msg.payload.sessions);
                break;
                
            case 'delta':
                this.canvas.updateRacer(msg.payload.id, msg.payload.changes);
                break;
                
            case 'complete':
                this.canvas.onRacerComplete(msg.payload);
                this.showNotification(msg.payload);
                break;
                
            case 'error':
                this.canvas.onRacerError(msg.payload);
                break;
                
            case 'lost':
                this.canvas.onRacerLost(msg.payload.id);
                break;
        }
    }
    
    showNotification(payload) {
        if (!('Notification' in window)) return;
        
        if (Notification.permission === 'granted') {
            new Notification(`🏁 ${payload.state.name} finished!`, {
                body: `Completed in ${formatDuration(payload.duration)}`,
                icon: '/assets/trophy.png'
            });
        } else if (Notification.permission !== 'denied') {
            Notification.requestPermission();
        }
    }
}
```

---

## Session Lifecycle Management

### Handling Session Disappearance

```go
type SessionStore struct {
    sessions map[string]*SessionState
    mu       sync.RWMutex
    
    // Track last seen time for cleanup
    lastSeen map[string]time.Time
    
    // Grace period before marking lost
    lostThreshold time.Duration
}

func (s *SessionStore) ReconcileSessions(discovered []TmuxSession) []StateChange {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    var changes []StateChange
    now := time.Now()
    discoveredIDs := make(map[string]bool)
    
    // Process discovered sessions
    for _, tmuxSess := range discovered {
        id := tmuxSess.PaneID
        discoveredIDs[id] = true
        s.lastSeen[id] = now
        
        if _, exists := s.sessions[id]; !exists {
            // New session
            state := &SessionState{
                ID:        id,
                Name:      tmuxSess.WindowName,
                Activity:  ActivityStarting,
                StartedAt: now,
                WorkingDir: tmuxSess.PanePath,
            }
            s.sessions[id] = state
            changes = append(changes, StateChange{Type: "new", Session: state})
        }
    }
    
    // Check for disappeared sessions
    for id, state := range s.sessions {
        if !discoveredIDs[id] && state.Activity != ActivityLost && state.Activity != ActivityComplete {
            lastSeen := s.lastSeen[id]
            if now.Sub(lastSeen) > s.lostThreshold {
                state.Activity = ActivityLost
                changes = append(changes, StateChange{Type: "lost", Session: state})
            }
        }
    }
    
    return changes
}
```

### Completed Session Retention

Keep completed sessions visible for a configurable period:

```go
func (s *SessionStore) CleanupCompleted(retentionPeriod time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    now := time.Now()
    for id, state := range s.sessions {
        if state.Activity == ActivityComplete || state.Activity == ActivityLost {
            if state.CompletedAt != nil && now.Sub(*state.CompletedAt) > retentionPeriod {
                delete(s.sessions, id)
                delete(s.lastSeen, id)
            }
        }
    }
}
```

---

## Configuration

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  
tmux:
  poll_interval: 500ms
  content_lines: 100
  
  # Sessions to track
  filters:
    commands: ["claude", "claude-race-wrapper"]
    window_prefixes: ["race-", "claude-"]
    
detection:
  # Priority-ordered detector chain
  chain:
    - type: marker
      markers:
        - "⚡ RACE:DONE"
        - "# CLAUDE_COMPLETE"
      min_confidence: 1.0
      
    - type: claude_code
      min_confidence: 0.9
      
    - type: shell_prompt
      patterns:
        - '\n\$\s*$'
        - '\n%\s*$'
        - '\n❯\s*$'
      min_idle: 3s
      min_chunks: 3
      min_confidence: 0.6
      
    - type: idle
      threshold: 30s
      min_confidence: 0.4
      
  errors:
    patterns:
      - '(?i)error:'
      - '(?i)fatal:'
      - 'APIError'
      - 'rate limit'

sessions:
  lost_threshold: 5s
  completed_retention: 5m
  
notifications:
  browser: true
  sound: true
  webhooks: []
  
frontend:
  show_debug: false
  animation_speed: 1.0
  theme: "default"
```

---

## Project Structure (Revised)

```
claude-racer/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go           # Entry point
│   ├── internal/
│   │   ├── tmux/
│   │   │   ├── monitor.go        # Session discovery & polling
│   │   │   └── capture.go        # Pane content capture
│   │   ├── session/
│   │   │   ├── store.go          # Session state management
│   │   │   ├── activity.go       # Activity classification
│   │   │   └── position.go       # Position calculation
│   │   ├── detection/
│   │   │   ├── chain.go          # Detector chain
│   │   │   ├── marker.go         # Marker detector
│   │   │   ├── claudecode.go     # Claude Code patterns
│   │   │   ├── prompt.go         # Shell prompt detector
│   │   │   └── idle.go           # Idle timeout detector
│   │   ├── ws/
│   │   │   ├── server.go         # WebSocket server
│   │   │   ├── broadcast.go      # Client management & broadcasting
│   │   │   └── protocol.go       # Message types
│   │   └── config/
│   │       └── config.go         # Configuration loading
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── index.html
│   ├── src/
│   │   ├── main.js               # Canvas initialization
│   │   ├── canvas/
│   │   │   ├── RaceCanvas.js     # Main rendering
│   │   │   ├── Track.js          # Track background
│   │   │   └── Particles.js      # Particle system
│   │   ├── entities/
│   │   │   └── Racer.js          # Car entity
│   │   ├── websocket.js          # WS client
│   │   └── notifications.js      # Browser notifications
│   ├── assets/
│   │   ├── sounds/
│   │   │   ├── victory.mp3
│   │   │   └── error.mp3
│   │   └── images/
│   │       └── trophy.png
│   └── styles.css
├── scripts/
│   ├── claude-race-wrapper.sh    # Wrapper for reliable completion
│   └── install.sh                # Installation script
├── config.yaml
├── Makefile
└── README.md
```

---

## Development Order (Revised)

### Phase 0: Mock System (Day 1 Morning)

Build mock data generator **first** to unblock frontend development:

```go
// backend/internal/mock/generator.go

type MockGenerator struct {
    sessions []*MockSession
}

type MockSession struct {
    ID         string
    Name       string
    Duration   time.Duration
    ErrorAt    *time.Duration // nil = no error
    Pattern    string         // "steady", "burst", "stall"
}

func (g *MockGenerator) GenerateState(elapsed time.Duration) []SessionState {
    var states []SessionState
    
    for _, mock := range g.sessions {
        state := SessionState{
            ID:   mock.ID,
            Name: mock.Name,
        }
        
        progress := float64(elapsed) / float64(mock.Duration)
        
        switch mock.Pattern {
        case "steady":
            state.RelativePosition = progress
        case "burst":
            // Fast start, slow middle, fast finish
            state.RelativePosition = easeInOutCubic(progress)
        case "stall":
            // Stalls at 60% for a while
            if progress < 0.4 {
                state.RelativePosition = progress * 1.5
            } else if progress < 0.7 {
                state.RelativePosition = 0.6
            } else {
                state.RelativePosition = 0.6 + (progress-0.7)*1.33
            }
        }
        
        // Determine activity from position delta
        if elapsed > mock.Duration {
            state.Activity = ActivityComplete
            state.RelativePosition = 1.0
        } else if mock.ErrorAt != nil && elapsed > *mock.ErrorAt {
            state.Activity = ActivityErrored
        } else if state.RelativePosition == g.lastPositions[mock.ID] {
            state.Activity = ActivityIdle
        } else {
            state.Activity = ActivityThinking
        }
        
        states = append(states, state)
    }
    
    return states
}
```

**CLI flag:** `./claude-racer --mock` uses mock data instead of tmux.

### Phase 1: Frontend Against Mocks (Day 1 Afternoon - Day 2 Morning)

Build complete frontend experience using mock data:
- Canvas rendering with all visual states
- Activity animations
- Victory/error effects
- Notifications
- Responsive layout

### Phase 2: Real Tmux Integration (Day 2 Afternoon)

- Session discovery
- Pane content capture
- Activity classification
- Chunk counting

### Phase 3: Completion Detection (Day 3 Morning)

- Implement detector chain
- Test against real Claude sessions
- Tune thresholds and patterns

### Phase 4: Polish & Edge Cases (Day 3 Afternoon)

- Handle session lifecycle edge cases
- Add configuration UI
- Sound effects
- Performance optimization

### Phase 5: Packaging (Day 4)

- Embed frontend in Go binary
- Cross-platform builds
- Installation scripts
- Documentation

---

## Testing Strategy

### Unit Tests

```go
// detection/marker_test.go
func TestMarkerDetector(t *testing.T) {
    d := &MarkerDetector{Markers: DefaultMarkers}
    
    tests := []struct {
        name    string
        content string
        want    bool
    }{
        {"exact match", "output\n⚡ RACE:DONE\n", true},
        {"no match", "still running...", false},
        {"partial match", "⚡ RACE:", false},
        {"embedded", "text ⚡ RACE:DONE more text", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := d.Detect(&SessionState{}, tt.content)
            if result.Detected != tt.want {
                t.Errorf("got %v, want %v", result.Detected, tt.want)
            }
        })
    }
}
```

### Integration Tests

```bash
#!/bin/bash
# test/integration.sh

# Start mock Claude sessions in tmux
tmux new-session -d -s test-race -n session1
tmux send-keys -t test-race:session1 'sleep 5; echo "⚡ RACE:DONE"' Enter

tmux new-window -t test-race -n session2  
tmux send-keys -t test-race:session2 'for i in {1..10}; do echo "Working $i"; sleep 1; done; echo "⚡ RACE:DONE"' Enter

# Start server
./claude-racer &
SERVER_PID=$!

# Wait and verify
sleep 12
curl -s http://localhost:8080/api/sessions | jq '.[] | select(.activity == "complete")' | wc -l | grep -q 2

kill $SERVER_PID
tmux kill-session -t test-race
```

---

## Performance Considerations

### Tmux Polling Optimization

```go
func (m *TmuxMonitor) optimizedPoll() {
    // Batch tmux commands to reduce fork overhead
    script := `
        tmux list-panes -a -F '#{pane_id}'
    `
    // ... capture all panes in single tmux call
    
    // Only capture content for panes with recent activity
    // (track last output hash per pane)
}
```

### WebSocket Throttling

```go
func (b *Broadcaster) throttledBroadcast() {
    ticker := time.NewTicker(100 * time.Millisecond) // Max 10 updates/sec
    var pendingDeltas []DeltaPayload
    
    for {
        select {
        case delta := <-b.deltaQueue:
            pendingDeltas = append(pendingDeltas, delta)
            
        case <-ticker.C:
            if len(pendingDeltas) > 0 {
                b.sendBatch(pendingDeltas)
                pendingDeltas = nil
            }
        }
    }
}
```

---

## Future Extensions

1. **Multi-machine support**: SSH tunnel to remote tmux sessions
2. **Persistent history**: SQLite for race history and stats
3. **Team mode**: Multiple users watching shared sessions
4. **Custom themes**: Skinnable car sprites and tracks
5. **CI integration**: Track Claude Code runs in CI pipelines
6. **Metrics export**: Prometheus metrics for session duration, success rate
