# TTS Daemon

A text-to-speech (TTS) daemon service that caches audio clips and provides both a CLI client and MCP (Model Context Protocol) interface for Claude and other AI assistants.

## Features

- **Azure Cognitive Services TTS**: High-quality text-to-speech using Azure's neural voices
- **SQLite Caching**: Automatically caches generated audio to avoid redundant API calls
- **Rate Limiting**: Configurable QPS (queries per second) limiting for Azure API calls
- **gRPC Communication**: Efficient client-daemon communication
- **Audio Playback**: Play audio directly using the client
- **MCP Support**: Exposes TTS functionality to Claude via Model Context Protocol
- **Multi-language Support**: Supports 20+ languages including English, French, Spanish, German, Japanese, Chinese, and more

## Architecture

```
┌─────────────────┐
│   Claude/User   │
└────────┬────────┘
         │
    ┌────┴─────┐
    │          │
┌───▼────┐ ┌──▼────────┐
│  CLI   │ │    MCP    │
│ Client │ │  Client   │
└───┬────┘ └──┬────────┘
    │         │
    └────┬────┘
         │ gRPC
    ┌────▼─────┐
    │  Daemon  │
    └────┬─────┘
         │
    ┌────┼────────┐
    │    │        │
┌───▼─┐ ┌▼──────┐ ┌▼────────┐
│Cache│ │Player │ │ Azure   │
│(DB) │ │(Beep) │ │   TTS   │
└─────┘ └───────┘ └─────────┘
```

## Installation

### Prerequisites

- Go 1.21 or later
- Azure Cognitive Services Speech subscription key
- macOS, Linux, or Windows

### Build from source

```bash
# Clone or navigate to the project directory
cd tts-daemon

# Download dependencies
go mod tidy

# Build binaries (development mode)
make build
# or manually:
# go build -o bin/tts-daemon ./cmd/tts-daemon
# go build -o bin/tts-client ./cmd/tts-client

# Build binaries (release/optimized mode - recommended for production)
make release

# Optionally, install to your PATH
go install ./cmd/tts-daemon
go install ./cmd/tts-client
```

**Build modes:**
- **Development (`make build`):** Standard build with debug symbols, useful for development
- **Release (`make release`):** Optimized build with stripped debug symbols and smaller binary size (~30% smaller), recommended for production use

You can also build individual components:
```bash
make daemon          # Build daemon only (dev)
make client          # Build client only (dev)
make release-daemon  # Build daemon only (release)
make release-client  # Build client only (release)
```

## Configuration

1. Copy the example configuration file:

```bash
mkdir -p ~/.config/tts-daemon
cp config.example.yaml ~/.config/tts-daemon/config.yaml
```

2. Edit `~/.config/tts-daemon/config.yaml` and add your Azure credentials:

```yaml
azure:
  subscription_key: "YOUR_AZURE_SUBSCRIPTION_KEY"
  region: "YOUR_AZURE_REGION"  # e.g., westus, eastus, westeurope
  max_qps: 10.0  # Maximum queries per second (default: 10)
  # Optional: Custom voice mappings to override defaults
  voices:
    en-US: "en-US-AriaNeural"     # Use a different voice for US English
    es-MX: "es-MX-JorgeNeural"    # Use male voice for Mexican Spanish
    fr: "fr-FR-HenriNeural"       # Use male voice for French

database:
  path: ""  # Default: ~/.local/share/tts-daemon/cache.db

server:
  address: "localhost"
  port: 50051

audio:
  sample_rate: 44100
  buffer_size: 4096
```

### Getting Azure Credentials

