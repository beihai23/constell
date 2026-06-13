import { useState, useMemo, useRef, useEffect, useCallback } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useResolvedUser } from '@/hooks/useResolvedUser';
import { usePullPresence } from '@/hooks/usePullPresence';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import { cn } from '@/lib/utils';
import type {
  Channel,
  SearchResults,
  UserSearchResult,
  MessageSearchResult,
  DMMessageSearchResult,
} from '@constell/sdk-js';

/** Exposed so MainLayout can focus the input via ⌘K. */
export const searchInputRef = { current: null as HTMLInputElement | null };

/**
 * Middle column (240px). Displays channel list or DM list depending on
 * the current route. The search input filters locally as you type;
 * pressing Enter calls the search API and shows results inline.
 */
export function ChannelList() {
  const { communityId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const client = useConstellClient();
  const communities = useCommunitiesStore((s) => s.communities);
  const channels = useCommunitiesStore((s) => s.channels);
  const currentChannelId = useCommunitiesStore((s) => s.currentChannelId);
  const channelUnreads = useUnreadStore((s) => s.channelUnreads);

  const isDMView = location.pathname.startsWith('/@me');

  // Resolve current community
  const community = communityId ? communities.get(communityId) : undefined;
  const channelList = communityId ? (channels.get(communityId) ?? []) : [];

  // Inline search filter (local) + API search results
  const [filter, setFilter] = useState('');
  const [searchResults, setSearchResults] = useState<SearchResults | null>(null);
  const [searchLoading, setSearchLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Expose ref for ⌘K
  useEffect(() => {
    searchInputRef.current = inputRef.current;
  });

  // Clear search results when filter is emptied
  useEffect(() => {
    if (!filter) setSearchResults(null);
  }, [filter]);

  // Enter → call search API
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && filter.trim()) {
        e.preventDefault();
        setSearchLoading(true);
        client
          .search(filter.trim(), { limit: 10 })
          .then(setSearchResults)
          .catch(() => setSearchResults(null))
          .finally(() => setSearchLoading(false));
      }
      if (e.key === 'Escape') {
        setFilter('');
        setSearchResults(null);
        inputRef.current?.blur();
      }
    },
    [client, filter],
  );

  const filteredChannels = useMemo(
    () =>
      filter
        ? channelList.filter((ch) =>
            ch.name.toLowerCase().includes(filter.toLowerCase()),
          )
        : channelList,
    [channelList, filter],
  );

  // If we have API search results, show them instead of the normal list
  const showSearch = searchResults !== null;

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
          onKeyDown={handleKeyDown}
          placeholder="Search... (Enter to search all)"
          className="h-7 w-full rounded bg-[#11111b] px-2 text-xs text-[#cdd6f4] placeholder:text-[#585b70] outline-none focus:ring-1 focus:ring-[#585b70]"
        />
      </div>

      {/* Scrollable content */}
      <ScrollArea className="flex-1">
        {showSearch ? (
          <SearchResultsList
            results={searchResults}
            loading={searchLoading}
            channels={channels}
            communities={communities}
            onSelect={() => {
              setFilter('');
              setSearchResults(null);
            }}
          />
        ) : isDMView ? (
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
// SearchResultsList — shown after Enter triggers API search
// ---------------------------------------------------------------------------

function SearchResultsList({
  results,
  loading,
  channels,
  communities,
  onSelect,
}: {
  results: SearchResults;
  loading: boolean;
  channels: Map<string, Channel[]>;
  communities: Map<string, { id: string; name: string }>;
  onSelect: () => void;
}) {
  const navigate = useNavigate();

  // The searcher actively pulls presence for every user referenced in the
  // results (matched users, DM peers) so each row shows current status —
  // not whatever was pushed earlier.
  const presenceIds = useMemo(() => {
    const ids = new Set<string>();
    for (const u of results.users) ids.add(u.id);
    for (const m of results.dmMessages) ids.add(m.peerId);
    return Array.from(ids);
  }, [results]);
  usePullPresence(presenceIds);

  if (loading) {
    return (
      <div className="px-2 py-4 text-center text-xs text-[#585b70]">Searching...</div>
    );
  }

  const hasAny =
    results.users.length > 0 ||
    results.messages.length > 0 ||
    results.dmMessages.length > 0;

  if (!hasAny) {
    return (
      <div className="px-2 py-4 text-center text-xs text-[#585b70]">No results found</div>
    );
  }

  return (
    <div className="px-2 space-y-2">
      {/* Users */}
      {results.users.length > 0 && (
        <div>
          <p className="mb-1 px-1 text-[10px] font-semibold tracking-wide text-[#585b70] uppercase">
            Users
          </p>
          {results.users.map((user) => (
            <UserResult
              key={user.id}
              user={user}
              onClick={() => {
                navigate(`/@me/${user.id}`);
                onSelect();
              }}
            />
          ))}
        </div>
      )}

      {/* Channel messages */}
      {results.messages.length > 0 && (
        <div>
          <p className="mb-1 px-1 text-[10px] font-semibold tracking-wide text-[#585b70] uppercase">
            Messages
          </p>
          {results.messages.map((msg) => {
            const chs = channels.get(msg.communityId);
            const ch = chs?.find((c) => c.id === msg.channelId);
            return (
              <MessageResult
                key={msg.id}
                content={msg.content}
                label={`#${ch?.name ?? msg.channelId.slice(0, 8)}`}
                onClick={() => {
                  navigate(`/${msg.communityId}/${msg.channelId}`);
                  onSelect();
                }}
              />
            );
          })}
        </div>
      )}

      {/* DM messages */}
      {results.dmMessages.length > 0 && (
        <div>
          <p className="mb-1 px-1 text-[10px] font-semibold tracking-wide text-[#585b70] uppercase">
            Direct Messages
          </p>
          {results.dmMessages.map((msg) => (
            <DMMessageResult
              key={msg.id}
              message={msg}
              onClick={() => {
                navigate(`/@me/${msg.peerId}`);
                onSelect();
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function UserResult({ user, onClick }: { user: UserSearchResult; onClick: () => void }) {
  const online = useUIStore((s) => s.onlineUsers.has(user.id));
  return (
    <button
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded px-2 py-1 text-sm text-[#a6adc8] transition-colors hover:bg-[#1e1e2e] hover:text-[#cdd6f4]"
    >
      <div className="relative shrink-0">
        <Avatar size="sm">
          {user.avatarUrl ? (
            <AvatarImage src={user.avatarUrl} alt={user.nickname} />
          ) : (
            <AvatarFallback>{user.nickname.charAt(0).toUpperCase()}</AvatarFallback>
          )}
        </Avatar>
        <span
          className={cn(
            'absolute right-0 bottom-0 h-2.5 w-2.5 rounded-full ring-2 ring-[#181825]',
            online ? 'bg-[#a6e3a1]' : 'bg-[#585b70]',
          )}
        />
      </div>
      <span className="truncate">{user.nickname}</span>
    </button>
  );
}

function MessageResult({
  content,
  label,
  onClick,
}: {
  content: string;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className="flex w-full flex-col rounded px-2 py-1 text-left transition-colors hover:bg-[#1e1e2e]"
    >
      <span className="truncate text-xs text-[#cdd6f4]">{content}</span>
      <span className="text-[10px] text-[#585b70]">{label}</span>
    </button>
  );
}

function DMMessageResult({
  message,
  onClick,
}: {
  message: DMMessageSearchResult;
  onClick: () => void;
}) {
  const peer = useResolvedUser(message.peerId);
  return (
    <MessageResult
      content={message.content}
      label={`DM with ${peer?.nickname ?? message.peerId.slice(0, 8)}`}
      onClick={onClick}
    />
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
        <DMListItem
          key={conv.peerId}
          conv={conv}
          selected={selectedPeerId === conv.peerId}
          onClick={() => navigate(`/@me/${conv.peerId}`)}
        />
      ))}
    </div>
  );
}

function DMListItem({
  conv,
  selected,
  onClick,
}: {
  conv: { peerId: string; lastContent: string; unread: number; online: boolean };
  selected: boolean;
  onClick: () => void;
}) {
  const peer = useResolvedUser(conv.peerId);
  const label = peer?.nickname ?? conv.peerId.slice(0, 8);
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2 rounded px-2 py-1.5 transition-colors',
        selected
          ? 'bg-[#313244] text-[#cdd6f4]'
          : 'text-[#a6adc8] hover:bg-[#1e1e2e] hover:text-[#cdd6f4]',
        conv.unread > 0 && !selected && 'text-[#cdd6f4] font-semibold',
      )}
    >
      <div className="relative shrink-0">
        <Avatar size="sm">
          {peer?.avatarUrl ? (
            <AvatarImage src={peer.avatarUrl} alt={label} />
          ) : (
            <AvatarFallback>{label.charAt(0).toUpperCase()}</AvatarFallback>
          )}
        </Avatar>
        <span
          className={cn(
            'absolute right-0 bottom-0 h-2.5 w-2.5 rounded-full ring-2 ring-[#181825]',
            conv.online ? 'bg-[#a6e3a1]' : 'bg-[#585b70]',
          )}
        />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{label}</p>
        {conv.lastContent && (
          <p className="truncate text-xs text-[#585b70]">{conv.lastContent}</p>
        )}
      </div>
      {conv.unread > 0 && !selected && (
        <span className="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-[#f38ba8] px-1 text-xs font-medium text-[#11111b]">
          {conv.unread > 99 ? '99+' : conv.unread}
        </span>
      )}
    </button>
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
