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
- tiny.en (~75MB) - Fastest
- base.en (~150MB) - Default, good balance
- small.en (~500MB) - Better accuracy
- medium.en (~1.5GB) - High accuracy
- large-v3 (~3GB) - Best accuracy

## Building

```bash
# Development
wails dev

# Production build
wails build
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
