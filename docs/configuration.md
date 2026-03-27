# Agent Racer Configuration

Agent Racer follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) for configuration and state files.

## Configuration File Location

The default configuration file location is:

```
~/.config/agent-racer/config.yaml
```

You can override this with the `--config` flag:

```bash
agent-racer --config /path/to/config.yaml
```

## XDG Environment Variables

Agent Racer respects the following XDG environment variables:

- `XDG_CONFIG_HOME` - Configuration files (default: `~/.config`)
- `XDG_STATE_HOME` - Application state (default: `~/.local/state`)

## Configuration Options

See `config.example.yaml` in the project root for a complete example with all available options.

### Server Settings

```yaml
server:
  port: 8080
  host: "127.0.0.1"
  allowed_origins: []
  auth_token: ""  # auto-generated if empty; weak placeholders (dev/test/changeme/default) are rejected
```

### Monitor Settings

```yaml
monitor:
  poll_interval: 1s
  snapshot_interval: 5s
  broadcast_throttle: 100ms
  session_stale_after: 2m
  completion_remove_after: 8s
  session_end_dir: ""  # Defaults to $XDG_STATE_HOME/agent-racer/session-end
```

### Model Context Limits

```yaml
models:
  default: 200000
  # Add model-specific overrides as needed
```

### Token Normalization

Controls how context utilization is derived for each agent source. Sources that report real token counts can use `usage`; others use heuristics. The `tokenEstimated` field on each session indicates whether the value is actual or heuristic.

```yaml
token_normalization:
  # Per-source strategy: "usage", "estimate", or "message_count"
  #   usage         - use real token counts from the source
  #   estimate      - estimate tokens from message count × tokens_per_message
  #   message_count - same as estimate
  # The "default" key applies to any source not listed explicitly.
  strategies:
    claude: usage
    codex: usage
    gemini: usage
    default: estimate
  # Estimated token cost per message. Used by estimate/message_count strategies,
  # and as a fallback for "usage" sources that haven't reported data yet.
  tokens_per_message: 2000
```

### Privacy

Controls what session metadata is exposed to connected clients. Useful when sharing a dashboard publicly or with a team.

```yaml
privacy:
  # Replace full directory paths with just the last component
  # e.g. "/home/user/secret-project" → "secret-project"
  mask_working_dirs: false
  # Replace session IDs with opaque short hashes
  mask_session_ids: false
  # Hide process IDs from broadcast data
  mask_pids: false
  # Hide tmux pane locations from broadcast data
  mask_tmux_targets: false
  # Allowlist: only broadcast sessions matching at least one glob pattern.
  # Empty = all sessions allowed. See "Path Filtering" below for details.
  allowed_paths: []
  # Denylist: exclude sessions matching any pattern.
  blocked_paths: []
```

#### Path Filtering

`allowed_paths` and `blocked_paths` accept glob patterns using Go `filepath.Match` syntax (`*` matches any non-separator sequence). Patterns are checked against the session's working directory and all its parent directories, so `/home/user/work/*` matches nested paths like `/home/user/work/foo/bar`.

When both are configured, `allowed_paths` is evaluated first (session must match at least one pattern), then `blocked_paths` excludes any remaining matches.

### Gamification

```yaml
gamification:
  battle_pass:
    # Enable the seasonal battle pass system (default: false)
    enabled: false
    # Current season identifier (e.g. "2025-07").
    # Changing this triggers a season rotation on next startup.
    season: ""
```

### Replay

Controls session replay recording. Replay files are stored in `$XDG_STATE_HOME/agent-racer/replays/`.

```yaml
replay:
  # Enable replay recording (default: true)
  enabled: true
  # Days of replay files to retain. 0 = keep forever. (default: 7)
  retention_days: 7
```

### Track

```yaml
track:
  # Track ID to use. Empty string uses the default linear track.
  active: ""
```

### Sound Configuration

The sound system supports fine-grained control over audio playback:

```yaml
sound:
  # Master enable/disable for all sounds
  enabled: true

  # Volume controls (0.0 - 1.0)
  master_volume: 1.0
  ambient_volume: 1.0  # Crowd, wind, engine hums
  sfx_volume: 1.0      # Gear shifts, victory, crashes, etc.

  # Selective enable/disable
  enable_ambient: true
  enable_sfx: true
```

#### Sound Categories

- **Ambient sounds**: Continuous background audio including crowd murmur, wind, and engine hums during thinking/tool use
- **Sound effects**: One-shot sounds for events like:
  - Gear shifts (activity transitions)
  - Victory chimes (session completion)
  - Crash sounds (errors)
  - Tool clicks (entering tool use)
  - Appear/disappear swooshes (sessions starting/ending)

#### Client-Side Sound Configuration

The frontend automatically fetches sound configuration from the server's `/api/config` endpoint on page load. All sound settings are applied dynamically without requiring a page reload.

You can still use the `M` keyboard shortcut to toggle mute regardless of the configured sound settings.

## Creating Your Configuration

1. Create the config directory:
   ```bash
   mkdir -p ~/.config/agent-racer
   ```

2. Copy the example configuration:
   ```bash
   cp config.example.yaml ~/.config/agent-racer/config.yaml
   ```

3. Edit the file to customize settings:
   ```bash
   $EDITOR ~/.config/agent-racer/config.yaml
   ```

4. Restart agent-racer for changes to take effect

## No Configuration File

If no configuration file exists, agent-racer will use sensible defaults. All features will work out of the box without requiring configuration.
