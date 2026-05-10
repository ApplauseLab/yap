import { useState, useEffect, useCallback } from 'react';
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
  SetOnboardingCompleted,
  GetRecordingHotkeyDisplayName,
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

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

interface OnboardingProps {
  onComplete: () => void;
}

type Step = 'welcome' | 'provider' | 'model' | 'download' | 'apikey' | 'permissions' | 'ready';
type Provider = 'local' | 'openai';

export function Onboarding({ onComplete }: OnboardingProps) {
  const [step, setStep] = useState<Step>('welcome');
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [selectedModel, setSelectedModel] = useState<string>('base.en');
  const [selectedProvider, setSelectedProvider] = useState<Provider>('local');
  const [apiKey, setApiKey] = useState<string>('');
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<DownloadProgress | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [permissionStatus, setPermissionStatus] = useState<string>('undetermined');
  const [hotkeyName, setHotkeyName] = useState<string>('Right Option');

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

    // Set up event listeners for download progress
    const progressHandler = (progress: DownloadProgress) => {
      setDownloadProgress(progress);
      setDownloadError(null);
    };

    const completeHandler = () => {
      setDownloadProgress(null);
      // Refresh models list
      GetModels().then((modelList: ModelInfo[]) => {
        setModels(modelList);
      });
      // Move to permissions step
      setStep('permissions');
    };

    const errorHandler = (data: { model: string; error: string }) => {
      setDownloadProgress(null);
      setDownloadError(data.error);
    };

    EventsOn('downloadProgress', progressHandler);
    EventsOn('downloadComplete', completeHandler);
    EventsOn('downloadError', errorHandler);

    return () => {
      EventsOff('downloadProgress');
      EventsOff('downloadComplete');
      EventsOff('downloadError');
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
    setStep('permissions');
  }, [apiKey]);

  const handleModelSelect = useCallback(async () => {
    await SetModel(selectedModel);
    const model = models.find(m => m.name === selectedModel);
    if (model?.downloaded) {
      // Model already downloaded, skip to permissions
      setStep('permissions');
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

  const handleSkipDownload = useCallback(() => {
    setStep('permissions');
  }, []);

  const handleCheckPermission = useCallback(async () => {
    const status = await CheckMicrophonePermission();
    setPermissionStatus(status);
  }, []);

  const handleRequestPermission = useCallback(async () => {
    const status = await RequestMicrophonePermission();
    setPermissionStatus(status);
  }, []);

  const handleComplete = useCallback(async () => {
    await SetOnboardingCompleted(true);
    onComplete();
  }, [onComplete]);

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
                  {model.name === 'small.en' && 'Higher accuracy for detailed transcription, slightly slower'}
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
              <p className="download-subtitle">{model?.size} — {progress.toFixed(0)}%</p>
            </>
          ) : (
            <>
              <h2 className="download-title">Preparing Download</h2>
              <p className="download-subtitle">Setting up {model?.displayName}...</p>
            </>
          )}
        </div>
        
        <div className="step-actions">
          {downloadError ? (
            <>
              <button className="secondary-button" onClick={handleSkipDownload}>
                Skip for Now
              </button>
              <button className="primary-button" onClick={handleRetryDownload}>
                Retry Download
              </button>
            </>
          ) : (
            <button className="secondary-button" onClick={handleSkipDownload}>
              Skip for Now
            </button>
          )}
        </div>
      </div>
    );
  };

  const renderPermissions = () => (
    <div className="onboarding-step permissions-step">
      <div className="step-header">
        <div className="permissions-icon">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
            <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/>
            <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
            <line x1="12" y1="19" x2="12" y2="22"/>
          </svg>
        </div>
        <h2>Microphone Access</h2>
        <p>Yap needs microphone access to transcribe your voice.</p>
      </div>
      
      <div className="permission-status">
        {permissionStatus === 'granted' ? (
          <div className="status-card granted">
            <div className="status-icon">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                <polyline points="22 4 12 14.01 9 11.01"/>
              </svg>
            </div>
            <div className="status-text">
              <strong>Access Granted</strong>
              <span>Yap can now listen to your voice</span>
            </div>
          </div>
        ) : permissionStatus === 'denied' ? (
          <div className="status-card denied">
            <div className="status-icon">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
              </svg>
            </div>
            <div className="status-text">
              <strong>Access Denied</strong>
              <span>Please enable microphone access in System Preferences &gt; Privacy &gt; Microphone</span>
            </div>
          </div>
        ) : (
          <div className="status-card pending">
            <div className="status-icon">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="12" y1="8" x2="12" y2="12"/>
                <line x1="12" y1="16" x2="12.01" y2="16"/>
              </svg>
            </div>
            <div className="status-text">
              <strong>Permission Required</strong>
              <span>Click the button below to grant access</span>
            </div>
          </div>
        )}
      </div>
      
      <div className="step-actions">
        <button className="secondary-button" onClick={() => setStep('model')}>
          Back
        </button>
        {permissionStatus === 'granted' ? (
          <button className="primary-button" onClick={() => setStep('ready')}>
            Continue
          </button>
        ) : (
          <>
            <button className="secondary-button" onClick={() => setStep('ready')}>
              Skip
            </button>
            <button className="primary-button" onClick={handleRequestPermission}>
              {permissionStatus === 'denied' ? 'Check Again' : 'Grant Access'}
            </button>
          </>
        )}
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
      <p className="ready-subtitle">Here's how to use Yap</p>
      
      <div className="hotkey-demo">
        <div className="hotkey-visual">
          <kbd className="hotkey-key">{hotkeyName}</kbd>
        </div>
        <div className="hotkey-instructions">
          <div className="instruction">
            <span className="instruction-num">1</span>
            <span>Press <strong>{hotkeyName}</strong> to start recording</span>
          </div>
          <div className="instruction">
            <span className="instruction-num">2</span>
            <span>Speak clearly into your microphone</span>
          </div>
          <div className="instruction">
            <span className="instruction-num">3</span>
            <span>Press <strong>{hotkeyName}</strong> again to transcribe and paste</span>
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
            <div className={`dot ${['model', 'download', 'apikey'].includes(step) ? 'active' : ['permissions', 'ready'].includes(step) ? 'completed' : ''}`} />
            {/* Permissions */}
            <div className={`dot ${step === 'permissions' ? 'active' : step === 'ready' ? 'completed' : ''}`} />
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
        {step === 'permissions' && renderPermissions()}
        {step === 'ready' && renderReady()}
      </div>
    </div>
  );
}
