import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ModelsLibrary, ModelsLibraryProps, filterModels } from './ModelsLibrary';
import { IDLE_DOWNLOAD } from './modelTypes';
import { MODELS, downloadState } from './test/fixtures';

function renderLibrary(overrides: Partial<ModelsLibraryProps> = {}) {
  const props: ModelsLibraryProps = {
    models: MODELS,
    activeModel: 'base.en',
    provider: 'local',
    downloadState: IDLE_DOWNLOAD,
    onDownload: vi.fn(() => Promise.resolve()),
    onCancel: vi.fn(() => Promise.resolve()),
    onUse: vi.fn(() => Promise.resolve()),
    onDelete: vi.fn(() => Promise.resolve()),
    ...overrides,
  };
  render(<ModelsLibrary {...props} />);
  return props;
}

const card = (name: string) => screen.getByTestId(`model-card-${name}`);
const visibleNames = () => screen.queryAllByTestId(/^model-card-/).map((el) => el.getAttribute('data-testid')!.replace('model-card-', ''));

describe('filterModels', () => {
  it('searches names and descriptive metadata', () => {
    expect(filterModels(MODELS, 'large', 'all').map((m) => m.name)).toEqual(['large-v3']);
    expect(filterModels(MODELS, 'low-power', 'all').map((m) => m.name)).toEqual(['tiny.en']);
    expect(filterModels(MODELS, 'recommended', 'all').map((m) => m.name)).toEqual(['base.en', 'small']);
    expect(filterModels(MODELS, 'multilingual most accurate', 'all').map((m) => m.name)).toEqual(['large-v3']);
  });

  it('filters by language and download status', () => {
    expect(filterModels(MODELS, '', 'english').map((m) => m.name)).toEqual(['tiny.en', 'base.en']);
    expect(filterModels(MODELS, '', 'multilingual').map((m) => m.name)).toEqual(['base', 'small', 'large-v3']);
    expect(filterModels(MODELS, '', 'downloaded').map((m) => m.name)).toEqual(['base.en', 'base']);
  });
});

