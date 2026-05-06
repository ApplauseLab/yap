import { useState, useEffect, useCallback } from 'react';
import './App.css';
import { RecordingOverlay } from './RecordingOverlay';
import {
  GetState,
  ToggleRecording,
  CancelRecording,
  GetModels,
  SetModel,
  SetProvider,
  SetOpenAIKey,
  DownloadModel,
  GetConfig,
  SetAutoPaste,
  GetHistory,
  ClearHistory,
  CopyHistoryItem,
  DeleteHistoryItem,
  GetAudioData,
  GetAudioInputDevices,
  SetAudioInputDevice,
  Quit,
} from '../wailsjs/go/main/App';
import { EventsOn, LogInfo } from '../wailsjs/runtime/runtime';

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

interface ModelInfo {
  name: string;
  displayName: string;
  size: string;
  downloaded: boolean;
  englishOnly: boolean;
}

interface Config {
  provider: string;
  model: string;
  openaiApiKey?: string;
  audioInputDevice?: string;
  autoPaste: boolean;
}

interface HistoryItem {
  id: string;
  text: string;
  timestamp: string;
  duration: number;
  audioPath?: string;
  hasAudio: boolean;
}

interface DownloadProgress {
  model: string;
  downloaded: number;
  total: number;
  progress: number;
}

interface AudioInputDevice {
  name: string;
  isDefault: boolean;
}

type Page = 'home' | 'settings' | 'history';

