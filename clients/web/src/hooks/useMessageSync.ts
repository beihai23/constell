import { useCallback, useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useMessagesStore } from '@/stores/messagesStore';
import { useSyncStore } from '@/stores/syncStore';

const POLL_INTERVAL_MS = 30_000;
const BACKFILL_LIMIT = 200;

/**
 * Guarantees the client view converges to server truth regardless of
 * push loss. Backfills every loaded DM/channel scope:
 *   - immediately on WebSocket (re)connect
 *   - when the tab becomes visible
 *   - every POLL_INTERVAL_MS while mounted
 *
 * Reads store state via getState() inside the async body so it always
 * sees the latest scopes/cursors (no stale-closure drift).
 */
export function useMessageSync() {
  const client = useConstellClient();

  const backfillAll = useCallback(async () => {
    const { dmMessages, mergeDMMessages, channelMessages, mergeChannelMessages } =
      useMessagesStore.getState();
    const sync = useSyncStore.getState();

    // DM scopes currently loaded in the client.
    for (const peerId of dmMessages.keys()) {
      const since = sync.getDMSeq(peerId);
      try {
        const res = await client.getDMHistory(peerId, { sinceSeq: since, limit: BACKFILL_LIMIT });
        mergeDMMessages(peerId, res.items);
        // Advance once with the batch max instead of per-item: advanceDM is
        // monotonic, so a single call with max == the same result as N calls,
        // but writes localStorage once instead of N times on a catch-up batch.
        const max = res.items.reduce((acc, m) => (m.seq > acc ? m.seq : acc), since);
        if (max > since) sync.advanceDM(peerId, max);
      } catch (err) {
        // Transient failure (network) — next tick retries. Log so a genuine
        // bug (malformed response, bad shape) isn't silently swallowed.
        console.warn('backfill DM failed', peerId, err);
      }
    }

    // Channel scopes currently loaded.
    for (const channelId of channelMessages.keys()) {
      const since = sync.getChannelSeq(channelId);
      try {
        const res = await client.getChannelHistory(channelId, { sinceSeq: since, limit: BACKFILL_LIMIT });
        mergeChannelMessages(channelId, res.items);
        const max = res.items.reduce((acc, m) => (m.seq > acc ? m.seq : acc), since);
        if (max > since) sync.advanceChannel(channelId, max);
      } catch (err) {
        console.warn('backfill channel failed', channelId, err);
      }
    }
  }, [client]);

  useEffect(() => {
    const onConnected = () => { void backfillAll(); };
    const onVisible = () => {
      if (document.visibilityState === 'visible') void backfillAll();
    };

    client.on('connected', onConnected);
    document.addEventListener('visibilitychange', onVisible);
    const timer = window.setInterval(() => void backfillAll(), POLL_INTERVAL_MS);

    return () => {
      client.off('connected', onConnected);
      document.removeEventListener('visibilitychange', onVisible);
      window.clearInterval(timer);
    };
  }, [client, backfillAll]);
}
