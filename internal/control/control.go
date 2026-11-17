// Package control lets a second `yap <command>` process drive the running app
// over a per-user Unix socket. Wayland compositors do not let applications
// grab global keys, so shortcuts are bound in the compositor to these commands.
package control

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Commands understood by the running app.
const (
	CommandToggle = "toggle"
	CommandStart  = "start"
	CommandStop   = "stop"
	CommandCancel = "cancel"
	CommandShow   = "show"
	CommandStatus = "status"
)

// ErrAlreadyRunning is returned by Listen when another instance answers on the socket.
var ErrAlreadyRunning = errors.New("another Yap instance is already running")

// ErrNotRunning is returned by Send when no instance is listening.
var ErrNotRunning = errors.New("Yap is not running")

const ioTimeout = 5 * time.Second

// IsCommand reports whether name is a command the running app understands.
func IsCommand(name string) bool {
	switch name {
	case CommandToggle, CommandStart, CommandStop, CommandCancel, CommandShow, CommandStatus:
		return true
	}
	return false
}

// SocketPath returns the per-user control socket path.
func SocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "yap.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("yap-%d.sock", os.Getuid()))
}

// Handler runs a command and returns a short reply for the caller.
type Handler func(command string) (string, error)

// Server accepts control commands for the running app.
type Server struct {
	listener net.Listener
	path     string
	handler  Handler
	wg       sync.WaitGroup
}

// Listen starts serving commands on path. A socket left behind by a crashed
// instance is replaced; a live one yields ErrAlreadyRunning.
func Listen(path string, handler Handler) (*Server, error) {
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		return nil, ErrAlreadyRunning
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove stale control socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to restrict control socket: %w", err)
	}
	s := &Server{listener: listener, path: path, handler: handler}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(ioTimeout))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		// A bare connect is how a new instance probes for a running one.
		return
	}
	command := strings.TrimSpace(line)
	if !IsCommand(command) {
		fmt.Fprintf(conn, "error unknown command %q\n", command)
		return
	}
	reply, err := s.run(command)
	conn.SetDeadline(time.Now().Add(ioTimeout))
	if err != nil {
		fmt.Fprintf(conn, "error %s\n", singleLine(err.Error()))
		return
	}
	fmt.Fprintf(conn, "ok %s\n", singleLine(reply))
}

func (s *Server) run(command string) (reply string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("command panicked: %v", r)
		}
	}()
	return s.handler(command)
}

// Close stops the server and removes its socket.
func (s *Server) Close() error {
	err := s.listener.Close()
	s.wg.Wait()
	os.Remove(s.path)
	return err
}

// Send delivers command to the running app and returns its reply.
func Send(path, command string) (string, error) {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return "", ErrNotRunning
	}
	defer conn.Close()
	// Stopping a recording replies only once the recorder has shut down.
	conn.SetDeadline(time.Now().Add(2 * ioTimeout))
	if _, err := fmt.Fprintf(conn, "%s\n", command); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read reply: %w", err)
	}
	status, message, _ := strings.Cut(strings.TrimSpace(line), " ")
	if status != "ok" {
		return "", errors.New(message)
	}
	return message, nil
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
