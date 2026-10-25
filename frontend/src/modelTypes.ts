export interface ModelInfo {
  name: string;
  displayName: string;
  size: string;
  sizeBytes: number;
  downloaded: boolean;
  englishOnly: boolean;
  provider: string;
  speed: number;
  accuracy: number;
  useCase: string;
  recommendation: string;
  favorite: boolean;
}

export type ModelDownloadStatus =
  | 'idle'
  | 'downloading'
  | 'cancelling'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface ModelDownloadState {
  model: string;
  status: ModelDownloadStatus;
  downloaded: number;
  total: number;
  progress: number;
  error: string;
}

export const IDLE_DOWNLOAD: ModelDownloadState = {
  model: '',
  status: 'idle',
  downloaded: 0,
  total: 0,
  progress: 0,
  error: '',
};

export function isDownloadActive(state: ModelDownloadState): boolean {
  return state.status === 'downloading' || state.status === 'cancelling';
}

export function isTerminal(state: ModelDownloadState): boolean {
  return state.status === 'completed' || state.status === 'failed' || state.status === 'cancelled';
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 MB';
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(2)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(bytes >= 1e8 ? 0 : 1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1e3))} KB`;
}

// Library rows show the shared display name with an EN badge; elsewhere the
// language must be part of the name to tell variants apart.
export function fullModelName(model: Pick<ModelInfo, 'displayName' | 'englishOnly'>): string {
  return model.englishOnly ? `${model.displayName} (English)` : model.displayName;
}

export function providerLabel(provider: string): string {
  return provider === 'local' ? 'OpenAI Whisper' : provider;
}

export function languageLabel(model: Pick<ModelInfo, 'englishOnly'>): string {
  return model.englishOnly ? 'English only' : 'Multilingual';
}

const SPEED_LABELS = ['', 'Slowest', 'Slow', 'Moderate', 'Fast', 'Fastest'];
const ACCURACY_LABELS = ['', 'Basic', 'Good', 'Better', 'High', 'Highest'];

export function speedLabel(rating: number): string {
  return SPEED_LABELS[rating] ?? '';
}

export function accuracyLabel(rating: number): string {
  return ACCURACY_LABELS[rating] ?? '';
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}
