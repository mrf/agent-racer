# Process Activity Detection for Churning State

## Problem

Cars only move/animate when new data appears in JSONL session logs. When Claude is actively processing (thinking, streaming from API) but hasn't written output yet, the cars appear idle. Users want cars to feel "alive" when work is happening.

## Solution

Monitor Claude process CPU and network activity to detect "churning" - active processing before output appears.

## Detection Strategy

### Signals

| Signal | Meaning | Reliability |
|--------|---------|-------------|
| TCP connections > 0 | Actively streaming from Anthropic API | High |
| CPU > 15% | Process doing work | Medium (could be local processing) |
| TCP > 0 AND CPU > 10% | Definitely churning | Highest |

### Session → Process Mapping

Sessions are mapped to processes by **working directory (CWD)**:

1. Session file path: `~/.claude/projects/-Users-mark-ferree-Projects-agent-racer/session.jsonl`
2. Decode to project path: `/Users/mark.ferree/Projects/agent-racer`
3. Find Claude process with matching CWD
4. Read that process's CPU% and TCP connection count

## Implementation

### 1. New File: `backend/internal/monitor/process_darwin.go`

macOS-specific process discovery using `ps` and `lsof`:

```go
package monitor

import (
    "bytes"
    "os/exec"
    "strconv"
    "strings"
)

// ProcessActivity holds CPU and network state for a process
type ProcessActivity struct {
    PID        int
    CPU        float64
    TCPConns   int
    WorkingDir string
}

// DiscoverProcessActivity returns activity info for all Claude processes.
// Uses ps for CPU% and CWD, lsof for TCP connection count.
func DiscoverProcessActivity() ([]ProcessActivity, error) {
    // Get PIDs of claude processes
    pgrep := exec.Command("pgrep", "-x", "claude")
    pidOut, err := pgrep.Output()
    if err != nil {
        return nil, nil // No claude processes running
    }

    pids := strings.Fields(string(pidOut))
    results := make([]ProcessActivity, 0, len(pids))

    for _, pidStr := range pids {
        pid, _ := strconv.Atoi(pidStr)
        if pid == 0 {
            continue
        }

        activity := ProcessActivity{PID: pid}

        // Get CPU% and working directory
        ps := exec.Command("ps", "-p", pidStr, "-o", "%cpu=,")
        if out, err := ps.Output(); err == nil {
            cpu, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
            activity.CPU = cpu
        }

        // Get CWD via lsof
        lsofCwd := exec.Command("lsof", "-a", "-p", pidStr, "-d", "cwd", "-Fn")
        if out, err := lsofCwd.Output(); err == nil {
            for _, line := range strings.Split(string(out), "\n") {
                if strings.HasPrefix(line, "n") {
                    activity.WorkingDir = line[1:]
                    break
                }
            }
        }

        // Count TCP connections
        lsofTcp := exec.Command("lsof", "-a", "-p", pidStr, "-i", "TCP")
        if out, err := lsofTcp.Output(); err == nil {
            lines := bytes.Count(out, []byte("\n"))
            if lines > 1 {
                activity.TCPConns = lines - 1 // Subtract header
            }
        }

        results = append(results, activity)
    }

    return results, nil
}

// IsChurning returns true if the process appears to be actively working
func (p ProcessActivity) IsChurning() bool {
    // Strong signal: has active network connections
    if p.TCPConns > 0 {
        return true
    }
    // Weaker signal: high CPU without network (local processing)
    return p.CPU > 15.0
}
```

### 2. New File: `backend/internal/monitor/process_linux.go`

Linux version using `/proc` filesystem (already partially exists):

```go
//go:build linux

package monitor

// Similar implementation but reading from /proc/<pid>/stat for CPU
// and /proc/<pid>/net/tcp for connections
```

### 3. Update: `backend/internal/session/state.go`

Add churning field:

```go
type SessionState struct {
    // ... existing fields ...

    // IsChurning indicates the process is actively working but hasn't
    // produced output yet (high CPU or active network connections)
    IsChurning bool `json:"isChurning,omitempty"`
}
```

### 4. Update: `backend/internal/monitor/monitor.go`

Add process activity polling to the monitor loop:

