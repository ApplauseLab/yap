import { useState, useEffect, useCallback, useRef } from 'react';
import './Onboarding.css';
import appIcon from './assets/appicon.png';
import {
  GetModels,
  DownloadModel,
  SetModel,
  SetProvider,
  SetOpenAIKey,
  CheckMicrophonePermission,
  RequestMicrophonePermission,
  CheckAccessibilityPermission,
  RequestAccessibilityPermission,
  ReregisterHotkey,
  SetOnboardingCompleted,
  GetRecordingHotkeyDisplayName,
  GetPlatform,
} from '../wailsjs/go/main/App';
import { EventsOn, LogError, LogInfo } from '../wailsjs/runtime/runtime';

interface ModelInfo {
  name: string;
  displayName: string;
  size: string;
  downloaded: boolean;
  englishOnly: boolean;
}

interface DownloadProgress {
  model: string;
  downloaded: number;
  total: number;
  progress: number;
}

interface AppState {
  state: string;
  recordingTime: number;
  lastTranscript: string;
  error: string;
}

interface OnboardingProps {
  onComplete: () => void;
}

type Step = 'welcome' | 'provider' | 'model' | 'download' | 'apikey' | 'micRequest' | 'micSuccess' | 'accessRequest' | 'accessSuccess' | 'hotkeyTest' | 'ready';
type Provider = 'local' | 'openai';
type HotkeyTestState = 'waiting' | 'recording' | 'success';

