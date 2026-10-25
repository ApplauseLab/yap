// Display labels for configured hotkey names (e.g. "rightoption", "f9").
// Keep the compact labels in sync with internal/hotkey/labels.go.

type Side = 'Left' | 'Right';

interface ModifierLabels {
  mac: string;
  windows: string;
  linux: string;
}

const modifiers: Record<string, ModifierLabels> = {
  alt: { mac: '⌥', windows: 'Alt', linux: 'Alt' },
  meta: { mac: '⌘', windows: 'Win', linux: 'Super' },
  ctrl: { mac: '⌃', windows: 'Ctrl', linux: 'Ctrl' },
  shift: { mac: '⇧', windows: 'Shift', linux: 'Shift' },
};

const modifierAliases: Record<string, string> = {
  option: 'alt',
  alt: 'alt',
  command: 'meta',
  cmd: 'meta',
  win: 'meta',
  super: 'meta',
  control: 'ctrl',
  ctrl: 'ctrl',
  shift: 'shift',
};

function parseModifier(key: string): { side: Side; modifier: ModifierLabels } | null {
  const match = key.match(/^(left|right)(.+)$/);
  if (!match) return null;
  const name = modifierAliases[match[2]];
  if (!name) return null;
  return { side: match[1] === 'left' ? 'Left' : 'Right', modifier: modifiers[name] };
}

function modifierLabel(modifier: ModifierLabels, platform: string): string {
  if (platform === 'darwin') return modifier.mac;
  if (platform === 'linux') return modifier.linux;
  return modifier.windows;
}

function arrowLabel(key: string): string | null {
  switch (key) {
    case 'left': case 'arrowleft': return '←';
    case 'right': case 'arrowright': return '→';
    case 'up': case 'arrowup': return '↑';
    case 'down': case 'arrowdown': return '↓';
    default: return null;
  }
}

/** Compact label for small badges, e.g. "⌥" on macOS or "Alt" on Windows. */
export function formatHotkeyShortLabel(keyName: string, platform: string): string {
  const k = keyName.trim().toLowerCase();
  const mac = platform === 'darwin';
  const modifier = parseModifier(k);
  if (modifier) return modifierLabel(modifier.modifier, platform);
  switch (k) {
    case '': return '';
    case 'capslock': return mac ? '⇪' : 'Caps';
    case 'fn': case 'function': return mac ? 'fn' : 'Fn';
    case 'escape': case 'esc': return 'Esc';
    case 'space': return 'Space';
    case 'tab': return mac ? '⇥' : 'Tab';
    case 'return': case 'enter': return mac ? '↩' : 'Enter';
    case 'backspace': return mac ? '⌫' : 'Bksp';
    case 'delete': return mac ? '⌫' : 'Del';
    case 'forwarddelete': return mac ? '⌦' : 'Del';
    default: return arrowLabel(k) ?? k.toUpperCase();
  }
}

/** Descriptive label for settings, e.g. "Right ⌥" on macOS or "Right Alt" on Windows. */
export function formatHotkeyLabel(keyName: string, platform: string): string {
  const k = keyName.trim().toLowerCase();
  const mac = platform === 'darwin';
  const modifier = parseModifier(k);
  if (modifier) return `${modifier.side} ${modifierLabel(modifier.modifier, platform)}`;
  switch (k) {
    case 'capslock': return 'Caps Lock';
    case 'fn': case 'function': return 'Fn';
    case 'escape': case 'esc': return 'Escape';
    case 'space': return 'Space';
    case 'tab': return 'Tab';
    case 'return': case 'enter': return mac ? 'Return' : 'Enter';
    case 'backspace': return 'Backspace';
    case 'delete': return 'Delete';
    case 'forwarddelete': return 'Forward Delete';
    default: return arrowLabel(k) ?? k.toUpperCase();
  }
}
