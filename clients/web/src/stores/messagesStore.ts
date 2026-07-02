import { create } from 'zustand';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

type SendStatus = 'sending' | 'sent' | 'failed';

/**
 * A message carries a server-assigned `seq` (BIGINT IDENTITY — monotonic and
 * unique per insert) once acknowledged: in REST history/backfill, in the live
 * WS push (the push carries seq), and after optimistic→real reconciliation.
 * Only a brand-new optimistic message (pre-ACK) lacks seq.
 */
const hasSeq = (m: { seq?: number }): m is { seq: number } => !!m.seq && m.seq > 0;
// A seq-less (optimistic, pre-ACK) message was just sent → sort as newest.
const seqOr = (m: { seq?: number }) => (hasSeq(m) ? m.seq : Number.MAX_SAFE_INTEGER);

/**
 * Authoritative message order: by `seq` ascending, with createdAt only as a
 * tiebreaker for same-seq collisions / a fallback between two seq-less
 * optimistic messages.
 *
 * seq is the only reliable total order here. createdAt is NOT: the server
 * emits it as Unix SECONDS (community-service `m.CreatedAt.Unix()`), while
 * optimistic messages use `Date.now()` MILLISECONDS (ChatInput) and the SDK
 * contract claims milliseconds — so the store holds a 1000× mix of seconds and
 * milliseconds. Sorting by createdAt scrambles adjacent messages whenever an
 * optimistic (ms) message coexists with server (seconds) messages — the "wrong
 * order" bug. (This is the second pass at it: an earlier createdAt-primary sort
 * replaced a seq sort over a now-obsolete "push omits seq" worry. The push
 * carries seq today, so seq is safe and is the correct primary key.)
 */
export const byChrono = (a: { createdAt?: number; seq?: number }, b: { createdAt?: number; seq?: number }) =>
  seqOr(a) - seqOr(b) || (a.createdAt ?? 0) - (b.createdAt ?? 0);

/**
 * Assign a local "high-water" seq to an optimistic (seq-less) message so it
 * sorts into its true position instead of being flung to the end of the list.
 *
 * local seq = max(seq already in the list — including earlier optimistic locals)
 * + 1. In the common (no-concurrency) case the server assigns the message the
 * very next seq, so on ACK the real seq equals this local value and the message
 * does not move. If a newer real message arrives during the ACK round-trip its
 * seq is higher, so it still sorts AFTER the optimistic → no misorder. (The only
 * residual is a sub-RTT concurrency burst where other users grab the intervening
 * seqs; that small jump self-heals on ACK.)
 *
 * Real messages (seq > 0) are returned unchanged. Exported for unit tests.
 */
export function withLocalSeq<T extends { seq?: number }>(existing: { seq?: number }[], m: T): T {
  if (hasSeq(m)) return m;
  let max = 0;
  for (const x of existing) if (hasSeq(x) && x.seq > max) max = x.seq;
  return { ...m, seq: max + 1 };
}

/**
 * Merge incoming into existing, dedup by id, sort by seq ascending.
 *
 * A seq-less update (realtime push with seq: 0, or an optimistic pre-ack)
 * must NOT overwrite a message that already carries a real server seq — that
 * would zero its sort key and fling it to the bottom of the list, which is
 * what makes the displayed order reshuffle between renders.
 */
function mergeByIdSeq<T extends { id: string; seq?: number }>(existing: T[], incoming: T[]): T[] {
  const byId = new Map<string, T>();
  for (const m of existing) byId.set(m.id, m);
  for (const m of incoming) {
    const cur = byId.get(m.id);
    if (cur && hasSeq(cur) && !hasSeq(m)) continue; // keep the seq'd copy
    byId.set(m.id, m); // overwrite with freshest
  }
  return [...byId.values()].sort(byChrono);
}

interface MessagesState {
  channelMessages: Map<string, ChannelMessage[]>;
  dmMessages: Map<string, DMMessage[]>;
  messageStatus: Map<string, SendStatus>;
  setChannelMessages: (channelId: string, messages: ChannelMessage[]) => void;
  appendChannelMessage: (channelId: string, message: ChannelMessage) => void;
  mergeChannelMessages: (channelId: string, incoming: ChannelMessage[]) => void;
  removeChannelMessage: (channelId: string, id: string) => void;
  setDMMessages: (peerId: string, messages: DMMessage[]) => void;
  appendDMMessage: (peerId: string, message: DMMessage) => void;
  mergeDMMessages: (peerId: string, incoming: DMMessage[]) => void;
  removeDMMessage: (peerId: string, id: string) => void;
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
      channelMessages.set(channelId, [...messages].sort(byChrono));
      return { channelMessages };
    }),

  // Idempotent: a duplicate id (from backfill + live push overlap) is a no-op.
  // An optimistic (seq-less) message gets a local high-water seq (withLocalSeq)
  // so it lands in its true position instead of the end of the list.
  appendChannelMessage: (channelId, message) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, mergeByIdSeq(existing, [withLocalSeq(existing, message)]));
      return { channelMessages };
    }),

  mergeChannelMessages: (channelId, incoming) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, mergeByIdSeq(existing, incoming));
      return { channelMessages };
    }),

  removeChannelMessage: (channelId, id) =>
    set((state) => {
      const channelMessages = new Map(state.channelMessages);
      const existing = channelMessages.get(channelId) ?? [];
      channelMessages.set(channelId, existing.filter((m) => m.id !== id));
      return { channelMessages };
    }),

  setDMMessages: (peerId, messages) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      dmMessages.set(peerId, [...messages].sort(byChrono));
      return { dmMessages };
    }),

  appendDMMessage: (peerId, message) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, mergeByIdSeq(existing, [withLocalSeq(existing, message)]));
      return { dmMessages };
    }),

  mergeDMMessages: (peerId, incoming) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, mergeByIdSeq(existing, incoming));
      return { dmMessages };
    }),

  removeDMMessage: (peerId, id) =>
    set((state) => {
      const dmMessages = new Map(state.dmMessages);
      const existing = dmMessages.get(peerId) ?? [];
      dmMessages.set(peerId, existing.filter((m) => m.id !== id));
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
