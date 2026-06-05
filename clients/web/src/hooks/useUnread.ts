import { useCallback } from 'react';
import { useConstellClient } from './useConstellClient';
import { useUnreadStore } from '@/stores/unreadStore';

/**
 * Provides unread count queries and mark-read actions.
 */
export function useUnread() {
  const client = useConstellClient();
  const dmUnreads = useUnreadStore((s) => s.dmUnreads);
  const channelUnreads = useUnreadStore((s) => s.channelUnreads);
  const setUnreads = useUnreadStore((s) => s.setUnreads);
  const clearUnread = useUnreadStore((s) => s.clearUnread);

  const getDMUnreadCount = useCallback(
    (conversationId: string) => dmUnreads.get(conversationId) ?? 0,
    [dmUnreads],
  );

  const getChannelUnreadCount = useCallback(
    (channelId: string) => channelUnreads.get(channelId) ?? 0,
    [channelUnreads],
  );

  const totalDMUnreads = useCallback(() => {
    let total = 0;
    for (const count of dmUnreads.values()) total += count;
    return total;
  }, [dmUnreads]);

  const totalChannelUnreads = useCallback(() => {
    let total = 0;
    for (const count of channelUnreads.values()) total += count;
    return total;
  }, [channelUnreads]);

  const refreshUnreads = useCallback(async () => {
    const data = await client.getUnreadCounts();
    setUnreads(data);
  }, [client, setUnreads]);

  const markDMRead = useCallback(
    async (conversationId: string) => {
      await client.markDMRead(conversationId);
      clearUnread('dm', conversationId);
    },
    [client, clearUnread],
  );

  const markChannelRead = useCallback(
    async (channelId: string) => {
      await client.markChannelRead(channelId);
      clearUnread('channel', channelId);
    },
    [client, clearUnread],
  );

  return {
    dmUnreads,
    channelUnreads,
    getDMUnreadCount,
    getChannelUnreadCount,
    totalDMUnreads,
    totalChannelUnreads,
    refreshUnreads,
    markDMRead,
    markChannelRead,
  };
}
