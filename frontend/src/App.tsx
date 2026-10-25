import { useState, useEffect, useCallback, useRef } from 'react';
import './App.css';
import { RecordingOverlay } from './RecordingOverlay';
import { formatHotkeyLabel, formatHotkeyShortLabel } from './hotkeyLabels';
import { Onboarding } from './Onboarding';
import { ModelsLibrary } from './ModelsLibrary';
import { useModelDownload } from './useModelDownload';
import { ModelInfo, errorMessage, formatBytes, fullModelName, isDownloadActive, isTerminal } from './modelTypes';
import packageJson from '../package.json';
// Sound playback is now handled natively in Go (internal/sounds)
import {
  GetState,
  ToggleRecording,
  CancelRecording,
  GetModels,
  SetModel,
  SetProvider,
  SetOpenAIKey,
  DownloadModel,
  CancelModelDownload,
  DeleteModel,
  SetModelFavorite,
  GetConfig,
  SetAutoPaste,
  SetSoundEnabled,
  GetHistory,
  ClearHistory,
  CopyHistoryItem,
  DeleteHistoryItem,
  GetAudioData,
  ShowInFolder,
  GetAudioInputDevices,
  SetAudioInputDevice,
  GetStats,
  GetRecordingHotkey,
  SetRecordingHotkey,
  GetCancelHotkey,
  SetCancelHotkey,
  GetPlatform,
  CheckAccessibilityPermission,
  RequestAccessibilityPermission,
  ReregisterHotkey,
  Quit,
  IsOnboardingCompleted,
} from '../wailsjs/go/main/App';
import { EventsOn, LogDebug, LogError, LogInfo } from '../wailsjs/runtime/runtime';

interface AppState {
  state: string;
  recordingTime: number;
  lastTranscript: string;
  error: string;
  currentModel: string;
  currentProvider: string;
  modelReady: boolean;
  hotkeyEnabled: boolean;
}

interface Config {
  provider: string;
  model: string;
  openaiApiKey?: string;
  audioInputDevice?: string;
  autoPaste: boolean;
  soundEnabled?: boolean;
}

interface HistoryItem {
  id: string;
  text: string;
  timestamp: string;
  duration: number;
  audioPath?: string;
  hasAudio: boolean;
}

interface AudioInputDevice {
  name: string;
  isDefault: boolean;
}

interface UsageStats {
  averageWPM: number;
  wordsThisWeek: number;
  recordingsThisWeek: number;
  timeSavedThisWeek: number; // in minutes
  totalRecordings: number;
  totalWords: number;
}

type Page = 'home' | 'models' | 'settings' | 'history';
type AccessibilityRecoveryState = 'checking' | 'needed' | 'recovering' | 'error' | 'complete' | 'not-needed';

