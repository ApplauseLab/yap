package transcribe

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"yap/internal/logger"
)

const (
	workerStartupTimeout = 30 * time.Second
	workerStopTimeout    = 2 * time.Second
	workerResponseLimit  = 1 << 20
	// workerDiagnosticsLimit bounds retained whisper-server stderr, which
	// grows with every request for the lifetime of the worker.
	workerDiagnosticsLimit = 64 << 10
	// Each Yap process records its worker in its own file so instances that
	// share a data directory (wails dev beside the installed app) never
	// overwrite or delete each other's records. The glob also matches the
	// single shared whisper-worker.pid written by Yap 0.4.2.
	workerPIDFilePattern = "whisper-worker-%d.pid"
	workerPIDFileGlob    = "whisper-worker*.pid"
)

type whisperWorker struct {
	cmd         *exec.Cmd
	done        chan struct{}
	waitErr     error
	client      *http.Client
	baseURL     string
	modelPath   string
	backendPath string
	metal       bool
	diagnostics *lockedBuffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) > workerDiagnosticsLimit {
		p = p[len(p)-workerDiagnosticsLimit:]
	}
	if overflow := b.buf.Len() + len(p) - workerDiagnosticsLimit; overflow > 0 {
		b.buf.Next(overflow)
	}
	b.buf.Write(p)
	return n, nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return trimCommandOutput(b.buf.String())
}