export function Onboarding({ onComplete }: OnboardingProps) {
  useEffect(() => {
    LogInfo('[frontend] Onboarding component mounted');
  }, []);

  const [step, setStep] = useState<Step>('welcome');
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [selectedModel, setSelectedModel] = useState<string>('base.en');
  const [selectedProvider, setSelectedProvider] = useState<Provider>('local');
  const [apiKey, setApiKey] = useState<string>('');
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<DownloadProgress | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [micPermissionStatus, setMicPermissionStatus] = useState<string>('undetermined');
  const [accessibilityStatus, setAccessibilityStatus] = useState<string>('undetermined');
  const [hotkeyName, setHotkeyName] = useState<string>('Right Option');
  const [platform, setPlatform] = useState<string>('darwin');
  const [hotkeyTestState, setHotkeyTestState] = useState<HotkeyTestState>('waiting');
  const [testTranscript, setTestTranscript] = useState<string>('');
  const [hotkeyTestError, setHotkeyTestError] = useState<string>('');
  const hotkeyTestStateRef = useRef<HotkeyTestState>('waiting');

  useEffect(() => {
    // Load models
    GetModels().then((modelList: ModelInfo[]) => {
      setModels(modelList);
      // Check if base.en is already downloaded
      const baseModel = modelList.find(m => m.name === 'base.en');
      if (baseModel?.downloaded) {
        setSelectedModel('base.en');
      }
    });

    // Get hotkey display name
    GetRecordingHotkeyDisplayName().then((name: string) => {
      setHotkeyName(name);
    });

    // Get platform
    GetPlatform().then((p: string) => {
      setPlatform(p);
    });

    // Set up event listeners for download progress
    const progressHandler = (progress: DownloadProgress) => {
      setDownloadProgress(progress);
      setDownloadError(null);
    };

    const completeHandler = async (data: { model: string }) => {
      setDownloadProgress(null);
      try {
        await SetModel(data.model);
      } catch (err) {
        setDownloadError(String(err));
        return;
      }
      // Refresh models list
      GetModels().then((modelList: ModelInfo[]) => {
        setModels(modelList);
      });
      // Move to mic permission request step
      setStep('micRequest');
    };

    const errorHandler = (data: { model: string; error: string }) => {
      setDownloadProgress(null);
      setDownloadError(data.error);
    };

    const cleanupProgress = EventsOn('downloadProgress', progressHandler);
    const cleanupComplete = EventsOn('downloadComplete', completeHandler);
    const cleanupError = EventsOn('downloadError', errorHandler);

    return () => {
      cleanupProgress();
      cleanupComplete();
      cleanupError();
    };
  }, []);

  const handleProviderSelect = useCallback(async () => {
    await SetProvider(selectedProvider);
    if (selectedProvider === 'openai') {
      setStep('apikey');
    } else {
      setStep('model');
    }
  }, [selectedProvider]);

  const handleApiKeySave = useCallback(async () => {
    if (!apiKey.trim()) {
      setApiKeyError('Please enter an API key');
      return;
    }
    if (!apiKey.startsWith('sk-')) {
      setApiKeyError('API key should start with "sk-"');
      return;
    }
    setApiKeyError(null);
    await SetOpenAIKey(apiKey);
    setStep('micRequest');
  }, [apiKey]);

  const handleModelSelect = useCallback(async () => {
    const model = models.find(m => m.name === selectedModel);
    if (model?.downloaded) {
      await SetModel(selectedModel);
      // Model already downloaded, skip to mic permission request
      setStep('micRequest');
    } else {
      // Start download
      setStep('download');
      setDownloadError(null);
      await DownloadModel(selectedModel);
    }
  }, [selectedModel, models]);

  const handleRetryDownload = useCallback(async () => {
    setDownloadError(null);
    await DownloadModel(selectedModel);
  }, [selectedModel]);

  const handleCheckMicPermission = useCallback(async () => {
    const status = await CheckMicrophonePermission();
    setMicPermissionStatus(status);
  }, []);

  const handleRequestMicPermission = useCallback(async () => {
    const status = await RequestMicrophonePermission();
    setMicPermissionStatus(status);
  }, []);

  const handleCheckAccessibilityPermission = useCallback(async () => {
    const granted = await CheckAccessibilityPermission();
    setAccessibilityStatus(granted ? 'granted' : 'undetermined');
  }, []);

  const handleRequestAccessibilityPermission = useCallback(async () => {
    const granted = await RequestAccessibilityPermission();
    setAccessibilityStatus(granted ? 'granted' : 'denied');
    // If granted, the hotkey is automatically re-registered by the backend
  }, []);

  const handleComplete = useCallback(async () => {
    await SetOnboardingCompleted(true);
    onComplete();
  }, [onComplete]);

  // Check mic permission when entering mic request step
  useEffect(() => {
    if (step === 'micRequest') {
      handleCheckMicPermission();
    }
  }, [step, handleCheckMicPermission]);

  // Skip the microphone prompt when macOS already has a grant for this app.
  useEffect(() => {
    if (step === 'micRequest' && micPermissionStatus === 'granted') {
      setStep('micSuccess');
    }
  }, [step, micPermissionStatus]);

  // Poll for accessibility permission when on access request step
  useEffect(() => {
    if (step === 'accessRequest') {
      handleCheckAccessibilityPermission();
      const interval = setInterval(async () => {
        const granted = await CheckAccessibilityPermission();
        if (granted) {
          setAccessibilityStatus('granted');
          setStep('accessSuccess');
        }
      }, 1000);
      return () => clearInterval(interval);
    }
  }, [step, handleCheckAccessibilityPermission]);

  // Handle hotkey test - listen for state changes
  useEffect(() => {
    if (step === 'hotkeyTest') {
      // Reset test state when entering this step
      setHotkeyTestState('waiting');
      setTestTranscript('');
      setHotkeyTestError('');
      hotkeyTestStateRef.current = 'waiting';
      
      // Make sure hotkey is registered
      ReregisterHotkey().catch((err) => LogError(`[frontend] ReregisterHotkey failed: ${err}`));
      
      const stateHandler = (state: AppState) => {
        if (state.state === 'recording' && hotkeyTestStateRef.current === 'waiting') {
          setHotkeyTestState('recording');
          setHotkeyTestError('');
          hotkeyTestStateRef.current = 'recording';
        } else if (state.state === 'transcribing' && hotkeyTestStateRef.current === 'recording') {
          setHotkeyTestState('success');
          hotkeyTestStateRef.current = 'success';
        } else if (state.state === 'ready' && hotkeyTestStateRef.current === 'success') {
          if (state.lastTranscript) {
            setTestTranscript(state.lastTranscript);
          }
        } else if (state.state === 'error' && hotkeyTestStateRef.current === 'recording') {
          setHotkeyTestState('waiting');
          setHotkeyTestError(state.error || 'Recording failed. Check microphone permission and try again.');
          hotkeyTestStateRef.current = 'waiting';
        }
      };
      
      const cleanup = EventsOn('stateChanged', stateHandler);
      return cleanup;
    }
  }, [step]);

  const renderWelcome = () => (
    <div className="onboarding-step welcome-step">
      <div className="welcome-icon">
        <img src={appIcon} alt="Yap" />
      </div>
      <h1 className="welcome-title">Welcome to Yap</h1>
      <p className="welcome-subtitle">Voice-to-text, instantly</p>
      <button className="primary-button" onClick={() => setStep('provider')}>
        Get Started
      </button>
    </div>
  );

  const renderProviderSelection = () => (
    <div className="onboarding-step provider-step">
      <div className="step-header">
        <h2>Choose Transcription Method</h2>
        <p>How would you like to transcribe your voice?</p>
      </div>
      
      <div className="provider-list">
        <div 
          className={`provider-card ${selectedProvider === 'local' ? 'selected' : ''}`}
          onClick={() => setSelectedProvider('local')}
        >
          <div className="provider-radio">
            <div className={`radio-dot ${selectedProvider === 'local' ? 'active' : ''}`} />
          </div>
          <div className="provider-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="4" y="4" width="16" height="16" rx="2" ry="2"/>
              <rect x="9" y="9" width="6" height="6"/>
              <line x1="9" y1="1" x2="9" y2="4"/>
              <line x1="15" y1="1" x2="15" y2="4"/>
              <line x1="9" y1="20" x2="9" y2="23"/>
              <line x1="15" y1="20" x2="15" y2="23"/>
              <line x1="20" y1="9" x2="23" y2="9"/>
              <line x1="20" y1="14" x2="23" y2="14"/>
              <line x1="1" y1="9" x2="4" y2="9"/>
              <line x1="1" y1="14" x2="4" y2="14"/>
            </svg>
          </div>
          <div className="provider-info">
            <div className="provider-name">
              Local (Whisper)
              <span className="recommended-badge">Recommended</span>
            </div>
            <div className="provider-desc">
              Runs entirely on your Mac. Private, fast, and works offline. Requires a one-time model download.
            </div>
          </div>
        </div>

        <div 
          className={`provider-card ${selectedProvider === 'openai' ? 'selected' : ''}`}
          onClick={() => setSelectedProvider('openai')}
        >
          <div className="provider-radio">
            <div className={`radio-dot ${selectedProvider === 'openai' ? 'active' : ''}`} />
          </div>
          <div className="provider-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 2L2 7l10 5 10-5-10-5z"/>
              <path d="M2 17l10 5 10-5"/>
              <path d="M2 12l10 5 10-5"/>
            </svg>
          </div>
          <div className="provider-info">
            <div className="provider-name">Cloud (OpenAI)</div>
            <div className="provider-desc">
              Uses OpenAI's Whisper API. Requires internet and an API key. Best accuracy for complex audio.
            </div>
          </div>
        </div>
      </div>
      
      <div className="step-actions">
        <button className="secondary-button" onClick={() => setStep('welcome')}>
          Back
        </button>
        <button className="primary-button" onClick={handleProviderSelect}>
          Continue
        </button>
      </div>
    </div>
  );

  const renderApiKeyInput = () => (
    <div className="onboarding-step apikey-step">
      <div className="step-header">
        <h2>OpenAI API Key</h2>
        <p>Enter your OpenAI API key to use cloud transcription.</p>
      </div>
      
      <div className="apikey-form">
        <input
          type="password"
          className="apikey-input"
          placeholder="sk-..."
          value={apiKey}
          onChange={(e) => {
            setApiKey(e.target.value);
            setApiKeyError(null);
          }}
        />
        {apiKeyError && <div className="apikey-error">{apiKeyError}</div>}
        <p className="apikey-hint">
          Get your API key from{' '}
          <a href="https://platform.openai.com/api-keys" target="_blank" rel="noopener noreferrer">
            platform.openai.com
          </a>
        </p>
      </div>
      
      <div className="step-actions">
        <button className="secondary-button" onClick={() => setStep('provider')}>
          Back
        </button>
        <button className="primary-button" onClick={handleApiKeySave}>
          Continue
        </button>
      </div>
    </div>
  );

  const renderModelSelection = () => {
    const recommendedModels = models.filter(m => ['base.en', 'small.en', 'tiny.en'].includes(m.name));
    
    return (
      <div className="onboarding-step model-step">
        <div className="step-header">
          <h2>Choose a Model</h2>
          <p>Select a transcription model. You can always change this later.</p>
        </div>
        
        <div className="model-list">
          {recommendedModels.map(model => (
            <div 
              key={model.name}
              className={`model-card ${selectedModel === model.name ? 'selected' : ''}`}
              onClick={() => setSelectedModel(model.name)}
            >
              <div className="model-radio">
                <div className={`radio-dot ${selectedModel === model.name ? 'active' : ''}`} />
              </div>
              <div className="model-info">
                <div className="model-header">
                  <div className="model-name">
                    {model.displayName}
                    {model.name === 'base.en' && <span className="recommended-badge">Recommended</span>}
                  </div>
                  <span className="model-size">{model.size}</span>
                </div>
                <div className="model-desc">
                  {model.name === 'tiny.en' && 'Fastest option, great for quick notes and simple dictation'}
                  {model.name === 'base.en' && 'Best balance of speed and accuracy for everyday use'}
                  {model.name === 'small.en' && 'Higher accuracy for detailed transcription, but much slower on CPU'}
                </div>
              </div>
              {model.downloaded && (
                <div className="model-downloaded">
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <polyline points="20 6 9 17 4 12"/>
                  </svg>
                </div>
              )}
            </div>
          ))}
        </div>
        
        <div className="step-actions">
          <button className="secondary-button" onClick={() => setStep('provider')}>
            Back
          </button>
          <button className="primary-button" onClick={handleModelSelect}>
            {models.find(m => m.name === selectedModel)?.downloaded ? 'Continue' : 'Download & Continue'}
          </button>
        </div>
      </div>
    );
  };

  const renderDownload = () => {
    const model = models.find(m => m.name === selectedModel);
    const progress = downloadProgress?.progress || 0;
    
    return (
      <div className="onboarding-step download-step">
        <div className="download-visual">
          <div className="download-icon-container">
            <div className="download-ring" style={{ '--progress': `${progress}%` } as React.CSSProperties}>
              <svg className="download-ring-svg" viewBox="0 0 100 100">
                <circle className="ring-bg" cx="50" cy="50" r="45" />
                <circle 
                  className="ring-progress" 
                  cx="50" 
                  cy="50" 
                  r="45"
                  strokeDasharray={`${progress * 2.83} 283`}
                />
              </svg>
            </div>
            <div className="download-icon">
              {downloadError ? (
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--danger)" strokeWidth="2">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="15" y1="9" x2="9" y2="15"/>
                  <line x1="9" y1="9" x2="15" y2="15"/>
                </svg>
              ) : downloadProgress ? (
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                  <polyline points="7 10 12 15 17 10"/>
                  <line x1="12" y1="15" x2="12" y2="3"/>
                </svg>
              ) : (
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <polyline points="20 6 9 17 4 12"/>
                </svg>
              )}
            </div>
          </div>
        </div>
        
        <div className="download-info">
          {downloadError ? (
            <>
              <h2 className="download-title error">Download Failed</h2>
              <p className="download-error">{downloadError}</p>
            </>
          ) : downloadProgress ? (
            <>
              <h2 className="download-title">Downloading {model?.displayName}</h2>
              <p className="download-subtitle">A local model is required before Yap can transcribe offline. {model?.size} - {progress.toFixed(0)}%</p>
            </>
          ) : (
            <>
              <h2 className="download-title">Preparing Download</h2>
              <p className="download-subtitle">A local model is required before Yap can transcribe offline.</p>
            </>
          )}
        </div>
        
        <div className="step-actions">
          {downloadError ? (
            <>
              <button className="secondary-button" onClick={() => setStep('model')}>
                Back
              </button>
              <button className="primary-button" onClick={handleRetryDownload}>
                Retry Download
              </button>
            </>
          ) : null}
        </div>
      </div>
    );
  };

  // Handle mic permission request and auto-advance on grant
  const handleMicPermissionRequest = useCallback(async () => {
    const status = await RequestMicrophonePermission();
    setMicPermissionStatus(status);
    if (status === 'granted') {
      setStep('micSuccess');
    }
  }, []);

  const renderMicRequest = () => {
    const micDenied = micPermissionStatus === 'denied';

    return (
      <div className="onboarding-step mic-request-step">
        {/* Pixel-art microphone illustration */}
        <div className="permission-illustration">
          <svg width="120" height="120" viewBox="0 0 24 24" fill="none" className="pixel-mic-icon">
            {/* Microphone body - pixel art style */}
            <rect x="10" y="2" width="4" height="10" rx="2" fill="var(--accent)" />
            {/* Microphone stand */}
            <path d="M7 10v2a5 5 0 0 0 10 0v-2" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" fill="none" />
            <line x1="12" y1="17" x2="12" y2="21" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" />
            <line x1="9" y1="21" x2="15" y2="21" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" />
            {/* Sound waves */}
            <path d="M19 8c1 1 1.5 2 1.5 4s-.5 3-1.5 4" stroke="var(--accent)" strokeWidth="1.5" strokeLinecap="round" fill="none" opacity="0.6" />
            <path d="M5 8c-1 1-1.5 2-1.5 4s.5 3 1.5 4" stroke="var(--accent)" strokeWidth="1.5" strokeLinecap="round" fill="none" opacity="0.6" />
          </svg>
        </div>
        
        <div className="step-header">
          <h2>{micDenied ? 'Microphone access is blocked' : 'Yap needs your microphone'}</h2>
          <p>
            {micDenied
              ? 'macOS will not show the permission dialog again. Open System Settings and enable Microphone access for Yap, then come back and re-check.'
              : 'To transcribe your voice, Yap needs access to your microphone. Click below and select "Allow" in the system dialog.'}
          </p>
        </div>

        {micDenied && (
          <div className="permission-warning">
            System Settings → Privacy & Security → Microphone → Yap
          </div>
        )}
        
        <div className="step-actions">
          <button className="secondary-button" onClick={() => setStep(selectedProvider === 'openai' ? 'apikey' : 'model')}>
            Back
          </button>
          {micDenied ? (
            <button className="primary-button" onClick={handleCheckMicPermission}>
              Re-check Permission
            </button>
          ) : (
            <button className="primary-button" onClick={handleMicPermissionRequest}>
              Allow Microphone
            </button>
          )}
        </div>
      </div>
    );
  };

  const renderMicSuccess = () => {
    const needsAccessibility = platform === 'darwin';
    
    return (
      <div className="onboarding-step success-celebration-step">
        {/* Pixel-art celebration */}
        <div className="celebration-illustration">
          <div className="celebration-icon">
            <svg width="80" height="80" viewBox="0 0 24 24" fill="none">
              <circle cx="12" cy="12" r="10" fill="rgba(48, 209, 88, 0.15)" />
              <path d="M8 12l3 3 5-6" stroke="var(--success)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
          {/* Confetti particles */}
          <div className="confetti-container">
            <div className="confetti c1"></div>
            <div className="confetti c2"></div>
            <div className="confetti c3"></div>
            <div className="confetti c4"></div>
            <div className="confetti c5"></div>
            <div className="confetti c6"></div>
          </div>
        </div>
        
        <h2 className="celebration-title">Microphone access granted!</h2>
        <p className="celebration-subtitle">
          {needsAccessibility ? "One more permission to go..." : "You're all set!"}
        </p>
        
        <div className="step-actions">
          <button 
            className="primary-button" 
            onClick={() => setStep(needsAccessibility ? 'accessRequest' : 'hotkeyTest')}
          >
            Continue
          </button>
        </div>
      </div>
    );
  };

  const renderAccessRequest = () => {
    return (
      <div className="onboarding-step access-request-step">
        {/* macOS System Preferences mockup */}
        <div className="system-prefs-mockup">
          <div className="mockup-window">
            {/* Window chrome */}
            <div className="mockup-titlebar">
              <div className="traffic-lights">
                <span className="light red"></span>
                <span className="light yellow"></span>
                <span className="light green"></span>
              </div>
              <span className="mockup-title">Privacy & Security</span>
            </div>
            
            {/* Content area */}
            <div className="mockup-content">
              {/* Sidebar */}
              <div className="mockup-sidebar">
                <div className="sidebar-item">
                  <div className="sidebar-icon"></div>
                </div>
                <div className="sidebar-item active">
                  <div className="sidebar-icon accessibility"></div>
                </div>
                <div className="sidebar-item">
                  <div className="sidebar-icon"></div>
                </div>
              </div>
              
              {/* Main content */}
              <div className="mockup-main">
                <div className="mockup-section-title">Accessibility</div>
                <div className="mockup-app-list">
                  <div className="mockup-app-row highlighted">
                    <img src={appIcon} alt="Yap" className="mockup-app-icon" />
                    <span className="mockup-app-name">Yap</span>
                    <div className="mockup-toggle on"></div>
                  </div>
                  <div className="mockup-app-row">
                    <div className="mockup-app-icon placeholder"></div>
                    <span className="mockup-app-name muted">Other App</span>
                    <div className="mockup-toggle"></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <div className="step-header">
          <h2>Grant Accessibility Access</h2>
          <p>Yap needs Accessibility access to detect your hotkey even when other apps are focused.</p>
        </div>
        
        <p className="access-instructions">
          Click the button below to open System Settings, then toggle <strong>Yap</strong> on in the Accessibility list. If Yap already appears enabled but this step does not advance, toggle it off and back on.
        </p>

        {accessibilityStatus === 'denied' && (
          <div className="permission-warning">
            System Settings → Privacy & Security → Accessibility → Yap
          </div>
        )}
        
        <div className="step-actions">
          <button className="secondary-button" onClick={() => setStep('micSuccess')}>
            Back
          </button>
          <button className="primary-button" onClick={handleRequestAccessibilityPermission}>
            Open Accessibility Settings
          </button>
          {accessibilityStatus === 'denied' && (
            <button className="secondary-button" onClick={handleCheckAccessibilityPermission}>
              Re-check Permission
            </button>
          )}
        </div>
        
        {accessibilityStatus === 'granted' && (
          <p className="permission-detected">Permission detected! Advancing...</p>
        )}
      </div>
    );
  };

  const renderAccessSuccess = () => {
    return (
      <div className="onboarding-step success-celebration-step">
        {/* Same checkmark + confetti as mic success */}
        <div className="celebration-illustration">
          <div className="celebration-icon">
            <svg width="80" height="80" viewBox="0 0 24 24" fill="none">
              <circle cx="12" cy="12" r="10" fill="rgba(48, 209, 88, 0.15)" />
              <path d="M8 12l3 3 5-6" stroke="var(--success)" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
          {/* Confetti particles */}
          <div className="confetti-container">
            <div className="confetti c1"></div>
            <div className="confetti c2"></div>
            <div className="confetti c3"></div>
            <div className="confetti c4"></div>
            <div className="confetti c5"></div>
            <div className="confetti c6"></div>
          </div>
        </div>
        
        <h2 className="celebration-title">All permissions granted!</h2>
        <p className="celebration-subtitle">You're ready to start using Yap</p>
        
        <div className="step-actions">
          <button className="primary-button" onClick={() => setStep('hotkeyTest')}>
            Continue
          </button>
        </div>
      </div>
    );
  };

  const renderHotkeyTest = () => (
    <div className="onboarding-step hotkey-test-step">
      <div className="step-header">
        <h2>Test Your Hotkey</h2>
        <p>Let's make sure the hotkey is working correctly.</p>
      </div>
      
      <div className="hotkey-test-area">
        <div className={`hotkey-test-visual ${hotkeyTestState}`}>
          {hotkeyTestState === 'waiting' && (
            <>
              <div className="hotkey-test-icon">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <rect x="4" y="4" width="16" height="16" rx="2"/>
                  <path d="M9 9h6v6H9z"/>
                </svg>
              </div>
              <div className="hotkey-test-label">Press <kbd>{hotkeyName}</kbd> to start recording</div>
            </>
          )}
          {hotkeyTestState === 'recording' && (
            <>
              <div className="hotkey-test-icon recording">
                <div className="recording-pulse"></div>
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/>
                  <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                </svg>
              </div>
              <div className="hotkey-test-label">Recording... Press <kbd>{hotkeyName}</kbd> again to stop</div>
            </>
          )}
          {hotkeyTestState === 'success' && (
            <>
              <div className="hotkey-test-icon success">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                  <polyline points="22 4 12 14.01 9 11.01"/>
                </svg>
              </div>
              <div className="hotkey-test-label">Hotkey is working!</div>
              {testTranscript && (
                <div className="hotkey-test-transcript">
                  <span className="transcript-label">You said:</span>
                  <p className="transcript-text">"{testTranscript}"</p>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {hotkeyTestState === 'waiting' && (
        <div className="hotkey-test-hint">
          <p>{hotkeyTestError || 'Having trouble? Make sure you granted Accessibility permission.'}</p>
        </div>
      )}
      
      <div className="step-actions">
        <button className="secondary-button" onClick={() => setStep(platform === 'darwin' ? 'accessSuccess' : 'micSuccess')}>
          Back
        </button>
        <button 
          className="primary-button" 
          onClick={() => setStep('ready')}
          disabled={hotkeyTestState !== 'success'}
        >
          Continue
        </button>
      </div>
    </div>
  );

  const renderReady = () => (
    <div className="onboarding-step ready-step">
      <div className="ready-icon">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" strokeWidth="1.5">
          <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
          <polyline points="22 4 12 14.01 9 11.01"/>
        </svg>
      </div>
      <h2 className="ready-title">You're All Set!</h2>
      <p className="ready-subtitle">Here's a quick tour of Yap</p>
      
      <div className="app-tour">
        <div className="tour-item">
          <div className="tour-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
              <polyline points="9 22 9 12 15 12 15 22"/>
            </svg>
          </div>
          <div className="tour-text">
            <strong>Home</strong>
            <span>Your dashboard with recording stats</span>
          </div>
        </div>
        <div className="tour-item">
          <div className="tour-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="12" cy="12" r="10"/>
              <polyline points="12 6 12 12 16 14"/>
            </svg>
          </div>
          <div className="tour-text">
            <strong>History</strong>
            <span>View and manage past transcriptions</span>
          </div>
        </div>
        <div className="tour-item">
          <div className="tour-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
          </div>
          <div className="tour-text">
            <strong>Settings</strong>
            <span>Customize hotkey, model, and more</span>
          </div>
        </div>
      </div>
      
      <button className="primary-button large" onClick={handleComplete}>
        Start Using Yap
      </button>
    </div>
  );

  return (
    <div className="onboarding">
      <div className="onboarding-container">
        {/* Progress indicator */}
        {step !== 'welcome' && (
          <div className="progress-dots">
            {/* Provider selection */}
            <div className={`dot ${step === 'provider' ? 'active' : 'completed'}`} />
            {/* Model/API key (depends on provider) */}
            <div className={`dot ${['model', 'download', 'apikey'].includes(step) ? 'active' : ['micRequest', 'micSuccess', 'accessRequest', 'accessSuccess', 'hotkeyTest', 'ready'].includes(step) ? 'completed' : ''}`} />
            {/* Permissions (mic + accessibility) */}
            <div className={`dot ${['micRequest', 'micSuccess', 'accessRequest', 'accessSuccess'].includes(step) ? 'active' : ['hotkeyTest', 'ready'].includes(step) ? 'completed' : ''}`} />
            {/* Hotkey Test */}
            <div className={`dot ${step === 'hotkeyTest' ? 'active' : step === 'ready' ? 'completed' : ''}`} />
            {/* Ready */}
            <div className={`dot ${step === 'ready' ? 'active' : ''}`} />
          </div>
        )}
        
        {/* Step content */}
        {step === 'welcome' && renderWelcome()}
        {step === 'provider' && renderProviderSelection()}
        {step === 'model' && renderModelSelection()}
        {step === 'download' && renderDownload()}
        {step === 'apikey' && renderApiKeyInput()}
        {step === 'micRequest' && renderMicRequest()}
        {step === 'micSuccess' && renderMicSuccess()}
        {step === 'accessRequest' && renderAccessRequest()}
        {step === 'accessSuccess' && renderAccessSuccess()}
        {step === 'hotkeyTest' && renderHotkeyTest()}
        {step === 'ready' && renderReady()}
      </div>
    </div>
  );
}
