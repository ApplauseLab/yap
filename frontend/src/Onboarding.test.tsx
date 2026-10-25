import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as Api from '../wailsjs/go/main/App';
import { Onboarding } from './Onboarding';
import { EventsEmit, listenerCount, resetListeners } from './test/runtimeMock';
import { MODELS, downloadState, model } from './test/fixtures';
import { IDLE_DOWNLOAD } from './modelTypes';

vi.mock('../wailsjs/runtime/runtime', async () => await import('./test/runtimeMock'));
vi.mock('../wailsjs/go/main/App', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>();
  return Object.fromEntries(Object.keys(actual).map((key) => [key, vi.fn(() => Promise.resolve())]));
});

const api = Api as unknown as Record<string, ReturnType<typeof vi.fn>>;

// tiny.en is not downloaded in the fixtures, so onboarding must download it.
async function startTinyDownload(prepare?: () => void) {
  api.GetModels.mockResolvedValue(MODELS);
  api.GetModelDownloadState.mockResolvedValue(IDLE_DOWNLOAD);
  api.GetRecordingHotkeyDisplayName.mockResolvedValue('Right Option');
  api.GetPlatform.mockResolvedValue('darwin');
  api.CheckMicrophonePermission.mockResolvedValue('undetermined');
  prepare?.();
  render(<Onboarding onComplete={() => {}} />);
  fireEvent.click(screen.getByRole('button', { name: 'Get Started' }));
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  fireEvent.click(await screen.findByRole('radio', { name: 'Whisper Tiny (English)' }));
  fireEvent.click(screen.getByRole('button', { name: 'Download & Continue' }));
  await waitFor(() => expect(api.DownloadModel).toHaveBeenCalledWith('tiny.en'));
}

describe('Onboarding model download', () => {
  beforeEach(() => {
    resetListeners();
    Object.values(api).forEach((fn) => {
      fn.mockReset();
      fn.mockImplementation(() => Promise.resolve());
    });
  });

  it('activates and advances only when its selected model completes', async () => {
    await startTinyDownload();
    expect(listenerCount('modelDownloadState')).toBe(1);

    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'downloading', downloaded: 38852357, total: 77704715, progress: 50 })));
    expect(screen.getByText('Downloading Whisper Tiny (English)')).toBeInTheDocument();
    expect(screen.getByText(/38\.9 MB of 77\.7 MB - 50%/)).toBeInTheDocument();

    // A different model completing must not advance or activate anything.
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'small', status: 'completed', progress: 100 })));
    expect(api.SetModel).not.toHaveBeenCalled();
    expect(screen.queryByText('Yap needs your microphone')).toBeNull();
    expect(document.querySelector('.download-step')).not.toBeNull();

    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'completed', downloaded: 77704715, total: 77704715, progress: 100 })));
    await waitFor(() => expect(api.SetModel).toHaveBeenCalledWith('tiny.en'));
    expect(api.SetModel).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Yap needs your microphone')).toBeInTheDocument();
  });

  it('shows failures inline and retries the same model', async () => {
    await startTinyDownload();
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'failed', error: 'download failed: HTTP 500' })));
    expect(screen.getByText('Download Failed')).toBeInTheDocument();
    expect(screen.getByText('download failed: HTTP 500')).toBeInTheDocument();
    expect(api.SetModel).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Retry Download' }));
    await waitFor(() => expect(api.DownloadModel).toHaveBeenCalledTimes(2));
    expect(api.DownloadModel).toHaveBeenLastCalledWith('tiny.en');
  });

  it('can cancel its download and return to model selection', async () => {
    await startTinyDownload();
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'downloading', downloaded: 1, total: 77704715, progress: 0 })));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(api.CancelModelDownload).toHaveBeenCalledWith('tiny.en'));
    expect(await screen.findByText('Choose a Model')).toBeInTheDocument();
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'completed', progress: 100 })));
    expect(api.SetModel).not.toHaveBeenCalled();
  });

  it('keeps waiting when the backend is already downloading the same model', async () => {
    await startTinyDownload(() => {
      api.DownloadModel.mockRejectedValue('another model is already downloading: tiny.en');
      // Mount sees idle; the backend is already downloading when rejected.
      api.GetModelDownloadState
        .mockResolvedValueOnce(IDLE_DOWNLOAD)
        .mockResolvedValue(downloadState({ model: 'tiny.en', status: 'downloading', total: 77704715 }));
    });
    await waitFor(() => expect(api.GetModelDownloadState).toHaveBeenCalledTimes(2));
    expect(screen.queryByText('Download Failed')).toBeNull();

    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'completed', progress: 100 })));
    await waitFor(() => expect(api.SetModel).toHaveBeenCalledWith('tiny.en'));
  });

  it('shows a busy rejection for a different model', async () => {
    await startTinyDownload(() => {
      api.DownloadModel.mockRejectedValue('another model is already downloading: small');
      api.GetModelDownloadState
        .mockResolvedValueOnce(IDLE_DOWNLOAD)
        .mockResolvedValue(downloadState({ model: 'small', status: 'downloading', total: 1 }));
    });
    expect(await screen.findByText('another model is already downloading: small')).toBeInTheDocument();
    act(() => EventsEmit('modelDownloadState', downloadState({ model: 'tiny.en', status: 'completed', progress: 100 })));
    expect(api.SetModel).not.toHaveBeenCalled();
  });

});

