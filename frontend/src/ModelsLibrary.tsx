import { KeyboardEvent as ReactKeyboardEvent, ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import './ModelsLibrary.css';
import {
  ModelDownloadState,
  ModelInfo,
  accuracyLabel,
  errorMessage,
  formatBytes,
  fullModelName,
  isDownloadActive,
  languageLabel,
  speedLabel,
} from './modelTypes';
import { ProviderLogo, VENDOR_NAMES, modelVendor } from './ProviderLogo';

export type ModelFilter = 'english' | 'multilingual' | 'downloaded' | 'favorites';
export type SortKey = 'name' | 'speed' | 'accuracy' | 'size';
export interface SortState {
  key: SortKey;
  dir: 1 | -1;
}

const FILTER_LABELS: Record<ModelFilter, string> = {
  english: 'English only',
  multilingual: 'Multilingual',
  downloaded: 'Downloaded',
  favorites: 'Favorites',
};
const FILTER_ORDER: ModelFilter[] = ['english', 'multilingual', 'downloaded', 'favorites'];

export interface ModelsLibraryProps {
  models: ModelInfo[];
  activeModel: string;
  provider: string;
  downloadState: ModelDownloadState;
  focusModel?: string | null;
  // Where to render search/filter controls (the page header). Inline when absent.
  toolbarTarget?: HTMLElement | null;
  onDownload: (model: string) => Promise<void>;
  onCancel: (model: string) => Promise<void>;
  onUse: (model: string) => Promise<void>;
  onDelete: (model: string) => Promise<void>;
  onToggleFavorite: (model: string, favorite: boolean) => Promise<void>;
}

function searchText(model: ModelInfo): string {
  return [
    model.displayName,
    model.name,
    model.useCase,
    model.recommendation,
    model.size,
    languageLabel(model),
    model.englishOnly ? 'en english' : 'multilingual',
    VENDOR_NAMES[modelVendor(model)],
    'whisper',
    `${speedLabel(model.speed)} speed`,
    `${accuracyLabel(model.accuracy)} accuracy`,
  ].join(' ').toLowerCase();
}

export function filterModels(models: ModelInfo[], query: string, filters: ReadonlySet<ModelFilter>): ModelInfo[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const english = filters.has('english');
  const multilingual = filters.has('multilingual');
  return models.filter((model) => {
    if (english !== multilingual) {
      if (english && !model.englishOnly) return false;
      if (multilingual && model.englishOnly) return false;
    }
    if (filters.has('downloaded') && !model.downloaded) return false;
    if (filters.has('favorites') && !model.favorite) return false;
    if (terms.length === 0) return true;
    const haystack = searchText(model);
    return terms.every((term) => haystack.includes(term));
  });
}

export function sortModels(models: ModelInfo[], sort: SortState): ModelInfo[] {
  const byName = (a: ModelInfo, b: ModelInfo) =>
    a.displayName.localeCompare(b.displayName) || Number(a.englishOnly) - Number(b.englishOnly);
  const value = (m: ModelInfo) => (sort.key === 'speed' ? m.speed : sort.key === 'accuracy' ? m.accuracy : m.sizeBytes);
  return [...models].sort((a, b) => {
    if (sort.key === 'name') return byName(a, b) * sort.dir;
    const diff = value(a) - value(b);
    return diff ? diff * sort.dir : byName(a, b);
  });
}

function Segments({ label, value, text }: { label: string; value: number; text: string }) {
  return (
    <span className="ml-segments" role="img" aria-label={`${label}: ${text} (${value} of 5)`} title={`${label}: ${text}`}>
      {[1, 2, 3, 4, 5].map((i) => (
        <span key={i} className={i <= value ? 'on' : ''} />
      ))}
    </span>
  );
}

function Icon({ name }: { name: 'star' | 'star-filled' | 'search' | 'filter' | 'download' | 'check' | 'alert' | 'back' | 'close' | 'chevron' }) {
  const common = { width: 16, height: 16, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true };
  switch (name) {
    case 'star':
    case 'star-filled':
      return (
        <svg {...common} fill={name === 'star-filled' ? 'currentColor' : 'none'}>
          <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
        </svg>
      );
    case 'search':
      return <svg {...common}><circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" /></svg>;
    case 'filter':
      return <svg {...common}><line x1="4" y1="6" x2="20" y2="6" /><line x1="7" y1="12" x2="17" y2="12" /><line x1="10" y1="18" x2="14" y2="18" /></svg>;
    case 'download':
      return <svg {...common}><circle cx="12" cy="12" r="10" /><polyline points="8 12 12 16 16 12" /><line x1="12" y1="7" x2="12" y2="16" /></svg>;
    case 'check':
      return <svg {...common}><polyline points="20 6 9 17 4 12" /></svg>;
    case 'alert':
      return <svg {...common}><circle cx="12" cy="12" r="10" /><line x1="12" y1="7" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>;
    case 'back':
      return <svg {...common}><polyline points="15 18 9 12 15 6" /></svg>;
    case 'close':
      return <svg {...common}><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>;
    case 'chevron':
      return <svg {...common}><polyline points="6 9 12 15 18 9" /></svg>;
  }
}

function ProgressRing({ progress }: { progress: number }) {
  const circumference = 2 * Math.PI * 9;
  return (
    <svg className="ml-ring" width="22" height="22" viewBox="0 0 22 22" aria-hidden="true">
      <circle cx="11" cy="11" r="9" className="track" />
      <circle
        cx="11" cy="11" r="9" className="fill"
        strokeDasharray={`${(Math.min(100, progress) / 100) * circumference} ${circumference}`}
        transform="rotate(-90 11 11)"
      />
    </svg>
  );
}

interface SpecItem {
  id: string;
  label: string;
  tone: 'yellow' | 'blue' | 'purple' | 'green' | 'grey';
  glyph?: ReactNode;
  text?: string;
}

export function modelSpecs(model: ModelInfo): SpecItem[] {
  const specs: SpecItem[] = [];
  if (model.speed >= 4) {
    specs.push({ id: 'fast', label: 'Fast', tone: 'yellow', glyph: <path d="M13 2 4 14h7l-1 8 9-12h-7z" fill="currentColor" stroke="none" /> });
  }
  if (model.accuracy >= 4) {
    specs.push({ id: 'accurate', label: 'High accuracy', tone: 'purple', glyph: <><circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="5" /><circle cx="12" cy="12" r="1" fill="currentColor" /></> });
  }
  specs.push(model.englishOnly
    ? { id: 'english', label: 'English only', tone: 'blue', text: 'EN' }
    : { id: 'multilingual', label: 'Multilingual', tone: 'blue', glyph: <><circle cx="12" cy="12" r="9" /><line x1="3" y1="12" x2="21" y2="12" /><path d="M12 3a14 14 0 0 1 0 18a14 14 0 0 1 0-18z" /></> });
  specs.push({ id: 'offline', label: 'Runs offline on this computer', tone: 'green', glyph: <><rect x="5" y="11" width="14" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /></> });
  return specs;
}

interface ModelStatus {
  downloading: boolean;
  cancelling: boolean;
  failed: boolean;
  cancelled: boolean;
  otherDownloading: boolean;
}

function statusFor(model: ModelInfo, downloadState: ModelDownloadState): ModelStatus {
  const isThis = downloadState.model === model.name;
  const active = isDownloadActive(downloadState);
  return {
    downloading: isThis && active,
    cancelling: isThis && downloadState.status === 'cancelling',
    failed: isThis && downloadState.status === 'failed' && !model.downloaded,
    cancelled: isThis && downloadState.status === 'cancelled' && !model.downloaded,
    otherDownloading: !isThis && active,
  };
}

interface DetailProps {
  model: ModelInfo;
  active: boolean;
  downloadState: ModelDownloadState;
  actionError: string;
  pending: boolean;
  onClose: () => void;
  onRun: (action: (name: string) => Promise<void>) => void;
  onDownload: (model: string) => Promise<void>;
  onCancel: (model: string) => Promise<void>;
  onUse: (model: string) => Promise<void>;
  onDelete: (model: string) => Promise<void>;
  onToggleFavorite: () => void;
  favoriteSaving: boolean;
}

function ModelDetail({ model, active, downloadState, actionError, pending, onClose, onRun, onDownload, onCancel, onUse, onDelete, onToggleFavorite, favoriteSaving }: DetailProps) {
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const closeRef = useRef<HTMLButtonElement>(null);
  const status = statusFor(model, downloadState);
  const total = downloadState.total || model.sizeBytes;
  const name = fullModelName(model);

  useEffect(() => {
    closeRef.current?.focus();
  }, [model.name]);

  useEffect(() => {
    setConfirmingDelete(false);
  }, [model.name, model.downloaded]);

  let primary: ReactNode;
  if (status.downloading) {
    primary = (
      <div className="ml-detail-progress">
        <div className="ml-progress" role="progressbar" aria-label={`Downloading ${name}`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(downloadState.progress)}>
          <div className="ml-progress-track"><div className="ml-progress-fill" style={{ width: `${Math.min(100, downloadState.progress)}%` }} /></div>
          <span className="ml-progress-text">
            {formatBytes(downloadState.downloaded)} of {formatBytes(total)} · {downloadState.progress.toFixed(0)}%
          </span>
        </div>
        <button className="ml-cta secondary" disabled={pending || status.cancelling} onClick={() => onRun(onCancel)}>
          {status.cancelling ? 'Cancelling…' : 'Cancel download'}
        </button>
      </div>
    );
  } else if (!model.downloaded) {
    primary = (
      <button
        className="ml-cta primary"
        disabled={pending || status.otherDownloading}
        title={status.otherDownloading ? 'Another model is downloading' : undefined}
        onClick={() => onRun(onDownload)}
      >
        {status.failed || status.cancelled ? `Retry download · ${model.size}` : `Download · ${model.size}`}
      </button>
    );
  } else if (active) {
    primary = <button className="ml-cta active" disabled>Active model</button>;
  } else {
    primary = <button className="ml-cta primary" disabled={pending} onClick={() => onRun(onUse)}>Use this model</button>;
  }

  return (
    <aside className="ml-detail" role="dialog" aria-label={`${name} details`}>
      <header className="ml-detail-header">
        <button ref={closeRef} className="ml-icon-btn" aria-label="Close details" onClick={onClose}><Icon name="back" /></button>
        <div className="ml-detail-title">
          <ProviderLogo vendor={modelVendor(model)} />
          <h2>{model.displayName}</h2>
          {model.englishOnly && <span className="ml-tag" title="English only">EN</span>}
        </div>
        <button
          className={`ml-icon-btn ml-star ${model.favorite ? 'on' : ''}`}
          aria-label={model.favorite ? `Remove ${name} from favorites` : `Add ${name} to favorites`}
          aria-pressed={model.favorite}
          disabled={favoriteSaving}
          onClick={onToggleFavorite}
        >
          <Icon name={model.favorite ? 'star-filled' : 'star'} />
        </button>
      </header>

      <div className="ml-detail-body">
        {model.recommendation && <span className="ml-recommend">{model.recommendation}</span>}
        <p className="ml-detail-desc">{model.useCase}</p>

        <dl className="ml-detail-card">
          <div><dt>Type</dt><dd>Voice model · {VENDOR_NAMES[modelVendor(model)]} Whisper</dd></div>
          <div><dt>Language</dt><dd>{model.englishOnly ? 'English only' : 'Multilingual (99 languages)'}</dd></div>
          <div>
            <dt>Specs</dt>
            <dd className="ml-specs">
              {modelSpecs(model).map((spec) => (
                <span key={spec.id} className={`ml-spec ${spec.tone}`} role="img" aria-label={spec.label} title={spec.label}>
                  {spec.text ? (
                    <span className="ml-spec-text" aria-hidden="true">{spec.text}</span>
                  ) : (
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">{spec.glyph}</svg>
                  )}
                </span>
              ))}
            </dd>
          </div>
          <div><dt>Speed</dt><dd><Segments label="Speed" value={model.speed} text={speedLabel(model.speed)} /><span className="ml-rating-text">{speedLabel(model.speed)}</span></dd></div>
          <div><dt>Accuracy</dt><dd><Segments label="Accuracy" value={model.accuracy} text={accuracyLabel(model.accuracy)} /><span className="ml-rating-text">{accuracyLabel(model.accuracy)}</span></dd></div>
          <div><dt>Size</dt><dd>{model.size}</dd></div>
          <div><dt>Status</dt><dd>{active && model.downloaded ? 'Active' : status.downloading ? 'Downloading' : model.downloaded ? 'Downloaded' : 'Not downloaded'}</dd></div>
        </dl>

        {status.failed && <p className="ml-error" role="alert">Download failed: {downloadState.error}</p>}
        {status.cancelled && <p className="ml-note">Download cancelled.</p>}
        {actionError && <p className="ml-error" role="alert">{actionError}</p>}
      </div>

      <footer className="ml-detail-footer">
        {primary}
        {model.downloaded && !active && !confirmingDelete && (
          <button className="ml-link danger" disabled={pending} onClick={() => setConfirmingDelete(true)}>Delete from this computer</button>
        )}
        {model.downloaded && !active && confirmingDelete && (
          <div className="ml-confirm">
            <span>Delete {model.size} from disk?</span>
            <button className="ml-cta danger small" disabled={pending} onClick={() => onRun(onDelete)}>Confirm delete</button>
            <button className="ml-cta secondary small" disabled={pending} onClick={() => setConfirmingDelete(false)}>Keep</button>
          </div>
        )}
        {model.downloaded && active && <p className="ml-note center">The active model can’t be deleted.</p>}
      </footer>
    </aside>
  );
}

export function ModelsLibrary({
  models,
  activeModel,
  provider,
  downloadState,
  focusModel,
  toolbarTarget,
  onDownload,
  onCancel,
  onUse,
  onDelete,
  onToggleFavorite,
}: ModelsLibraryProps) {
  const [query, setQuery] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [filters, setFilters] = useState<Set<ModelFilter>>(new Set());
  const [menuOpen, setMenuOpen] = useState(false);
  const [sort, setSort] = useState<SortState>({ key: 'name', dir: 1 });
  const [selected, setSelected] = useState<string | null>(focusModel ?? null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState<string | null>(null);
  // Optimistic star state, kept only while the save is in flight; afterwards
  // the refreshed models prop is the source of truth.
  const [favoriteOverrides, setFavoriteOverrides] = useState<Record<string, boolean>>({});
  const menuRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const withFavorites = useMemo(
    () => models.map((m) => (m.name in favoriteOverrides ? { ...m, favorite: favoriteOverrides[m.name] } : m)),
    [models, favoriteOverrides],
  );
  const visible = useMemo(() => sortModels(filterModels(withFavorites, query, filters), sort), [withFavorites, query, filters, sort]);
  const selectedModel = withFavorites.find((m) => m.name === selected) ?? null;
  const counts = useMemo(() => ({
    english: withFavorites.filter((m) => m.englishOnly).length,
    multilingual: withFavorites.filter((m) => !m.englishOnly).length,
    downloaded: withFavorites.filter((m) => m.downloaded).length,
    favorites: withFavorites.filter((m) => m.favorite).length,
  }), [withFavorites]);

  useEffect(() => {
    if (focusModel) setSelected(focusModel);
  }, [focusModel]);

  useEffect(() => {
    if (searchOpen) searchRef.current?.focus();
  }, [searchOpen]);

  useEffect(() => {
    if (!menuOpen) return;
    const onPointer = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    document.addEventListener('mousedown', onPointer);
    return () => document.removeEventListener('mousedown', onPointer);
  }, [menuOpen]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // App cancels an active recording on Escape and marks the event handled.
      if (e.key !== 'Escape' || e.defaultPrevented) return;
      if (menuOpen) setMenuOpen(false);
      else if (selected) closeDetail();
      else if (searchOpen) { setSearchOpen(false); setQuery(''); }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  });

  const tableRef = useRef<HTMLTableElement>(null);

  // Return focus to the row that opened the panel, so keyboard users keep
  // their place.
  const closeDetail = () => {
    const name = selected;
    setSelected(null);
    if (!name) return;
    const cell = tableRef.current?.querySelector<HTMLElement>(`[data-testid="model-row-${name}"] .ml-name-cell`);
    cell?.focus();
  };

  const run = async (name: string, action: (model: string) => Promise<void>) => {
    setPending(name);
    setErrors((prev) => ({ ...prev, [name]: '' }));
    try {
      await action(name);
    } catch (err) {
      setErrors((prev) => ({ ...prev, [name]: errorMessage(err) }));
      setSelected(name);
    } finally {
      setPending(null);
    }
  };

  const clearFavoriteOverride = (name: string) => {
    setFavoriteOverrides((prev) => {
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const toggleFavorite = async (model: ModelInfo) => {
    if (model.name in favoriteOverrides) return;
    const next = !model.favorite;
    setFavoriteOverrides((prev) => ({ ...prev, [model.name]: next }));
    try {
      await onToggleFavorite(model.name, next);
    } catch (err) {
      setErrors((prev) => ({ ...prev, [model.name]: `Couldn’t update favorites: ${errorMessage(err)}` }));
      setSelected(model.name);
    } finally {
      clearFavoriteOverride(model.name);
    }
  };

  const toggleFilter = (filter: ModelFilter) => {
    setFilters((prev) => {
      const next = new Set(prev);
      if (next.has(filter)) next.delete(filter);
      else next.add(filter);
      return next;
    });
  };

  const toggleSort = (key: SortKey) => {
    setSort((prev) => (prev.key === key ? { key, dir: prev.dir === 1 ? -1 : 1 } : { key, dir: key === 'name' ? 1 : -1 }));
  };

  const sortHeader = (key: SortKey, label: string, className: string) => {
    const activeSort = sort.key === key;
    return (
      <th className={className} aria-sort={activeSort ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none'}>
        <button className={`ml-sort ${activeSort ? 'active' : ''}`} onClick={() => toggleSort(key)}>
          {label}
          <span className={`ml-sort-arrow ${activeSort && sort.dir === -1 ? 'down' : ''}`} aria-hidden="true">{activeSort ? '⌃' : ''}</span>
        </button>
      </th>
    );
  };

  const toolbar = (
    <div className="ml-toolbar">
      {filters.size > 0 && (
        <div className="ml-chips">
          {FILTER_ORDER.filter((f) => filters.has(f)).map((f) => (
            <span key={f} className="ml-chip">
              {FILTER_LABELS[f]}
              <button aria-label={`Remove ${FILTER_LABELS[f]} filter`} onClick={() => toggleFilter(f)}><Icon name="close" /></button>
            </span>
          ))}
        </div>
      )}
      {searchOpen ? (
        <div className="ml-search">
          <Icon name="search" />
          <input
            ref={searchRef}
            type="search"
            placeholder="Search models"
            aria-label="Search models"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button className="ml-icon-btn tiny" aria-label="Close search" onClick={() => { setSearchOpen(false); setQuery(''); }}><Icon name="close" /></button>
        </div>
      ) : (
        <button className="ml-icon-btn round" aria-label="Search models" onClick={() => setSearchOpen(true)}><Icon name="search" /></button>
      )}
      <div className="ml-menu-anchor" ref={menuRef}>
        <button
          className={`ml-icon-btn round ${filters.size > 0 || menuOpen ? 'on' : ''}`}
          aria-label="Filter models"
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((open) => !open)}
        >
          <Icon name="filter" />
        </button>
        {menuOpen && (
          <div className="ml-menu" role="menu" aria-label="Filter models">
            {FILTER_ORDER.map((f) => (
              <button
                key={f}
                role="menuitemcheckbox"
                aria-checked={filters.has(f)}
                className="ml-menu-item"
                onClick={() => toggleFilter(f)}
              >
                <span className={`ml-check ${filters.has(f) ? 'on' : ''}`} aria-hidden="true">{filters.has(f) && <Icon name="check" />}</span>
                <span className="ml-menu-label">{FILTER_LABELS[f]}</span>
                <span className="ml-menu-count">{counts[f]}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );

  const rowStatus = (model: ModelInfo) => {
    const status = statusFor(model, downloadState);
    const name = fullModelName(model);
    if (status.downloading) {
      return (
        <span className="ml-status-cell" title={status.cancelling ? 'Cancelling…' : `Downloading ${downloadState.progress.toFixed(0)}%`}>
          <span className="ml-size">{downloadState.progress.toFixed(0)}%</span>
          <span className={`ml-status-icon ${status.cancelling ? 'dim' : ''}`} aria-label={`Downloading ${name}`}><ProgressRing progress={downloadState.progress} /></span>
        </span>
      );
    }
    if (model.downloaded) {
      const isActive = model.name === activeModel;
      return (
        <span className="ml-status-cell">
          {isActive ? <span className="ml-active-pill">Active</span> : <span className="ml-size">{model.size}</span>}
          <span className={`ml-status-icon done ${isActive ? 'active' : ''}`} role="img" aria-label={isActive ? `${name} is active` : `${name} downloaded`} title={isActive ? 'Active' : 'Downloaded'}>
            <Icon name="check" />
          </span>
        </span>
      );
    }
    return (
      <span className="ml-status-cell">
        <span className="ml-size">{model.size}</span>
        <button
          className={`ml-status-icon btn ${status.failed ? 'failed' : ''}`}
          aria-label={status.failed || status.cancelled ? `Retry downloading ${name}` : `Download ${name}`}
          title={status.otherDownloading ? 'Another model is downloading' : status.failed ? `Download failed: ${downloadState.error}` : `Download · ${model.size}`}
          disabled={status.otherDownloading || pending === model.name}
          onClick={(e) => { e.stopPropagation(); run(model.name, onDownload); }}
        >
          <Icon name={status.failed ? 'alert' : 'download'} />
        </button>
      </span>
    );
  };

  const onRowKey = (e: ReactKeyboardEvent, model: ModelInfo) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setSelected(model.name);
    }
  };

  return (
    <div className={`models-page ${selectedModel ? 'with-detail' : ''}`}>
      {toolbarTarget ? createPortal(toolbar, toolbarTarget) : toolbar}

      {provider === 'openai' && (
        <p className="ml-provider-note">
          OpenAI is your current provider, so local models aren’t used until you switch back to Local in Settings.
        </p>
      )}

      <div className="ml-table-wrap">
        <table className="ml-table" ref={tableRef}>
          <thead>
            <tr>
              <th className="col-star"><span className="sr-only">Favorite</span></th>
              {sortHeader('name', 'Model name', 'col-name')}
              {sortHeader('speed', 'Speed', 'col-rating')}
              {sortHeader('accuracy', 'Accuracy', 'col-rating')}
              {sortHeader('size', 'Size', 'col-status')}
            </tr>
          </thead>
          <tbody>
            {visible.map((model) => {
              const name = fullModelName(model);
              return (
                <tr
                  key={model.name}
                  data-testid={`model-row-${model.name}`}
                  className={`${selected === model.name ? 'selected' : ''} ${model.name === activeModel ? 'active' : ''}`}
                  onClick={() => setSelected(model.name)}
                >
                  <td className="col-star">
                    <button
                      className={`ml-icon-btn ml-star ${model.favorite ? 'on' : ''}`}
                      aria-label={model.favorite ? `Remove ${name} from favorites` : `Add ${name} to favorites`}
                      aria-pressed={model.favorite}
                      aria-disabled={model.name in favoriteOverrides || undefined}
                      onClick={(e) => { e.stopPropagation(); toggleFavorite(model); }}
                    >
                      <Icon name={model.favorite ? 'star-filled' : 'star'} />
                    </button>
                  </td>
                  <td className="col-name">
                    <span className="ml-name-cell" role="button" tabIndex={0} aria-label={`Show ${name} details`} onKeyDown={(e) => onRowKey(e, model)}>
                      <ProviderLogo vendor={modelVendor(model)} />
                      <span className="ml-name">{model.displayName}</span>
                      {model.englishOnly && <span className="ml-tag" title="English only">EN</span>}
                      {model.recommendation.startsWith('Recommended') && <span className="ml-tag accent" title={model.recommendation}>Recommended</span>}
                    </span>
                  </td>
                  <td className="col-rating"><Segments label="Speed" value={model.speed} text={speedLabel(model.speed)} /></td>
                  <td className="col-rating"><Segments label="Accuracy" value={model.accuracy} text={accuracyLabel(model.accuracy)} /></td>
                  <td className="col-status">{rowStatus(model)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {visible.length === 0 && <p className="ml-empty">No models match your search or filters.</p>}
      </div>

      {selectedModel && (
        <ModelDetail
          model={selectedModel}
          active={selectedModel.name === activeModel}
          downloadState={downloadState}
          actionError={errors[selectedModel.name] ?? ''}
          pending={pending === selectedModel.name}
          onClose={closeDetail}
          onRun={(action) => run(selectedModel.name, action)}
          onDownload={onDownload}
          onCancel={onCancel}
          onUse={onUse}
          onDelete={onDelete}
          onToggleFavorite={() => toggleFavorite(selectedModel)}
          favoriteSaving={selectedModel.name in favoriteOverrides}
        />
      )}
    </div>
  );
}
