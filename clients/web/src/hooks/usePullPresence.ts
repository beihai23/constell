import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useUIStore } from '@/stores/uiStore';

/**
 * Pull presence for a set of user IDs from the server (Redis = source of
 * truth) and reconcile the onlineUsers set in uiStore.
 *
 * Presence cannot be push-only: the set of "who needs to see user X's
 * status" is dynamic (search results, newly opened DMs, late-joining
 * members) and unknowable at connect time. So every view that displays a
 * user's online dot pulls it on demand here. The push events
 * (user_online/user_offline) remain as a real-time optimization for views
 * already rendered.
 *
 * Pass a stable array (e.g. derived from state via useMemo) — the effect
 * re-runs when the serialized ID list changes.
 */
export function usePullPresence(ids: string[]) {
  const client = useConstellClient();
  const setOnline = useUIStore((s) => s.setOnline);
  const setOffline = useUIStore((s) => s.setOffline);

  const key = ids.join(',');

  useEffect(() => {
    if (!key) return;
    const idList = key.split(',').filter(Boolean);
    if (idList.length === 0) return;

    let cancelled = false;
    client
      .getPresence(idList)
      .then((res) => {
        if (cancelled) return;
        for (const id of res.online) setOnline(id);
        for (const id of res.offline) setOffline(id);
      })
      .catch(() => {
        // Presence is non-critical — push events will still update over time.
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, key, setOnline, setOffline]);
}
