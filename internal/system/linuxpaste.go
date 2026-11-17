package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// linuxPaster simulates the paste shortcut on Linux. The environment is
// injected so the tool selection can be tested on any platform.
type linuxPaster struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
	run      func(name string, args ...string) ([]byte, error)
}

func newLinuxPaster() linuxPaster {
	return linuxPaster{
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	}
}

// Terminals paste with Ctrl+Shift+V; Ctrl+V reaches the shell instead.
var terminalWindowClasses = []string{
	"alacritty", "kitty", "ghostty", "foot", "wezterm", "konsole", "ptyxis",
	"gnome-terminal", "org.gnome.console", "blackbox", "tilix", "terminator",
	"xfce4-terminal", "warp",
}

func isTerminalWindowClass(class string) bool {
	class = strings.ToLower(class)
	if class == "" {
		return false
	}
	for _, terminal := range terminalWindowClasses {
		if strings.Contains(class, terminal) {
			return true
		}
	}
	return false
}

type hyprWindow struct {
	Class string `json:"class"`
}

func (p linuxPaster) has(tool string) bool {
	_, err := p.lookPath(tool)
	return err == nil
}

func (p linuxPaster) hyprActiveWindow() hyprWindow {
	var window hyprWindow
	if out, err := p.run("hyprctl", "activewindow", "-j"); err == nil {
		json.Unmarshal(out, &window)
	}
	return window
}

func (p linuxPaster) paste() error {
	if !isWaylandEnv(p.getenv) {
		return p.xdotool(false)
	}

	hyprland := p.getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && p.has("hyprctl")
	var window hyprWindow
	if hyprland {
		window = p.hyprActiveWindow()
	}
	shift := isTerminalWindowClass(window.Class)

	type backend struct {
		tool  string
		paste func(shift bool) error
	}
	var backends []backend
	if hyprland {
		backends = append(backends, backend{"hyprctl", p.hyprctl})
	}
	// xdotool comes last: on Wayland it only reaches XWayland windows.
	backends = append(backends, backend{"wtype", p.wtype}, backend{"ydotool", p.ydotool}, backend{"xdotool", p.xdotool})

	var failures []error
	for _, b := range backends {
		if !p.has(b.tool) {
			continue
		}
		err := b.paste(shift)
		if err == nil {
			return nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return errors.New("failed to simulate paste: install wtype or ydotool (the text is on the clipboard)")
	}
	return fmt.Errorf("failed to simulate paste: %w", errors.Join(failures...))
}

func (p linuxPaster) exec(name string, args ...string) error {
	if out, err := p.run(name, args...); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// hyprctl asks Hyprland to send the shortcut to the active window. Hyprland
// 0.55 moved dispatchers to Lua, and a Lua config ignores the classic syntax
// without reporting an error, so the Lua form is tried first; older versions
// reject it and get the classic form.
func (p linuxPaster) hyprctl(shift bool) error {
	mods := "CTRL"
	if shift {
		mods = "CTRL SHIFT"
	}
	luaErr := p.hyprDispatch(fmt.Sprintf(`hl.dsp.send_shortcut({ mods = "%s", key = "V" })`, mods))
	if luaErr == nil {
		return nil
	}
	// An empty window selector targets the active window.
	if err := p.hyprDispatch("sendshortcut", mods+", V,"); err != nil {
		return errors.Join(luaErr, err)
	}
	return nil
}

func (p linuxPaster) hyprDispatch(args ...string) error {
	out, err := p.run("hyprctl", append([]string{"dispatch"}, args...)...)
	reply := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("hyprctl: %w: %s", err, reply)
	}
	// hyprctl exits 0 even when the dispatcher rejects the request.
	if reply != "ok" {
		return fmt.Errorf("hyprctl: %s", reply)
	}
	return nil
}

func (p linuxPaster) wtype(shift bool) error {
	if shift {
		return p.exec("wtype", "-M", "ctrl", "-M", "shift", "-k", "v", "-m", "shift", "-m", "ctrl")
	}
	return p.exec("wtype", "-M", "ctrl", "-k", "v", "-m", "ctrl")
}

// ydotool takes Linux input event codes: 29 Left Ctrl, 42 Left Shift, 47 V.
func (p linuxPaster) ydotool(shift bool) error {
	if shift {
		return p.exec("ydotool", "key", "29:1", "42:1", "47:1", "47:0", "42:0", "29:0")
	}
	return p.exec("ydotool", "key", "29:1", "47:1", "47:0", "29:0")
}

func (p linuxPaster) xdotool(shift bool) error {
	keys := "ctrl+v"
	if shift {
		keys = "ctrl+shift+v"
	}
	if err := p.exec("xdotool", "key", keys); err != nil {
		return fmt.Errorf("failed to simulate paste (ensure xdotool is installed): %w", err)
	}
	return nil
}
