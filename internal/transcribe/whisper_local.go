package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// LocalEngine uses local whisper.cpp for transcription
type LocalEngine struct {
	modelMu             sync.RWMutex
	model               Model
	modelsDir           string
	whisperBin          string // Path to whisper CLI binary
	whisperServerBin    string
	whisperServerSet    bool
	backendMu           sync.Mutex
	metalUnavailable    bool
	workerMu            sync.Mutex
	worker              *whisperWorker
	lastBackend         string
	serverUnavailable   bool
	lifecycleMu         sync.Mutex
	lifecycleCtx        context.Context
	lifecycleCancel     context.CancelFunc
	lifecycleGeneration uint64
	closed              bool
}

type whisperBackend struct {
	path  string
	metal bool
}

// NewLocalEngine creates a new local whisper.cpp engine
func NewLocalEngine(modelsDir string) *LocalEngine {
	engine := &LocalEngine{
		model:     ModelBaseEn,
		modelsDir: modelsDir,
	}
	engine.lifecycleCtx, engine.lifecycleCancel = context.WithCancel(context.Background())
	return engine
}

// SetWhisperBinary sets the path to the whisper CLI binary
func (e *LocalEngine) SetWhisperBinary(path string) {
	e.whisperBin = path
}

// SetWhisperServerBinary sets the persistent whisper server binary path.
func (e *LocalEngine) SetWhisperServerBinary(path string) {
	e.resetOperations()
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	e.stopWorkerLocked()
	e.whisperServerBin = path
	e.whisperServerSet = true
}

// Transcribe converts audio samples to text
func (e *LocalEngine) Transcribe(ctx context.Context, samples []float32) (string, error) {
	wavData, err := samplesToWAV(samples)
	if err != nil {
		return "", fmt.Errorf("failed to convert samples: %w", err)
	}
	return e.TranscribeWAV(ctx, wavData)
}

// TranscribeWAV transcribes WAV audio data using whisper CLI
func (e *LocalEngine) TranscribeWAV(ctx context.Context, wavData []byte) (string, error) {
	generation, err := e.currentOperationGeneration()
	if err != nil {
		return "", err
	}

	modelPath, language := e.modelSnapshot()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", fmt.Errorf("model file not found: %s. Please download the model first", modelPath)
	}

	var workerErr error
	if e.findWhisperServerBinary() != "" {
		text, err := e.transcribePersistent(ctx, wavData, modelPath, language)
		if err == nil {
			return text, nil
		}
		workerErr = err
		if !e.operationGenerationIsCurrent(generation) {
			return "", workerErr
		}
	}

	whisperBin := e.findWhisperBinary()
	if whisperBin == "" {
		if workerErr != nil {
			return "", fmt.Errorf("persistent whisper worker failed: %w; whisper-cli fallback not found", workerErr)
		}
		return "", fmt.Errorf("whisper runtime not found. Please install whisper.cpp or set the binary path")
	}

	// Create temp file for audio
	tmpFile, err := os.CreateTemp("", "whisper-input-*.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	outputPath := tmpFile.Name() + ".txt"
	defer os.Remove(outputPath)

	if _, err := tmpFile.Write(wavData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write audio: %w", err)
	}
	tmpFile.Close()

	baseArgs := []string{
		"-m", modelPath,
		"-f", tmpFile.Name(),
		"-l", language,
		"--no-timestamps",
		"-otxt",
		"-of", tmpFile.Name(),
	}
	for _, backend := range e.transcriptionBackends(whisperBin) {
		_ = os.Remove(outputPath)
		args := append([]string{}, baseArgs...)
		if strings.Contains(backend.path, "libggml-cpu") {
			args = append(args, "--no-gpu")
		}
		cmdEnv := envWithValue(os.Environ(), "GGML_BACKEND_PATH", backend.path)
		cmd := exec.CommandContext(ctx, whisperBin, args...)
		cmd.Env = cmdEnv
		// whisper-cli echoes the transcript on stdout; keep only stderr so
		// diagnostics never write dictated text to the log file.
		var stderr bytes.Buffer
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr
		hideChildConsole(cmd)

		err := cmd.Run()
		output := stderr.Bytes()
		if err != nil {
			if backend.metal && ctx.Err() == nil {
				e.disableMetal()
				continue
			}
			return "", e.commandError("whisper failed", err, output, whisperBin, modelPath, cmdEnv)
		}

		textBytes, err := os.ReadFile(outputPath)
		if err != nil {
			return "", fmt.Errorf("failed to read whisper output file %q: %w; command output=%q", outputPath, err, trimCommandOutput(string(output)))
		}

		e.workerMu.Lock()
		e.lastBackend = "whisper-cli"
		e.workerMu.Unlock()
		return strings.TrimSpace(string(textBytes)), nil
	}
	return "", fmt.Errorf("no usable whisper backend found")
}

