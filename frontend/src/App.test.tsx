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

function setup({
  state = appState(), models = MODELS, download = IDLE_DOWNLOAD,
  platform = 'linux' as string | Promise<string>, hotkeys = ['rightalt', 'escape'],
} = {}) {
  api.IsOnboardingCompleted.mockResolvedValue(true);
  api.GetPlatform.mockImplementation(() => Promise.resolve(platform));
  api.GetState.mockResolvedValue(state);
  api.GetModels.mockResolvedValue(models);
  api.GetConfig.mockResolvedValue({ provider: state.currentProvider, model: state.currentModel, autoPaste: true });
  api.GetHistory.mockResolvedValue([]);
  api.GetAudioInputDevices.mockResolvedValue([]);
  api.GetStats.mockResolvedValue({ averageWPM: 0, wordsThisWeek: 0, recordingsThisWeek: 0, timeSavedThisWeek: 0, totalRecordings: 0, totalWords: 0 });
  api.GetRecordingHotkey.mockResolvedValue(hotkeys[0]);
  api.GetCancelHotkey.mockResolvedValue(hotkeys[1]);
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
    expect(document.querySelector('.page-title')).toHaveTextContent('Models library');
    expect(await screen.findByTestId('model-row-large-v3')).toBeInTheDocument();
    // The toolbar is rendered into the page header.
    expect(within(document.querySelector('.main-header') as HTMLElement).getByRole('button', { name: 'Filter models' })).toBeInTheDocument();
    expect(listenerCount('modelDownloadState')).toBe(1);
    expect(listenerCount('stateChanged')).toBe(1);
    expect(listenerCount('downloadComplete')).toBe(0);
  });

  it('Home lists Browse models second and drops the changelog', async () => {
    setup();
    await screen.findByText('Get started');
    const titles = Array.from(document.querySelectorAll('.action-list .action-title')).map((el) => el.textContent);
    expect(titles).toEqual(['Start recording', 'Browse models', 'Settings', 'View history']);
    expect(screen.queryByText('What\'s new')).toBeNull();
    expect(document.querySelector('.whats-new-section')).toBeNull();
  });

  it('Settings summarises the active model and links into the library', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: /^Settings/ }));
    const row = (await screen.findByText('Whisper Base (English)')).closest('.setting-row') as HTMLElement;
    expect(row).toHaveTextContent('148 MB · English only · Downloaded');
    expect(screen.queryByRole('combobox', { name: /model/i })).toBeNull();
    fireEvent.click(within(row).getByRole('button', { name: 'Browse Models' }));
    expect(await screen.findByRole('dialog', { name: 'Whisper Base (English) details' })).toBeInTheDocument();
  });

  it('Settings hides local model controls when OpenAI is active', async () => {
    setup({ state: appState({ currentProvider: 'openai' }) });
    fireEvent.click(await screen.findByRole('button', { name: /^Settings/ }));
    expect(await screen.findByText(/OpenAI transcribes with its hosted Whisper model/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Browse Models' })).toBeNull();
  });

  it('Home routes a missing model to the library or downloads that exact model', async () => {
    setup({ state: appState({ currentModel: 'small', modelReady: false }) });
    const prompt = (await screen.findByText('Model "Whisper Small" needs to be downloaded')).closest('.download-prompt') as HTMLElement;
    fireEvent.click(within(prompt).getByRole('button', { name: 'Download (488 MB)' }));
    await waitFor(() => expect(api.DownloadModel).toHaveBeenCalledWith('small'));

    fireEvent.click(within(prompt).getByRole('button', { name: 'Browse models' }));
    expect(await screen.findByRole('dialog', { name: 'Whisper Small details' })).toBeInTheDocument();
  });

  it('Home shows download start errors inline', async () => {
    setup({ state: appState({ currentModel: 'small', modelReady: false }) });
    api.DownloadModel.mockRejectedValue('another model is already downloading: tiny');
    fireEvent.click(await screen.findByRole('button', { name: 'Download (488 MB)' }));
    expect(await screen.findByText('another model is already downloading: tiny')).toBeInTheDocument();
  });

  it('Home progress is labelled by the downloading model, not the active one', async () => {
    setup({ download: downloadState({ model: 'large-v3', status: 'downloading', downloaded: 1e9, total: 3095033483, progress: 32.3 }) });
    expect(await screen.findByText('Downloading Whisper Large v3')).toBeInTheDocument();
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
    fireEvent.click(await screen.findByTestId('model-row-base'));
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Use this model' }));
    await waitFor(() => expect(api.SetModel).toHaveBeenCalledWith('base'));
  });

  it('shows backend delete rejections inline', async () => {
    setup();
    api.DeleteModel.mockRejectedValue('the active model cannot be deleted');
    fireEvent.click(await screen.findByRole('button', { name: /^Models/ }));
    fireEvent.click(await screen.findByTestId('model-row-base'));
    const panel = within(screen.getByRole('dialog'));
    fireEvent.click(panel.getByRole('button', { name: 'Delete from this computer' }));
    fireEvent.click(panel.getByRole('button', { name: 'Confirm delete' }));
    expect(await panel.findByRole('alert')).toHaveTextContent('the active model cannot be deleted');
  });

  it('persists favorites through the backend and refreshes models', async () => {
    setup();
    fireEvent.click(await screen.findByRole('button', { name: /^Models/ }));
    const before = api.GetModels.mock.calls.length;
    fireEvent.click(within(await screen.findByTestId('model-row-small')).getByRole('button', { name: 'Add Whisper Small to favorites' }));
    await waitFor(() => expect(api.SetModelFavorite).toHaveBeenCalledWith('small', true));
    await waitFor(() => expect(api.GetModels.mock.calls.length).toBeGreaterThan(before));
  });
});

describe('App hotkey labels', () => {
  beforeEach(() => {
    resetListeners();
    Object.values(api).forEach((fn) => {
      fn.mockReset();
      fn.mockImplementation(() => Promise.resolve());
    });
    api.CheckAccessibilityPermission.mockResolvedValue(true);
  });

  it('never flashes macOS symbols on Windows while the platform loads', async () => {
    let resolvePlatform!: (p: string) => void;
    setup({ platform: new Promise<string>((resolve) => { resolvePlatform = resolve; }) });
    await screen.findByText('Start recording');
    expect(document.querySelector('.action-shortcut')).toBeNull();
    expect(document.querySelector('.hotkey-hint')).toBeNull();
    expect(document.body).not.toHaveTextContent('⌥');

    await act(async () => resolvePlatform('windows'));
    await waitFor(() => expect(document.querySelector('.action-shortcut')).toHaveTextContent(/^Alt$/));
    expect(document.querySelector('.hotkey-hint kbd')).toHaveTextContent(/^Right Alt$/);
    expect(document.body).not.toHaveTextContent('⌥');
  });

  it('shows the configured keys in the recording overlay', async () => {
    setup({ platform: 'darwin', hotkeys: ['rightcommand', 'f10'] });
    await waitFor(() => expect(listenerCount('stateChanged')).toBe(1));
    act(() => EventsEmit('stateChanged', appState({ state: 'recording' })));
    const stop = await screen.findByRole('button', { name: /^Stop/ });
    expect(stop.querySelector('kbd')).toHaveTextContent(/^⌘$/);
    expect(screen.getByRole('button', { name: /^Cancel/ }).querySelector('kbd')).toHaveTextContent(/^F10$/);
  });
});
