package system

import (
	"errors"
	"strings"
	"testing"
)

type fakePasteEnv struct {
	env     map[string]string
	tools   map[string]bool
	outputs map[string]string
	fail    map[string]bool
	calls   []string
}

func (f *fakePasteEnv) paster() linuxPaster {
	return linuxPaster{
		getenv: func(key string) string { return f.env[key] },
		lookPath: func(tool string) (string, error) {
			if f.tools[tool] {
				return "/usr/bin/" + tool, nil
			}
			return "", errors.New("not found")
		},
		run: func(name string, args ...string) ([]byte, error) {
			call := name + " " + strings.Join(args, " ")
			f.calls = append(f.calls, call)
			if f.fail[name] {
				return []byte("boom"), errors.New("exit status 1")
			}
			for prefix, out := range f.outputs {
				if strings.HasPrefix(call, prefix) {
					return []byte(out), nil
				}
			}
			return nil, nil
		},
	}
}

var (
	waylandEnv  = map[string]string{"WAYLAND_DISPLAY": "wayland-1"}
	hyprlandEnv = map[string]string{"WAYLAND_DISPLAY": "wayland-1", "HYPRLAND_INSTANCE_SIGNATURE": "abc"}
)

func TestLinuxPaste(t *testing.T) {
	tests := []struct {
		name    string
		env     fakePasteEnv
		want    []string
		wantErr string
	}{
		{
			name: "X11 keeps using xdotool",
			env:  fakePasteEnv{tools: map[string]bool{"xdotool": true, "wtype": true}},
			want: []string{"xdotool key ctrl+v"},
		},
		{
			name: "Hyprland sends the shortcut through the compositor",
			env: fakePasteEnv{env: hyprlandEnv, tools: map[string]bool{"hyprctl": true, "wtype": true},
				outputs: map[string]string{"hyprctl activewindow": `{"class":"chromium","xwayland":false}`, "hyprctl dispatch": "ok\n"}},
			want: []string{"hyprctl activewindow -j", `hyprctl dispatch hl.dsp.send_shortcut({ mods = "CTRL", key = "V" })`},
		},
		{
			name: "Hyprland terminals get Ctrl+Shift+V",
			env: fakePasteEnv{env: hyprlandEnv, tools: map[string]bool{"hyprctl": true},
				outputs: map[string]string{"hyprctl activewindow": `{"class":"com.mitchellh.ghostty"}`, "hyprctl dispatch": "ok"}},
			want: []string{"hyprctl activewindow -j", `hyprctl dispatch hl.dsp.send_shortcut({ mods = "CTRL SHIFT", key = "V" })`},
		},
		{
			name: "Hyprland before the Lua config gets the classic dispatcher",
			env: fakePasteEnv{env: hyprlandEnv, tools: map[string]bool{"hyprctl": true, "wtype": true},
				outputs: map[string]string{"hyprctl activewindow": `{"class":"firefox"}`, "hyprctl dispatch hl.": "Invalid dispatcher", "hyprctl dispatch sendshortcut": "ok"}},
			want: []string{"hyprctl activewindow -j", `hyprctl dispatch hl.dsp.send_shortcut({ mods = "CTRL", key = "V" })`, "hyprctl dispatch sendshortcut CTRL, V,"},
		},
		{
			name: "a rejected Hyprland dispatch falls back to wtype",
			env: fakePasteEnv{env: hyprlandEnv, tools: map[string]bool{"hyprctl": true, "wtype": true},
				outputs: map[string]string{"hyprctl activewindow": `{"class":"Alacritty"}`, "hyprctl dispatch": "Invalid dispatcher"}},
			want: []string{"hyprctl activewindow -j", `hyprctl dispatch hl.dsp.send_shortcut({ mods = "CTRL SHIFT", key = "V" })`, "hyprctl dispatch sendshortcut CTRL SHIFT, V,", "wtype -M ctrl -M shift -k v -m shift -m ctrl"},
		},
		{
			name: "other compositors use wtype",
			env:  fakePasteEnv{env: waylandEnv, tools: map[string]bool{"wtype": true, "ydotool": true, "xdotool": true}},
			want: []string{"wtype -M ctrl -k v -m ctrl"},
		},
		{
			name: "wtype failure falls back to ydotool",
			env:  fakePasteEnv{env: waylandEnv, tools: map[string]bool{"wtype": true, "ydotool": true}, fail: map[string]bool{"wtype": true}},
			want: []string{"wtype -M ctrl -k v -m ctrl", "ydotool key 29:1 47:1 47:0 29:0"},
		},
		{
			name:    "Wayland without any input tool names what to install",
			env:     fakePasteEnv{env: map[string]string{"XDG_SESSION_TYPE": "wayland"}},
			wantErr: "install wtype",
		},
		{
			name:    "every backend failing reports the failures",
			env:     fakePasteEnv{env: waylandEnv, tools: map[string]bool{"wtype": true}, fail: map[string]bool{"wtype": true}},
			want:    []string{"wtype -M ctrl -k v -m ctrl"},
			wantErr: "wtype: exit status 1: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.paster().paste()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("paste() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("paste() error = %v, want it to contain %q", err, tt.wantErr)
			}
			if strings.Join(tt.env.calls, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("commands run:\n%s\nwant:\n%s", strings.Join(tt.env.calls, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestIsTerminalWindowClass(t *testing.T) {
	for class, want := range map[string]bool{
		"Alacritty": true, "kitty": true, "com.mitchellh.ghostty": true, "org.wezfurlong.wezterm": true,
		"footclient": true, "chromium": false, "firefox": false, "": false, "code": false,
	} {
		if got := isTerminalWindowClass(class); got != want {
			t.Errorf("isTerminalWindowClass(%q) = %v, want %v", class, got, want)
		}
	}
}