// SetModel sets the model to use
func (e *LocalEngine) SetModel(model Model) error {
	if e.GetModel() == model {
		return nil
	}
	e.resetOperations()
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	e.modelMu.Lock()
	defer e.modelMu.Unlock()
	if e.model != model {
		e.stopWorkerLocked()
	}
	e.model = model
	return nil
}

// Close stops the persistent local transcription worker.
func (e *LocalEngine) Close() error {
	e.lifecycleMu.Lock()
	e.closed = true
	e.lifecycleGeneration++
	e.lifecycleCancel()
	e.lifecycleMu.Unlock()
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	e.stopWorkerLocked()
	return nil
}

// Open allows worker startup after switching back to the local provider.
func (e *LocalEngine) Open() {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if !e.closed {
		return
	}
	e.lifecycleCtx, e.lifecycleCancel = context.WithCancel(context.Background())
	e.lifecycleGeneration++
	e.closed = false
}

// GetModel returns the current model
func (e *LocalEngine) GetModel() Model {
	e.modelMu.RLock()
	defer e.modelMu.RUnlock()
	return e.model
}

// Prewarm starts the persistent worker and loads the selected model.
func (e *LocalEngine) Prewarm(ctx context.Context) error {
	operationCtx, cancel, err := e.operationContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	modelPath := e.getModelPath()
	if _, err := os.Stat(modelPath); err != nil {
		return err
	}
	_, err = e.ensureWorkerLocked(operationCtx, modelPath)
	return err
}

func (e *LocalEngine) operationContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if e.closed {
		return nil, nil, fmt.Errorf("local transcription engine is closed")
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(e.lifecycleCtx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}, nil
}

func (e *LocalEngine) resetOperations() {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if e.closed {
		return
	}
	e.lifecycleCancel()
	e.lifecycleCtx, e.lifecycleCancel = context.WithCancel(context.Background())
	e.lifecycleGeneration++
}

func (e *LocalEngine) currentOperationGeneration() (uint64, error) {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if e.closed {
		return 0, fmt.Errorf("local transcription engine is closed")
	}
	return e.lifecycleGeneration, nil
}

func (e *LocalEngine) operationGenerationIsCurrent(generation uint64) bool {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return !e.closed && e.lifecycleGeneration == generation
}

// Backend reports the active local transcription backend.
func (e *LocalEngine) Backend() string {
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	if e.worker != nil {
		if e.worker.metal {
			return "metal-worker"
		}
		return "cpu-worker"
	}
	if e.lastBackend != "" {
		return e.lastBackend
	}
	return "not-started"
}

// IsAvailable checks if the engine is ready
func (e *LocalEngine) IsAvailable() bool {
	// Check if model file exists
	modelPath := e.getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return false
	}

	return e.findWhisperServerBinary() != "" || e.findWhisperBinary() != ""
}

// ValidateRuntime checks that the resolved whisper-cli can launch with the same
// environment used for transcription.
func (e *LocalEngine) ValidateRuntime(ctx context.Context) error {
	whisperBin := e.findWhisperBinary()
	if whisperBin == "" {
		return fmt.Errorf("whisper-cli not found. Please install whisper.cpp or set the binary path")
	}

	cmd := exec.CommandContext(ctx, whisperBin, "--help")
	cmd.Env = e.whisperEnv(os.Environ(), whisperBin)
	hideChildConsole(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return e.commandError("whisper runtime validation failed", err, output, whisperBin, e.getModelPath(), cmd.Env)
	}
	return nil
}

