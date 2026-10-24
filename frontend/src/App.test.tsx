import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as Api from '../wailsjs/go/main/App';
import App from './App';
import { EventsEmit, listenerCount, resetListeners } from './test/runtimeMock';
import { MODELS, downloadState } from './test/fixtures';
import { IDLE_DOWNLOAD } from './modelTypes';

vi.mock('../wailsjs/runtime/runtime', async () => await import('./test/runtimeMock'));
vi.mock('../wailsjs/go/main/App', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>();
  return Object.fromEntries(Object.keys(actual).map((key) => [key, vi.fn(() => Promise.resolve())]));
});

const api = Api as unknown as Record<string, ReturnType<typeof vi.fn>>;

function appState(overrides: Record<string, unknown> = {}) {
  return {
    state: 'ready', recordingTime: 0, lastTranscript: '', error: '',
    currentModel: 'base.en', currentProvider: 'local', modelReady: true, hotkeyEnabled: true,
    ...overrides,
  };
}

function setup({ state = appState(), models = MODELS, download = IDLE_DOWNLOAD } = {}) {
  api.IsOnboardingCompleted.mockResolvedValue(true);
  api.GetPlatform.mockResolvedValue('linux');
  api.GetState.mockResolvedValue(state);
  api.GetModels.mockResolvedValue(models);
  api.GetConfig.mockResolvedValue({ provider: state.currentProvider, model: state.currentModel, autoPaste: true });
  api.GetHistory.mockResolvedValue([]);
  api.GetAudioInputDevices.mockResolvedValue([]);
  api.GetStats.mockResolvedValue({ averageWPM: 0, wordsThisWeek: 0, recordingsThisWeek: 0, timeSavedThisWeek: 0, totalRecordings: 0, totalWords: 0 });
  api.GetRecordingHotkey.mockResolvedValue('rightalt');
  api.GetCancelHotkey.mockResolvedValue('escape');
  api.GetModelDownloadState.mockResolvedValue(download);
  return render(<StrictMode><App /></StrictMode>);
}

const nav = (name: string) => within(document.querySelector('.sidebar-nav') as HTMLElement).getByRole('button', { name: new RegExp(`^${name}`) });

describe('App model integration', () => {
  beforeEach(() => {
    resetListeners();
    Object.values(api).forEach((fn) => {
      fn.mockReset();
      fn.mockImplementation(() => Promise.resolve());
    });
  });

  it('adds a Models page to the sidebar and keeps a single download listener', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: /^Models/ }));
    expect(document.querySelector('.page-title')).toHaveTextContent('Models');
    expect(await screen.findByTestId('model-card-large-v3')).toBeInTheDocument();
    expect(listenerCount('modelDownloadState')).toBe(1);
    expect(listenerCount('stateChanged')).toBe(1);
    expect(listenerCount('downloadComplete')).toBe(0);
  });

  it('Settings summarises the active model and links into the library', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: /^Settings/ }));
    const row = (await screen.findByText('Base (English)')).closest('.setting-row') as HTMLElement;
    expect(row).toHaveTextContent('148 MB · English only · Downloaded');
    expect(screen.queryByRole('combobox', { name: /model/i })).toBeNull();
    fireEvent.click(within(row).getByRole('button', { name: 'Browse Models' }));
    expect(await screen.findByTestId('model-card-base.en')).toHaveClass('focused');
  });

  it('Settings hides local model controls when OpenAI is active', async () => {
    setup({ state: appState({ currentProvider: 'openai' }) });
    fireEvent.click(await screen.findByRole('button', { name: /^Settings/ }));
    expect(await screen.findByText(/OpenAI transcribes with its hosted Whisper model/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Browse Models' })).toBeNull();
  });

  it('Home routes a missing model to the library or downloads that exact model', async () => {
    setup({ state: appState({ currentModel: 'small', modelReady: false }) });
    const prompt = (await screen.findByText('Model "Small" needs to be downloaded')).closest('.download-prompt') as HTMLElement;
    fireEvent.click(within(prompt).getByRole('button', { name: 'Download (488 MB)' }));
    await waitFor(() => expect(api.DownloadModel).toHaveBeenCalledWith('small'));

    fireEvent.click(within(prompt).getByRole('button', { name: 'Browse models' }));
    expect(await screen.findByTestId('model-card-small')).toHaveClass('focused');
  });

  it('Home shows download start errors inline', async () => {
    setup({ state: appState({ currentModel: 'small', modelReady: false }) });
    api.DownloadModel.mockRejectedValue('another model is already downloading: tiny');
    fireEvent.click(await screen.findByRole('button', { name: 'Download (488 MB)' }));
    expect(await screen.findByText('another model is already downloading: tiny')).toBeInTheDocument();
  });

  it('Home progress is labelled by the downloading model, not the active one', async () => {
    setup({ download: downloadState({ model: 'large-v3', status: 'downloading', downloaded: 1e9, total: 3095033483, progress: 32.3 }) });
    expect(await screen.findByText('Downloading Large V3')).toBeInTheDocument();
    expect(screen.getByText('1.00 GB of 3.10 GB')).toBeInTheDocument();
    expect(nav('Models')).toHaveTextContent('32%');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(api.CancelModelDownload).toHaveBeenCalledWith('large-v3'));
  });

  it('download completion refreshes availability without activating the model', async () => {
    setup();
    await screen.findByText('Get started');
    const before = api.GetModels.mock.calls.length;
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'small', status: 'downloading', total: 10 })));
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'small', status: 'completed', downloaded: 10, total: 10, progress: 100 })));
    await waitFor(() => expect(api.GetModels.mock.calls.length).toBeGreaterThan(before));
    expect(api.SetModel).not.toHaveBeenCalled();
  });

  it('activates a downloaded model from the library with one action', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: /^Models/ }));
    fireEvent.click(within(await screen.findByTestId('model-card-base')).getByRole('button', { name: 'Use this model' }));
    await waitFor(() => expect(api.SetModel).toHaveBeenCalledWith('base'));
  });

  it('shows backend delete rejections inline', async () => {
    setup();
    api.DeleteModel.mockRejectedValue('the active model cannot be deleted');
    fireEvent.click(await screen.findByRole('button', { name: /^Models/ }));
    const card = within(await screen.findByTestId('model-card-base'));
    fireEvent.click(card.getByRole('button', { name: 'Delete' }));
    fireEvent.click(card.getByRole('button', { name: 'Confirm delete' }));
    expect(await card.findByRole('alert')).toHaveTextContent('the active model cannot be deleted');
  });
});
