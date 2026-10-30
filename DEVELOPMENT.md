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

## macOS release signing

Releases are signed with a Developer ID certificate and notarized, otherwise Gatekeeper
rejects the download as damaged and Homebrew will not accept the cask.

### Required repository secrets

| Secret | What it is |
|--------|------------|
| `MACOS_CERT_P12` | Developer ID Application cert + private key, exported as `.p12`, base64 encoded |
| `MACOS_CERT_PASSWORD` | Password set when exporting the `.p12` |
| `KEYCHAIN_PASSWORD` | Any random string; unlocks the temporary CI keychain |
| `MACOS_SIGN_IDENTITY` | Exact identity, e.g. `Developer ID Application: Example Inc. (TEAMID1234)` |
| `AC_KEY_ID`, `AC_ISSUER_ID`, `AC_PRIVATE_KEY` | App Store Connect API key for `notarytool`; `AC_PRIVATE_KEY` is the `.p8` contents |
| `HOMEBREW_TAP_TOKEN` | PAT with `contents:write` on `ApplauseLab/homebrew-tap` |

An App Store Connect API key is used rather than an Apple ID and app-specific password so
notarization does not break when someone changes their password.

Export the certificate with:

```bash
security find-identity -v -p codesigning          # confirm the identity string
base64 -i DeveloperID.p12 | pbcopy                # paste into MACOS_CERT_P12
```

### Testing without publishing

The `release` job is skipped when the version is `dev`, so this builds, signs, notarizes
and verifies a DMG as a workflow artifact without creating a tag or release:

```bash
gh workflow run release.yml -f version=dev
```

Without `MACOS_CERT_P12` the build falls back to ad-hoc signing and skips notarization,
which keeps forks working. Any non-`dev` version fails instead, so a real release can
never silently ship unnotarized.

### Building and signing locally

```bash
scripts/ci/build-portaudio-macos.sh "$TMPDIR/portaudio"
scripts/ci/build-whisper-macos.sh v1.9.5 "$TMPDIR/whisper.cpp"

PKG_CONFIG_PATH="$TMPDIR/portaudio/lib/pkgconfig" \
  CGO_CFLAGS="-I$TMPDIR/portaudio/include" \
  CGO_LDFLAGS="-L$TMPDIR/portaudio/lib" \
  wails build -platform darwin/arm64 -o Yap

PORTAUDIO_PREFIX="$TMPDIR/portaudio" \
  scripts/ci/bundle-macos-runtime.sh build/bin/Yap.app "$TMPDIR/whisper.cpp"
```

`bundle-macos-runtime.sh` signs ad-hoc unless `MACOS_SIGN_IDENTITY` is set, and smoke
tests the whisper runtime both before and after signing. That post-signing run is what
catches the Hardened Runtime breaking the ggml backends, so keep it.

`CGO_CFLAGS`/`CGO_LDFLAGS` are needed in addition to `PKG_CONFIG_PATH`: Go's build cache
keys on those but not on `PKG_CONFIG_PATH`, so a cached object from an earlier build will
otherwise relink Homebrew's PortAudio.

### Deployment target

`MACOS_DEPLOYMENT_TARGET` in the release workflow is the floor for every macOS binary, and
`bundle-macos-runtime.sh` fails the build if any bundled Mach-O requires a newer macOS
than `LSMinimumSystemVersion` claims. PortAudio and whisper.cpp are built from source
rather than installed from Homebrew because a bottle is compiled for whichever macOS it
was bottled on, which would silently make the runner's OS the app's real minimum.

### Homebrew cask

`packaging/homebrew/yap.rb` is the source of truth. The `update-homebrew-tap` job rewrites
its version and `sha256` from the published DMG and commits the result to
`ApplauseLab/homebrew-tap` as `Casks/yap.rb`. The repository must exist before the first
stable release; the job is skipped for pre-releases so they never become the default
`brew install` target.

To validate cask changes locally:

```bash
brew tap-new applauselab/captest --no-git
cp packaging/homebrew/yap.rb "$(brew --repository applauselab/captest)/Casks/"
brew style --cask applauselab/captest
brew untap applauselab/captest
```

Check that the cask still loads with `brew info --cask`, not just that `brew style`
passes: `brew style --fix` rewrites the `depends_on macos:` constraint into a form that
drops the minimum version, and the string form `":big_sur"` passes style but fails to
load.

### Changing the signing team

The signing identity comes from `MACOS_SIGN_IDENTITY`, so moving to another Apple team is
a secrets change rather than a code change. It does reset TCC, because macOS keys privacy
permissions on the bundle identifier *and* the signature's team, so every user has to
re-grant Microphone and Accessibility. Config and history survive, as they are plain files
under `~/Library/Application Support/yap` rather than Keychain items.
