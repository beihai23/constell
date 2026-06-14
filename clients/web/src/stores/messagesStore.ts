import { create } from 'zustand';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

type SendStatus = 'sending' | 'sent' | 'failed';

/**
 * Sort key for a message. Real messages carry a positive server-assigned seq
 * (BIGINT IDENTITY starts at 1). A live-pushed message arrives with seq: 0
 * (the WS push payload doesn't carry seq — REST is authoritative), and an
 * optimistic pre-ack message has no seq yet. Both must sort to the END
 * (newest position), not the front — so any falsy/≤0 seq maps to MAX.
 */
const seqOr = (m: { seq?: number }) => (m.seq && m.seq > 0 ? m.seq : Number.MAX_SAFE_INTEGER);

/** Merge incoming into existing, dedup by id, sort by seq ascending. */
function mergeByIdSeq<T extends { id: string; seq?: number }>(existing: T[], incoming: T[]): T[] {
  const byId = new Map<string, T>();
  for (const m of existing) byId.set(m.id, m);
  for (const m of incoming) byId.set(m.id, m); // overwrite with freshest
  return [...byId.values()].sort((a, b) => seqOr(a) - seqOr(b));
}

interface MessagesState {
  channelMessages: Map<string, ChannelMessage[]>;
  dmMessages: Map<string, DMMessage[]>;
  messageStatus: Map<string, SendStatus>;
  setChannelMessages: (channelId: string, messages: ChannelMessage[]) => void;
  appendChannelMessage: (channelId: string, message: ChannelMessage) => void;
  mergeChannelMessages: (channelId: string, incoming: ChannelMessage[]) => void;
  setDMMessages: (peerId: string, messages: DMMessage[]) => void;
  appendDMMessage: (peerId: string, message: DMMessage) => void;
  mergeDMMessages: (peerId: string, incoming: DMMessage[]) => void;
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
      channelMessages.set(channelId, [...messages].sort((a, b) => seqOr(a) - seqOr(b)));
      return { channelMessages };
    }),

  // Idempotent: a duplicate id (from backfill + live push overlap) is a no-op.
  appendChannelMessage: (channelId, message) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, mergeByIdSeq(existing, [message]));
      return { channelMessages };
    }),

  mergeChannelMessages: (channelId, incoming) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, mergeByIdSeq(existing, incoming));
      return { channelMessages };
    }),

  setDMMessages: (peerId, messages) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      dmMessages.set(peerId, [...messages].sort((a, b) => seqOr(a) - seqOr(b)));
      return { dmMessages };
    }),

  appendDMMessage: (peerId, message) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, mergeByIdSeq(existing, [message]));
      return { dmMessages };
    }),

  mergeDMMessages: (peerId, incoming) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, mergeByIdSeq(existing, incoming));
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
    set({ channelMessages: new Map(), dmMessages: new Map(), messageStatus: new Map() }),
}));
