import { create } from 'zustand';
import type { WSStatus } from '@constell/sdk-js';

type View = 'community' | 'dm';

interface UIState {
  view: View;
  showMemberList: boolean;
  showSearch: boolean;
  onlineUsers: Set<string>;
  wsStatus: WSStatus;
  setView: (view: View) => void;
  setShowMemberList: (show: boolean) => void;
  toggleMemberList: () => void;
  setShowSearch: (show: boolean) => void;
  setOnline: (userId: string) => void;
  setOffline: (userId: string) => void;
  setWsStatus: (status: WSStatus) => void;
  reset: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  view: 'community',
  showMemberList: false,
  showSearch: false,
  onlineUsers: new Set(),
  wsStatus: 'DISCONNECTED',

  setView: (view) => set({ view }),
  setShowMemberList: (show) => set({ showMemberList: show }),
  toggleMemberList: () =>
    set((state) => ({ showMemberList: !state.showMemberList })),
  setShowSearch: (show) => set({ showSearch: show }),

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

  reset: () =>
    set({
      view: 'community',
      showMemberList: false,
      showSearch: false,
      onlineUsers: new Set(),
      wsStatus: 'DISCONNECTED',
    }),
}));
