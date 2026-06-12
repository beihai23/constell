import { useState, useMemo, useRef, useEffect } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import { cn } from '@/lib/utils';
import type { Channel } from '@constell/sdk-js';

/** Exposed so MainLayout can focus the input via ⌘K. */
export const searchInputRef = { current: null as HTMLInputElement | null };

/**
 * Middle column (240px). Displays channel list or DM list depending on
 * the current route. The search input filters the list inline.
 */
export function ChannelList() {
  const { communityId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const communities = useCommunitiesStore((s) => s.communities);
  const channels = useCommunitiesStore((s) => s.channels);
  const currentChannelId = useCommunitiesStore((s) => s.currentChannelId);
  const channelUnreads = useUnreadStore((s) => s.channelUnreads);

  const isDMView = location.pathname.startsWith('/@me');

  // Resolve current community
  const community = communityId ? communities.get(communityId) : undefined;
  const channelList = communityId ? (channels.get(communityId) ?? []) : [];

  // Inline search filter
  const [filter, setFilter] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // Expose ref for ⌘K
  useEffect(() => {
    searchInputRef.current = inputRef.current;
  });

  const filteredChannels = useMemo(
    () =>
      filter
        ? channelList.filter((ch) =>
            ch.name.toLowerCase().includes(filter.toLowerCase()),
          )
        : channelList,
    [channelList, filter],
  );

  return (
    <div className="flex w-60 shrink-0 flex-col bg-[#181825]">
      {/* Header */}
      <div className="flex h-12 items-center px-4 shadow-sm">
        <h2 className="truncate text-sm font-semibold text-[#cdd6f4]">
          {isDMView ? 'Direct Messages' : community?.name ?? 'Select a Community'}
        </h2>
      </div>

      {/* Inline search input */}
      <div className="px-2 py-2">
        <input
          ref={inputRef}
          type="text"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Search..."
          className="h-7 w-full rounded bg-[#11111b] px-2 text-xs text-[#cdd6f4] placeholder:text-[#585b70] outline-none focus:ring-1 focus:ring-[#585b70]"
        />
      </div>

      {/* Scrollable content */}
      <ScrollArea className="flex-1">
        {isDMView ? (
          <DMList filter={filter} />
        ) : communityId ? (
          <div className="px-2">
            <div className="mb-1 flex items-center px-1">
              <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                Text Channels
              </span>
            </div>
            {filteredChannels.map((channel) => (
              <ChannelItem
                key={channel.id}
                channel={channel}
                selected={currentChannelId === channel.id}
                unread={channelUnreads.get(channel.id) ?? 0}
                onClick={() => navigate(`/${communityId}/${channel.id}`)}
              />
            ))}
            {filteredChannels.length === 0 && channelList.length > 0 && (
              <p className="px-1 py-2 text-xs text-[#585b70]">No matching channels</p>
            )}
            {channelList.length === 0 && (
              <p className="px-1 py-2 text-xs text-[#585b70]">No channels yet</p>
            )}
          </div>
        ) : (
          <div className="flex items-center justify-center p-4">
            <p className="text-xs text-[#585b70]">Select a community or open DMs</p>
          </div>
        )}
      </ScrollArea>
    </div>
  );
}

// ---------------------------------------------------------------------------
// DMList (inline version with filter support)
// ---------------------------------------------------------------------------

function DMList({ filter }: { filter: string }) {
  const navigate = useNavigate();
  const location = useLocation();
  const dmMessages = useMessagesStore((s) => s.dmMessages);
  const dmUnreads = useUnreadStore((s) => s.dmUnreads);
  const onlineUsers = useUIStore((s) => s.onlineUsers);

  const conversations = useMemo(() => {
    const peerIds = Array.from(dmMessages.keys());
    const list: { peerId: string; lastContent: string; unread: number; online: boolean }[] =
      peerIds.map((peerId) => {
        const msgs = dmMessages.get(peerId) ?? [];
        const last = msgs[msgs.length - 1];
        return {
          peerId,
          lastContent: last ? truncate(last.content, 30) : '',
          unread: dmUnreads.get(peerId) ?? 0,
          online: onlineUsers.has(peerId),
        };
      });

    // Include unread-only peers not yet in dmMessages
    for (const [peerId, count] of dmUnreads) {
      if (!peerIds.includes(peerId)) {
        list.push({
          peerId,
          lastContent: '',
          unread: count,
          online: onlineUsers.has(peerId),
        });
      }
    }

    // Sort: unread first
    list.sort((a, b) => {
      if (a.unread > 0 && b.unread === 0) return -1;
      if (b.unread > 0 && a.unread === 0) return 1;
      return 0;
    });

    // Apply filter
    if (filter) {
      const q = filter.toLowerCase();
      return list.filter((c) => c.peerId.toLowerCase().includes(q));
    }
    return list;
  }, [dmMessages, dmUnreads, onlineUsers, filter]);

  const selectedPeerId = getPeerIdFromPath(location.pathname);

  if (conversations.length === 0) {
    return (
      <div className="flex items-center justify-center p-4">
        <p className="text-xs text-[#585b70]">
          {filter ? 'No matching conversations' : 'No conversations yet'}
        </p>
      </div>
    );
  }

  return (
    <div className="px-2 space-y-0.5">
      {conversations.map((conv) => (
        <button
          key={conv.peerId}
          onClick={() => navigate(`/@me/${conv.peerId}`)}
          className={cn(
            'flex w-full items-center gap-2 rounded px-2 py-1.5 transition-colors',
            selectedPeerId === conv.peerId
              ? 'bg-[#313244] text-[#cdd6f4]'
              : 'text-[#a6adc8] hover:bg-[#1e1e2e] hover:text-[#cdd6f4]',
            conv.unread > 0 && selectedPeerId !== conv.peerId && 'text-[#cdd6f4] font-semibold',
          )}
        >
          <div className="relative shrink-0">
            <Avatar size="sm">
              <AvatarFallback>
                {conv.peerId.charAt(0).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <span
              className={cn(
                'absolute right-0 bottom-0 h-2.5 w-2.5 rounded-full ring-2 ring-[#181825]',
                conv.online ? 'bg-[#a6e3a1]' : 'bg-[#585b70]',
              )}
            />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{conv.peerId}</p>
            {conv.lastContent && (
              <p className="truncate text-xs text-[#585b70]">{conv.lastContent}</p>
            )}
          </div>
          {conv.unread > 0 && selectedPeerId !== conv.peerId && (
            <span className="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-[#f38ba8] px-1 text-xs font-medium text-[#11111b]">
              {conv.unread > 99 ? '99+' : conv.unread}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// ChannelItem
// ---------------------------------------------------------------------------

interface ChannelItemProps {
  channel: Channel;
  selected: boolean;
  unread: number;
  onClick: () => void;
}

function ChannelItem({ channel, selected, unread, onClick }: ChannelItemProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-1.5 rounded px-2 py-1 text-sm transition-colors',
        selected
          ? 'bg-[#313244] text-[#cdd6f4]'
          : 'text-[#a6adc8] hover:bg-[#1e1e2e] hover:text-[#cdd6f4]',
        unread > 0 && !selected && 'text-[#cdd6f4] font-semibold',
      )}
    >
      <span className="text-[#585b70]">#</span>
      <span className="truncate">{channel.name}</span>
      {unread > 0 && !selected && (
        <span className="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-[#f38ba8] px-1 text-xs font-medium text-[#11111b]">
          {unread > 99 ? '99+' : unread}
        </span>
      )}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getPeerIdFromPath(pathname: string): string | null {
  const segments = pathname.split('/').filter(Boolean);
  if (segments.length >= 2 && segments[0] === '@me') {
    return segments[1];
  }
  return null;
}

function truncate(str: string, max: number): string {
  if (str.length <= max) return str;
  return str.slice(0, max) + '...';
}
