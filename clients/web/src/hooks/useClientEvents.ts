import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUsersStore } from '@/stores/usersStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { WSStatus, type ChannelMessage, type DMMessage } from '@constell/sdk-js';

/**
 * Subscribes to SDK client events and routes them into Zustand stores.
 * Call once at the app root level (inside ClientProvider).
 */
export function useClientEvents() {
  const client = useConstellClient();
  const user = useAuthStore((s) => s.user);
  const appendChannelMessage = useMessagesStore((s) => s.appendChannelMessage);
  const appendDMMessage = useMessagesStore((s) => s.appendDMMessage);
  const setUserProfile = useUsersStore((s) => s.setUser);
  const incrementUnread = useUnreadStore((s) => s.incrementUnread);
  const setOnline = useUIStore((s) => s.setOnline);
  const setOffline = useUIStore((s) => s.setOffline);
  const setWsStatus = useUIStore((s) => s.setWsStatus);

  useEffect(() => {
    // Seed the current user's profile into the users cache so their own
    // messages resolve to a nickname instantly (no fetch, no UUID flash).
    // Only seed when we actually have a nickname — a token-derived user may
    // carry an empty nickname, and seeding that would shadow a real fetch.
    if (user?.nickname) setUserProfile(user);

    const onChannelMessage = (msg: ChannelMessage) => {
      appendChannelMessage(msg.channelId, msg);
      // Only tally unread for messages the user isn't already looking at:
      // own messages, and messages in the channel currently being viewed,
      // don't increment (UNREAD-1). Read via getState() to avoid a stale
      // closure across re-subscribes.
      const activeChannelId = useUIStore.getState().activeChannelId;
      if (msg.authorId !== user?.id && msg.channelId !== activeChannelId) {
        incrementUnread('channel', msg.channelId);
      } else if (msg.channelId === activeChannelId && msg.authorId !== user?.id) {
        // Keep the SERVER in sync: the user is viewing this channel, so tell
        // the server the message is read (otherwise its unread count drifts
        // and overwrites the correct local 0 on next load). Fire-and-forget.
        client.markChannelRead(msg.channelId).catch(() => {});
      }
    };

    const onDMReceived = (msg: DMMessage) => {
      appendDMMessage(msg.senderId, msg);
      const activePeerId = useUIStore.getState().activePeerId;
      if (msg.senderId !== activePeerId) {
        incrementUnread('dm', msg.senderId);
      }
    };

    const onUserOnline = ({ userId }: { userId: string }) => setOnline(userId);
    const onUserOffline = ({ userId }: { userId: string }) => setOffline(userId);

    const onConnected = () => setWsStatus(WSStatus.Connected);
    const onDisconnected = () => setWsStatus(WSStatus.Disconnected);

    client.on('channel_message', onChannelMessage);
    client.on('dm_received', onDMReceived);
    client.on('user_online', onUserOnline);
    client.on('user_offline', onUserOffline);
    client.on('connected', onConnected);
    client.on('disconnected', onDisconnected);

    return () => {
      client.off('channel_message', onChannelMessage);
      client.off('dm_received', onDMReceived);
      client.off('user_online', onUserOnline);
      client.off('user_offline', onUserOffline);
      client.off('connected', onConnected);
      client.off('disconnected', onDisconnected);
    };
  }, [client, user, user?.id, appendChannelMessage, appendDMMessage, setUserProfile, incrementUnread, setOnline, setOffline, setWsStatus]);
}
