import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUsersStore } from '@/stores/usersStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

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
      if (msg.authorId !== user?.id) {
        incrementUnread('channel', msg.channelId);
      }
    };

    const onDMReceived = (msg: DMMessage) => {
      appendDMMessage(msg.senderId, msg);
      incrementUnread('dm', msg.senderId);
    };

    const onUserOnline = ({ userId }: { userId: string }) => setOnline(userId);
    const onUserOffline = ({ userId }: { userId: string }) => setOffline(userId);

    const onConnected = () => setWsStatus('CONNECTED');
    const onDisconnected = () => setWsStatus('DISCONNECTED');

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
