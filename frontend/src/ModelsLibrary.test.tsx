import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ModelFilter, ModelsLibrary, ModelsLibraryProps, filterModels, sortModels } from './ModelsLibrary';
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
    onToggleFavorite: vi.fn(() => Promise.resolve()),
    ...overrides,
  };
  const utils = render(<ModelsLibrary {...props} />);
  return { props, ...utils };
}

const row = (name: string) => screen.getByTestId(`model-row-${name}`);
const visibleNames = () => screen.queryAllByTestId(/^model-row-/).map((el) => el.getAttribute('data-testid')!.replace('model-row-', ''));
const detail = () => screen.getByRole('dialog');
const filters = (...f: ModelFilter[]) => new Set<ModelFilter>(f);
const names = (models: { name: string }[]) => models.map((m) => m.name);

describe('filterModels / sortModels', () => {
  it('searches names, vendor and descriptive metadata', () => {
    expect(names(filterModels(MODELS, 'large', filters()))).toEqual(['large-v3']);
    expect(names(filterModels(MODELS, 'low-power', filters()))).toEqual(['tiny.en']);
    expect(names(filterModels(MODELS, 'recommended', filters()))).toEqual(['base.en', 'small']);
    expect(names(filterModels(MODELS, 'openai multilingual most accurate', filters()))).toEqual(['large-v3']);
  });

  it('combines language, download and favorite filters', () => {
    expect(names(filterModels(MODELS, '', filters('english')))).toEqual(['tiny.en', 'base.en']);
    expect(names(filterModels(MODELS, '', filters('multilingual')))).toEqual(['base', 'small', 'large-v3']);
    expect(names(filterModels(MODELS, '', filters('english', 'multilingual')))).toHaveLength(MODELS.length);
    expect(names(filterModels(MODELS, '', filters('downloaded')))).toEqual(['base.en', 'base']);
    expect(names(filterModels(MODELS, '', filters('downloaded', 'favorites')))).toEqual(['base']);
  });

  it('sorts by name with multilingual before English variants, and by ratings and size', () => {
    expect(names(sortModels(MODELS, { key: 'name', dir: 1 }))).toEqual(['base', 'base.en', 'large-v3', 'small', 'tiny.en']);
    expect(names(sortModels(MODELS, { key: 'name', dir: -1 }))).toEqual(['tiny.en', 'small', 'large-v3', 'base.en', 'base']);
    expect(names(sortModels(MODELS, { key: 'speed', dir: -1 }))[0]).toBe('tiny.en');
    expect(names(sortModels(MODELS, { key: 'accuracy', dir: -1 }))[0]).toBe('large-v3');
    expect(names(sortModels(MODELS, { key: 'size', dir: 1 }))[0]).toBe('tiny.en');
  });
});

