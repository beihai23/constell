import { create } from 'zustand';
import type { Community, Channel } from '@constell/sdk-js';

interface CommunitiesState {
  communities: Map<string, Community>;
  channels: Map<string, Channel[]>;
  currentCommunityId: string | null;
  currentChannelId: string | null;
  setCommunities: (communities: Community[]) => void;
  addCommunity: (community: Community) => void;
  setChannels: (communityId: string, channels: Channel[]) => void;
  addChannel: (communityId: string, channel: Channel) => void;
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
  addCommunity: (community) =>
    set((state) => {
      const communities = new Map(state.communities);
      communities.set(community.id, community);
      return { communities };
    }),
  setChannels: (communityId, chs) =>
    set((state) => {
      const channels = new Map(state.channels);
      channels.set(communityId, chs);
      return { channels };
    }),
  addChannel: (communityId, channel) =>
    set((state) => {
      const channels = new Map(state.channels);
      const existing = channels.get(communityId) ?? [];
      // Avoid duplicates if the channel was pushed via WS before the REST
      // response arrives.
      if (existing.some((c) => c.id === channel.id)) return {};
      channels.set(communityId, [...existing, channel]);
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
