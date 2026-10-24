import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as Api from '../wailsjs/go/main/App';
import { Onboarding } from './Onboarding';
import { EventsEmit, listenerCount, resetListeners } from './test/runtimeMock';
import { MODELS, downloadState } from './test/fixtures';
import { IDLE_DOWNLOAD } from './modelTypes';

vi.mock('../wailsjs/runtime/runtime', async () => await import('./test/runtimeMock'));
vi.mock('../wailsjs/go/main/App', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>();
  return Object.fromEntries(Object.keys(actual).map((key) => [key, vi.fn(() => Promise.resolve())]));
});

const api = Api as unknown as Record<string, ReturnType<typeof vi.fn>>;

// tiny.en is not downloaded in the fixtures, so onboarding must download it.
async function startTinyDownload() {
  api.GetModels.mockResolvedValue(MODELS);
  api.GetModelDownloadState.mockResolvedValue(IDLE_DOWNLOAD);
  api.GetRecordingHotkeyDisplayName.mockResolvedValue('Right Option');
  api.GetPlatform.mockResolvedValue('darwin');
  api.CheckMicrophonePermission.mockResolvedValue('undetermined');
  render(<Onboarding onComplete={() => {}} />);
  fireEvent.click(screen.getByRole('button', { name: 'Get Started' }));
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  fireEvent.click(await screen.findByText('Tiny (English)'));
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
    expect(screen.getByText('Downloading Tiny (English)')).toBeInTheDocument();
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
});
