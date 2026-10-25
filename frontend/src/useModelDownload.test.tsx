import { StrictMode } from 'react';
import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as Api from '../wailsjs/go/main/App';
import { EventsEmit, listenerCount, resetListeners } from './test/runtimeMock';
import { MODEL_DOWNLOAD_EVENT, useModelDownload } from './useModelDownload';

vi.mock('../wailsjs/runtime/runtime', async () => await import('./test/runtimeMock'));
vi.mock('../wailsjs/go/main/App', () => ({ GetModelDownloadState: vi.fn() }));

function Probe() {
  const state = useModelDownload();
  return <span data-testid="state">{`${state.model}:${state.status}:${state.progress}`}</span>;
}

describe('useModelDownload', () => {
  beforeEach(() => {
    resetListeners();
    vi.mocked(Api.GetModelDownloadState).mockReset();
  });

  it('recovers existing state and follows events with one listener under StrictMode', async () => {
    vi.mocked(Api.GetModelDownloadState).mockResolvedValue({
      model: 'small', status: 'downloading', downloaded: 10, total: 100, progress: 10, error: '',
    } as any);
    const { unmount } = render(<StrictMode><Probe /></StrictMode>);
    expect(await screen.findByText('small:downloading:10')).toBeInTheDocument();
    expect(listenerCount(MODEL_DOWNLOAD_EVENT)).toBe(1);

    act(() => EventsEmit(MODEL_DOWNLOAD_EVENT, { model: 'small', status: 'completed', downloaded: 100, total: 100, progress: 100, error: '' }));
    expect(screen.getByTestId('state')).toHaveTextContent('small:completed:100');

    unmount();
    expect(listenerCount(MODEL_DOWNLOAD_EVENT)).toBe(0);
  });

  it('does not let a slow initial fetch overwrite a newer event', async () => {
    let resolve!: (value: any) => void;
    vi.mocked(Api.GetModelDownloadState).mockReturnValue(new Promise((r) => { resolve = r; }));
    render(<Probe />);
    act(() => EventsEmit(MODEL_DOWNLOAD_EVENT, { model: 'tiny', status: 'downloading', downloaded: 5, total: 10, progress: 50, error: '' }));
    await act(async () => resolve({ model: '', status: 'idle', downloaded: 0, total: 0, progress: 0, error: '' }));
    expect(screen.getByTestId('state')).toHaveTextContent('tiny:downloading:50');
  });
});
