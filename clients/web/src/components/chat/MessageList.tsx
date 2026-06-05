import { useRef, useCallback, useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useParams } from 'react-router';
import { useMessagesStore } from '@/stores/messagesStore';
import { useAuthStore } from '@/stores/authStore';
import { useConstellClient } from '@/hooks/useConstellClient';
import { MessageBubble } from './MessageBubble';
import type { ChannelMessage, DMMessage } from '@constell/sdk-js';

// ---------------------------------------------------------------------------
// Unified message shape for rendering
// ---------------------------------------------------------------------------

interface RenderableMessage {
  id: string;
  data: ChannelMessage | DMMessage;
  isOwn: boolean;
}

// ---------------------------------------------------------------------------
// MessageList — virtual-scrolled message list
// ---------------------------------------------------------------------------

export function MessageList() {
  const { channelId, peerId } = useParams();
  const client = useConstellClient();
  const user = useAuthStore((s) => s.user);
  const channelMessagesMap = useMessagesStore((s) => s.channelMessages);
  const dmMessagesMap = useMessagesStore((s) => s.dmMessages);
  const setChannelMessages = useMessagesStore((s) => s.setChannelMessages);
  const setDMMessages = useMessagesStore((s) => s.setDMMessages);

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

  // Scroll container ref
  const scrollRef = useRef<HTMLDivElement>(null);

  // Track if user is near bottom (within 100px)
  const isNearBottomRef = useRef(true);
  const wasAtTopRef = useRef(false);

  // Virtualizer — estimate 60px per row
  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 60,
    overscan: 10,
  });

  // Auto-scroll to bottom when new message arrives AND user is near bottom
  useEffect(() => {
    if (!scrollRef.current) return;
    if (isNearBottomRef.current && messages.length > 0) {
      // Scroll to bottom after messages change
      const el = scrollRef.current;
      requestAnimationFrame(() => {
        el.scrollTop = el.scrollHeight;
      });
    }
  }, [messages.length]);

  // Load history on mount or route change
  useEffect(() => {
    if (channelId) {
      client.getChannelHistory(channelId).then((result) => {
        setChannelMessages(channelId, result.items);
      }).catch(() => {
        // Silently fail — empty state shown
      });
    }
    if (peerId) {
      client.getDMHistory(peerId).then((result) => {
        setDMMessages(peerId, result.items);
      }).catch(() => {
        // Silently fail
      });
    }
  }, [channelId, peerId, client, setChannelMessages, setDMMessages]);

  // Load more when scrolled near top
  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;

    // Track bottom position
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;

    // Load more when near top (within 200px)
    if (el.scrollTop < 200 && !wasAtTopRef.current) {
      wasAtTopRef.current = true;
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
        });
      }
    }
  }, [channelId, peerId, client, channelMessagesMap, dmMessagesMap, setChannelMessages, setDMMessages]);

  // Empty state
  if (messages.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm text-[#585b70]">No messages yet</p>
      </div>
    );
  }

  return (
    <div
      ref={scrollRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto"
    >
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
                status="sent"
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