describe('ModelsLibrary', () => {
  it('lists every model with language, recommendation, size and ratings', () => {
    renderLibrary();
    expect(visibleNames()).toEqual(MODELS.map((m) => m.name));
    const tiny = within(card('tiny.en'));
    expect(tiny.getByText('English only')).toBeInTheDocument();
    expect(tiny.getByText('Best for speed')).toHaveClass('recommend');
    expect(tiny.getByText('78 MB')).toBeInTheDocument();
    expect(tiny.getByLabelText('Speed: Fastest (5 of 5)')).toBeInTheDocument();
    expect(tiny.getByLabelText('Accuracy: Basic (1 of 5)')).toBeInTheDocument();
    expect(within(card('large-v3')).getByText('Multilingual')).toBeInTheDocument();
  });

  it('search input and filter buttons narrow the list', () => {
    renderLibrary();
    fireEvent.change(screen.getByLabelText('Search models'), { target: { value: 'small' } });
    expect(visibleNames()).toEqual(['small']);
    fireEvent.change(screen.getByLabelText('Search models'), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Downloaded (2)' }));
    expect(visibleNames()).toEqual(['base.en', 'base']);
    fireEvent.click(screen.getByRole('button', { name: 'English only' }));
    expect(visibleNames()).toEqual(['tiny.en', 'base.en']);
    fireEvent.change(screen.getByLabelText('Search models'), { target: { value: 'zzz' } });
    expect(screen.getByText('No models match your search.')).toBeInTheDocument();
  });

  it('renders the right actions for unavailable, downloaded and active models', () => {
    renderLibrary();
    const unavailable = within(card('small'));
    expect(unavailable.getByRole('button', { name: 'Download (488 MB)' })).toBeEnabled();
    expect(unavailable.queryByRole('button', { name: 'Use this model' })).toBeNull();

    const downloaded = within(card('base'));
    expect(downloaded.getByRole('button', { name: 'Use this model' })).toBeInTheDocument();
    expect(downloaded.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
    expect(downloaded.getByText('Downloaded')).toBeInTheDocument();

    const active = within(card('base.en'));
    expect(active.getByText('Active')).toBeInTheDocument();
    expect(active.queryByRole('button', { name: 'Delete' })).toBeNull();
    expect(active.queryByRole('button', { name: 'Use this model' })).toBeNull();
    expect(active.getByText(/cannot be deleted/)).toBeInTheDocument();
  });

  it('shows progress, bytes and cancel for the downloading model only', async () => {
    const props = renderLibrary({
      downloadState: downloadState({ model: 'small', status: 'downloading', downloaded: 243800983, total: 487601967, progress: 50 }),
    });
    const downloading = within(card('small'));
    expect(downloading.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '50');
    expect(downloading.getByText('244 MB of 488 MB · 50%')).toBeInTheDocument();
    expect(downloading.getByText('Downloading')).toBeInTheDocument();
    fireEvent.click(downloading.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(props.onCancel).toHaveBeenCalledWith('small'));

    // Other unavailable models cannot start a second download.
    const other = within(card('large-v3')).getByRole('button', { name: 'Download (3.1 GB)' });
    expect(other).toBeDisabled();
    expect(other).toHaveAttribute('title', 'Another model is downloading');
  });

  it('disables cancel while cancelling', () => {
    renderLibrary({ downloadState: downloadState({ model: 'small', status: 'cancelling', progress: 10, downloaded: 1, total: 2 }) });
    expect(within(card('small')).getByRole('button', { name: 'Cancelling…' })).toBeDisabled();
  });

  it('shows inline failures and retries the failed model', async () => {
    const props = renderLibrary({ downloadState: downloadState({ model: 'small', status: 'failed', error: 'download failed: HTTP 503' }) });
    const failed = within(card('small'));
    expect(failed.getByRole('alert')).toHaveTextContent('Download failed: download failed: HTTP 503');
    fireEvent.click(failed.getByRole('button', { name: 'Retry download' }));
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('small'));
  });

  it('offers retry after cancellation', async () => {
    const props = renderLibrary({ downloadState: downloadState({ model: 'large-v3', status: 'cancelled' }) });
    const cancelled = within(card('large-v3'));
    expect(cancelled.getByText('Download cancelled.')).toBeInTheDocument();
    fireEvent.click(cancelled.getByRole('button', { name: 'Retry download' }));
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('large-v3'));
  });

  it('shows rejected actions inline instead of alerting', async () => {
    const onDownload = vi.fn(() => Promise.reject('another model is already downloading: base'));
    renderLibrary({ onDownload });
    fireEvent.click(within(card('small')).getByRole('button', { name: 'Download (488 MB)' }));
    expect(await within(card('small')).findByRole('alert')).toHaveTextContent('another model is already downloading: base');
  });

  it('activates and deletes downloaded models through callbacks', async () => {
    const props = renderLibrary();
    const downloaded = within(card('base'));
    fireEvent.click(downloaded.getByRole('button', { name: 'Use this model' }));
    await waitFor(() => expect(props.onUse).toHaveBeenCalledWith('base'));

    fireEvent.click(downloaded.getByRole('button', { name: 'Delete' }));
    expect(props.onDelete).not.toHaveBeenCalled();
    fireEvent.click(downloaded.getByRole('button', { name: 'Keep' }));
    expect(props.onDelete).not.toHaveBeenCalled();
    fireEvent.click(downloaded.getByRole('button', { name: 'Delete' }));
    fireEvent.click(downloaded.getByRole('button', { name: 'Confirm delete' }));
    await waitFor(() => expect(props.onDelete).toHaveBeenCalledWith('base'));
  });

  it('explains that local models are unused while OpenAI is active', () => {
    renderLibrary({ provider: 'openai' });
    expect(screen.getByText(/OpenAI is your current provider/)).toBeInTheDocument();
  });

  it('highlights the focused model', () => {
    renderLibrary({ focusModel: 'large-v3' });
    expect(card('large-v3')).toHaveClass('focused');
    expect(card('small')).not.toHaveClass('focused');
  });
});