```go
type Monitor struct {
    // ... existing fields ...

    // processActivity caches the latest process activity by working dir
    processActivity map[string]ProcessActivity
}

func (m *Monitor) poll() {
    now := time.Now()

    // Poll process activity (every tick or less frequently)
    activities, _ := DiscoverProcessActivity()
    m.processActivity = make(map[string]ProcessActivity)
    for _, a := range activities {
        if a.WorkingDir != "" {
            m.processActivity[a.WorkingDir] = a
        }
    }

    // ... existing discovery and parsing ...

    // After updating session state, check churning
    for _, state := range updates {
        if activity, ok := m.processActivity[state.WorkingDir]; ok {
            state.IsChurning = activity.IsChurning()
            state.PID = activity.PID
        }
    }
}
```

### 5. Update: `frontend/src/entities/Racer.js`

Animate on churning state:

```javascript
// In update():
update(state) {
    const oldActivity = this.state.activity;
    const wasChurning = this.state.isChurning;
    this.state = state;

    // Detect churning start
    if (state.isChurning && !wasChurning) {
        this.springVel += 1.0; // Subtle bounce
    }
}

// In animate():
animate(particles, dt) {
    // ... existing animation ...

    // Churning animation: subtle activity even without state change
    if (this.state.isChurning && activity !== 'thinking' && activity !== 'tool_use') {
        // Slow wheel rotation
        this.wheelAngle += 0.02 * dtScale;

        // Occasional exhaust puff
        if (particles && Math.random() > 0.95) {
            particles.emit('exhaust', this.displayX - 17 * S, this.displayY + 1 * S, 1);
        }

        // Subtle engine vibration
        this.springVel += (Math.random() - 0.5) * 0.3;
    }
}
```

### 6. Update: `frontend/src/canvas/RaceCanvas.js`

Handle churning in racer updates:

```javascript
// In updateRacer():
updateRacer(state) {
    let racer = this.racers.get(state.id);
    if (!racer) {
        racer = new Racer(state);
        this.racers.set(state.id, racer);
    }
    racer.update(state);

    // Play subtle sound on churning start
    if (state.isChurning && !racer.wasChurning) {
        this.sounds?.play('engine_idle');
    }
    racer.wasChurning = state.isChurning;
}
```

## Performance Considerations

### Polling Frequency

- Process activity polling: Every 1-2 seconds (configurable)
- Lighter than session file parsing
- Can be done in parallel with existing poll loop

### Command Overhead

Per poll cycle for N claude processes:
- 1x `pgrep -x claude`
- Nx `ps -p <pid> -o %cpu=`
- Nx `lsof -a -p <pid> -d cwd -Fn`
- Nx `lsof -a -p <pid> -i TCP`

Optimization: Combine into fewer commands:
```bash
# Single ps call for all processes
ps -eo pid,%cpu,command | grep claude

# Single lsof call for all claude processes
lsof -c claude -d cwd,txt -i TCP
```

### Caching

- CWD rarely changes during a session - cache the PID→CWD mapping
- Only refresh mapping when a new session is discovered
- CPU and TCP must be polled fresh each cycle

## Configuration

Add to `config.yaml`:

```yaml
monitor:
  # Existing
  poll_interval: 1s

  # New
  process_poll_interval: 1s  # How often to check CPU/network
  churning_cpu_threshold: 15.0  # CPU% above which to consider churning
  churning_requires_network: false  # If true, only churn with TCP > 0
```

## Testing

### Manual Testing

1. Start agent-racer backend
2. Open Claude Code in a project
3. Ask Claude a complex question that takes time to respond
4. Observe: car should show subtle animation before output appears

### Unit Tests

```go
func TestProcessActivity_IsChurning(t *testing.T) {
    tests := []struct {
        name     string
        activity ProcessActivity
        want     bool
    }{
        {"high CPU", ProcessActivity{CPU: 50.0, TCPConns: 0}, true},
        {"has TCP", ProcessActivity{CPU: 5.0, TCPConns: 2}, true},
        {"idle", ProcessActivity{CPU: 2.0, TCPConns: 0}, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := tt.activity.IsChurning(); got != tt.want {
                t.Errorf("IsChurning() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Rollout

1. Backend changes first - add `isChurning` to state
2. Verify field appears in WebSocket messages
3. Frontend changes - add churning animations
4. Tune thresholds based on real-world testing

## Future Enhancements

- **Linux support**: Use `/proc` filesystem instead of `ps`/`lsof`
- **Per-model thresholds**: Different models may have different CPU profiles
- **Streaming detection**: Detect partial writes to session file
- **Memory tracking**: RSS growth as additional signal