func (e *LocalEngine) findWhisperBinary() string {
	if e.whisperBin != "" {
		if isExecutable(e.whisperBin) {
			return e.whisperBin
		}
		if p, err := exec.LookPath(e.whisperBin); err == nil {
			return p
		}
	}

	for _, p := range bundledWhisperCandidates() {
		if isExecutable(p) {
			return p
		}
	}

	for _, p := range systemWhisperCandidates() {
		if isExecutable(p) {
			return p
		}
	}

	for _, name := range whisperBinaryNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func (e *LocalEngine) findWhisperServerBinary() string {
	if e.whisperServerSet {
		if isExecutable(e.whisperServerBin) {
			return e.whisperServerBin
		}
		if p, err := exec.LookPath(e.whisperServerBin); err == nil {
			return p
		}
		return ""
	}

	for _, p := range bundledWhisperServerCandidates() {
		if isExecutable(p) {
			return p
		}
	}
	for _, name := range whisperServerBinaryNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func bundledWhisperCandidates() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	exeDir := filepath.Dir(exe)
	names := whisperBinaryNames()
	candidates := make([]string, 0, len(names)*4)
	for _, name := range names {
		switch runtime.GOOS {
		case "darwin":
			candidates = append(candidates,
				filepath.Join(exeDir, "..", "Resources", "bin", name),
				filepath.Join(exeDir, name),
			)
		case "windows":
			candidates = append(candidates,
				filepath.Join(exeDir, "bin", name),
				filepath.Join(exeDir, name),
			)
		default:
			candidates = append(candidates,
				filepath.Join(exeDir, name),
				filepath.Join(exeDir, "bin", name),
				filepath.Join(exeDir, "..", "lib", "yap", "bin", name),
			)
		}
	}
	return candidates
}

func bundledWhisperServerCandidates() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	exeDir := filepath.Dir(exe)
	candidates := make([]string, 0, 4)
	for _, name := range whisperServerBinaryNames() {
		switch runtime.GOOS {
		case "darwin":
			candidates = append(candidates, filepath.Join(exeDir, "..", "Resources", "bin", name), filepath.Join(exeDir, name))
		case "windows":
			candidates = append(candidates, filepath.Join(exeDir, "bin", name), filepath.Join(exeDir, name))
		default:
			candidates = append(candidates, filepath.Join(exeDir, name), filepath.Join(exeDir, "bin", name), filepath.Join(exeDir, "..", "lib", "yap", "bin", name))
		}
	}
	return candidates
}

func systemWhisperCandidates() []string {
	home := os.Getenv("HOME")
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/opt/homebrew/bin/whisper-cli",
			"/usr/local/bin/whisper-cli",
			filepath.Join(home, ".local/bin/whisper-cli"),
		}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "whisper.cpp", "bin", "whisper-cli.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "whisper.cpp", "whisper-cli.exe"),
		}
	default:
		return []string{
			"/usr/bin/whisper-cli",
			"/usr/local/bin/whisper-cli",
			filepath.Join(home, ".local/bin/whisper-cli"),
		}
	}
}

func whisperBinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"whisper-cli.exe", "whisper-cli"}
	}
	return []string{"whisper-cli"}
}

func whisperServerBinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"whisper-server.exe", "whisper-server"}
	}
	return []string{"whisper-server"}
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (e *LocalEngine) whisperEnv(env []string, whisperBin string) []string {
	binDir := filepath.Dir(whisperBin)
	if runtime.GOOS == "windows" {
		return append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	backendCandidates := []string{
		filepath.Join(binDir, "..", "libexec", preferredAppleCPUBackend()),
		filepath.Join(binDir, "..", "libexec", "libggml-cpu-apple_m1.so"),
		filepath.Join(binDir, "..", "libexec", "libggml-cpu-apple_m2_m3.so"),
		filepath.Join(binDir, "..", "libexec", "libggml-cpu-apple_m4.so"),
	}
	for _, p := range backendCandidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return envWithValue(env, "GGML_BACKEND_PATH", p)
		}
	}
	return env
}

