import { create } from 'zustand';
import type { UnreadCounts } from '@constell/sdk-js';

type UnreadType = 'dm' | 'channel';

interface UnreadState {
  dmUnreads: Map<string, number>;
  channelUnreads: Map<string, number>;
  setUnreads: (data: UnreadCounts) => void;
  incrementUnread: (type: UnreadType, id: string) => void;
  clearUnread: (type: UnreadType, id: string) => void;
  reset: () => void;
}

export const useUnreadStore = create<UnreadState>((set) => ({
  dmUnreads: new Map(),
  channelUnreads: new Map(),

  setUnreads: (data) =>
    set({
      dmUnreads: new Map(
        data.dmConversations.map((c) => [c.conversationId, c.count]),
      ),
      channelUnreads: new Map(
        data.channels.map((c) => [c.channelId, c.count]),
      ),
    }),

  incrementUnread: (type, id) =>
    set((state) => {
      if (type === 'dm') {
        const dmUnreads = new Map(state.dmUnreads);
        dmUnreads.set(id, (dmUnreads.get(id) ?? 0) + 1);
        return { dmUnreads };
      }
      const channelUnreads = new Map(state.channelUnreads);
      channelUnreads.set(id, (channelUnreads.get(id) ?? 0) + 1);
      return { channelUnreads };
    }),

  clearUnread: (type, id) =>
    set((state) => {
      if (type === 'dm') {
        const dmUnreads = new Map(state.dmUnreads);
        dmUnreads.delete(id);
        return { dmUnreads };
      }
      const channelUnreads = new Map(state.channelUnreads);
      channelUnreads.delete(id);
      return { channelUnreads };
    }),

  reset: () => set({ dmUnreads: new Map(), channelUnreads: new Map() }),
}));
