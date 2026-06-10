import { create } from 'zustand';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

type SendStatus = 'sending' | 'sent' | 'failed';

interface MessagesState {
  channelMessages: Map<string, ChannelMessage[]>;
  dmMessages: Map<string, DMMessage[]>;
  /** Tracks optimistic send status for temp messages keyed by tempId/messageId. */
  messageStatus: Map<string, SendStatus>;
  setChannelMessages: (channelId: string, messages: ChannelMessage[]) => void;
  appendChannelMessage: (channelId: string, message: ChannelMessage) => void;
  setDMMessages: (peerId: string, messages: DMMessage[]) => void;
  appendDMMessage: (peerId: string, message: DMMessage) => void;
  setMessageStatus: (id: string, status: SendStatus) => void;
  removeMessageStatus: (id: string) => void;
  clearMessages: () => void;
}

export const useMessagesStore = create<MessagesState>((set) => ({
  channelMessages: new Map(),
  dmMessages: new Map(),
  messageStatus: new Map(),

  setChannelMessages: (channelId, messages) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      channelMessages.set(channelId, messages);
      return { channelMessages };
    }),

  appendChannelMessage: (channelId, message) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, [...existing, message]);
      return { channelMessages };
    }),

  setDMMessages: (peerId, messages) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      dmMessages.set(peerId, messages);
      return { dmMessages };
    }),

  appendDMMessage: (peerId, message) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, [...existing, message]);
      return { dmMessages };
    }),

  setMessageStatus: (id, status) =>
    set((state) => {
      const messageStatus = new Map(state.messageStatus);
      messageStatus.set(id, status);
      return { messageStatus };
    }),

  removeMessageStatus: (id) =>
    set((state) => {
      const messageStatus = new Map(state.messageStatus);
      messageStatus.delete(id);
      return { messageStatus };
    }),

  clearMessages: () =>
    set({
      channelMessages: new Map(),
      dmMessages: new Map(),
      messageStatus: new Map(),
    }),
}));
