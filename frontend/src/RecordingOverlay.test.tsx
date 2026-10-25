import { fireEvent, render, screen } from '@testing-library/react';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { RecordingOverlay } from './RecordingOverlay';
import { formatHotkeyShortLabel } from './hotkeyLabels';

beforeAll(() => {
  // jsdom has no canvas; the overlay skips drawing when there is no context.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});

function renderOverlay(stopKeyLabel: string, cancelKeyLabel: string) {
  const onStop = vi.fn();
  const onCancel = vi.fn();
  render(
    <RecordingOverlay
      isRecording
      onStop={onStop}
      onCancel={onCancel}
      stopKeyLabel={stopKeyLabel}
      cancelKeyLabel={cancelKeyLabel}
    />,
  );
  return {
    onStop,
    onCancel,
    stop: screen.getByRole('button', { name: /^Stop/ }),
    cancel: screen.getByRole('button', { name: /^Cancel/ }),
  };
}

describe('RecordingOverlay', () => {
  it('shows the configured Windows hotkeys instead of macOS symbols', () => {
    const { stop, cancel } = renderOverlay(
      formatHotkeyShortLabel('rightalt', 'windows'),
      formatHotkeyShortLabel('escape', 'windows'),
    );
    expect(stop.querySelector('kbd')).toHaveTextContent(/^Alt$/);
    expect(cancel.querySelector('kbd')).toHaveTextContent(/^Esc$/);
    expect(document.body).not.toHaveTextContent('⌥');
  });

  it('shows Right Command as ⌘ on macOS', () => {
    const { stop } = renderOverlay(
      formatHotkeyShortLabel('rightcommand', 'darwin'),
      formatHotkeyShortLabel('escape', 'darwin'),
    );
    expect(stop.querySelector('kbd')).toHaveTextContent(/^⌘$/);
    expect(document.body).not.toHaveTextContent('⌥');
  });

  it('shows function-key hotkeys', () => {
    const { stop, cancel } = renderOverlay('F9', 'F10');
    expect(stop).toHaveTextContent('StopF9');
    expect(cancel).toHaveTextContent('CancelF10');
  });

  it('omits the key badge when no label is known', () => {
    const { stop } = renderOverlay('', 'Esc');
    expect(stop.querySelector('kbd')).toBeNull();
  });

  it('still wires the Stop and Cancel buttons', () => {
    const { stop, cancel, onStop, onCancel } = renderOverlay('Alt', 'Esc');
    fireEvent.click(stop);
    fireEvent.click(cancel);
    expect(onStop).toHaveBeenCalledOnce();
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it('renders nothing when not recording', () => {
    const { container } = render(
      <RecordingOverlay isRecording={false} onStop={() => {}} onCancel={() => {}} stopKeyLabel="Alt" cancelKeyLabel="Esc" />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