function App() {
  useEffect(() => {
    LogInfo('[frontend] App component mounted');
  }, []);

  const [appState, setAppState] = useState<AppState>({
    state: 'ready',
    recordingTime: 0,
    lastTranscript: '',
    error: '',
    currentModel: 'base.en',
    currentProvider: 'local',
    modelReady: false,
    hotkeyEnabled: false,
  });
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [config, setConfig] = useState<Config | null>(null);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [selectedHistory, setSelectedHistory] = useState<HistoryItem | null>(null);
  const [currentPage, setCurrentPage] = useState<Page>('home');
  const [apiKey, setApiKey] = useState('');
  const downloadState = useModelDownload();
  const [focusModel, setFocusModel] = useState<string | null>(null);
  const [headerActions, setHeaderActions] = useState<HTMLDivElement | null>(null);
  const [homeDownloadError, setHomeDownloadError] = useState('');
  const [isPlaying, setIsPlaying] = useState(false);
  const [audioContext, setAudioContext] = useState<AudioContext | null>(null);
  const [audioSource, setAudioSource] = useState<AudioBufferSourceNode | null>(null);
  const [audioDevices, setAudioDevices] = useState<AudioInputDevice[]>([]);
  const [selectedAudioDevice, setSelectedAudioDevice] = useState<string>('');
  const [stats, setStats] = useState<UsageStats>({
    averageWPM: 0,
    wordsThisWeek: 0,
    recordingsThisWeek: 0,
    timeSavedThisWeek: 0,
    totalRecordings: 0,
    totalWords: 0,
  });
  const [currentHotkey, setCurrentHotkey] = useState<string>('rightoption');
  const [cancelHotkey, setCancelHotkey] = useState<string>('escape');
  const [showOnboarding, setShowOnboarding] = useState<boolean | null>(null);
  const [platform, setPlatform] = useState<string>('darwin');
  const [hotkeyLabelsReady, setHotkeyLabelsReady] = useState<boolean>(false);
  const [isCapturingHotkey, setIsCapturingHotkey] = useState<boolean>(false);
  const [isCapturingCancelKey, setIsCapturingCancelKey] = useState<boolean>(false);
  const [accessibilityRecovery, setAccessibilityRecovery] = useState<AccessibilityRecoveryState>('checking');
  const [accessibilityRecoveryError, setAccessibilityRecoveryError] = useState('');
  const hotkeyInputRef = useRef<HTMLDivElement>(null);
  const cancelKeyInputRef = useRef<HTMLDivElement>(null);
  const accessibilityRecoveryInFlightRef = useRef(false);

  // Check if onboarding is needed on mount and get platform
  useEffect(() => {
    IsOnboardingCompleted().then((completed: boolean) => {
      setShowOnboarding(!completed);
    });
    GetPlatform().then((p: string) => setPlatform(p));
  }, []);

  const finishAccessibilityRecovery = useCallback(async () => {
    if (accessibilityRecoveryInFlightRef.current) return;
    accessibilityRecoveryInFlightRef.current = true;
    setAccessibilityRecovery('recovering');
    setAccessibilityRecoveryError('');
    try {
      await ReregisterHotkey();
      const state = await GetState() as AppState;
      if (!state.hotkeyEnabled) {
        throw new Error('Yap could not register the global hotkey');
      }
      setAppState(state);
      setAccessibilityRecovery('complete');
      LogInfo('[frontend] Accessibility recovery completed; hotkey re-registered');
      window.setTimeout(() => setAccessibilityRecovery('not-needed'), 700);
    } catch (err) {
      const message = String(err);
      setAccessibilityRecoveryError(message);
      setAccessibilityRecovery('error');
      LogError(`[frontend] Accessibility recovery failed: ${message}`);
    } finally {
      accessibilityRecoveryInFlightRef.current = false;
    }
  }, []);

  const checkAccessibilityRecovery = useCallback(async () => {
    try {
      const granted = await CheckAccessibilityPermission();
      if (granted) {
        await finishAccessibilityRecovery();
      } else {
        setAccessibilityRecovery((current) => current === 'error' ? current : 'needed');
      }
    } catch (err) {
      const message = String(err);
      setAccessibilityRecoveryError(message);
      setAccessibilityRecovery('error');
      LogError(`[frontend] Accessibility recovery check failed: ${message}`);
    }
  }, [finishAccessibilityRecovery]);

  const handleOpenAccessibilitySettings = useCallback(async () => {
    setAccessibilityRecovery('recovering');
    setAccessibilityRecoveryError('');
    try {
      const granted = await RequestAccessibilityPermission();
      if (granted) {
        await finishAccessibilityRecovery();
      } else {
        setAccessibilityRecovery('needed');
      }
    } catch (err) {
      const message = String(err);
      setAccessibilityRecoveryError(message);
      setAccessibilityRecovery('error');
      LogError(`[frontend] Opening Accessibility settings failed: ${message}`);
    }
  }, [finishAccessibilityRecovery]);

  useEffect(() => {
    if (showOnboarding !== false) return;
    if (platform !== 'darwin') {
      setAccessibilityRecovery('not-needed');
      return;
    }
    CheckAccessibilityPermission().then((granted) => {
      if (granted) {
        setAccessibilityRecovery('not-needed');
      } else {
        setAccessibilityRecovery('needed');
        LogInfo('[frontend] Showing Accessibility recovery after startup');
      }
    }).catch((err) => {
      setAccessibilityRecoveryError(String(err));
      setAccessibilityRecovery('error');
    });
  }, [showOnboarding, platform]);

  useEffect(() => {
    if (showOnboarding !== false || platform !== 'darwin') return;
    if (!['needed', 'error'].includes(accessibilityRecovery)) return;

    // Poll only while waiting for the grant. After a registration error, retry
    // on focus or button press so a persistent failure cannot loop every second.
    const interval = accessibilityRecovery === 'needed'
      ? window.setInterval(checkAccessibilityRecovery, 1000)
      : undefined;
    const handleFocus = () => checkAccessibilityRecovery();
    window.addEventListener('focus', handleFocus);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener('focus', handleFocus);
    };
  }, [showOnboarding, platform, accessibilityRecovery, checkAccessibilityRecovery]);

  useEffect(() => {
    LogInfo('[frontend] App initial data load started');
    GetState().then((state: AppState) => setAppState(state));
    GetModels().then((models: ModelInfo[]) => setModels(models));
    GetConfig().then((cfg: Config) => {
      setConfig(cfg);
      setApiKey(cfg.openaiApiKey || '');
      setSelectedAudioDevice(cfg.audioInputDevice || '');
    });
    GetHistory().then((h: HistoryItem[]) => setHistory(h));
    GetAudioInputDevices().then((devices: AudioInputDevice[]) => setAudioDevices(devices));
    GetStats().then((s: UsageStats) => setStats(s));
    // Hotkey badges stay hidden until the real keys and platform are known,
    // so Windows never flashes the macOS defaults.
    Promise.allSettled([
      GetRecordingHotkey().then((h: string) => setCurrentHotkey(h)),
      GetCancelHotkey().then((h: string) => setCancelHotkey(h)),
      GetPlatform().then((p: string) => setPlatform(p)),
    ]).then(() => setHotkeyLabelsReady(true));
    LogInfo('[frontend] App initial data load requested');

    LogInfo('[frontend] Setting up EventsOn for stateChanged');
    const unsubscribers = [EventsOn('stateChanged', (state: AppState) => {
      LogInfo('[frontend] stateChanged event received: ' + state.state);
      setAppState(state);
    })];
    LogInfo('[frontend] EventsOn setup complete');
    unsubscribers.push(EventsOn('historyChanged', (h: HistoryItem[]) => {
      setHistory(h);
      if (h.length > 0 && !selectedHistory) {
        setSelectedHistory(h[0]);
      }
      // Refresh stats when history changes (new transcription)
      GetStats().then((s: UsageStats) => setStats(s));
    }));
    return () => unsubscribers.forEach((unsubscribe) => unsubscribe());
  }, []);

  const refreshModels = useCallback(() => GetModels().then((m: ModelInfo[]) => setModels(m)), []);

  const refreshAppState = useCallback(() => {
    GetState().then((s: AppState) => setAppState(s));
  }, []);

  // Download completion only refreshes availability; activation is always an
  // explicit user action (or Onboarding's own selected model).
  useEffect(() => {
    if (!isTerminal(downloadState)) return;
    refreshModels();
    refreshAppState();
  }, [downloadState.status, downloadState.model, refreshModels, refreshAppState]);

  useEffect(() => {
    let interval: number | undefined;
    if (appState.state === 'recording') {
      interval = window.setInterval(() => {
        GetState().then((state: AppState) => setAppState(state));
      }, 100);
    }
    return () => { if (interval) clearInterval(interval); };
  }, [appState.state]);

  // Track previous state for UI updates
  const prevStateRef = useRef<string>('ready');
  
  // NOTE: Sound playback is now handled natively in Go (internal/sounds)
  // This ensures reliable playback even when app is idle or in background

  // Track if we triggered via button
  const buttonTriggeredRef = useRef(false);

  // Track state changes
  useEffect(() => {
    const prevState = prevStateRef.current;
    const currentState = appState.state;
    
    // Track state transitions for button trigger flag
    if (prevState !== 'recording' && currentState === 'recording') {
      // Recording started
    }
    
    buttonTriggeredRef.current = false;
    prevStateRef.current = currentState;
  }, [appState.state]);

  const handleToggleRecording = useCallback(async () => {
    const isCurrentlyRecording = appState.state === 'recording';
    try {
      if (!isCurrentlyRecording) {
        buttonTriggeredRef.current = true;
      }
      // Sound playback is handled in Go before recording starts
      await ToggleRecording();
    } catch (err) { LogError(`[frontend] ToggleRecording failed: ${err}`); }
  }, [appState.state]);

  const handleCancelRecording = useCallback(async () => {
    try { await CancelRecording(); } catch (err) { LogError(`[frontend] CancelRecording failed: ${err}`); }
  }, []);

  // Global escape key handler - always active at app level
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && appState.state === 'recording') {
        e.preventDefault();
        handleCancelRecording();
      }
    };
    
    // Use document to capture all keyboard events
    document.addEventListener('keydown', handleKeyDown, true);
    return () => document.removeEventListener('keydown', handleKeyDown, true);
  }, [appState.state, handleCancelRecording]);

  const stopAudio = useCallback(() => {
    if (audioSource) {
      audioSource.stop();
      setAudioSource(null);
    }
    setIsPlaying(false);
  }, [audioSource]);

  const playAudio = useCallback(async (id: string) => {
    LogDebug(`[frontend] playAudio called with id: ${id}`);
    
    // Stop any currently playing audio - wrap in try-catch to handle already-stopped sources
    if (audioSource) {
      try {
        audioSource.stop();
      } catch {
        // Already stopped, ignore InvalidStateError
      }
      setAudioSource(null);
    }

    try {
      // Get audio data as base64
      LogDebug('[frontend] Fetching audio data');
      const base64Data = await GetAudioData(id);
      LogDebug(`[frontend] Got audio data, length: ${base64Data.length}`);
      
      // Decode base64 to binary
      const binaryString = atob(base64Data);
      const bytes = new Uint8Array(binaryString.length);
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i);
      }
      LogDebug(`[frontend] Decoded bytes: ${bytes.length}`);
      
      // Create or reuse AudioContext (recreate if closed)
      let ctx = audioContext;
      if (!ctx || ctx.state === 'closed') {
        ctx = new AudioContext();
        setAudioContext(ctx);
      }
      LogDebug(`[frontend] AudioContext state: ${ctx.state}`);
      
      // Resume if suspended (needed for some browsers)
      if (ctx.state === 'suspended') {
        await ctx.resume();
      }
      
      // Decode audio data
      LogDebug('[frontend] Decoding audio buffer');
      const audioBuffer = await ctx.decodeAudioData(bytes.buffer);
      LogDebug(`[frontend] Audio buffer decoded, duration: ${audioBuffer.duration}`);
      
      // Create and play source
      const source = ctx.createBufferSource();
      source.buffer = audioBuffer;
      source.connect(ctx.destination);
      source.onended = () => {
        setIsPlaying(false);
        setAudioSource(null);
      };
      source.start();
      LogDebug('[frontend] Audio playback started');
      
      setAudioSource(source);
      setIsPlaying(true);
    } catch (err) {
      LogError(`[frontend] Failed to play audio: ${err}`);
      setIsPlaying(false);
    }
  }, [audioContext, audioSource]);

  const handleDownloadModel = useCallback(async (model: string) => {
    await DownloadModel(model);
  }, []);

  const handleCancelDownload = useCallback(async (model: string) => {
    await CancelModelDownload(model);
  }, []);

  const handleUseModel = useCallback(async (model: string) => {
    await SetModel(model);
    refreshModels();
    refreshAppState();
  }, [refreshModels, refreshAppState]);

  const handleDeleteModel = useCallback(async (model: string) => {
    try {
      await DeleteModel(model);
    } finally {
      refreshModels();
      refreshAppState();
    }
  }, [refreshModels, refreshAppState]);

  const handleToggleFavorite = useCallback(async (model: string, favorite: boolean) => {
    try {
      await SetModelFavorite(model, favorite);
    } finally {
      await refreshModels();
    }
  }, [refreshModels]);

  const openModelsLibrary = useCallback((focus?: string) => {
    setFocusModel(focus ?? null);
    setCurrentPage('models');
  }, []);

  const handleHomeDownload = useCallback(async (model: string) => {
    setHomeDownloadError('');
    try {
      await DownloadModel(model);
    } catch (err) {
      setHomeDownloadError(errorMessage(err));
    }
  }, []);

  const handleProviderChange = useCallback(async (provider: string) => {
    await SetProvider(provider);
    GetState().then((s: AppState) => setAppState(s));
  }, []);

  const handleSaveApiKey = useCallback(async () => {
    await SetOpenAIKey(apiKey);
    GetState().then((s: AppState) => setAppState(s));
  }, [apiKey]);

  const handleAutoPasteChange = useCallback(async (enabled: boolean) => {
    await SetAutoPaste(enabled);
    GetConfig().then((c: Config) => setConfig(c));
  }, []);

  const handleSoundEnabledChange = useCallback(async (enabled: boolean) => {
    await SetSoundEnabled(enabled);
    GetConfig().then((c: Config) => setConfig(c));
  }, []);

  const handleAudioDeviceChange = useCallback(async (deviceName: string) => {
    setSelectedAudioDevice(deviceName);
    await SetAudioInputDevice(deviceName);
  }, []);

  const handleHotkeyChange = useCallback(async (keyName: string) => {
    setCurrentHotkey(keyName);
    try {
      await SetRecordingHotkey(keyName);
    } catch (error) {
      LogError(`[frontend] Failed to set hotkey: ${error}`);
    }
  }, []);

  const handleCancelKeyChange = useCallback(async (keyName: string) => {
    setCancelHotkey(keyName);
    try {
      await SetCancelHotkey(keyName);
    } catch (error) {
      LogError(`[frontend] Failed to set cancel key: ${error}`);
    }
  }, []);

  // Map a keyboard event to a key name
  const mapKeyEventToKeyName = useCallback((e: React.KeyboardEvent<HTMLDivElement>): string | null => {
    const key = e.key;
    const code = e.code;
    
    // Map modifier keys by their location
    if (key === 'Alt') {
      return code === 'AltRight' ? (platform === 'darwin' ? 'rightoption' : 'rightalt') 
                                 : (platform === 'darwin' ? 'leftoption' : 'leftalt');
    }
    if (key === 'Control') {
      return code === 'ControlRight' ? 'rightctrl' : 'leftctrl';
    }
    if (key === 'Shift') {
      return code === 'ShiftRight' ? 'rightshift' : 'leftshift';
    }
    if (key === 'Meta') {
      return code === 'MetaRight' ? (platform === 'darwin' ? 'rightcommand' : 'rightwin')
                                  : (platform === 'darwin' ? 'leftcommand' : 'leftwin');
    }
    if (key === 'CapsLock') return 'capslock';
    if (key === 'Fn' || code === 'Fn') return 'fn';
    
    // Map special keys
    if (key === 'Escape') return 'escape';
    if (key === ' ') return 'space';
    if (key === 'Tab') return 'tab';
    if (key === 'Enter') return 'return';
    if (key === 'Backspace') return 'backspace';
    if (key === 'Delete') return 'delete';
    if (key === 'ArrowLeft') return 'left';
    if (key === 'ArrowRight') return 'right';
    if (key === 'ArrowUp') return 'up';
    if (key === 'ArrowDown') return 'down';
    
    // Function keys
    if (key.match(/^F\d+$/)) return key.toLowerCase();
    
    // Letter and number keys
    if (key.length === 1 && key.match(/[a-zA-Z0-9]/)) {
      return key.toLowerCase();
    }
    
    return null;
  }, [platform]);

  // Handle hotkey capture from key press
  const handleHotkeyCapture = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!isCapturingHotkey) return;
    
    e.preventDefault();
    e.stopPropagation();
    
    const keyName = mapKeyEventToKeyName(e);
    if (keyName) {
      handleHotkeyChange(keyName);
      setIsCapturingHotkey(false);
    }
  }, [isCapturingHotkey, mapKeyEventToKeyName, handleHotkeyChange]);

  // Handle cancel key capture from key press
  const handleCancelKeyCapture = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!isCapturingCancelKey) return;
    
    e.preventDefault();
    e.stopPropagation();
    
    const keyName = mapKeyEventToKeyName(e);
    if (keyName) {
      handleCancelKeyChange(keyName);
      setIsCapturingCancelKey(false);
    }
  }, [isCapturingCancelKey, mapKeyEventToKeyName, handleCancelKeyChange]);

  // Click outside to cancel capture
  useEffect(() => {
    if (!isCapturingHotkey && !isCapturingCancelKey) return;
    
    const handleClickOutside = (e: MouseEvent) => {
      if (isCapturingHotkey && hotkeyInputRef.current && !hotkeyInputRef.current.contains(e.target as Node)) {
        setIsCapturingHotkey(false);
      }
      if (isCapturingCancelKey && cancelKeyInputRef.current && !cancelKeyInputRef.current.contains(e.target as Node)) {
        setIsCapturingCancelKey(false);
      }
    };
    
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isCapturingHotkey, isCapturingCancelKey]);

  const formatKeyDisplay = (keyName: string): string => formatHotkeyLabel(keyName, platform);
  const getHotkeyShortDisplay = (keyName: string): string => formatHotkeyShortLabel(keyName, platform);

  const formatTime = (seconds: number): string => {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${secs.toString().padStart(2, '0')}`;
  };

  const formatDuration = (seconds: number): string => {
    return `${seconds.toFixed(1)}s`;
  };

  const currentModel = models.find(m => m.name === appState.currentModel);
  const needsDownload = appState.currentProvider === 'local' && currentModel && !currentModel.downloaded;
  const downloadActive = isDownloadActive(downloadState);
  const downloadingModel = models.find(m => m.name === downloadState.model);
  const currentModelFailed = downloadState.model === appState.currentModel && downloadState.status === 'failed';
  const currentModelCancelled = downloadState.model === appState.currentModel && downloadState.status === 'cancelled';

  const handleOnboardingComplete = useCallback(() => {
    setShowOnboarding(false);
    // Refresh models and state after onboarding
    GetModels().then((models: ModelInfo[]) => setModels(models));
    GetState().then((state: AppState) => setAppState(state));
  }, []);

  // Show loading state while checking onboarding status
  if (showOnboarding === null) {
    return (
      <div className="app-loading">
        <div className="loading-spinner" />
      </div>
    );
  }

  // Show onboarding if not completed
  if (showOnboarding) {
    return <Onboarding onComplete={handleOnboardingComplete} />;
  }

  if (accessibilityRecovery === 'checking') {
    return (
      <div className="app-loading">
        <div className="loading-spinner" />
      </div>
    );
  }

  if (accessibilityRecovery !== 'not-needed') {
    const isRecovering = accessibilityRecovery === 'recovering';
    const isComplete = accessibilityRecovery === 'complete';
    return (
      <div className="accessibility-recovery">
        <div className="accessibility-recovery-card">
          <div className={`accessibility-recovery-icon ${isComplete ? 'complete' : ''}`}>
            {isComplete ? (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            ) : (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                <path d="M9 12h6" />
                <path d="M12 9v6" />
              </svg>
            )}
          </div>

          <div className="accessibility-recovery-copy">
            <span className="accessibility-recovery-eyebrow">One quick macOS check</span>
            <h1>{isComplete ? 'Yap is ready again' : 'Yap needs Accessibility access again'}</h1>
            {isComplete ? (
              <p>Your recording hotkey has been restored. Opening Yap...</p>
            ) : (
              <>
                <p>After an update, macOS can stop recognizing Yap's existing permission. Your models, history, and settings are safe.</p>
                <div className="accessibility-recovery-steps">
                  <span>1</span>
                  <p>Open Accessibility Settings.</p>
                  <span>2</span>
                  <p>If Yap is already enabled, toggle it off and back on.</p>
                  <span>3</span>
                  <p>Return here. Yap will detect access automatically.</p>
                </div>
              </>
            )}
          </div>

          {!isComplete && (
            <div className="accessibility-recovery-actions">
              <button className="recovery-primary" onClick={handleOpenAccessibilitySettings} disabled={isRecovering}>
                {isRecovering ? 'Checking Access...' : 'Open Accessibility Settings'}
              </button>
              <button className="recovery-secondary" onClick={checkAccessibilityRecovery} disabled={isRecovering}>
                Check Again
              </button>
            </div>
          )}

          {accessibilityRecovery === 'error' && (
            <p className="accessibility-recovery-error">{accessibilityRecoveryError || 'Yap still cannot register the recording hotkey. Toggle access off and back on, then check again.'}</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <>
    <div className="app">
      {/* Sidebar */}
      <div className="sidebar">
        <div className="sidebar-header" style={{ '--wails-draggable': 'drag' } as React.CSSProperties} />

        <nav className="sidebar-nav">
          <button 
            className={`nav-item ${currentPage === 'home' ? 'active' : ''}`}
            onClick={() => setCurrentPage('home')}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
              <polyline points="9 22 9 12 15 12 15 22"/>
            </svg>
            <span>Home</span>
          </button>

          <button
            className={`nav-item ${currentPage === 'models' ? 'active' : ''}`}
            onClick={() => openModelsLibrary()}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
              <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
              <line x1="12" y1="22.08" x2="12" y2="12"/>
            </svg>
            <span>Models</span>
            {downloadActive && <span className="badge" aria-label="Download in progress">{downloadState.progress.toFixed(0)}%</span>}
          </button>

          <button 
            className={`nav-item ${currentPage === 'settings' ? 'active' : ''}`}
            onClick={() => setCurrentPage('settings')}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
            <span>Settings</span>
          </button>

          <button 
            className={`nav-item ${currentPage === 'history' ? 'active' : ''}`}
            onClick={() => setCurrentPage('history')}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="10"/>
              <polyline points="12 6 12 12 16 14"/>
            </svg>
            <span>History</span>
          </button>
        </nav>

        <div className="sidebar-footer">
          {hotkeyLabelsReady && (
            <div className="hotkey-hint">
              <kbd>{formatKeyDisplay(currentHotkey)}</kbd>
              <span>to record</span>
            </div>
          )}
        </div>
      </div>

      {/* Main Content */}
      <div className="main">
        {/* Draggable header area */}
        <div className="main-header" style={{ '--wails-draggable': 'drag' } as React.CSSProperties}>
          <span className="page-title">
            {currentPage === 'home' ? 'Home' : currentPage === 'models' ? 'Models library' : currentPage === 'settings' ? 'Settings' : 'History'}
          </span>
          {currentPage === 'models' && (
            <div
              className="header-actions"
              ref={setHeaderActions}
              style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}
            />
          )}
        </div>

        {currentPage === 'home' && (
          <div className="home-page">
            {/* Stats Bar */}
            <div className="stats-bar">
              <div>
                <div className="stat-value">{Math.round(stats.averageWPM)}</div>
                <div className="stat-label">WPM</div>
              </div>
              <div>
                <div className="stat-value">{stats.wordsThisWeek}</div>
                <div className="stat-label">Words</div>
              </div>
              <div>
                <div className="stat-value">{stats.recordingsThisWeek}</div>
                <div className="stat-label">Recordings</div>
              </div>
              <div>
                <div className="stat-value">{Math.round(stats.timeSavedThisWeek)}</div>
                <div className="stat-label">Min Saved</div>
              </div>
            </div>

            {/* Download prompt if needed */}
            {needsDownload && !(downloadActive && downloadState.model === appState.currentModel) && (
              <div className="download-prompt home-download">
                <p>
                  {currentModelFailed
                    ? `Downloading "${currentModel ? fullModelName(currentModel) : appState.currentModel}" failed.`
                    : currentModelCancelled
                      ? `Download of "${currentModel ? fullModelName(currentModel) : appState.currentModel}" was cancelled.`
                      : `Model "${currentModel ? fullModelName(currentModel) : appState.currentModel}" needs to be downloaded`}
                </p>
                {currentModelFailed && <p className="download-prompt-error">{downloadState.error}</p>}
                {homeDownloadError && <p className="download-prompt-error">{homeDownloadError}</p>}
                <div className="download-prompt-actions">
                  <button
                    onClick={() => handleHomeDownload(appState.currentModel)}
                    disabled={downloadActive}
                    title={downloadActive ? 'Another model is downloading' : undefined}
                  >
                    {currentModelFailed || currentModelCancelled ? 'Retry download' : `Download (${currentModel?.size})`}
                  </button>
                  <button className="link-button" onClick={() => openModelsLibrary(appState.currentModel)}>
                    Browse models
                  </button>
                </div>
              </div>
            )}

            {downloadActive && (
              <div className="download-progress">
                <div className="download-icon">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                    <polyline points="7 10 12 15 17 10"/>
                    <line x1="12" y1="15" x2="12" y2="3"/>
                  </svg>
                </div>
                <p>
                  {downloadState.status === 'cancelling' ? 'Cancelling' : 'Downloading'} {downloadingModel ? fullModelName(downloadingModel) : downloadState.model}
                </p>
                <div className="download-subtitle">
                  {formatBytes(downloadState.downloaded)} of {formatBytes(downloadState.total || downloadingModel?.sizeBytes || 0)}
                </div>
                <div className="progress-bar">
                  <div className="progress-fill" style={{ width: `${downloadState.progress}%` }} />
                </div>
                <span>{downloadState.progress.toFixed(0)}%</span>
                <div className="download-progress-actions">
                  <button className="link-button" onClick={() => openModelsLibrary(downloadState.model)}>
                    View in Models
                  </button>
                  <button
                    className="link-button"
                    disabled={downloadState.status === 'cancelling'}
                    onClick={() => handleCancelDownload(downloadState.model).catch((err) => LogError(`[frontend] Cancel download failed: ${err}`))}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {/* Get Started Section */}
            <div className="get-started-section">
              <h3 className="section-title">Get started</h3>
              <div className="action-list">
                <div
                  className={`action-item ${needsDownload ? 'disabled' : ''}`}
                  onClick={needsDownload ? undefined : handleToggleRecording}
                >
                  <div className="action-icon">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <circle cx="12" cy="12" r="10"/>
                      <polygon points="10 8 16 12 10 16 10 8"/>
                    </svg>
                  </div>
                  <div className="action-content">
                    <span className="action-title">Start recording</span>
                    <span className="action-desc">
                      {needsDownload ? 'Download selected model first' : 'Turn your voice to text with a single click'}
                    </span>
                  </div>
                  {hotkeyLabelsReady && (
                    <kbd className="action-shortcut">{getHotkeyShortDisplay(currentHotkey)}</kbd>
                  )}
                </div>

                <div className="action-item" onClick={() => setCurrentPage('settings')}>
                  <div className="action-icon">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <circle cx="12" cy="12" r="3"/>
                      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
                    </svg>
                  </div>
                  <div className="action-content">
                    <span className="action-title">Settings</span>
                    <span className="action-desc">Configure provider, audio device, hotkeys, and more</span>
                  </div>
                </div>

                <div className="action-item" onClick={() => openModelsLibrary()}>
                  <div className="action-icon">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                      <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
                      <line x1="12" y1="22.08" x2="12" y2="12"/>
                    </svg>
                  </div>
                  <div className="action-content">
                    <span className="action-title">Browse models</span>
                    <span className="action-desc">Compare, download, and switch transcription models</span>
                  </div>
                </div>

                <div className="action-item" onClick={() => setCurrentPage('history')}>
                  <div className="action-icon">
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <circle cx="12" cy="12" r="10"/>
                      <polyline points="12 6 12 12 16 14"/>
                    </svg>
                  </div>
                  <div className="action-content">
                    <span className="action-title">View history</span>
                    <span className="action-desc">Browse and replay past transcriptions</span>
                  </div>
                </div>
              </div>
            </div>

            {/* What's New Section */}
            <div className="whats-new-section">
              <h3 className="section-title">What's new</h3>
              <div className="changelog-list">
                <div className="changelog-item">
                  <span className="changelog-date">May 2026</span>
                  <div className="changelog-content">
                    <span className="changelog-title">Native overlay with waveform</span>
                    <span className="changelog-desc">Recording overlay now shows above all apps with animated waveform visualization.</span>
                  </div>
                </div>
                <div className="changelog-item">
                  <span className="changelog-date">May 2026</span>
                  <div className="changelog-content">
                    <span className="changelog-title">Auto-paste transcriptions</span>
                    <span className="changelog-desc">Transcribed text is automatically pasted into your active text field.</span>
                  </div>
                </div>
                <div className="changelog-item">
                  <span className="changelog-date">May 2026</span>
                  <div className="changelog-content">
                    <span className="changelog-title">Usage statistics</span>
                    <span className="changelog-desc">Track your words per minute, time saved, and weekly usage.</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Error display */}
            {appState.error && appState.state === 'error' && (
              <div className="error-message">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="12" y1="8" x2="12" y2="12"/>
                  <line x1="12" y1="16" x2="12.01" y2="16"/>
                </svg>
                <span>{appState.error}</span>
              </div>
            )}
          </div>
        )}

        {currentPage === 'models' && (
          <ModelsLibrary
            models={models}
            activeModel={appState.currentModel}
            provider={appState.currentProvider}
            downloadState={downloadState}
            focusModel={focusModel}
            toolbarTarget={headerActions}
            onDownload={handleDownloadModel}
            onCancel={handleCancelDownload}
            onUse={handleUseModel}
            onDelete={handleDeleteModel}
            onToggleFavorite={handleToggleFavorite}
          />
        )}

        {currentPage === 'settings' && (
          <div className="settings-page">

            <section className="settings-section">
              <h2>Transcription</h2>
              
              <div className="setting-row">
                <div className="setting-info">
                  <label>Provider</label>
                  <p>Choose between local processing or cloud API</p>
                </div>
                <div className="toggle-buttons">
                  <button 
                    className={appState.currentProvider === 'local' ? 'active' : ''}
                    onClick={() => handleProviderChange('local')}
                  >
                    Local
                  </button>
                  <button 
                    className={appState.currentProvider === 'openai' ? 'active' : ''}
                    onClick={() => handleProviderChange('openai')}
                  >
                    OpenAI
                  </button>
                </div>
              </div>

              {appState.currentProvider === 'local' ? (
                <div className="setting-row">
                  <div className="setting-info">
                    <label>Model</label>
                    <p>Larger models are more accurate but slower</p>
                  </div>
                  <div className="model-summary">
                    <div className="model-summary-text">
                      <strong>{currentModel ? fullModelName(currentModel) : appState.currentModel}</strong>
                      {currentModel
                        ? `${currentModel.size} · ${currentModel.englishOnly ? 'English only' : 'Multilingual'} · ${
                            downloadActive && downloadState.model === currentModel.name
                              ? `Downloading ${downloadState.progress.toFixed(0)}%`
                              : currentModel.downloaded ? 'Downloaded' : 'Not downloaded'
                          }`
                        : ''}
                    </div>
                    <button className="link-button" onClick={() => openModelsLibrary(appState.currentModel)}>
                      Browse Models
                    </button>
                  </div>
                </div>
              ) : (
                <div className="setting-row">
                  <div className="setting-info">
                    <label>Model</label>
                    <p>OpenAI transcribes with its hosted Whisper model, so local models are not used.</p>
                  </div>
                </div>
              )}

              {appState.currentProvider === 'openai' && (
                <div className="setting-row">
                  <div className="setting-info">
                    <label>OpenAI API Key</label>
                    <p>Required for cloud transcription</p>
                  </div>
                  <div className="api-key-input">
                    <input
                      type="password"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      placeholder="sk-..."
                    />
                    <button onClick={handleSaveApiKey}>Save</button>
                  </div>
                </div>
              )}
            </section>

            <section className="settings-section">
              <h2>Audio Input</h2>
              
              <div className="setting-row">
                <div className="setting-info">
                  <label>Microphone</label>
                  <p>Select the audio input device for recording</p>
                </div>
                <select 
                  value={selectedAudioDevice}
                  onChange={(e) => handleAudioDeviceChange(e.target.value)}
                >
                  <option value="">System Default</option>
                  {audioDevices.map(device => (
                    <option key={device.name} value={device.name}>
                      {device.name}{device.isDefault ? ' (Default)' : ''}
                    </option>
                  ))}
                </select>
              </div>
            </section>

            <section className="settings-section">
              <h2>Behavior</h2>
              
              <div className="setting-row">
                <div className="setting-info">
                  <label>Auto-paste</label>
                  <p>Automatically paste transcription into active app</p>
                </div>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={config?.autoPaste ?? true}
                    onChange={(e) => handleAutoPasteChange(e.target.checked)}
                  />
                  <span className="slider" />
                </label>
              </div>

              <div className="setting-row">
                <div className="setting-info">
                  <label>Sound feedback</label>
                  <p>Play sound when starting/stopping recording</p>
                </div>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={config?.soundEnabled !== false}
                    onChange={(e) => handleSoundEnabledChange(e.target.checked)}
                  />
                  <span className="slider" />
                </label>
              </div>
            </section>

            <section className="settings-section">
              <h2>Keyboard Shortcuts</h2>
              
              <div className="setting-row">
                <div className="setting-info">
                  <label>Recording Key</label>
                  <p>Press this key to start/stop recording</p>
                </div>
                <div
                  ref={hotkeyInputRef}
                  className={`hotkey-capture ${isCapturingHotkey ? 'capturing' : ''}`}
                  tabIndex={0}
                  onClick={() => setIsCapturingHotkey(true)}
                  onKeyDown={handleHotkeyCapture}
                  onBlur={() => setIsCapturingHotkey(false)}
                >
                  {isCapturingHotkey ? (
                    <span className="capture-prompt">Press any key...</span>
                  ) : (
                    <span className="hotkey-display">{formatKeyDisplay(currentHotkey)}</span>
                  )}
                </div>
              </div>
              
              <div className="setting-row">
                <div className="setting-info">
                  <label>Cancel Key</label>
                  <p>Press this key to cancel recording</p>
                </div>
                <div
                  ref={cancelKeyInputRef}
                  className={`hotkey-capture ${isCapturingCancelKey ? 'capturing' : ''}`}
                  tabIndex={0}
                  onClick={() => setIsCapturingCancelKey(true)}
                  onKeyDown={handleCancelKeyCapture}
                  onBlur={() => setIsCapturingCancelKey(false)}
                >
                  {isCapturingCancelKey ? (
                    <span className="capture-prompt">Press any key...</span>
                  ) : (
                    <span className="hotkey-display">{formatKeyDisplay(cancelHotkey)}</span>
                  )}
                </div>
              </div>
              
              <div className="hotkey-status">
                <span className={`status ${appState.hotkeyEnabled ? 'active' : ''}`}>
                  {appState.hotkeyEnabled ? 'Hotkeys Active' : 'Hotkeys Need Accessibility Access'}
                </span>
                {!appState.hotkeyEnabled && platform === 'darwin' && (
                  <p>Open System Settings → Privacy &amp; Security → Accessibility, then toggle Yap off and back on.</p>
                )}
              </div>
            </section>

            <div className="settings-footer">
              <span>v{packageJson.version}</span>
              <span className="powered-link">Powered by <a href="https://applauselab.ai" target="_blank" rel="noopener noreferrer">applauselab.ai</a></span>
            </div>
          </div>
        )}

        {currentPage === 'history' && (
          <div className="history-page">
            <div className="history-list">
              <div className="history-header">
                <h2>History</h2>
                {history.length > 0 && (
                  <button className="clear-btn" onClick={() => ClearHistory()}>Clear all</button>
                )}
              </div>
              
              {history.length === 0 ? (
                <div className="empty-state">
                  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <circle cx="12" cy="12" r="10"/>
                    <polyline points="12 6 12 12 16 14"/>
                  </svg>
                  <p>No recordings yet</p>
                  <span>Your transcriptions will appear here</span>
                </div>
              ) : (
                <div className="history-items">
                  {history.map((item) => (
                    <button
                      key={item.id}
                      className={`history-item ${selectedHistory?.id === item.id ? 'selected' : ''}`}
                      onClick={() => setSelectedHistory(item)}
                    >
                      <p className="history-text">{item.text}</p>
                      <div className="history-meta">
                        <span>{item.timestamp}</span>
                        <span>{formatDuration(item.duration)}</span>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>

            <div className="history-detail">
              {selectedHistory ? (
                <>
                  <div className="detail-header">
                    <span>{selectedHistory.timestamp}</span>
                    <div className="detail-actions">
                      {selectedHistory.hasAudio && (
                        <button 
                          className={`play-btn ${isPlaying ? 'playing' : ''}`}
                          onClick={() => isPlaying ? stopAudio() : playAudio(selectedHistory.id)}
                          title={isPlaying ? 'Stop' : 'Play'}
                        >
                          {isPlaying ? (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                              <rect x="6" y="4" width="4" height="16" rx="1"/>
                              <rect x="14" y="4" width="4" height="16" rx="1"/>
                            </svg>
                          ) : (
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                              <polygon points="5 3 19 12 5 21 5 3"/>
                            </svg>
                          )}
                        </button>
                      )}
                      <button onClick={() => CopyHistoryItem(selectedHistory.id)} title="Copy">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
                        </svg>
                      </button>
                      {selectedHistory.hasAudio && (
                        <button onClick={() => ShowInFolder(selectedHistory.id).catch((error) => {
                          LogError(`[frontend] ShowInFolder failed: ${error}`);
                          alert(`Could not show recording in folder: ${error}`);
                        })} title="Show in Folder">
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                          </svg>
                        </button>
                      )}
                      <button 
                        className="delete-btn"
                        onClick={() => {
                          DeleteHistoryItem(selectedHistory.id);
                          setSelectedHistory(null);
                        }}
                        title="Delete"
                      >
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <polyline points="3 6 5 6 21 6"/>
                          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                          <line x1="10" y1="11" x2="10" y2="17"/>
                          <line x1="14" y1="11" x2="14" y2="17"/>
                        </svg>
                      </button>
                    </div>
                  </div>
                  <div className="detail-content">
                    <p>{selectedHistory.text}</p>
                  </div>
                  <div className="detail-footer">
                    <span>Duration: {formatDuration(selectedHistory.duration)}</span>
                  </div>
                </>
              ) : (
                <div className="empty-detail">
                  <p>Select a recording to view details</p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>

    {/* Recording Overlay - outside main app container */}
    <RecordingOverlay
      isRecording={appState.state === 'recording'}
      onStop={handleToggleRecording}
      onCancel={handleCancelRecording}
      stopKeyLabel={getHotkeyShortDisplay(currentHotkey)}
      cancelKeyLabel={getHotkeyShortDisplay(cancelHotkey)}
    />
    </>
  );
}

export default App;
