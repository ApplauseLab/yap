type Callback = (...data: any[]) => void;

const listeners = new Map<string, Set<Callback>>();

export function EventsOn(eventName: string, callback: Callback): () => void {
  if (!listeners.has(eventName)) listeners.set(eventName, new Set());
  const wrapped: Callback = (...data) => callback(...data);
  listeners.get(eventName)!.add(wrapped);
  return () => {
    listeners.get(eventName)?.delete(wrapped);
  };
}

export function EventsEmit(eventName: string, ...data: any[]): void {
  listeners.get(eventName)?.forEach((cb) => cb(...data));
}

export function listenerCount(eventName: string): number {
  return listeners.get(eventName)?.size ?? 0;
}

export function resetListeners(): void {
  listeners.clear();
}

export const LogInfo = (_message: string) => {};
export const LogError = (_message: string) => {};
export const LogDebug = (_message: string) => {};
export const LogWarning = (_message: string) => {};
