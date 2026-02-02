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
  auth_token: ""
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
