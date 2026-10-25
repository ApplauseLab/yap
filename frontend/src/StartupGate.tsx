import { useEffect, useState, ReactNode } from 'react';
import { WaitForStartup } from '../wailsjs/go/main/App';

// Do not mount App's effects (or onboarding) while Go is creating managers.
export function StartupGate({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    WaitForStartup().then(() => {
      if (active) setReady(true);
    }).catch((err: unknown) => {
      if (active) setError(String(err));
    });
    return () => { active = false; };
  }, []);
  if (error) return <main role="alert">Yap could not start: {error}</main>;
  if (!ready) return <main role="status">Starting Yap…</main>;
  return <>{children}</>;
}
