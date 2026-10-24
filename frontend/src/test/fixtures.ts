import { IDLE_DOWNLOAD, ModelDownloadState, ModelInfo } from '../modelTypes';

export function model(overrides: Partial<ModelInfo> & Pick<ModelInfo, 'name'>): ModelInfo {
  return {
    displayName: overrides.name,
    size: '148 MB',
    sizeBytes: 147964211,
    downloaded: false,
    englishOnly: overrides.name.endsWith('.en'),
    provider: 'local',
    speed: 3,
    accuracy: 3,
    useCase: 'General dictation.',
    recommendation: '',
    ...overrides,
  };
}

export const MODELS: ModelInfo[] = [
  model({ name: 'tiny.en', displayName: 'Tiny (English)', size: '78 MB', sizeBytes: 77704715, speed: 5, accuracy: 1, recommendation: 'Best for speed', useCase: 'Quick notes on low-power machines.' }),
  model({ name: 'base.en', displayName: 'Base (English)', speed: 4, accuracy: 2, recommendation: 'Recommended', downloaded: true, useCase: 'Everyday English dictation.' }),
  model({ name: 'base', displayName: 'Base', speed: 4, accuracy: 2, downloaded: true, useCase: 'Everyday dictation in many languages.' }),
  model({ name: 'small', displayName: 'Small', size: '488 MB', sizeBytes: 487601967, recommendation: 'Recommended multilingual', useCase: 'Best balance for languages other than English.' }),
  model({ name: 'large-v3', displayName: 'Large V3', size: '3.1 GB', sizeBytes: 3095033483, speed: 1, accuracy: 5, recommendation: 'Most accurate', useCase: 'Maximum accuracy in any language.' }),
];

export function downloadState(overrides: Partial<ModelDownloadState>): ModelDownloadState {
  return { ...IDLE_DOWNLOAD, ...overrides };
}
