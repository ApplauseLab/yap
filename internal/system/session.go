package system

import (
	"os"
	"runtime"
)

// IsWaylandSession reports whether Yap runs in a Linux Wayland session, where
// X11 key grabs and xdotool only reach XWayland windows.
func IsWaylandSession() bool {
	return runtime.GOOS == "linux" && isWaylandEnv(os.Getenv)
}

// IsHyprlandSession reports whether the Wayland compositor is Hyprland.
func IsHyprlandSession() bool {
	return IsWaylandSession() && os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != ""
}

func isWaylandEnv(getenv func(string) string) bool {
	return getenv("WAYLAND_DISPLAY") != "" || getenv("XDG_SESSION_TYPE") == "wayland"
}
