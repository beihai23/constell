import { create } from 'zustand';
import type { Community, Channel } from '@constell/sdk-js';

interface CommunitiesState {
  communities: Map<string, Community>;
  channels: Map<string, Channel[]>;
  currentCommunityId: string | null;
  currentChannelId: string | null;
  setCommunities: (communities: Community[]) => void;
  setChannels: (communityId: string, channels: Channel[]) => void;
  selectCommunity: (id: string | null) => void;
  selectChannel: (id: string | null) => void;
  reset: () => void;
}

export const useCommunitiesStore = create<CommunitiesState>((set) => ({
  communities: new Map(),
  channels: new Map(),
  currentCommunityId: null,
  currentChannelId: null,
  setCommunities: (list) =>
    set({ communities: new Map(list.map((c) => [c.id, c])) }),
  setChannels: (communityId, chs) =>
    set((state) => {
      const channels = new Map(state.channels);
      channels.set(communityId, chs);
      return { channels };
    }),
  selectCommunity: (id) => set({ currentCommunityId: id, currentChannelId: null }),
  selectChannel: (id) => set({ currentChannelId: id }),
  reset: () =>
    set({
      communities: new Map(),
      channels: new Map(),
      currentCommunityId: null,
      currentChannelId: null,
    }),
}));
