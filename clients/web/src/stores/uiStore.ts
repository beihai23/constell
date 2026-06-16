import { create } from 'zustand';
import { WSStatus } from '@constell/sdk-js';

type View = 'community' | 'dm';

interface UIState {
  view: View;
  showMemberList: boolean;
  onlineUsers: Set<string>;
  wsStatus: WSStatus;
  setView: (view: View) => void;
  setShowMemberList: (show: boolean) => void;
  toggleMemberList: () => void;
  setOnline: (userId: string) => void;
  setOffline: (userId: string) => void;
  setWsStatus: (status: WSStatus) => void;
  reset: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  view: 'community',
  showMemberList: false,
  onlineUsers: new Set(),
  wsStatus: WSStatus.Disconnected,

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

  reset: () =>
    set({
      view: 'community',
      showMemberList: false,
      onlineUsers: new Set(),
      wsStatus: WSStatus.Disconnected,
    }),
}));
