package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yap/internal/control"
)

type fakeControlTarget struct {
	state RecordingState
	calls []string
	err   error
}

func (f *fakeControlTarget) ToggleRecording() error {
	f.calls = append(f.calls, "toggle")
	if f.err != nil {
		return f.err
	}
	if f.state == StateRecording {
		f.state = StateTranscribing
	} else {
		f.state = StateRecording
	}
	return nil
}
func (f *fakeControlTarget) StartRecording() error { f.calls = append(f.calls, "start"); return f.err }
func (f *fakeControlTarget) StopRecording() error  { f.calls = append(f.calls, "stop"); return f.err }
func (f *fakeControlTarget) CancelRecording() error {
	f.calls = append(f.calls, "cancel")
	return f.err
}
func (f *fakeControlTarget) ShowWindow() { f.calls = append(f.calls, "show") }
func (f *fakeControlTarget) recordingState() RecordingState {
	return f.state
}

func TestRunControlCommandDrivesTheApp(t *testing.T) {
	target := &fakeControlTarget{state: StateReady}
	for _, step := range []struct{ command, reply string }{
		{"status", "ready"}, {"toggle", "recording"}, {"toggle", "transcribing"},
		{"start", "transcribing"}, {"stop", "transcribing"}, {"cancel", "transcribing"}, {"show", "transcribing"},
	} {
		reply, err := runControlCommand(target, step.command)
		if err != nil || reply != step.reply {
			t.Fatalf("runControlCommand(%q) = %q, %v; want %q", step.command, reply, err, step.reply)
		}
	}
	if got := strings.Join(target.calls, ","); got != "toggle,toggle,start,stop,cancel,show" {
		t.Fatalf("app calls = %s", got)
	}

	target.err = errors.New("not recording")
	if _, err := runControlCommand(target, "cancel"); err == nil || err.Error() != "not recording" {
		t.Fatalf("cancel error = %v", err)
	}
	if _, err := runControlCommand(target, "quit"); err == nil {
		t.Fatal("unknown command succeeded")
	}
}

