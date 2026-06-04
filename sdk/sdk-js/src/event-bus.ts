/**
 * EventBus — generic typed event emitter.
 *
 * Used internally by ConstellClient to dispatch gateway events
 * (dm_received, channel_message, user_online, …) to consumer handlers.
 */

export type EventHandler = (...args: any[]) => void;

export interface EventMap {
  [key: string]: EventHandler;
}

export class EventBus<T extends EventMap> {
  private handlers = new Map<keyof T, Set<EventHandler>>();

  /** Register a handler for `event`. Multiple handlers per event are supported. */
  on<K extends keyof T>(event: K, handler: T[K]): void {
    let set = this.handlers.get(event);
    if (!set) {
      set = new Set();
      this.handlers.set(event, set);
    }
    set.add(handler);
  }

  /** Remove a previously registered handler for `event`. */
  off<K extends keyof T>(event: K, handler: T[K]): void {
    this.handlers.get(event)?.delete(handler);
  }

  /**
   * Emit `event` with the given arguments.
   *
   * Each handler is called independently — if one throws, the error is
   * silently swallowed so remaining handlers still execute.
   */
  emit<K extends keyof T>(event: K, ...args: Parameters<T[K]>): void {
    const set = this.handlers.get(event);
    if (!set) return;
    for (const handler of set) {
      try {
        handler(...args);
      } catch {
        // swallow — don't break other handlers
      }
    }
  }

  /** Remove every handler for every event. */
  removeAllListeners(): void {
    this.handlers.clear();
  }
}