func (e *LocalEngine) transcribePersistent(ctx context.Context, wavData []byte, modelPath string, language string) (string, error) {
	operationCtx, cancel, err := e.operationContext(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()
	ctx = operationCtx

	e.workerMu.Lock()
	defer e.workerMu.Unlock()

	worker, err := e.ensureWorkerLocked(ctx, modelPath)
	if err != nil {
		if ctx.Err() == nil {
			e.serverUnavailable = true
		}
		return "", err
	}
	text, err := worker.transcribe(ctx, wavData, language)
	if err == nil {
		e.lastBackend = workerBackendName(worker)
		return text, nil
	}

	e.stopWorkerLocked()
	if !worker.metal || ctx.Err() != nil {
		if ctx.Err() == nil {
			e.serverUnavailable = true
		}
		return "", err
	}

	e.disableMetal()
	logger.Warning(fmt.Sprintf("Persistent Metal worker failed; retrying on CPU: %v", err))
	worker, startErr := e.startCPUWorkerLocked(ctx, modelPath)
	if startErr != nil {
		if ctx.Err() == nil {
			e.serverUnavailable = true
		}
		return "", fmt.Errorf("Metal worker failed: %v; CPU worker failed to start: %w", err, startErr)
	}
	text, err = worker.transcribe(ctx, wavData, language)
	if err == nil {
		e.lastBackend = workerBackendName(worker)
	} else {
		e.stopWorkerLocked()
		if ctx.Err() == nil {
			e.serverUnavailable = true
		}
	}
	return text, err
}

func (e *LocalEngine) ensureWorkerLocked(ctx context.Context, modelPath string) (*whisperWorker, error) {
	if e.serverUnavailable {
		return nil, fmt.Errorf("persistent whisper worker is disabled for this session")
	}
	if e.worker != nil && e.worker.modelPath == modelPath && e.worker.running() {
		return e.worker, nil
	}
	e.stopWorkerLocked()

	serverBin := e.findWhisperServerBinary()
	if serverBin == "" {
		return nil, fmt.Errorf("whisper-server not found")
	}
	binDir := filepath.Dir(serverBin)
	if e.metalEnabled() {
		metalPath := filepath.Join(binDir, "..", "libexec", "libggml-metal.so")
		if isExecutable(metalPath) {
			worker, err := e.startWorkerLocked(ctx, serverBin, modelPath, whisperBackend{path: metalPath, metal: true})
			if err == nil {
				return worker, nil
			}
			if ctx.Err() != nil {
				return nil, err
			}
			e.disableMetal()
		}
	}
	return e.startCPUWorkerLocked(ctx, modelPath)
}

func (e *LocalEngine) startCPUWorkerLocked(ctx context.Context, modelPath string) (*whisperWorker, error) {
	serverBin := e.findWhisperServerBinary()
	if serverBin == "" {
		return nil, fmt.Errorf("whisper-server not found")
	}
	cpuPath := envValue(e.whisperEnv(nil, serverBin), "GGML_BACKEND_PATH")
	return e.startWorkerLocked(ctx, serverBin, modelPath, whisperBackend{path: cpuPath})
}

func (e *LocalEngine) startWorkerLocked(ctx context.Context, serverBin string, modelPath string, backend whisperBackend) (*whisperWorker, error) {
	port, err := availableLoopbackPort()
	if err != nil {
		return nil, err
	}
	token, err := randomWorkerToken()
	if err != nil {
		return nil, err
	}
	requestPath := "/yap-" + token
	args := []string{"-m", modelPath, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--request-path", requestPath, "--inference-path", "/inference", "--no-timestamps"}
	if strings.Contains(backend.path, "libggml-cpu") {
		args = append(args, "--no-gpu")
	}

	diagnostics := &lockedBuffer{}
	cmd := exec.Command(serverBin, args...)
	cmd.Env = envWithValue(os.Environ(), "GGML_BACKEND_PATH", backend.path)
	cmd.Stdout = io.Discard
	cmd.Stderr = diagnostics
	hideChildConsole(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Record the worker before the (up to workerStartupTimeout) model load so
	// that it is reaped on next launch even if Yap dies while it starts.
	e.writeWorkerPID(cmd.Process.Pid)
	if err := startWorkerWatchdog(cmd.Process.Pid); err != nil {
		logger.Warning(fmt.Sprintf("Failed to start Whisper worker watchdog: %v", err))
	}
	if err := bindToProcessLifetime(cmd.Process); err != nil {
		logger.Warning(fmt.Sprintf("Whisper worker pid=%d is not bound to Yap's lifetime: %v", cmd.Process.Pid, err))
	}

	worker := &whisperWorker{
		cmd:         cmd,
		done:        make(chan struct{}),
		client:      &http.Client{Timeout: 5 * time.Minute},
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d%s", port, requestPath),
		modelPath:   modelPath,
		backendPath: backend.path,
		metal:       backend.metal,
		diagnostics: diagnostics,
	}
	go func() {
		worker.waitErr = cmd.Wait()
		close(worker.done)
	}()

	startupCtx, cancel := context.WithTimeout(ctx, workerStartupTimeout)
	defer cancel()
	if err := worker.waitReady(startupCtx); err != nil {
		worker.stop()
		e.removeWorkerPID()
		return nil, fmt.Errorf("worker startup failed: %w%s; backend=%q output=%q", err, illegalInstructionHint(err), backend.path, diagnostics.String())
	}
	e.worker = worker
	e.lastBackend = workerBackendName(worker)
	logger.Info(fmt.Sprintf("Persistent Whisper worker ready: backend=%s model=%q pid=%d", e.lastBackend, modelPath, cmd.Process.Pid))
	return worker, nil
}

func (e *LocalEngine) stopWorkerLocked() {
	if e.worker != nil {
		e.worker.stop()
		e.worker = nil
		e.removeWorkerPID()
	}
}

func (e *LocalEngine) workerPIDPath() string {
	if e.modelsDir == "" {
		return ""
	}
	return filepath.Join(e.modelsDir, fmt.Sprintf(workerPIDFilePattern, os.Getpid()))
}

func (e *LocalEngine) writeWorkerPID(pid int) {
	path := e.workerPIDPath()
	if path == "" {
		return
	}
	record := fmt.Sprintf("%d %d", pid, os.Getpid())
	if err := os.WriteFile(path, []byte(record), 0o600); err != nil {
		logger.Warning(fmt.Sprintf("Failed to record Whisper worker pid: %v", err))
	}
}

func (e *LocalEngine) removeWorkerPID() {
	if path := e.workerPIDPath(); path != "" {
		_ = os.Remove(path)
	}
}

// ReapOrphanedWorker stops whisper-server processes left running by previous
// Yap processes that exited without shutting them down (crash, force quit,
// SIGKILL).
func (e *LocalEngine) ReapOrphanedWorker() {
	if e.modelsDir == "" {
		return
	}
	paths, err := filepath.Glob(filepath.Join(e.modelsDir, workerPIDFileGlob))
	if err != nil {
		return
	}
	for _, path := range paths {
		reapWorkerRecord(path)
	}
}

func reapWorkerRecord(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		_ = os.Remove(path)
		return
	}
	pid, pidErr := strconv.Atoi(fields[0])
	parentPID, parentErr := strconv.Atoi(fields[1])
	if pidErr != nil || parentErr != nil || pid <= 0 || parentPID <= 0 {
		_ = os.Remove(path)
		return
	}
	// Another running Yap instance (e.g. wails dev beside the installed app)
	// may share this data directory; never touch a worker whose owner lives.
	if parentPID != os.Getpid() && processAlive(parentPID) {
		return
	}
	_ = os.Remove(path)
	if !isWhisperServerProcess(pid) {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if err := process.Kill(); err != nil {
		logger.Warning(fmt.Sprintf("Failed to stop orphaned Whisper worker pid=%d: %v", pid, err))
		return
	}
	logger.Info(fmt.Sprintf("Stopped orphaned Whisper worker pid=%d", pid))
}

func (w *whisperWorker) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.baseURL+"/", nil)
		if err != nil {
			return err
		}
		resp, err := w.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && resp.Header.Get("Server") == "whisper.cpp" {
				return nil
			}
		}
		select {
		case <-w.done:
			return fmt.Errorf("process exited: %w", w.waitErr)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *whisperWorker) transcribe(ctx context.Context, wavData []byte, language string) (string, error) {
	started := time.Now()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "recording.wav")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(wavData); err != nil {
		return "", err
	}
	_ = form.WriteField("response_format", "text")
	_ = form.WriteField("language", language)
	_ = form.WriteField("no_timestamps", "true")
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/inference", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, workerResponseLimit+1))
	if err != nil {
		return "", err
	}
	if len(data) > workerResponseLimit {
		return "", fmt.Errorf("worker response exceeds %d bytes", workerResponseLimit)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("worker returned HTTP %d: %s", resp.StatusCode, trimCommandOutput(string(data)))
	}
	logger.Info(fmt.Sprintf("Persistent Whisper inference complete: backend=%s elapsed=%s", workerBackendName(w), time.Since(started).Round(time.Millisecond)))
	return strings.TrimSpace(string(data)), nil
}

func workerBackendName(worker *whisperWorker) string {
	if worker != nil && worker.metal {
		return "metal-worker"
	}
	return "cpu-worker"
}

func (w *whisperWorker) running() bool {
	select {
	case <-w.done:
		return false
	default:
		return w.cmd.Process != nil
	}
}

func (w *whisperWorker) stop() {
	if w == nil || w.cmd.Process == nil {
		return
	}
	// Windows does not support delivering os.Interrupt to child processes.
	if runtime.GOOS != "windows" {
		_ = w.cmd.Process.Signal(os.Interrupt)
		select {
		case <-w.done:
			return
		case <-time.After(workerStopTimeout):
		}
	}
	_ = w.cmd.Process.Kill()
	select {
	case <-w.done:
	case <-time.After(workerStopTimeout):
	}
}

func availableLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func randomWorkerToken() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