describe('ModelsLibrary table', () => {
  it('lists every model with logo, EN badge, recommendation, ratings and size', () => {
    renderLibrary();
    expect(visibleNames()).toEqual(['base', 'base.en', 'large-v3', 'small', 'tiny.en']);
    const tiny = within(row('tiny.en'));
    expect(tiny.getByText('Whisper Tiny')).toBeInTheDocument();
    expect(tiny.getByText('EN')).toHaveAttribute('title', 'English only');
    expect(tiny.getByRole('img', { name: 'OpenAI' })).toBeInTheDocument();
    expect(tiny.getByRole('img', { name: 'Speed: Fastest (5 of 5)' })).toBeInTheDocument();
    expect(tiny.getByRole('img', { name: 'Accuracy: Basic (1 of 5)' })).toBeInTheDocument();
    expect(tiny.getByText('78 MB')).toBeInTheDocument();
    expect(within(row('large-v3')).queryByText('EN')).toBeNull();
    expect(within(row('small')).getByText('Recommended')).toBeInTheDocument();
  });

  it('shows active, downloaded and downloadable states in the status column', () => {
    renderLibrary();
    expect(within(row('base.en')).getByText('Active')).toBeInTheDocument();
    expect(within(row('base.en')).getByRole('img', { name: 'Whisper Base (English) is active' })).toBeInTheDocument();
    expect(within(row('base')).getByRole('img', { name: 'Whisper Base downloaded' })).toBeInTheDocument();
    expect(within(row('small')).getByRole('button', { name: 'Download Whisper Small' })).toBeEnabled();
  });

  it('sorts when a column header is clicked', () => {
    renderLibrary();
    fireEvent.click(screen.getByRole('button', { name: 'Size' }));
    expect(visibleNames()[0]).toBe('large-v3');
    fireEvent.click(screen.getByRole('button', { name: 'Size' }));
    expect(visibleNames()[0]).toBe('tiny.en');
    fireEvent.click(screen.getByRole('button', { name: /Model name/ }));
    expect(visibleNames()[0]).toBe('base');
  });

  it('searches from the toolbar', () => {
    renderLibrary();
    fireEvent.click(screen.getByRole('button', { name: 'Search models' }));
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search models' }), { target: { value: 'small' } });
    expect(visibleNames()).toEqual(['small']);
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search models' }), { target: { value: 'zzz' } });
    expect(screen.getByText('No models match your search or filters.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Close search' }));
    expect(visibleNames()).toHaveLength(MODELS.length);
  });

  it('filters through the menu and removable chips', () => {
    renderLibrary();
    fireEvent.click(screen.getByRole('button', { name: 'Filter models' }));
    const menu = screen.getByRole('menu', { name: 'Filter models' });
    expect(within(menu).getByRole('menuitemcheckbox', { name: /Downloaded/ })).toHaveTextContent('Downloaded2');
    fireEvent.click(within(menu).getByRole('menuitemcheckbox', { name: /Downloaded/ }));
    expect(visibleNames()).toEqual(['base', 'base.en']);
    fireEvent.click(within(menu).getByRole('menuitemcheckbox', { name: /Favorites/ }));
    expect(visibleNames()).toEqual(['base']);
    expect(within(menu).getByRole('menuitemcheckbox', { name: /Favorites/ })).toHaveAttribute('aria-checked', 'true');

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Remove Favorites filter' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove Downloaded filter' }));
    expect(visibleNames()).toHaveLength(MODELS.length);
  });

  it('toggles favorites optimistically and rolls back on failure', async () => {
    const onToggleFavorite = vi.fn((_model: string, _favorite: boolean): Promise<void> => Promise.resolve());
    renderLibrary({ onToggleFavorite });
    const star = within(row('small')).getByRole('button', { name: 'Add Whisper Small to favorites' });
    fireEvent.click(star);
    expect(onToggleFavorite).toHaveBeenCalledWith('small', true);
    expect(within(row('small')).getByRole('button', { name: 'Remove Whisper Small from favorites' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('dialog')).toBeNull();

    onToggleFavorite.mockImplementationOnce(() => Promise.reject('config is read-only'));
    fireEvent.click(within(row('small')).getByRole('button', { name: 'Remove Whisper Small from favorites' }));
    await waitFor(() => expect(within(row('small')).getByRole('button', { name: 'Remove Whisper Small from favorites' })).toBeInTheDocument());
  });

  it('downloads from the row icon without opening details', async () => {
    const { props } = renderLibrary();
    fireEvent.click(within(row('small')).getByRole('button', { name: 'Download Whisper Small' }));
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('small'));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('opens details with the error when a row download is rejected', async () => {
    renderLibrary({ onDownload: vi.fn(() => Promise.reject('another model is already downloading: base')) });
    fireEvent.click(within(row('small')).getByRole('button', { name: 'Download Whisper Small' }));
    const panel = await screen.findByRole('dialog', { name: 'Whisper Small details' });
    expect(within(panel).getByRole('alert')).toHaveTextContent('another model is already downloading: base');
  });

  it('shows progress in the row and disables other downloads', () => {
    renderLibrary({ downloadState: downloadState({ model: 'small', status: 'downloading', downloaded: 243800983, total: 487601967, progress: 50 }) });
    expect(within(row('small')).getByText('50%')).toBeInTheDocument();
    expect(within(row('small')).getByLabelText('Downloading Whisper Small')).toBeInTheDocument();
    const other = within(row('large-v3')).getByRole('button', { name: 'Download Whisper Large v3' });
    expect(other).toBeDisabled();
    expect(other).toHaveAttribute('title', 'Another model is downloading');
  });

  it('marks failed downloads in the row and offers retry', async () => {
    const { props } = renderLibrary({ downloadState: downloadState({ model: 'small', status: 'failed', error: 'server returned HTTP 503' }) });
    const retry = within(row('small')).getByRole('button', { name: 'Retry downloading Whisper Small' });
    expect(retry).toHaveAttribute('title', 'Download failed: server returned HTTP 503');
    fireEvent.click(retry);
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('small'));
  });

  it('explains that local models are unused while OpenAI is active', () => {
    renderLibrary({ provider: 'openai' });
    expect(screen.getByText(/OpenAI is your current provider/)).toBeInTheDocument();
  });

  it('renders the toolbar into a supplied header target', () => {
    const target = document.createElement('div');
    document.body.appendChild(target);
    renderLibrary({ toolbarTarget: target });
    expect(within(target).getByRole('button', { name: 'Filter models' })).toBeInTheDocument();
    target.remove();
  });
});

