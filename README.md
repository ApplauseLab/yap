# Applause Whisper

A speech-to-text desktop app for macOS that lets you dictate text and automatically paste it into any application.

## Features

- **Local Transcription**: Uses whisper.cpp for offline, private speech-to-text
- **OpenAI API**: Optional cloud-based transcription using OpenAI's Whisper API
- **Multiple Models**: Choose from tiny, base, small, medium, or large models
- **Auto-Paste**: Automatically pastes transcribed text into the active application
- **Floating Window**: Always-on-top window for easy access
- **Model Management**: Download models on-demand

## Requirements

- macOS 11.0 or later
- [PortAudio](https://www.portaudio.com/) (installed via Homebrew)
- For local transcription: whisper.cpp CLI (optional, or use OpenAI API)

## Installation

### Install Dependencies

```bash
# Install PortAudio
brew install portaudio

# Optional: Install whisper.cpp for local transcription
brew install whisper-cpp
# Or build from source: https://github.com/ggerganov/whisper.cpp
```

### Build from Source

```bash
# Clone the repository
git clone <repo-url>
cd applause-whisper

# Install Wails CLI if not already installed
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Build the app
wails build

# Run the app
open build/bin/applause-whisper.app
```

### Development

```bash
# Run in development mode with hot reload
wails dev
```

## Usage

1. **Start Recording**: Click the record button or use the hotkey (coming soon: Cmd+Shift+Space)
2. **Speak**: Say what you want to transcribe
3. **Stop Recording**: Click the button again to stop
4. **Auto-Paste**: The transcribed text is automatically copied to clipboard and pasted

### Settings

Click the gear icon to access settings:

- **Provider**: Choose between Local (whisper.cpp) or OpenAI
- **Model**: Select the Whisper model size (affects accuracy vs speed)
- **OpenAI API Key**: Enter your key if using the OpenAI provider
- **Auto-Paste**: Toggle automatic pasting

### Models

| Model | Size | Speed | Accuracy |
|-------|------|-------|----------|
| tiny | ~75MB | Fastest | Basic |
| base | ~150MB | Fast | Good |
| small | ~500MB | Medium | Better |
| medium | ~1.5GB | Slow | High |
| large-v3 | ~3GB | Slowest | Highest |

English-only models (e.g., `base.en`) are slightly faster and more accurate for English.

## Architecture

```
applause-whisper/
├── main.go                 # Wails app entry point
├── app.go                  # Main app logic
├── internal/
│   ├── audio/             # PortAudio recording
│   ├── transcribe/        # Transcription engines
│   ├── system/            # Clipboard & paste
│   ├── hotkey/            # Global hotkey (placeholder)
│   └── models/            # Model & config management
└── frontend/              # React UI
```

## License

MIT License
