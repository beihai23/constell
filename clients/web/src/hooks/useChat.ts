import { useCallback } from 'react';
import { useConstellClient } from './useConstellClient';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import type { ChannelMessage } from '@constell/sdk-js';

/**
 * Provides chat actions and current channel/DM message selectors.
 */
export function useChat() {
  const client = useConstellClient();
  const currentChannelId = useCommunitiesStore((s) => s.currentChannelId);
  const currentCommunityId = useCommunitiesStore((s) => s.currentCommunityId);
  const setChannelMessages = useMessagesStore((s) => s.setChannelMessages);
  const setDMMessages = useMessagesStore((s) => s.setDMMessages);
  const clearUnread = useUnreadStore((s) => s.clearUnread);

  // Subscribe to the full map; derive current channel's messages from the
  // communitiesStore's currentChannelId (which lives in the hook closure).
  const channelMessagesMap = useMessagesStore((s) => s.channelMessages);
  const dmMessagesMap = useMessagesStore((s) => s.dmMessages);

  const channelMessages: ChannelMessage[] = currentChannelId
    ? (channelMessagesMap.get(currentChannelId) ?? [])
    : [];

  const loadChannelHistory = useCallback(
    async (channelId: string) => {
      const result = await client.getChannelHistory(channelId);
      setChannelMessages(channelId, result.items);
      clearUnread('channel', channelId);
    },
    [client, setChannelMessages, clearUnread],
  );

  const loadDMHistory = useCallback(
    async (peerId: string) => {
      const result = await client.getDMHistory(peerId);
      setDMMessages(peerId, result.items);
      clearUnread('dm', peerId);
    },
    [client, setDMMessages, clearUnread],
  );

  const sendChannelMessage = useCallback(
    (channelId: string, content: string, fileIds?: string[]) =>
      client.sendChannelMessage(channelId, content, fileIds),
    [client],
  );

  const sendDM = useCallback(
    (receiverId: string, content: string, fileIds?: string[]) =>
      client.sendDM(receiverId, content, fileIds),
    [client],
  );

  return {
    currentChannelId,
    currentCommunityId,
    channelMessages,
    channelMessagesMap,
    dmMessagesMap,
    loadChannelHistory,
    loadDMHistory,
    sendChannelMessage,
    sendDM,
  };
}
