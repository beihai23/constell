import { create } from 'zustand';

const STORAGE_KEY = 'constell.sync.seq';

interface PersistedSeq {
  dm: Record<string, number>;      // peerId -> last seen seq
  channel: Record<string, number>; // channelId -> last seen seq
}

function load(): PersistedSeq {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw) as PersistedSeq;
  } catch {
    // ignore corrupt storage
  }
  return { dm: {}, channel: {} };
}

function persist(s: PersistedSeq) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
  } catch {
    // ignore quota / private-mode errors
  }
}

interface SyncState {
  dm: Record<string, number>;
  channel: Record<string, number>;
  getDMSeq: (peerId: string) => number;
  getChannelSeq: (channelId: string) => number;
  advanceDM: (peerId: string, seq: number) => void;
  advanceChannel: (channelId: string, seq: number) => void;
}

const initial = load();

export const useSyncStore = create<SyncState>((set, get) => ({
  dm: initial.dm,
  channel: initial.channel,

  getDMSeq: (peerId) => get().dm[peerId] ?? 0,
  getChannelSeq: (channelId) => get().channel[channelId] ?? 0,

  advanceDM: (peerId, seq) =>
    set((state) => {
      if (seq <= (state.dm[peerId] ?? 0)) return state; // monotonic; never go backwards
      const dm = { ...state.dm, [peerId]: seq };
      persist({ dm, channel: state.channel });
      return { dm };
    }),

  advanceChannel: (channelId, seq) =>
    set((state) => {
      if (seq <= (state.channel[channelId] ?? 0)) return state;
      const channel = { ...state.channel, [channelId]: seq };
      persist({ dm: state.dm, channel });
      return { channel };
    }),
}));
