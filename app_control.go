package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"yap/internal/control"
	"yap/internal/system"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const cliUsage = `Usage: yap [command]

Without a command, Yap starts normally. Commands control the running app, so
they can be bound to shortcuts in your compositor or window manager:

  toggle   Start recording, or stop and transcribe
  start    Start recording
  stop     Stop recording and transcribe
  cancel   Discard the current recording
  show     Show the Yap window
  status   Print the current state (ready, recording, transcribing, error)
`

// runCLI handles `yap <command>`. It reports whether args named a command and
// the exit code to use; any other invocation starts the app as usual.
func runCLI(args []string, socket string, stdout, stderr io.Writer) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0
	}
	switch command := args[0]; {
	case command == "help" || command == "--help" || command == "-h":
		fmt.Fprint(stdout, cliUsage)
		return true, 0
	case control.IsCommand(command):
		reply, err := control.Send(socket, command)
		if err != nil {
			fmt.Fprintf(stderr, "yap %s: %v\n", command, err)
			return true, 1
		}
		if reply != "" {
			fmt.Fprintln(stdout, reply)
		}
		return true, 0
	}
	return false, 0
}

// runLinuxCLI serves `yap <command>` and keeps Yap to a single instance, which
// the control socket needs. It only returns when this process should start
// the app.
func runLinuxCLI() {
	if goruntime.GOOS != "linux" {
		return
	}
	socket := control.SocketPath()
	if handled, code := runCLI(os.Args[1:], socket, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	if _, err := control.Send(socket, control.CommandShow); err == nil {
		fmt.Println("Yap is already running; showing its window")
		os.Exit(0)
	}
}

// controlTarget is the part of the app that control commands drive.
type controlTarget interface {
	ToggleRecording() error
	StartRecording() error
	StopRecording() error
	CancelRecording() error
	ShowWindow()
	recordingState() RecordingState
}

// runControlCommand executes a command from `yap <command>` and replies with
// the resulting state.
func runControlCommand(target controlTarget, command string) (string, error) {
	var err error
	switch command {
	case control.CommandToggle:
		err = target.ToggleRecording()
	case control.CommandStart:
		err = target.StartRecording()
	case control.CommandStop:
		err = target.StopRecording()
	case control.CommandCancel:
		err = target.CancelRecording()
	case control.CommandShow:
		target.ShowWindow()
	case control.CommandStatus:
	default:
		return "", fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return "", err
	}
	return string(target.recordingState()), nil
}

func (a *App) recordingState() RecordingState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// startControlServer accepts `yap <command>` from other processes. Linux only:
// macOS and Windows register their global hotkeys natively.
func (a *App) startControlServer() {
	if goruntime.GOOS != "linux" {
		return
	}
	server, err := control.Listen(control.SocketPath(), func(command string) (string, error) {
		runtime.LogInfo(a.ctx, "Control command received: "+command)
		return runControlCommand(a, command)
	})
	if errors.Is(err, control.ErrAlreadyRunning) {
		runtime.LogWarning(a.ctx, "Another Yap instance owns the control socket; `yap toggle` will reach that instance")
		return
	}
	if err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to start control server: %v", err))
		return
	}
	a.controlServer = server
	runtime.LogInfo(a.ctx, "Control server listening on "+control.SocketPath())
}

// ExternalShortcuts describes shortcuts that the desktop, not Yap, must bind.
type ExternalShortcuts struct {
	// Required is true when Yap cannot register global hotkeys itself.
	Required bool `json:"required"`
	// ConfigFile is where Snippet belongs, or empty when the desktop binds
	// commands through its own settings.
	ConfigFile string `json:"configFile"`
	// Snippet holds the config lines, or the bare commands, to bind.
	Snippet string `json:"snippet"`
}

// GetExternalShortcuts tells the frontend whether recording shortcuts are
// bound in the compositor (Wayland) and what to add there.
func (a *App) GetExternalShortcuts() ExternalShortcuts {
	if !system.IsWaylandSession() {
		return ExternalShortcuts{}
	}
	executable := controlExecutable(os.Getenv, os.Executable)
	return shortcutGuide(detectShortcutDesktop(), executable+" "+control.CommandToggle, executable+" "+control.CommandCancel)
}

// shortcutDesktop is what decides the bind syntax to show.
type shortcutDesktop struct {
	hyprland bool
	// luaConfig is the Hyprland 0.55+ config format; false means hyprlang.
	luaConfig bool
	omarchy   bool
}

func detectShortcutDesktop() shortcutDesktop {
	if !system.IsHyprlandSession() {
		return shortcutDesktop{}
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return shortcutDesktop{
		hyprland:  true,
		luaConfig: exists(filepath.Join(configDir, "hypr", "hyprland.lua")),
		omarchy: os.Getenv("OMARCHY_PATH") != "" ||
			exists(filepath.Join(home, ".local", "share", "omarchy")) || exists("/usr/share/omarchy"),
	}
}

func shortcutGuide(desktop shortcutDesktop, toggle, cancel string) ExternalShortcuts {
	guide := ExternalShortcuts{Required: true, Snippet: toggle + "\n" + cancel}
	lua := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	switch {
	case !desktop.hyprland:
	case desktop.luaConfig && desktop.omarchy:
		guide.ConfigFile = "~/.config/hypr/bindings.lua"
		guide.Snippet = fmt.Sprintf("o.bind(\"SUPER + ALT + Y\", \"Yap dictation\", %s)\no.bind(\"SUPER + ALT + ESCAPE\", \"Yap cancel\", %s)", lua(toggle), lua(cancel))
	case desktop.luaConfig:
		guide.ConfigFile = "~/.config/hypr/hyprland.lua"
		guide.Snippet = fmt.Sprintf("hl.bind(\"SUPER + ALT + Y\", hl.dsp.exec_cmd(%s))\nhl.bind(\"SUPER + ALT + ESCAPE\", hl.dsp.exec_cmd(%s))", lua(toggle), lua(cancel))
	default:
		guide.ConfigFile = "~/.config/hypr/hyprland.conf"
		if desktop.omarchy {
			guide.ConfigFile = "~/.config/hypr/bindings.conf"
		}
		guide.Snippet = fmt.Sprintf("bind = SUPER ALT, Y, exec, %s\nbind = SUPER ALT, Escape, exec, %s", toggle, cancel)
	}
	return guide
}

// controlExecutable returns the shell-ready path users should bind. Inside an
// AppImage the process runs from a temporary mount, so the AppImage file
// itself is the stable path.
func controlExecutable(getenv func(string) string, executable func() (string, error)) string {
	path := getenv("APPIMAGE")
	if path == "" {
		var err error
		if path, err = executable(); err != nil || path == "" {
			return "yap"
		}
	}
	if strings.ContainsAny(path, " \t'\"\\$`") {
		return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
	}
	return path
}
