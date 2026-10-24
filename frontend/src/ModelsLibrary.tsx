import { useEffect, useMemo, useRef, useState } from 'react';
import './ModelsLibrary.css';
import {
  ModelDownloadState,
  ModelInfo,
  accuracyLabel,
  errorMessage,
  formatBytes,
  isDownloadActive,
  languageLabel,
  speedLabel,
} from './modelTypes';

export type ModelFilter = 'all' | 'english' | 'multilingual' | 'downloaded';

const FILTERS: { id: ModelFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'english', label: 'English only' },
  { id: 'multilingual', label: 'Multilingual' },
  { id: 'downloaded', label: 'Downloaded' },
];

export interface ModelsLibraryProps {
  models: ModelInfo[];
  activeModel: string;
  provider: string;
  downloadState: ModelDownloadState;
  focusModel?: string | null;
  onDownload: (model: string) => Promise<void>;
  onCancel: (model: string) => Promise<void>;
  onUse: (model: string) => Promise<void>;
  onDelete: (model: string) => Promise<void>;
}

export function filterModels(models: ModelInfo[], query: string, filter: ModelFilter): ModelInfo[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  return models.filter((model) => {
    if (filter === 'english' && !model.englishOnly) return false;
    if (filter === 'multilingual' && model.englishOnly) return false;
    if (filter === 'downloaded' && !model.downloaded) return false;
    if (terms.length === 0) return true;
    const haystack = [
      model.displayName,
      model.name,
      model.useCase,
      model.recommendation,
      model.size,
      languageLabel(model),
      `${speedLabel(model.speed)} speed`,
      `${accuracyLabel(model.accuracy)} accuracy`,
    ].join(' ').toLowerCase();
    return terms.every((term) => haystack.includes(term));
  });
}

function Meter({ label, value, text }: { label: string; value: number; text: string }) {
  return (
    <div className="ml-meter" aria-label={`${label}: ${text} (${value} of 5)`}>
      <span className="ml-meter-label">{label}</span>
      <span className="ml-meter-dots" aria-hidden="true">
        {[1, 2, 3, 4, 5].map((i) => (
          <span key={i} className={`ml-meter-dot ${i <= value ? 'on' : ''}`} />
        ))}
      </span>
      <span className="ml-meter-text">{text}</span>
    </div>
  );
}

interface CardProps {
  model: ModelInfo;
  active: boolean;
  downloadState: ModelDownloadState;
  focused: boolean;
  onDownload: (model: string) => Promise<void>;
  onCancel: (model: string) => Promise<void>;
  onUse: (model: string) => Promise<void>;
  onDelete: (model: string) => Promise<void>;
}