describe('Onboarding model choice', () => {
  beforeEach(() => {
    resetListeners();
    Object.values(api).forEach((fn) => {
      fn.mockReset();
      fn.mockImplementation(() => Promise.resolve());
    });
  });

  async function openModelStep() {
    api.GetModels.mockResolvedValue(MODELS.concat(model({ name: 'small.en', displayName: 'Whisper Small', size: '488 MB' })));
    api.GetModelDownloadState.mockResolvedValue(IDLE_DOWNLOAD);
    api.GetRecordingHotkeyDisplayName.mockResolvedValue('Right Option');
    api.GetPlatform.mockResolvedValue('darwin');
    render(<Onboarding onComplete={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: 'Get Started' }));
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    return screen.findByRole('radiogroup', { name: 'Choose a Model' });
  }

  it('groups the cards and describes size, purpose, and download state', async () => {
    const group = await openModelStep();
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((r) => r.getAttribute('aria-label'))).toEqual([
      'Whisper Tiny (English)',
      'Whisper Base (English), recommended',
      'Whisper Small (English)',
    ]);
    const base = within(group).getByRole('radio', { name: 'Whisper Base (English), recommended' });
    expect(base).toHaveAccessibleDescription(/148 MB.*Best balance of speed and accuracy.*Already downloaded/);
    const small = within(group).getByRole('radio', { name: 'Whisper Small (English)' });
    expect(small).toHaveAccessibleDescription(/488 MB/);
    expect(small).not.toHaveAccessibleDescription(/Already downloaded/);
    expect(within(base).getByText('Recommended')).toBeInTheDocument();
    expect(within(small).queryByText('Recommended')).toBeNull();
    for (const radio of radios) expect(within(radio).getByText('EN')).toBeInTheDocument();
  });

  it('uses one Tab stop and moves the selection with arrow keys', async () => {
    const group = await openModelStep();
    const radio = (name: string) => within(group).getByRole('radio', { name });
    const tiny = radio('Whisper Tiny (English)');
    const base = radio('Whisper Base (English), recommended');
    const small = radio('Whisper Small (English)');
    expect(base).toHaveAttribute('aria-checked', 'true');
    expect([tiny, base, small].map((r) => r.tabIndex)).toEqual([-1, 0, -1]);

    fireEvent.keyDown(base, { key: 'ArrowDown' });
    expect(small).toHaveAttribute('aria-checked', 'true');
    expect(small).toHaveFocus();
    expect([tiny, base, small].map((r) => r.tabIndex)).toEqual([-1, -1, 0]);

    fireEvent.keyDown(small, { key: 'ArrowDown' });
    expect(tiny).toHaveAttribute('aria-checked', 'true');
    expect(tiny).toHaveFocus();

    fireEvent.keyDown(tiny, { key: 'ArrowUp' });
    expect(small).toHaveAttribute('aria-checked', 'true');

    fireEvent.keyDown(base, { key: ' ' });
    expect(base).toHaveAttribute('aria-checked', 'true');
    expect(small).toHaveAttribute('aria-checked', 'false');
  });
});