func (e *LocalEngine) transcriptionBackends(whisperBin string) []whisperBackend {
	binDir := filepath.Dir(whisperBin)
	backends := make([]whisperBackend, 0, 2)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && e.metalEnabled() {
		metalPath := filepath.Join(binDir, "..", "libexec", "libggml-metal.so")
		if info, err := os.Stat(metalPath); err == nil && !info.IsDir() {
			backends = append(backends, whisperBackend{path: metalPath, metal: true})
		}
	}

	cpuPath := envValue(e.whisperEnv(nil, whisperBin), "GGML_BACKEND_PATH")
	backends = append(backends, whisperBackend{path: cpuPath})
	return backends
}

func (e *LocalEngine) metalEnabled() bool {
	e.backendMu.Lock()
	defer e.backendMu.Unlock()
	return !e.metalUnavailable
}

func (e *LocalEngine) disableMetal() {
	e.backendMu.Lock()
	e.metalUnavailable = true
	e.backendMu.Unlock()
}

func preferredAppleCPUBackend() string {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return "libggml-cpu-apple_m1.so"
	}

	brand, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "libggml-cpu-apple_m1.so"
	}
	cpu := string(brand)
	switch {
	case strings.Contains(cpu, "M4"):
		return "libggml-cpu-apple_m4.so"
	case strings.Contains(cpu, "M2"), strings.Contains(cpu, "M3"):
		return "libggml-cpu-apple_m2_m3.so"
	default:
		return "libggml-cpu-apple_m1.so"
	}
}

func (e *LocalEngine) commandError(prefix string, err error, output []byte, whisperBin string, modelPath string, env []string) error {
	backendPath := envValue(env, "GGML_BACKEND_PATH")
	if backendPath == "" {
		backendPath = "(unset)"
	}

	return fmt.Errorf(
		"%s: %w%s; whisperBin=%q modelPath=%q GGML_BACKEND_PATH=%q output=%q",
		prefix,
		err,
		illegalInstructionHint(err),
		whisperBin,
		modelPath,
		backendPath,
		trimCommandOutput(string(output)),
	)
}

// illegalInstructionHint explains crashes of the bundled x86-64 whisper.cpp
// runtime, which is built for an AVX2 (x86-64-v3) baseline.
func illegalInstructionHint(err error) string {
	if runtime.GOARCH != "amd64" || !isIllegalInstructionExit(err) {
		return ""
	}
	return " (illegal instruction: this CPU likely lacks AVX2, which the bundled whisper.cpp runtime requires)"
}

func envWithValue(env []string, key string, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	if value != "" {
		result = append(result, prefix+value)
	}
	return result
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func trimCommandOutput(output string) string {
	output = strings.TrimSpace(output)
	const maxLen = 4000
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// Name returns the provider name
func (e *LocalEngine) Name() Provider {
	return ProviderLocal
}

// modelSnapshot returns the current model's file path and whisper.cpp
// language together, so a concurrent SetModel cannot mix the two.
func (e *LocalEngine) modelSnapshot() (path string, language string) {
	e.modelMu.RLock()
	defer e.modelMu.RUnlock()
	return filepath.Join(e.modelsDir, fmt.Sprintf("ggml-%s.bin", e.model)), e.model.WhisperLanguage()
}

// getModelPath returns the full path to the model file
func (e *LocalEngine) getModelPath() string {
	e.modelMu.RLock()
	defer e.modelMu.RUnlock()
	modelFile := fmt.Sprintf("ggml-%s.bin", e.model)
	return filepath.Join(e.modelsDir, modelFile)
}

// GetModelPath returns the expected model file path (for download)
func (e *LocalEngine) GetModelPath() string {
	return e.getModelPath()
}

// GetModelsDir returns the models directory
func (e *LocalEngine) GetModelsDir() string {
	return e.modelsDir
}
