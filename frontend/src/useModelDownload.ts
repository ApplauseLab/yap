import { useEffect, useState } from 'react';
import { GetModelDownloadState } from '../wailsjs/go/main/App';
import { EventsOn, LogError } from '../wailsjs/runtime/runtime';
import { IDLE_DOWNLOAD, ModelDownloadState } from './modelTypes';

export const MODEL_DOWNLOAD_EVENT = 'modelDownloadState';

// Tracks the backend's single model download. The initial fetch recovers state
// after navigation or remounting; events keep it current afterwards.
export function useModelDownload(): ModelDownloadState {
  const [state, setState] = useState<ModelDownloadState>(IDLE_DOWNLOAD);

  useEffect(() => {
    let disposed = false;
    let receivedEvent = false;

    const unsubscribe = EventsOn(MODEL_DOWNLOAD_EVENT, (next: ModelDownloadState) => {
      receivedEvent = true;
      if (!disposed) setState(next);
    });

    GetModelDownloadState()
      .then((initial) => {
        if (!disposed && !receivedEvent) setState(initial as ModelDownloadState);
      })
      .catch((err) => LogError(`[frontend] GetModelDownloadState failed: ${err}`));

    return () => {
      disposed = true;
      unsubscribe();
    };
  }, []);

  return state;
}
