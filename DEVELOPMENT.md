# Yap - Development Guide

## Project Structure

```
yap/
├── app.go                 # Main app logic (recording, transcription, state)
├── main.go                # Wails entry point, window config
├── internal/
│   ├── audio/             # PortAudio recording
│   ├── hotkey/            # Global hotkey (Right Option key)
│   ├── models/            # Model management, config
│   ├── system/            # Clipboard, paste operations
│   ├── transcribe/        # Whisper.cpp and OpenAI transcription
│   └── tray/              # Menu bar icon (systray)
├── frontend/
│   └── src/
│       ├── App.tsx        # React UI
│       └── App.css        # Styles
└── build/
    ├── appicon.icns       # macOS app icon
    └── appicon.png        # PNG app icon
```

## TMux Session Layout

The development tmux session `yap` has 3 windows:

| Window | Name    | Purpose                          |
|--------|---------|----------------------------------|
| 0      | opencode| Reserved for opencode/AI coding  |
| 1      | wails   | `wails dev` - main app server    |
| 2      | shell   | General shell commands           |

### Starting Development

```bash
# Create tmux session (if not exists)
tmux new-session -d -s yap -n opencode
tmux new-window -t yap -n wails
tmux new-window -t yap -n shell

# Start wails dev in window 1
tmux send-keys -t yap:wails "cd /path/to/yap && wails dev" Enter

# Attach to session
tmux attach -t yap
```

### Window Navigation

- `Ctrl+b 0` - Go to opencode window
- `Ctrl+b 1` - Go to wails window  
- `Ctrl+b 2` - Go to shell window

## Key Features

### Hotkey: Right Option Key
- Press **Right Option (⌥)** to start/stop recording
- Requires Accessibility permissions in System Preferences

### Transcription Providers
- **Local**: Uses whisper.cpp with downloaded models
- **OpenAI**: Uses OpenAI Whisper API (requires API key)

### Models (Local)
Supported models, their sizes, and their speed and accuracy ratings are defined
in one catalog in `internal/models/catalog.go`. Users browse, download, activate,
and delete them on the **Models** page. Only one download runs at a time.
Downloading a model never activates it. The one exception is onboarding, which
activates the model the user picked there.

## Building

```bash
# Development
wails dev

# Production build
wails build
```

## Testing

```bash
go test ./...
cd frontend && npm test
```

### Isolated development data

Set `YAP_CONFIG_DIR` to keep config, models, history, and logs out of the
installed app's data directory. Set `YAP_MODEL_BASE_URL` to download models from
a local mirror, e.g. to exercise failed or truncated downloads:

```bash
YAP_CONFIG_DIR=/tmp/yap-dev YAP_MODEL_BASE_URL=http://127.0.0.1:34199 \
  wails dev -devserver localhost:34135
```

## Requirements

- Go 1.21+
- Node.js 18+
- Wails CLI v2
- PortAudio (`brew install portaudio`)
- whisper.cpp CLI (`brew install whisper-cpp`)

## Permissions

The app requires:
- **Microphone access** - for recording
- **Accessibility access** - for global hotkey (Right Option)
- **Automation** - for auto-paste feature