function ModelCard({ model, active, downloadState, focused, onDownload, onCancel, onUse, onDelete }: CardProps) {
  const [pending, setPending] = useState(false);
  const [actionError, setActionError] = useState('');
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const cardRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (focused && cardRef.current && typeof cardRef.current.scrollIntoView === 'function') {
      cardRef.current.scrollIntoView({ block: 'center', behavior: 'smooth' });
    }
  }, [focused]);

  const isThisModel = downloadState.model === model.name;
  const downloading = isThisModel && isDownloadActive(downloadState);
  const otherDownloading = !isThisModel && isDownloadActive(downloadState);
  const failed = isThisModel && downloadState.status === 'failed' && !model.downloaded;
  const cancelled = isThisModel && downloadState.status === 'cancelled' && !model.downloaded;

  const run = async (action: (name: string) => Promise<void>) => {
    setPending(true);
    setActionError('');
    try {
      await action(model.name);
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  const total = downloadState.total || model.sizeBytes;

  let status: string;
  if (active && model.downloaded) status = 'Active';
  else if (active) status = 'Selected, not downloaded';
  else if (downloading) status = downloadState.status === 'cancelling' ? 'Cancelling' : 'Downloading';
  else if (model.downloaded) status = 'Downloaded';
  else status = 'Not downloaded';

  return (
    <article
      ref={cardRef}
      className={`ml-card ${active ? 'active' : ''} ${focused ? 'focused' : ''}`}
      data-testid={`model-card-${model.name}`}
      aria-label={model.displayName}
    >
      <header className="ml-card-header">
        <div className="ml-card-title">
          <h3>{model.displayName}</h3>
          <div className="ml-badges">
            {model.recommendation && <span className="ml-badge recommend">{model.recommendation}</span>}
            <span className={`ml-badge ${model.englishOnly ? 'english' : 'multilingual'}`}>{languageLabel(model)}</span>
          </div>
        </div>
        <span className={`ml-status ${active && model.downloaded ? 'active' : model.downloaded ? 'downloaded' : ''}`}>
          {status}
        </span>
      </header>

      <p className="ml-usecase">{model.useCase}</p>

      <div className="ml-meta">
        <span className="ml-size">{model.size}</span>
        <Meter label="Speed" value={model.speed} text={speedLabel(model.speed)} />
        <Meter label="Accuracy" value={model.accuracy} text={accuracyLabel(model.accuracy)} />
      </div>

      {downloading && (
        <div className="ml-progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(downloadState.progress)}>
          <div className="ml-progress-track">
            <div className="ml-progress-fill" style={{ width: `${Math.min(100, downloadState.progress)}%` }} />
          </div>
          <span className="ml-progress-text">
            {formatBytes(downloadState.downloaded)} of {formatBytes(total)} · {downloadState.progress.toFixed(0)}%
          </span>
        </div>
      )}

      {failed && <p className="ml-error" role="alert">Download failed: {downloadState.error}</p>}
      {cancelled && <p className="ml-note">Download cancelled.</p>}
      {actionError && <p className="ml-error" role="alert">{actionError}</p>}

      <div className="ml-actions">
        {downloading && (
          <button
            className="ml-btn secondary"
            disabled={pending || downloadState.status === 'cancelling'}
            onClick={() => run(onCancel)}
          >
            {downloadState.status === 'cancelling' ? 'Cancelling…' : 'Cancel'}
          </button>
        )}

        {!downloading && !model.downloaded && (
          <button
            className="ml-btn primary"
            disabled={pending || otherDownloading}
            title={otherDownloading ? 'Another model is downloading' : undefined}
            onClick={() => run(onDownload)}
          >
            {failed || cancelled ? 'Retry download' : `Download (${model.size})`}
          </button>
        )}

        {model.downloaded && !active && !confirmingDelete && (
          <>
            <button className="ml-btn primary" disabled={pending} onClick={() => run(onUse)}>
              Use this model
            </button>
            <button className="ml-btn danger-ghost" disabled={pending} onClick={() => setConfirmingDelete(true)}>
              Delete
            </button>
          </>
        )}

        {model.downloaded && !active && confirmingDelete && (
          <>
            <span className="ml-confirm-text">Delete {model.size} from disk?</span>
            <button
              className="ml-btn danger"
              disabled={pending}
              onClick={async () => {
                await run(onDelete);
                setConfirmingDelete(false);
              }}
            >
              Confirm delete
            </button>
            <button className="ml-btn secondary" disabled={pending} onClick={() => setConfirmingDelete(false)}>
              Keep
            </button>
          </>
        )}

        {model.downloaded && active && (
          <span className="ml-active-note">In use for transcription · cannot be deleted</span>
        )}
      </div>
    </article>
  );
}

export function ModelsLibrary({
  models,
  activeModel,
  provider,
  downloadState,
  focusModel,
  onDownload,
  onCancel,
  onUse,
  onDelete,
}: ModelsLibraryProps) {
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<ModelFilter>('all');

  const visible = useMemo(() => filterModels(models, query, filter), [models, query, filter]);
  const downloadedCount = models.filter((m) => m.downloaded).length;

  return (
    <div className="models-page">
      <div className="ml-intro">
        <p>
          Models run privately on this computer. English-only models are faster and more accurate for English;
          multilingual models understand other languages. Larger models are more accurate but slower.
        </p>
        {provider === 'openai' && (
          <p className="ml-provider-note">
            OpenAI is your current provider, so these local models are not used until you switch back to Local in Settings.
          </p>
        )}
      </div>

      <div className="ml-toolbar">
        <input
          type="search"
          className="ml-search"
          placeholder="Search models"
          aria-label="Search models"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div className="ml-filters" role="group" aria-label="Filter models">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              className={filter === f.id ? 'active' : ''}
              aria-pressed={filter === f.id}
              onClick={() => setFilter(f.id)}
            >
              {f.label}
              {f.id === 'downloaded' ? ` (${downloadedCount})` : ''}
            </button>
          ))}
        </div>
      </div>

      {visible.length === 0 ? (
        <p className="ml-empty">No models match your search.</p>
      ) : (
        <div className="ml-grid">
          {visible.map((model) => (
            <ModelCard
              key={model.name}
              model={model}
              active={model.name === activeModel}
              downloadState={downloadState}
              focused={model.name === focusModel}
              onDownload={onDownload}
              onCancel={onCancel}
              onUse={onUse}
              onDelete={onDelete}
            />
          ))}
        </div>
      )}
    </div>
  );
}
