import { create } from 'zustand';
import { WSStatus } from '@constell/sdk-js';

type View = 'community' | 'dm';

interface UIState {
  view: View;
  showMemberList: boolean;
  onlineUsers: Set<string>;
  wsStatus: WSStatus;
  /** Channel the user is currently viewing (null when not in a channel). Used
   *  to suppress unread increments for messages the user is already seeing. */
  activeChannelId: string | null;
  /** DM peer the user is currently viewing (null otherwise). Same purpose. */
  activePeerId: string | null;
  setView: (view: View) => void;
  setShowMemberList: (show: boolean) => void;
  toggleMemberList: () => void;
  setOnline: (userId: string) => void;
  setOffline: (userId: string) => void;
  setWsStatus: (status: WSStatus) => void;
  setActiveChannel: (id: string | null) => void;
  setActivePeer: (id: string | null) => void;
  reset: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  view: 'community',
  showMemberList: false,
  onlineUsers: new Set(),
  wsStatus: WSStatus.Disconnected,
  activeChannelId: null,
  activePeerId: null,

  setView: (view) => set({ view }),
  setShowMemberList: (show) => set({ showMemberList: show }),
  toggleMemberList: () =>
    set((state) => ({ showMemberList: !state.showMemberList })),

  setOnline: (userId) =>
    set((state) => {
      const onlineUsers = new Set(state.onlineUsers);
      onlineUsers.add(userId);
      return { onlineUsers };
    }),

  setOffline: (userId) =>
    set((state) => {
      const onlineUsers = new Set(state.onlineUsers);
      onlineUsers.delete(userId);
      return { onlineUsers };
    }),

  setWsStatus: (wsStatus) => set({ wsStatus }),
  setActiveChannel: (activeChannelId) => set({ activeChannelId }),
  setActivePeer: (activePeerId) => set({ activePeerId }),

  reset: () =>
    set({
      view: 'community',
      showMemberList: false,
      onlineUsers: new Set(),
      wsStatus: WSStatus.Disconnected,
      activeChannelId: null,
      activePeerId: null,
    }),
}));
