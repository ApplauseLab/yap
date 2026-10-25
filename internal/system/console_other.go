//go:build !windows

package system

import "os/exec"

func hideConsole(*exec.Cmd) {}
