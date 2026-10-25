import { describe, expect, it } from 'vitest';
import { formatHotkeyLabel, formatHotkeyShortLabel } from './hotkeyLabels';
// Shared with internal/hotkey/labels_test.go so the native overlays match.
import shortLabelCases from '../../internal/hotkey/testdata/short_labels.json';

type Platform = 'darwin' | 'windows' | 'linux';

describe('formatHotkeyShortLabel', () => {
  it('has shared label cases', () => {
    expect(shortLabelCases.length).toBeGreaterThan(0);
  });

  for (const platform of ['darwin', 'windows', 'linux'] as Platform[]) {
    it(`matches the shared table on ${platform}`, () => {
      for (const c of shortLabelCases) {
        expect([c.key, formatHotkeyShortLabel(c.key, platform)]).toEqual([c.key, c[platform]]);
      }
    });
  }

  it('never shows macOS symbols on Windows', () => {
    for (const c of shortLabelCases) {
      expect(formatHotkeyShortLabel(c.key, 'windows')).not.toMatch(/[⌥⌘⌃⇧⇪⇥↩⌫⌦]/);
    }
  });
});

describe('formatHotkeyLabel', () => {
  it.each([
    ['rightoption', 'darwin', 'Right ⌥'],
    ['rightalt', 'windows', 'Right Alt'],
    ['leftalt', 'linux', 'Left Alt'],
    ['rightcommand', 'darwin', 'Right ⌘'],
    ['rightwin', 'windows', 'Right Win'],
    ['rightsuper', 'linux', 'Right Super'],
    ['leftctrl', 'darwin', 'Left ⌃'],
    ['leftctrl', 'windows', 'Left Ctrl'],
    ['rightshift', 'windows', 'Right Shift'],
    ['capslock', 'windows', 'Caps Lock'],
    ['fn', 'darwin', 'Fn'],
    ['escape', 'windows', 'Escape'],
    ['return', 'darwin', 'Return'],
    ['return', 'windows', 'Enter'],
    ['space', 'linux', 'Space'],
    ['arrowup', 'windows', '↑'],
    ['f9', 'windows', 'F9'],
    ['q', 'darwin', 'Q'],
  ])('%s on %s is %s', (key, platform, want) => {
    expect(formatHotkeyLabel(key, platform)).toBe(want);
  });
});
