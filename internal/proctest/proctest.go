//go:build !windows

// Package proctest inspects child processes from tests. It is only imported
// by _test.go files, so it is never linked into the application.
package proctest

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Child is a direct child process as reported by ps.
type Child struct {
	PID   int
	State string
	Comm  string
}

// Zombie reports whether the child has exited but has not been reaped.
func (c Child) Zombie() bool {
	return strings.HasPrefix(c.State, "Z")
}

// Children lists the direct children of pid.
func Children(pid int) ([]Child, error) {
	var out strings.Builder
	cmd := exec.Command("ps", "-axo", "pid=,ppid=,stat=,comm=")
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	// ps is itself a child of the calling test process; leave it out.
	psPID := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	var children []Child
	for _, line := range strings.Split(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil || ppid != pid {
			continue
		}
		childPID, err := strconv.Atoi(fields[0])
		if err != nil || childPID == psPID {
			continue
		}
		children = append(children, Child{PID: childPID, State: fields[2], Comm: strings.Join(fields[3:], " ")})
	}
	return children, nil
}

// Zombies lists the unreaped children of pid.
func Zombies(pid int) ([]Child, error) {
	children, err := Children(pid)
	if err != nil {
		return nil, err
	}
	var zombies []Child
	for _, child := range children {
		if child.Zombie() {
			zombies = append(zombies, child)
		}
	}
	return zombies, nil
}

// WaitNoChildren polls until pid has no children matching keep, returning
// the remaining matches if the timeout expires first. A nil keep matches
// every child.
func WaitNoChildren(pid int, timeout time.Duration, keep func(Child) bool) ([]Child, error) {
	deadline := time.Now().Add(timeout)
	for {
		children, err := Children(pid)
		if err != nil {
			return nil, err
		}
		var remaining []Child
		for _, child := range children {
			if keep == nil || keep(child) {
				remaining = append(remaining, child)
			}
		}
		if len(remaining) == 0 || time.Now().After(deadline) {
			return remaining, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}
