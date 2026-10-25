package hotkey

import (
	"runtime"
	"strings"
)

// ShortLabel returns a compact label for a configured key name on the
// current platform, suitable for small badges such as "Stop ⌥".
func ShortLabel(name string) string {
	return ShortLabelFor(name, runtime.GOOS)
}

// ShortLabelFor returns a compact key label for the given GOOS value. macOS
// uses its modifier symbols; other platforms use short key names.
// Keep in sync with frontend/src/hotkeyLabels.ts.
func ShortLabelFor(name, goos string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	mac := goos == "darwin"
	pick := func(macLabel, pcLabel string) string {
		if mac {
			return macLabel
		}
		return pcLabel
	}
	switch k {
	case "":
		return ""
	case "rightoption", "leftoption", "rightalt", "leftalt":
		return pick("⌥", "Alt")
	case "rightcommand", "leftcommand", "rightcmd", "leftcmd",
		"rightwin", "leftwin", "rightsuper", "leftsuper":
		if goos == "linux" {
			return "Super"
		}
		return pick("⌘", "Win")
	case "rightcontrol", "leftcontrol", "rightctrl", "leftctrl":
		return pick("⌃", "Ctrl")
	case "rightshift", "leftshift":
		return pick("⇧", "Shift")
	case "capslock":
		return pick("⇪", "Caps")
	case "fn", "function":
		return pick("fn", "Fn")
	case "escape", "esc":
		return "Esc"
	case "space":
		return "Space"
	case "tab":
		return pick("⇥", "Tab")
	case "return", "enter":
		return pick("↩", "Enter")
	case "backspace":
		return pick("⌫", "Bksp")
	case "delete":
		return pick("⌫", "Del")
	case "forwarddelete":
		return pick("⌦", "Del")
	case "left", "arrowleft":
		return "←"
	case "right", "arrowright":
		return "→"
	case "up", "arrowup":
		return "↑"
	case "down", "arrowdown":
		return "↓"
	default:
		return strings.ToUpper(k)
	}
}