describe('ModelsLibrary detail panel', () => {
  it('opens from a row with description, specs, ratings and size', () => {
    renderLibrary();
    fireEvent.click(row('large-v3'));
    const panel = within(detail());
    expect(detail()).toHaveAccessibleName('Whisper Large v3 details');
    expect(panel.getByText('Most accurate')).toBeInTheDocument();
    expect(panel.getByText('Maximum accuracy in any language.')).toBeInTheDocument();
    expect(panel.getByText('Multilingual (99 languages)')).toBeInTheDocument();
    expect(panel.getByRole('img', { name: 'High accuracy' })).toBeInTheDocument();
    expect(panel.getByRole('img', { name: 'Multilingual' })).toBeInTheDocument();
    expect(panel.queryByRole('img', { name: 'Fast' })).toBeNull();
    expect(panel.getByRole('img', { name: 'Accuracy: Highest (5 of 5)' })).toBeInTheDocument();
    expect(panel.getByText('3.1 GB')).toBeInTheDocument();
    expect(panel.getByRole('button', { name: 'Download · 3.1 GB' })).toBeEnabled();
  });

  it('opens from the keyboard and closes with Escape or the back button', () => {
    renderLibrary();
    fireEvent.keyDown(within(row('tiny.en')).getByRole('button', { name: 'Show Whisper Tiny (English) details' }), { key: 'Enter' });
    expect(detail()).toHaveAccessibleName('Whisper Tiny (English) details');
    expect(within(detail()).getByRole('img', { name: 'English only' })).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    fireEvent.click(row('tiny.en'));
    fireEvent.click(screen.getByRole('button', { name: 'Close details' }));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('opens the focused model on mount', () => {
    renderLibrary({ focusModel: 'small' });
    expect(detail()).toHaveAccessibleName('Whisper Small details');
    expect(row('small')).toHaveClass('selected');
  });

  it('downloads, shows progress and cancels', async () => {
    const { props, rerender } = renderLibrary({ focusModel: 'small' });
    fireEvent.click(within(detail()).getByRole('button', { name: 'Download · 488 MB' }));
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('small'));

    rerender(<ModelsLibrary {...props} downloadState={downloadState({ model: 'small', status: 'downloading', downloaded: 243800983, total: 487601967, progress: 50 })} />);
    expect(within(detail()).getByRole('progressbar', { name: 'Downloading Whisper Small' })).toHaveAttribute('aria-valuenow', '50');
    expect(within(detail()).getByText('244 MB of 488 MB · 50%')).toBeInTheDocument();
    fireEvent.click(within(detail()).getByRole('button', { name: 'Cancel download' }));
    await waitFor(() => expect(props.onCancel).toHaveBeenCalledWith('small'));

    rerender(<ModelsLibrary {...props} downloadState={downloadState({ model: 'small', status: 'cancelling', progress: 50, downloaded: 1, total: 2 })} />);
    expect(within(detail()).getByRole('button', { name: 'Cancelling…' })).toBeDisabled();

    rerender(<ModelsLibrary {...props} downloadState={downloadState({ model: 'small', status: 'cancelled' })} />);
    expect(within(detail()).getByText('Download cancelled.')).toBeInTheDocument();
    expect(within(detail()).getByRole('button', { name: 'Retry download · 488 MB' })).toBeEnabled();
  });

  it('shows inline failures with retry', async () => {
    const { props } = renderLibrary({ focusModel: 'small', downloadState: downloadState({ model: 'small', status: 'failed', error: 'server returned HTTP 503' }) });
    expect(within(detail()).getByRole('alert')).toHaveTextContent('Download failed: server returned HTTP 503');
    fireEvent.click(within(detail()).getByRole('button', { name: 'Retry download · 488 MB' }));
    await waitFor(() => expect(props.onDownload).toHaveBeenCalledWith('small'));
  });

  it('activates and deletes downloaded inactive models, with confirmation', async () => {
    const { props } = renderLibrary({ focusModel: 'base' });
    fireEvent.click(within(detail()).getByRole('button', { name: 'Use this model' }));
    await waitFor(() => expect(props.onUse).toHaveBeenCalledWith('base'));

    fireEvent.click(within(detail()).getByRole('button', { name: 'Delete from this computer' }));
    fireEvent.click(within(detail()).getByRole('button', { name: 'Keep' }));
    expect(props.onDelete).not.toHaveBeenCalled();
    fireEvent.click(within(detail()).getByRole('button', { name: 'Delete from this computer' }));
    fireEvent.click(within(detail()).getByRole('button', { name: 'Confirm delete' }));
    await waitFor(() => expect(props.onDelete).toHaveBeenCalledWith('base'));
  });

  it('protects the active model', () => {
    renderLibrary({ focusModel: 'base.en' });
    expect(within(detail()).getByRole('button', { name: 'Active model' })).toBeDisabled();
    expect(within(detail()).queryByRole('button', { name: 'Delete from this computer' })).toBeNull();
    expect(within(detail()).getByText('The active model can’t be deleted.')).toBeInTheDocument();
  });

  it('shows rejected actions inline', async () => {
    renderLibrary({ focusModel: 'base', onDelete: vi.fn(() => Promise.reject('the active model cannot be deleted')) });
    fireEvent.click(within(detail()).getByRole('button', { name: 'Delete from this computer' }));
    fireEvent.click(within(detail()).getByRole('button', { name: 'Confirm delete' }));
    expect(await within(detail()).findByRole('alert')).toHaveTextContent('the active model cannot be deleted');
  });

  it('stars from the detail header', async () => {
    const { props } = renderLibrary({ focusModel: 'small' });
    await act(async () => {
      fireEvent.click(within(detail()).getByRole('button', { name: 'Add Whisper Small to favorites' }));
    });
    expect(props.onToggleFavorite).toHaveBeenCalledWith('small', true);
    expect(within(row('small')).getByRole('button', { name: 'Remove Whisper Small from favorites' })).toBeInTheDocument();
  });
});