function App() {
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
  const [downloadProgress, setDownloadProgress] = useState<DownloadProgress | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [audioContext, setAudioContext] = useState<AudioContext | null>(null);
  const [audioSource, setAudioSource] = useState<AudioBufferSourceNode | null>(null);
  const [audioDevices, setAudioDevices] = useState<AudioInputDevice[]>([]);
  const [selectedAudioDevice, setSelectedAudioDevice] = useState<string>('');

  useEffect(() => {
    GetState().then((state: AppState) => setAppState(state));
    GetModels().then((models: ModelInfo[]) => setModels(models));
    GetConfig().then((cfg: Config) => {
      setConfig(cfg);
      setApiKey(cfg.openaiApiKey || '');
      setSelectedAudioDevice(cfg.audioInputDevice || '');
    });
    GetHistory().then((h: HistoryItem[]) => setHistory(h));
    GetAudioInputDevices().then((devices: AudioInputDevice[]) => setAudioDevices(devices));

    LogInfo('Setting up EventsOn for stateChanged');
    const cleanup = EventsOn('stateChanged', (state: AppState) => {
      LogInfo('stateChanged event received: ' + state.state);
      setAppState(state);
    });
    LogInfo('EventsOn setup complete');
    EventsOn('historyChanged', (h: HistoryItem[]) => {
      setHistory(h);
      if (h.length > 0 && !selectedHistory) {
        setSelectedHistory(h[0]);
      }
    });
    EventsOn('downloadProgress', (progress: DownloadProgress) => setDownloadProgress(progress));
    EventsOn('downloadComplete', () => {
      setDownloadProgress(null);
      GetModels().then((models: ModelInfo[]) => setModels(models));
    });
    EventsOn('downloadError', (data: { model: string; error: string }) => {
      setDownloadProgress(null);
      alert(`Download failed: ${data.error}`);
    });
  }, []);

  useEffect(() => {
    let interval: number | undefined;
    if (appState.state === 'recording') {
      interval = window.setInterval(() => {
        GetState().then((state: AppState) => setAppState(state));
      }, 100);
    }
    return () => { if (interval) clearInterval(interval); };
  }, [appState.state]);

  const handleToggleRecording = useCallback(async () => {
    try { await ToggleRecording(); } catch (err) { console.error(err); }
  }, []);

  const handleCancelRecording = useCallback(async () => {
    try { await CancelRecording(); } catch (err) { console.error(err); }
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
    // Stop any currently playing audio
    if (audioSource) {
      audioSource.stop();
    }

    try {
      // Get audio data as base64
      const base64Data = await GetAudioData(id);
      
      // Decode base64 to binary
      const binaryString = atob(base64Data);
      const bytes = new Uint8Array(binaryString.length);
      for (let i = 0; i < binaryString.length; i++) {
        bytes[i] = binaryString.charCodeAt(i);
      }
      
      // Create or reuse AudioContext
      let ctx = audioContext;
      if (!ctx) {
        ctx = new AudioContext();
        setAudioContext(ctx);
      }
      
      // Decode audio data
      const audioBuffer = await ctx.decodeAudioData(bytes.buffer);
      
      // Create and play source
      const source = ctx.createBufferSource();
      source.buffer = audioBuffer;
      source.connect(ctx.destination);
      source.onended = () => {
        setIsPlaying(false);
        setAudioSource(null);
      };
      source.start();
      
      setAudioSource(source);
      setIsPlaying(true);
    } catch (err) {
      console.error('Failed to play audio:', err);
      setIsPlaying(false);
    }
  }, [audioContext, audioSource]);

  const handleModelChange = useCallback(async (model: string) => {
    await SetModel(model);
    GetModels().then((m: ModelInfo[]) => setModels(m));
  }, []);

  const handleProviderChange = useCallback(async (provider: string) => {
    await SetProvider(provider);
    GetState().then((s: AppState) => setAppState(s));
  }, []);

  const handleDownloadModel = useCallback(async (model: string) => {
    await DownloadModel(model);
  }, []);

  const handleSaveApiKey = useCallback(async () => {
    await SetOpenAIKey(apiKey);
    GetState().then((s: AppState) => setAppState(s));
  }, [apiKey]);

  const handleAutoPasteChange = useCallback(async (enabled: boolean) => {
    await SetAutoPaste(enabled);
    GetConfig().then((c: Config) => setConfig(c));
  }, []);

  const handleAudioDeviceChange = useCallback(async (deviceName: string) => {
    setSelectedAudioDevice(deviceName);
    await SetAudioInputDevice(deviceName);
  }, []);

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
            {history.length > 0 && <span className="badge">{history.length}</span>}
          </button>
        </nav>

        <div className="sidebar-footer">
          <div className="hotkey-hint">
            <kbd>Right ⌥</kbd>
            <span>to record</span>
          </div>
          {/* Debug: show current state */}
          <div style={{fontSize: '10px', color: '#666', marginTop: '8px'}}>
            State: {appState.state}
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="main">
        {/* Draggable header area */}
        <div className="main-header" style={{ '--wails-draggable': 'drag' } as React.CSSProperties}>
          <span className="page-title">
            {currentPage === 'home' ? 'Home' : currentPage === 'settings' ? 'Settings' : 'History'}
          </span>
        </div>

        {currentPage === 'home' && (
          <div className="home-page">
            <div className="home-content">
              {/* Status */}
              <div className={`status-badge ${appState.state}`}>
                {appState.state === 'recording' && <div className="pulse" />}
                <span>{appState.state === 'ready' ? 'Ready' : 
                       appState.state === 'recording' ? `Recording ${formatTime(appState.recordingTime)}` :
                       appState.state === 'transcribing' ? 'Transcribing...' : 'Error'}</span>
              </div>

              {/* Record Button */}
              <button
                className={`record-button ${appState.state}`}
                onClick={handleToggleRecording}
                disabled={appState.state === 'transcribing' || needsDownload}
              >
                <div className="record-button-inner">
                  {appState.state === 'recording' ? (
                    <svg width="40" height="40" viewBox="0 0 24 24" fill="currentColor">
                      <rect x="6" y="6" width="12" height="12" rx="2"/>
                    </svg>
                  ) : appState.state === 'transcribing' ? (
                    <div className="spinner" />
                  ) : (
                    <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                      <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
                      <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                      <line x1="12" y1="19" x2="12" y2="23"/>
                      <line x1="8" y1="23" x2="16" y2="23"/>
                    </svg>
                  )}
                </div>
              </button>

              <p className="record-hint">
                {appState.state === 'recording' ? 'Click to stop' : 'Click to start recording'}
              </p>

              {/* Download prompt */}
              {needsDownload && !downloadProgress && (
                <div className="download-prompt">
                  <p>Model "{currentModel?.displayName}" needs to be downloaded</p>
                  <button onClick={() => handleDownloadModel(appState.currentModel)}>
                    Download ({currentModel?.size})
                  </button>
                </div>
              )}

              {downloadProgress && (
                <div className="download-progress">
                  <p>Downloading {downloadProgress.model}...</p>
                  <div className="progress-bar">
                    <div className="progress-fill" style={{ width: `${downloadProgress.progress}%` }} />
                  </div>
                  <span>{downloadProgress.progress.toFixed(1)}%</span>
                </div>
              )}

              {/* Last transcript */}
              {appState.lastTranscript && appState.state === 'ready' && (
                <div className="last-transcript">
                  <div className="transcript-header">
                    <span>Last transcription</span>
                    <span className="copied">Copied to clipboard</span>
                  </div>
                  <p>{appState.lastTranscript}</p>
                </div>
              )}

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

            {/* Info bar */}
            <div className="info-bar">
              <div className="info-item">
                <span className="label">Provider</span>
                <span className="value">{appState.currentProvider === 'openai' ? 'OpenAI' : 'Local'}</span>
              </div>
              <div className="info-item">
                <span className="label">Model</span>
                <span className="value">{appState.currentModel}</span>
              </div>
              <div className="info-item">
                <span className="label">Hotkey</span>
                <span className={`value ${appState.hotkeyEnabled ? 'active' : ''}`}>
                  {appState.hotkeyEnabled ? 'Active' : 'Inactive'}
                </span>
              </div>
            </div>
          </div>
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

              <div className="setting-row">
                <div className="setting-info">
                  <label>Model</label>
                  <p>Larger models are more accurate but slower</p>
                </div>
                <select 
                  value={appState.currentModel}
                  onChange={(e) => handleModelChange(e.target.value)}
                >
                  {models.map(m => (
                    <option key={m.name} value={m.name}>
                      {m.displayName} ({m.size}) {m.downloaded ? '✓' : '↓'}
                    </option>
                  ))}
                </select>
              </div>

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
            </section>

            <section className="settings-section">
              <h2>Keyboard Shortcut</h2>
              <div className="hotkey-display">
                <kbd>Right ⌥</kbd>
                <span className={`status ${appState.hotkeyEnabled ? 'active' : ''}`}>
                  {appState.hotkeyEnabled ? 'Active' : 'Not registered'}
                </span>
              </div>
              <p className="hotkey-note">Press Right Option key to start/stop recording</p>
            </section>

            <div className="settings-footer">
              <span>v1.0.0</span>
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
                          {isPlaying ? 'Stop' : 'Play'}
                        </button>
                      )}
                      <button onClick={() => CopyHistoryItem(selectedHistory.id)}>
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
                        </svg>
                        Copy
                      </button>
                      <button 
                        className="delete-btn"
                        onClick={() => {
                          DeleteHistoryItem(selectedHistory.id);
                          setSelectedHistory(null);
                        }}
                      >
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                          <polyline points="3 6 5 6 21 6"/>
                          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                          <line x1="10" y1="11" x2="10" y2="17"/>
                          <line x1="14" y1="11" x2="14" y2="17"/>
                        </svg>
                        Delete
                      </button>
                    </div>
                  </div>
                  <div className="detail-content">
                    <p>{selectedHistory.text}</p>
                  </div>
                  <div className="detail-footer">
                    <span>Duration: {formatDuration(selectedHistory.duration)}</span>
                    {selectedHistory.hasAudio && <span className="has-audio-badge">Audio available</span>}
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
    />
    </>
  );
}

export default App;
