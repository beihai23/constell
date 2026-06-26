import { useRef, useCallback, useEffect, useState } from 'react';
import { ArrowDown } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useParams } from 'react-router';
import { toast } from 'sonner';
import { useMessagesStore } from '@/stores/messagesStore';
import { useSyncStore } from '@/stores/syncStore';
import { useAuthStore } from '@/stores/authStore';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useMessageSend } from '@/hooks/useMessageSend';
import { MessageBubble } from './MessageBubble';
import { Skeleton } from '@/components/ui/skeleton';
import { Spinner } from '@/components/ui/spinner';
import { EmptyState, ErrorState } from '@/components/ui/state';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

// ---------------------------------------------------------------------------
// Unified message shape for rendering
// ---------------------------------------------------------------------------

interface RenderableMessage {
  id: string;
  data: ChannelMessage | DMMessage;
  isOwn: boolean;
}

type LoadState = 'loading' | 'ready' | 'error';

// ---------------------------------------------------------------------------
// MessageList — virtual-scrolled message list
// ---------------------------------------------------------------------------

export function MessageList() {
  const { channelId, peerId } = useParams();
  const client = useConstellClient();
  const user = useAuthStore((s) => s.user);
  const channelMessagesMap = useMessagesStore((s) => s.channelMessages);
  const dmMessagesMap = useMessagesStore((s) => s.dmMessages);
  const messageStatus = useMessagesStore((s) => s.messageStatus);
  const removeMessageStatus = useMessagesStore((s) => s.removeMessageStatus);
  const setChannelMessages = useMessagesStore((s) => s.setChannelMessages);
  const mergeChannelMessages = useMessagesStore((s) => s.mergeChannelMessages);
  const removeChannelMessage = useMessagesStore((s) => s.removeChannelMessage);
  const setDMMessages = useMessagesStore((s) => s.setDMMessages);
  const advanceDM = useSyncStore((s) => s.advanceDM);
  const advanceChannel = useSyncStore((s) => s.advanceChannel);

  // Retry handler for failed sends (MSG-SEND-3) — passed to each MessageBubble.
  const { retry } = useMessageSend({ channelId, peerId });

  // Current messages based on route
  const messages: RenderableMessage[] = peerId
    ? (dmMessagesMap.get(peerId) ?? []).map((m) => ({
        id: m.id,
        data: m,
        isOwn: m.senderId === user?.id,
      }))
    : channelId
      ? (channelMessagesMap.get(channelId) ?? []).map((m) => ({
          id: m.id,
          data: m,
          isOwn: m.authorId === user?.id,
        }))
      : [];

  // History load status: distinguishes "loading" from "empty" (MSG-HIST-1)
  // and surfaces fetch failure with a retry (MSG-HIST-4) instead of swallowing.
  const [loadState, setLoadState] = useState<LoadState>('loading');

  // Scroll container ref
  const scrollRef = useRef<HTMLDivElement>(null);

  // Track if user is near bottom (within 100px)
  const isNearBottomRef = useRef(true);
  const wasAtTopRef = useRef(false);

  // New-message pill (MSG-HIST-6): count of unseen messages accumulated while
  // the user was scrolled away from the bottom. Reset when they reach bottom.
  const [newCount, setNewCount] = useState(0);
  const prevLenRef = useRef(0);

  // Virtualizer — estimate 60px per row
  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 60,
    overscan: 10,
  });

  // Auto-scroll to bottom when new message arrives AND user is near bottom
  // On messages change: if near bottom, auto-scroll to the newest; otherwise
  // the user is reading history, so do NOT yank their position — instead tally
  // the newly-arrived messages into the "new messages" pill (MSG-HIST-6).
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const delta = messages.length - prevLenRef.current;
    prevLenRef.current = messages.length;
    if (messages.length === 0) return;
    if (isNearBottomRef.current) {
      requestAnimationFrame(() => {
        el.scrollTop = el.scrollHeight;
      });
    } else if (delta > 0) {
      setNewCount((c) => c + delta);
    }
  }, [messages.length]);

  // Scroll to the newest message and clear the pill.
  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    isNearBottomRef.current = true;
    setNewCount(0);
  }, []);

  // Load history on mount or route change. The backend returns pages in
  // DESC order (newest first) for cursor pagination, so reverse to ASC
  // (oldest at top, newest at bottom) for chat display. Factored out so the
  // ErrorState retry can re-run it.
  const loadHistory = useCallback(() => {
    if (!channelId && !peerId) return;
    setLoadState('loading');
    const finish = (result: { items: ChannelMessage[] } | { items: DMMessage[] }) => {
      const items = result.items as (ChannelMessage | DMMessage)[];
      const maxSeq = items.reduce((acc, m) => (m.seq > acc ? m.seq : acc), 0);
      return { items, maxSeq };
    };
    if (channelId) {
      client
        .getChannelHistory(channelId)
        .then((result) => {
          const { items, maxSeq } = finish(result);
          setChannelMessages(channelId, [...items].reverse() as ChannelMessage[]);
          // Seed the backfill cursor so the next useMessageSync tick fetches
          // only NEWER messages, not the whole page.
          if (maxSeq > 0) advanceChannel(channelId, maxSeq);
          setLoadState('ready');
        })
        .catch(() => setLoadState('error'));
      return;
    }
    if (peerId) {
      client
        .getDMHistory(peerId)
        .then((result) => {
          const { items, maxSeq } = finish(result);
          setDMMessages(peerId, [...items].reverse() as DMMessage[]);
          if (maxSeq > 0) advanceDM(peerId, maxSeq);
          setLoadState('ready');
        })
        .catch(() => setLoadState('error'));
    }
  }, [channelId, peerId, client, setChannelMessages, setDMMessages, advanceChannel, advanceDM]);

  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  // Loading-older indicator for top-of-list pagination (MSG-HIST-3).
  const [loadingMore, setLoadingMore] = useState(false);
  // Delete-confirm target (MSG-DEL-1): message id pending deletion.
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Edit handler (MSG-EDIT-1). Commits the edit via SDK, then merges the
  // updated message (new content + bumped updated_at → "(edited)") into the
  // store. Only own channel messages expose the affordance. Attachments are
  // preserved from the existing copy (the edit response omits them).
  const handleEdit = useCallback(
    async (messageId: string, content: string) => {
      if (!channelId) return;
      const updated = await client.editChannelMessage(channelId, messageId, content);
      const existing = channelMessagesMap.get(channelId)?.find((m) => m.id === messageId);
      const merged = existing && existing.attachments.length > 0
        ? { ...updated, attachments: existing.attachments }
        : updated;
      mergeChannelMessages(channelId, [merged]);
    },
    [channelId, client, mergeChannelMessages, channelMessagesMap],
  );

  const handleDelete = useCallback(async () => {
    if (!channelId || !deleteTargetId) return;
    setDeleting(true);
    try {
      await client.deleteChannelMessage(channelId, deleteTargetId);
      removeChannelMessage(channelId, deleteTargetId);
      removeMessageStatus(deleteTargetId);
      setDeleteTargetId(null);
      toast.success('Message deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete message');
    } finally {
      setDeleting(false);
    }
  }, [channelId, deleteTargetId, client, removeChannelMessage, removeMessageStatus]);

  // Load more when scrolled near top
  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;

    // Track bottom position; clear the new-message pill once the user reaches
    // the bottom (MSG-HIST-6).
    const wasNear = isNearBottomRef.current;
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    if (!wasNear && isNearBottomRef.current) {
      setNewCount(0);
    }

    // Load more when near top (within 200px) — only after initial load settled.
    if (el.scrollTop < 200 && !wasAtTopRef.current && loadState === 'ready') {
      wasAtTopRef.current = true;
      setLoadingMore(true);
      if (channelId) {
        client.getChannelHistory(channelId, { limit: 50 }).then((result) => {
          // Prepend older messages
          const existing = channelMessagesMap.get(channelId) ?? [];
          const existingIds = new Set(existing.map((m) => m.id));
          const older = result.items.filter((m) => !existingIds.has(m.id));
          if (older.length > 0) {
            setChannelMessages(channelId, [...older, ...existing]);
          }
        }).catch(() => {}).finally(() => {
          wasAtTopRef.current = false;
          setLoadingMore(false);
        });
      }
      if (peerId) {
        client.getDMHistory(peerId, { limit: 50 }).then((result) => {
          const existing = dmMessagesMap.get(peerId) ?? [];
          const existingIds = new Set(existing.map((m) => m.id));
          const older = result.items.filter((m) => !existingIds.has(m.id));
          if (older.length > 0) {
            setDMMessages(peerId, [...older, ...existing]);
          }
        }).catch(() => {}).finally(() => {
          wasAtTopRef.current = false;
          setLoadingMore(false);
        });
      }
    }
  }, [channelId, peerId, client, channelMessagesMap, dmMessagesMap, setChannelMessages, setDMMessages, loadState]);

  // Error: surface with retry instead of swallowing (MSG-HIST-4). Only when
  // we have NO messages — if backfill (useMessageSync) already merged some in
  // via a parallel request, the list is usable and we must not block on an
  // error banner for a request the user can already see past.
  if (loadState === 'error' && messages.length === 0) {
    return (
      <ErrorState
        title="Couldn't load messages"
        retry={loadHistory}
      />
    );
  }

  // Loading or empty — but only treat as empty when not loading (MSG-HIST-1).
  // If messages arrived (history or backfill), skip straight to the list.
  if (messages.length === 0) {
    if (loadState === 'loading') {
      return (
        <div className="flex-1 overflow-hidden px-4 py-2" aria-busy="true">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="flex gap-3 py-2">
              <Skeleton className="size-10 shrink-0 rounded-full" />
              <div className="flex-1 space-y-2 pt-1">
                <Skeleton className="h-3 w-28" />
                <Skeleton className="h-3 w-3/5" />
              </div>
            </div>
          ))}
        </div>
      );
    }
    return <EmptyState title="No messages yet" />;
  }

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto"
      >
        {loadingMore && (
          <div
            data-slot="pagination-loading"
            role="status"
            aria-live="polite"
            className="flex items-center justify-center gap-2 py-2 text-xs text-muted-foreground"
          >
            <Spinner className="size-3" />
            Loading older messages…
          </div>
        )}
        <div
          style={{
            height: virtualizer.getTotalSize(),
            width: '100%',
            position: 'relative',
          }}
        >
          {virtualizer.getVirtualItems().map((virtualRow) => {
            const msg = messages[virtualRow.index];
            return (
              <div
                key={msg.id}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualRow.start}px)`,
                }}
                data-index={virtualRow.index}
                ref={virtualizer.measureElement}
              >
                <MessageBubble
                  message={msg.data}
                  isOwn={msg.isOwn}
                  status={messageStatus.get(msg.id) ?? 'sent'}
                  onRetry={() => retry(msg.id)}
                  canDelete={!!channelId && msg.isOwn}
                  onDelete={() => setDeleteTargetId(msg.id)}
                  canEdit={!!channelId && msg.isOwn}
                  onEdit={(content) => handleEdit(msg.id, content)}
                />
              </div>
            );
          })}
        </div>
      </div>

      {/* "↓ N new" pill — shown when new messages arrived while scrolled up
          (MSG-HIST-6). Clicking returns to the bottom. */}
      {newCount > 0 && (
        <button
          data-slot="new-messages-pill"
          onClick={scrollToBottom}
          className="absolute bottom-4 left-1/2 z-10 flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground shadow-lg transition-all hover:bg-primary/90"
        >
          <ArrowDown className="size-3" />
          {newCount > 99 ? '99+' : newCount} new message{newCount === 1 ? '' : 's'}
        </button>
      )}

      {/* Delete confirmation (MSG-DEL-1) */}
      <Dialog open={deleteTargetId !== null} onOpenChange={(o) => { if (!o) setDeleteTargetId(null); }}>
        <DialogContent className="gap-4 rounded-xl border border-[#313244] bg-[#1e1e2e] p-5 text-[#cdd6f4] shadow-2xl sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="text-base font-semibold text-[#cdd6f4]">Delete message?</DialogTitle>
            <DialogDescription className="text-[#585b70]">
              This message will be removed. This can&apos;t be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleteTargetId(null)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