1. Go to [Azure Portal](https://portal.azure.com)
2. Create a new "Speech Services" resource
3. Copy the subscription key and region from the resource's "Keys and Endpoint" page

## Usage

### Starting the Daemon

```bash
# Start with default config location (~/.config/tts-daemon/config.yaml)
./bin/tts-daemon

# Or specify a custom config file
./bin/tts-daemon -config /path/to/config.yaml
```

The daemon will:
- Initialize the SQLite cache database
- Connect to Azure TTS API
- Start listening on the configured gRPC port (default: 50051)
- Log cache statistics on startup

### Using the CLI Client

#### Fetch audio (stores in cache, doesn't play)

```bash
./bin/tts-client "Hello, world!"
./bin/tts-client -lang fr-FR "Bonjour le monde!"
./bin/tts-client -lang es-ES "¡Hola mundo!"
```

#### Play audio

```bash
./bin/tts-client -play "Hello, world!"
./bin/tts-client -play -lang ja-JP "こんにちは世界"

# With verbose output
./bin/tts-client -v -play "Hello, world!"
# Output:
# Audio played successfully
# (from cache)
```

#### Check cache only (don't fetch from Azure)

```bash
./bin/tts-client -cache-only "Hello, world!"
```

#### Force refresh (bypass cache and refetch)

Useful when you've changed voice settings and want to update cached audio:

```bash
./bin/tts-client -force "Hello, world!"
./bin/tts-client -f -play -lang es-MX "el camino"  # Short form
```

#### Delete cached entry

Remove a specific cached audio entry:

```bash
./bin/tts-client -D -lang es-MX "el camino"
```

#### Connect to custom daemon address

```bash
./bin/tts-client -address localhost:50051 "Hello, world!"
```

### CLI Options

```
-address string
    Daemon server address (default "localhost:50051")
-cache-only
    Only check cache, don't fetch from Azure
-D
    Delete cached entry
-f, -force
    Force refresh from Azure, bypassing cache
-lang string
    Language code (e.g., en-US, fr-FR, es-ES) (default "en-US")
-mcp
    Run in MCP mode
-play
    Play audio (default: just fetch)
-v, -verbose
    Enable verbose output
```

**Note:** By default, the client is silent on success (no output). Use `-v` or `-verbose` to see detailed information about cache hits, audio sizes, etc. Errors are always displayed.

### Using with Claude (MCP Mode)

The client can run in MCP mode to expose TTS functionality to Claude and other AI assistants.

#### MCP Configuration

Add to your Claude MCP settings (e.g., `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "tts": {
      "command": "/path/to/bin/tts-client",
      "args": ["-mcp"]
    }
  }
}
```

Make sure the daemon is running before starting Claude.

#### Available MCP Tools

1. **fetch_tts**: Fetch and cache audio without playing
   - Parameters: `text` (required), `language_code` (optional, default: en-US)

2. **play_tts**: Fetch (if needed), cache, and play audio
   - Parameters: `text` (required), `language_code` (optional, default: en-US)

#### Example Claude Interactions

```
User: Can you fetch the French pronunciation for "Bonjour, comment allez-vous?"
Claude: [uses fetch_tts tool with language_code: "fr-FR"]

User: Play that for me
Claude: [uses play_tts tool with the same text]
```

## Supported Languages

The daemon supports all of the languages that Azure TTS supports, [see details](https://learn.microsoft.com/en-us/azure/ai-services/speech-service/language-support?tabs=tts).

For a shorter language code (e.g., "fr" instead of "fr-FR"), the daemon will use the most common regional variant.

### Customizing Voices

You can override the default voice for any language by adding voice mappings to your config file:

```yaml
azure:
  voices:
    en-US: "en-US-AriaNeural"      # Different US English voice
    es-MX: "es-MX-JorgeNeural"     # Male Mexican Spanish voice
    fr: "fr-FR-HenriNeural"        # Male French voice
    ja-JP: "ja-JP-KeitaNeural"     # Male Japanese voice
```

**Finding Available Voices:**

Browse available voices at [Azure's voice gallery](https://speech.microsoft.com/portal/voicegallery). Each voice name follows the pattern: `{locale}-{VoiceName}Neural`

Examples:
- `en-US-AriaNeural` - US English, female
- `en-US-GuyNeural` - US English, male
- `es-MX-DaliaNeural` - Mexican Spanish, female
- `es-MX-JorgeNeural` - Mexican Spanish, male
- `fr-FR-DeniseNeural` - French, female
- `fr-FR-HenriNeural` - French, male

**How It Works:**
1. When you request TTS for a language code (e.g., `es-MX`)
2. The daemon first checks your custom voice mappings
3. If found, uses your custom voice
4. If not found, falls back to the built-in default

This allows you to use male/female voices, regional accents, or specialized voices (like child voices or elderly voices) for any language.

## How Caching Works

1. Text is normalized (lowercased, whitespace trimmed, punctuation removed)
2. A SHA-256 hash is generated from `language_code:normalized_text`
3. The cache is checked using this hash
4. If found, cached audio is returned immediately
5. If not found, audio is fetched from Azure and stored in the cache
6. All audio is stored in MP3 format (16kHz, 128kbps, mono)

This ensures:
- Fast repeated requests for the same text
- Reduced Azure API costs
- Works offline for cached content

## Rate Limiting

The daemon enforces a configurable rate limit on Azure API calls using the `golang.org/x/time/rate` package. This prevents hitting Azure's API limits and controls costs.

Default: 10 requests per second (configurable via `azure.max_qps` in config)

## Running as a System Service

Running the TTS daemon as a system service ensures it starts automatically at boot and restarts if it crashes.

### macOS (launchd)

#### 1. Create the LaunchAgent configuration

Create `~/Library/LaunchAgents/com.biesnecker.tts-daemon.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.biesnecker.tts-daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/YOUR_USERNAME/path/to/bin/tts-daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/Users/YOUR_USERNAME/.local/share/tts-daemon/daemon.err</string>
    <key>StandardOutPath</key>
    <string>/Users/YOUR_USERNAME/.local/share/tts-daemon/daemon.out</string>
    <key>WorkingDirectory</key>
    <string>/Users/YOUR_USERNAME</string>
</dict>
</plist>
```

**Important:** Replace `YOUR_USERNAME` and update paths to match your actual installation.

#### 2. Load the service

```bash
# Load and start immediately
launchctl load ~/Library/LaunchAgents/com.biesnecker.tts-daemon.plist

# The daemon will now start automatically at login
```

#### 3. Manage the service

```bash
# Check if running
launchctl list | grep tts-daemon

# Stop the service
launchctl unload ~/Library/LaunchAgents/com.biesnecker.tts-daemon.plist

# Start the service (if not set to RunAtLoad)
launchctl start com.biesnecker.tts-daemon

# View logs
tail -f ~/.local/share/tts-daemon/daemon.out
tail -f ~/.local/share/tts-daemon/daemon.err
```

### Linux (systemd)

#### 1. Create the systemd service file

**Option A: User service (recommended)** - Runs as your user, starts at login:

Create `~/.config/systemd/user/tts-daemon.service`:

```ini
[Unit]
Description=TTS Daemon
After=network.target

[Service]
Type=simple
ExecStart=%h/path/to/bin/tts-daemon
Restart=always
RestartSec=10
StandardOutput=append:%h/.local/share/tts-daemon/daemon.log
StandardError=append:%h/.local/share/tts-daemon/daemon.err

[Install]
WantedBy=default.target
```

**Option B: System service** - Runs as a system service (requires root):

Create `/etc/systemd/system/tts-daemon.service`:

```ini
[Unit]
Description=TTS Daemon
After=network.target

[Service]
Type=simple
User=YOUR_USERNAME
ExecStart=/home/YOUR_USERNAME/path/to/bin/tts-daemon
Restart=always
RestartSec=10
StandardOutput=append:/home/YOUR_USERNAME/.local/share/tts-daemon/daemon.log
StandardError=append:/home/YOUR_USERNAME/.local/share/tts-daemon/daemon.err

[Install]
WantedBy=multi-user.target
```

#### 2. Enable and start the service

**For user service:**

```bash
# Reload systemd to recognize new service
systemctl --user daemon-reload

# Enable service to start at login
systemctl --user enable tts-daemon

# Start the service now
systemctl --user start tts-daemon

# Check status
systemctl --user status tts-daemon
```

**For system service:**

```bash
# Reload systemd to recognize new service
sudo systemctl daemon-reload

# Enable service to start at boot
sudo systemctl enable tts-daemon

# Start the service now
sudo systemctl start tts-daemon

# Check status
sudo systemctl status tts-daemon
```

#### 3. Manage the service

```bash
# View logs (user service)
journalctl --user -u tts-daemon -f

# View logs (system service)
sudo journalctl -u tts-daemon -f

# Stop the service
systemctl --user stop tts-daemon  # or: sudo systemctl stop tts-daemon

# Restart the service
systemctl --user restart tts-daemon  # or: sudo systemctl restart tts-daemon

# Disable autostart
systemctl --user disable tts-daemon  # or: sudo systemctl disable tts-daemon
```

### Windows (Task Scheduler)

#### 1. Create a batch file wrapper

Create `C:\Program Files\tts-daemon\start-daemon.bat`:

```batch
@echo off
cd /d "%USERPROFILE%"
"C:\Program Files\tts-daemon\tts-daemon.exe" >> "%USERPROFILE%\.local\share\tts-daemon\daemon.log" 2>&1
```

#### 2. Create a scheduled task

**Using Task Scheduler GUI:**

1. Open Task Scheduler (search for "Task Scheduler" in Start Menu)
2. Click "Create Task..." (not "Create Basic Task")
3. **General tab:**
   - Name: `TTS Daemon`
   - Description: `Text-to-Speech daemon service`
   - Check "Run whether user is logged on or not"
   - Check "Run with highest privileges" (if needed)
   - Configure for: Windows 10/11
4. **Triggers tab:**
   - Click "New..."
   - Begin the task: "At startup" or "At log on"
   - Check "Enabled"
   - Click "OK"
5. **Actions tab:**
   - Click "New..."
   - Action: "Start a program"
   - Program/script: `C:\Program Files\tts-daemon\start-daemon.bat`
   - Click "OK"
6. **Conditions tab:**
   - Uncheck "Start the task only if the computer is on AC power" (if laptop)
7. **Settings tab:**
   - Check "Allow task to be run on demand"
   - Check "If the running task does not end when requested, force it to stop"
   - If the task fails, restart every: "1 minute"
   - Attempt to restart up to: "3 times"
8. Click "OK" to save

**Using Command Line (PowerShell as Administrator):**

```powershell
$action = New-ScheduledTaskAction -Execute "C:\Program Files\tts-daemon\start-daemon.bat"
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -LogonType S4U -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask -TaskName "TTS Daemon" -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description "Text-to-Speech daemon service"
```

#### 3. Manage the task

**Using GUI:**
- Open Task Scheduler
- Find "TTS Daemon" in the task list
- Right-click for options (Run, End, Disable, Delete, Properties)

**Using Command Line:**

```powershell
# Start the task manually
Start-ScheduledTask -TaskName "TTS Daemon"

# Stop the task
Stop-ScheduledTask -TaskName "TTS Daemon"

# Disable the task
Disable-ScheduledTask -TaskName "TTS Daemon"

# Enable the task
Enable-ScheduledTask -TaskName "TTS Daemon"

# View task status
Get-ScheduledTask -TaskName "TTS Daemon" | Get-ScheduledTaskInfo
```

### Verifying the Service is Running

After setting up the service, verify it's working:

```bash
# Test with the client
./bin/tts-client -v "Hello, world!"

# If successful, you should see output like:
# Audio fetched successfully
# Cache key: ...
# Audio size: ... bytes
# (fetched from Azure)
```

### Troubleshooting

**Service won't start:**
- Check that the config file exists at `~/.config/tts-daemon/config.yaml`
- Verify Azure credentials are correct
- Check log files for error messages
- Ensure binary has execute permissions: `chmod +x bin/tts-daemon`

**Can't connect from client:**
- Verify daemon is running: `ps aux | grep tts-daemon` (macOS/Linux)
- Check firewall settings if connecting remotely
- Verify port 50051 is not in use by another process

**Daemon crashes on startup:**
- Check log files for detailed error messages
- Try running manually first: `./bin/tts-daemon` to see errors
- Verify database directory is writable: `~/.local/share/tts-daemon/`

## Development

### Project Structure

```
tts-daemon/
├── cmd/
│   ├── tts-daemon/      # Daemon main entry point
│   └── tts-client/      # Client main entry point
├── internal/
│   ├── config/          # Configuration parsing
│   ├── daemon/          # gRPC server implementation
│   ├── player/          # Audio playback (beep wrapper)
│   └── tts/            # TTS service, Azure client, caching
├── proto/               # gRPC protocol definitions
├── bin/                 # Built binaries
├── config.example.yaml  # Example configuration
├── generate.sh          # Script to regenerate gRPC code
└── README.md
```

### Regenerating gRPC Code

If you modify `proto/tts.proto`:

```bash
./generate.sh
```

This will regenerate `proto/tts.pb.go` and `proto/tts_grpc.pb.go`.

## Troubleshooting

### Daemon won't start

- Check that your Azure credentials are correct in the config file
- Verify the gRPC port (50051) isn't already in use
- Check logs for detailed error messages

### Client can't connect

- Ensure the daemon is running
- Verify the address and port match the daemon's configuration
- Check firewall settings if connecting remotely

### Audio won't play

- Ensure your system has audio output configured
- Check that the MP3 codec is supported
- Try fetching without playing first to verify the issue is with playback

### Rate limiting errors

- Reduce `max_qps` in the configuration
- Wait a moment before retrying
- Check Azure service limits for your subscription tier

## License

This project is licensed under an MIT License. Please see LICENSE.txt for details.

## Contributing

This is a personal project, but suggestions and improvements are welcome!