func TestCLIReachesTheRunningApp(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "yap-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "yap.sock")

	var stdout, stderr bytes.Buffer
	if handled, code := runCLI([]string{"toggle"}, socket, &stdout, &stderr); !handled || code != 1 {
		t.Fatalf("toggle without a running app: handled=%v code=%d", handled, code)
	}
	if !strings.Contains(stderr.String(), "Yap is not running") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	target := &fakeControlTarget{state: StateReady}
	server, err := control.Listen(socket, func(command string) (string, error) {
		return runControlCommand(target, command)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	stdout.Reset()
	stderr.Reset()
	if handled, code := runCLI([]string{"toggle"}, socket, &stdout, &stderr); !handled || code != 0 {
		t.Fatalf("toggle: handled=%v code=%d stderr=%q", handled, code, stderr.String())
	}
	if stdout.String() != "recording\n" || target.state != StateRecording {
		t.Fatalf("stdout = %q, state = %s", stdout.String(), target.state)
	}

	target.err = errors.New("no model downloaded")
	stderr.Reset()
	if handled, code := runCLI([]string{"toggle"}, socket, &stdout, &stderr); !handled || code != 1 {
		t.Fatalf("failing toggle: handled=%v code=%d", handled, code)
	}
	if !strings.Contains(stderr.String(), "no model downloaded") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCLILeavesNormalLaunchesAlone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{nil, {}, {"-psn_0_12345"}, {"/some/file.wav"}} {
		if handled, _ := runCLI(args, "/nonexistent/yap.sock", &stdout, &stderr); handled {
			t.Fatalf("runCLI(%v) was treated as a command", args)
		}
	}
	if handled, code := runCLI([]string{"--help"}, "/nonexistent/yap.sock", &stdout, &stderr); !handled || code != 0 {
		t.Fatalf("--help: handled=%v code=%d", handled, code)
	}
	if !strings.Contains(stdout.String(), "toggle") {
		t.Fatalf("usage = %q", stdout.String())
	}
}

func TestControlExecutable(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	binary := func() (string, error) { return "/usr/bin/yap", nil }
	for _, tt := range []struct {
		name string
		env  map[string]string
		exe  func() (string, error)
		want string
	}{
		{"installed binary", nil, binary, "/usr/bin/yap"},
		{"AppImage path wins over its temporary mount", map[string]string{"APPIMAGE": "/home/me/Apps/Yap.AppImage"}, binary, "/home/me/Apps/Yap.AppImage"},
		{"paths with spaces are quoted", map[string]string{"APPIMAGE": "/home/me/My Apps/Yap.AppImage"}, binary, "'/home/me/My Apps/Yap.AppImage'"},
		{"unknown path falls back to PATH lookup", nil, func() (string, error) { return "", errors.New("unknown") }, "yap"},
	} {
		if got := controlExecutable(env(tt.env), tt.exe); got != tt.want {
			t.Errorf("%s: controlExecutable() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestShortcutGuide(t *testing.T) {
	const toggle, cancel = "/opt/Yap.AppImage toggle", "/opt/Yap.AppImage cancel"
	for _, tt := range []struct {
		name    string
		desktop shortcutDesktop
		file    string
		snippet string
	}{
		{
			name:    "Omarchy 4 (Lua)",
			desktop: shortcutDesktop{hyprland: true, luaConfig: true, omarchy: true},
			file:    "~/.config/hypr/bindings.lua",
			snippet: `o.bind("SUPER + ALT + Y", "Yap dictation", "/opt/Yap.AppImage toggle")` + "\n" +
				`o.bind("SUPER + ALT + ESCAPE", "Yap cancel", "/opt/Yap.AppImage cancel")`,
		},
		{
			name:    "Hyprland with a Lua config",
			desktop: shortcutDesktop{hyprland: true, luaConfig: true},
			file:    "~/.config/hypr/hyprland.lua",
			snippet: `hl.bind("SUPER + ALT + Y", hl.dsp.exec_cmd("/opt/Yap.AppImage toggle"))` + "\n" +
				`hl.bind("SUPER + ALT + ESCAPE", hl.dsp.exec_cmd("/opt/Yap.AppImage cancel"))`,
		},
		{
			name:    "Omarchy 3 (hyprlang)",
			desktop: shortcutDesktop{hyprland: true, omarchy: true},
			file:    "~/.config/hypr/bindings.conf",
			snippet: "bind = SUPER ALT, Y, exec, /opt/Yap.AppImage toggle\nbind = SUPER ALT, Escape, exec, /opt/Yap.AppImage cancel",
		},
		{
			name:    "Hyprland with a hyprlang config",
			desktop: shortcutDesktop{hyprland: true},
			file:    "~/.config/hypr/hyprland.conf",
			snippet: "bind = SUPER ALT, Y, exec, /opt/Yap.AppImage toggle\nbind = SUPER ALT, Escape, exec, /opt/Yap.AppImage cancel",
		},
		{
			name:    "other Wayland desktops get the bare commands",
			snippet: toggle + "\n" + cancel,
		},
	} {
		got := shortcutGuide(tt.desktop, toggle, cancel)
		if !got.Required || got.ConfigFile != tt.file || got.Snippet != tt.snippet {
			t.Errorf("%s: shortcutGuide() = %+v", tt.name, got)
		}
	}

	// A quoted path must survive being embedded in a Lua string.
	got := shortcutGuide(shortcutDesktop{hyprland: true, luaConfig: true}, `'/my "apps"/Yap.AppImage' toggle`, "c")
	if want := `hl.dsp.exec_cmd("'/my \"apps\"/Yap.AppImage' toggle")`; !strings.Contains(got.Snippet, want) {
		t.Errorf("Lua snippet = %s, want it to contain %s", got.Snippet, want)
	}
}
