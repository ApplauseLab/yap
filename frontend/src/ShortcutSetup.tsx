import { useState } from 'react';
import { main } from '../wailsjs/go/models';
import './ShortcutSetup.css';

export type ExternalShortcuts = main.ExternalShortcuts;

export const NO_EXTERNAL_SHORTCUTS: ExternalShortcuts = { required: false, configFile: '', snippet: '' };

// Wayland compositors own global shortcuts, so Yap shows what to bind instead
// of capturing a key.
export function ShortcutSetup({ shortcuts }: { shortcuts: ExternalShortcuts }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(shortcuts.snippet);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // The snippet stays selectable when the clipboard is unavailable.
    }
  };

  return (
    <div className="shortcut-setup">
      <p>
        {shortcuts.configFile
          ? <>On Hyprland, shortcuts are bound in your config. Add these lines to <code>{shortcuts.configFile}</code> and change the keys to taste:</>
          : <>On Wayland, shortcuts are bound in your desktop's keyboard settings. Bind keys to these commands to start/stop and to cancel a recording:</>}
      </p>
      <div className="shortcut-setup-snippet">
        <pre aria-label="Shortcut commands">{shortcuts.snippet}</pre>
        <button type="button" className="shortcut-setup-copy" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
    </div>
  );
}
