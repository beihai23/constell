import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useAuthStore } from '@/stores/authStore';
import {
  Command,
  CommandInput,
  CommandList,
  CommandGroup,
  CommandItem,
  CommandSeparator,
} from '@/components/ui/command';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import { Hash, MessageSquare } from 'lucide-react';
import { toast } from 'sonner';
import type {
  SearchResults,
  UserSearchResult,
  MessageSearchResult,
  DMMessageSearchResult,
  CommunitySearchResult,
} from '@constell/sdk-js';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface SearchDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// ---------------------------------------------------------------------------
// Empty state (no query yet)
// ---------------------------------------------------------------------------

function EmptyStart() {
  return (
    <div className="flex flex-col items-center gap-2 py-8 text-[#585b70]">
      <p className="text-sm">Start typing to search...</p>
      <p className="text-xs">
        <kbd className="rounded bg-[#313244] px-1.5 py-0.5 text-[#cdd6f4]">Esc</kbd> to close
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Loading skeletons
// ---------------------------------------------------------------------------

function SearchLoading() {
  return (
    <div className="space-y-3 p-2">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="flex items-center gap-2">
          <Skeleton className="h-8 w-8 rounded-full" />
          <div className="flex-1 space-y-1">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-3 w-40" />
          </div>
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Relevance indicator
// ---------------------------------------------------------------------------

function RelevanceIndicator({ score }: { score: number }) {
  if (score == null || isNaN(score)) return null;
  const pct = Math.round(Math.min(score, 1) * 100);
  return (
    <span
      className="ml-auto shrink-0 text-[10px] tabular-nums text-[#585b70]"
      title={`Relevance: ${pct}%`}
    >
      {pct}%
    </span>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SearchDialog({ open, onOpenChange }: SearchDialogProps) {
  const navigate = useNavigate();
  const client = useConstellClient();
  const channels = useCommunitiesStore((s) => s.channels);
  const communities = useCommunitiesStore((s) => s.communities);
  const currentUser = useAuthStore((s) => s.user);

  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResults | null>(null);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Reset state when dialog opens/closes
  useEffect(() => {
    if (!open) {
      setQuery('');
      setResults(null);
      setLoading(false);
      setSearched(false);
    } else {
      // Focus input after dialog opens
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [open]);

  // Debounced search
  const doSearch = useCallback(
    async (q: string) => {
      if (!q.trim()) {
        setResults(null);
        setLoading(false);
        setSearched(false);
        return;
      }
      setLoading(true);
      try {
        const res = await client.search(q, { limit: 10 });
        setResults(res);
      } catch {
        setResults(null);
      } finally {
        setLoading(false);
        setSearched(true);
      }
    },
    [client],
  );

  // Handle input change with 300ms debounce
  const handleInputChange = useCallback(
    (value: string) => {
      setQuery(value);
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => doSearch(value), 300);
    },
    [doSearch],
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  // Resolve channel name from store
  const getChannelName = useCallback(
    (communityId: string, channelId: string): string => {
      const chs = channels.get(communityId);
      const ch = chs?.find((c) => c.id === channelId);
      return ch ? `#${ch.name}` : channelId;
    },
    [channels],
  );

  // Resolve community name from store
  const getCommunityName = useCallback(
    (communityId: string): string => {
      return communities.get(communityId)?.name ?? communityId;
    },
    [communities],
  );

  // Resolve peer nickname — for DMs, if the peer is the current user, show "you"
  // Otherwise we don't have the peer's nickname cached, so show a shortened ID
  const getPeerLabel = useCallback(
    (peerId: string): string => {
      if (currentUser && peerId === currentUser.id) return 'you';
      // Shorten UUID to first 8 chars for readability
      return peerId.slice(0, 8);
    },
    [currentUser],
  );

  // Navigation handlers
  const goToUser = useCallback(
    (userId: string) => {
      navigate(`/@me/${userId}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange],
  );

  const goToChannelMessage = useCallback(
    (communityId: string, channelId: string) => {
      navigate(`/${communityId}/${channelId}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange],
  );

  const goToDM = useCallback(
    (peerId: string) => {
      navigate(`/@me/${peerId}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange],
  );

  // Row click only opens a community the user already belongs to — it must NOT
  // silently join. Joining is a deliberate action on the explicit Join button.
  const openCommunity = useCallback(
    (community: CommunitySearchResult) => {
      if (!community.joined) return;
      navigate(`/${community.id}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange],
  );

  const joinAndOpen = useCallback(
    async (community: CommunitySearchResult) => {
      try {
        await client.joinCommunity(community.id);
      } catch {
        toast.error('Failed to join community');
        return;
      }
      navigate(`/${community.id}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange, client],
  );

  const hasResults =
    results &&
    (results.users.length > 0 ||
      results.messages.length > 0 ||
      results.dmMessages.length > 0 ||
      results.communities.length > 0);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-[20%] translate-y-0 gap-0 overflow-hidden rounded-xl border-[#313244] bg-[#1e1e2e] p-0 text-[#cdd6f4] shadow-2xl sm:max-w-lg" showCloseButton={false}>
        <DialogHeader className="sr-only">
          <DialogTitle>Search</DialogTitle>
          <DialogDescription>
            Search for users, channel messages, and direct messages
          </DialogDescription>
        </DialogHeader>
        <Command className="bg-transparent" shouldFilter={false}>
          <div className="border-b border-[#313244] px-3 py-2">
            <CommandInput
              ref={inputRef as React.Ref<HTMLInputElement>}
              placeholder="Search users, messages..."
              value={query}
              onValueChange={handleInputChange}
              className="bg-transparent text-[#cdd6f4] placeholder:text-[#585b70]"
            />
          </div>

          {/* Content area */}
          {!query.trim() ? (
            <EmptyStart />
          ) : loading ? (
            <SearchLoading />
          ) : searched && !hasResults ? (
            <div className="py-8 text-center text-sm text-[#585b70]">
              No results found for &ldquo;{query}&rdquo;
            </div>
          ) : results && hasResults ? (
            <CommandList className="max-h-80">
              {/* Communities group */}
              {results.communities.length > 0 && (
                <CommandGroup heading="Communities">
                  {results.communities.map((c) => (
                    <CommunityResultItem
                      key={c.id}
                      community={c}
                      onOpen={() => openCommunity(c)}
                      onJoin={() => joinAndOpen(c)}
                    />
                  ))}
                </CommandGroup>
              )}

              {results.communities.length > 0 &&
                results.users.length > 0 && <CommandSeparator />}

              {/* Users group */}
              {results.users.length > 0 && (
                <CommandGroup heading="Users">
                  {results.users.map((user) => (
                    <UserResultItem
                      key={user.id}
                      user={user}
                      onSelect={() => goToUser(user.id)}
                    />
                  ))}
                </CommandGroup>
              )}

              {results.users.length > 0 &&
                results.messages.length > 0 && <CommandSeparator />}

              {/* Channel Messages group */}
              {results.messages.length > 0 && (
                <CommandGroup heading="Channel Messages">
                  {results.messages.map((msg) => (
                    <ChannelMessageItem
                      key={msg.id}
                      message={msg}
                      channelLabel={getChannelName(msg.communityId, msg.channelId)}
                      communityLabel={getCommunityName(msg.communityId)}
                      onSelect={() =>
                        goToChannelMessage(msg.communityId, msg.channelId)
                      }
                    />
                  ))}
                </CommandGroup>
              )}

              {results.messages.length > 0 &&
                results.dmMessages.length > 0 && <CommandSeparator />}

              {/* DM Messages group */}
              {results.dmMessages.length > 0 && (
                <CommandGroup heading="Direct Messages">
                  {results.dmMessages.map((msg) => (
                    <DMMessageItem
                      key={msg.id}
                      message={msg}
                      peerLabel={getPeerLabel(msg.peerId)}
                      onSelect={() => goToDM(msg.peerId)}
                    />
                  ))}
                </CommandGroup>
              )}
            </CommandList>
          ) : null}
        </Command>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Result item sub-components
// ---------------------------------------------------------------------------

function UserResultItem({
  user,
  onSelect,
}: {
  user: UserSearchResult;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[#cdd6f4] data-[selected=true]:bg-[#313244]"
    >
      <Avatar size="sm">
        {user.avatarUrl ? (
          <AvatarImage src={user.avatarUrl} alt={user.nickname} />
        ) : (
          <AvatarFallback>{user.nickname.charAt(0).toUpperCase()}</AvatarFallback>
        )}
      </Avatar>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{user.nickname}</p>
      </div>
      <RelevanceIndicator score={user.relevance} />
    </CommandItem>
  );
}

function ChannelMessageItem({
  message,
  channelLabel,
  communityLabel,
  onSelect,
}: {
  message: MessageSearchResult;
  channelLabel: string;
  communityLabel: string;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[#cdd6f4] data-[selected=true]:bg-[#313244]"
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-[#313244]">
        <Hash className="h-3 w-3 text-[#585b70]" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{message.content}</p>
        <p className="text-xs text-[#585b70]">
          {channelLabel} in {communityLabel}
        </p>
      </div>
      <RelevanceIndicator score={message.relevance} />
    </CommandItem>
  );
}

function DMMessageItem({
  message,
  peerLabel,
  onSelect,
}: {
  message: DMMessageSearchResult;
  peerLabel: string;
  onSelect: () => void;
}) {
  return (
    <CommandItem
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[#cdd6f4] data-[selected=true]:bg-[#313244]"
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-[#313244]">
        <MessageSquare className="h-3 w-3 text-[#585b70]" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{message.content}</p>
        <p className="text-xs text-[#585b70]">DM with {peerLabel}</p>
      </div>
      <RelevanceIndicator score={message.relevance} />
    </CommandItem>
  );
}

function CommunityResultItem({
  community,
  onOpen,
  onJoin,
}: {
  community: CommunitySearchResult;
  onOpen: () => void;
  onJoin: () => void;
}) {
  return (
    <CommandItem
      onSelect={onOpen}
      className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-[#cdd6f4] data-[selected=true]:bg-[#313244]"
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded bg-[#313244]">
        <Hash className="h-3 w-3 text-[#585b70]" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{community.name}</p>
        <p className="text-xs text-[#585b70]">{community.memberCount} members</p>
      </div>
      {community.joined ? (
        <span className="shrink-0 text-xs text-[#585b70]">Joined</span>
      ) : (
        // Real button with stopped propagation so clicking it joins without
        // also firing the row's onSelect (which no-ops for non-members anyway).
        <button
          type="button"
          onPointerDown={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation();
            e.preventDefault();
            onJoin();
          }}
          className="shrink-0 cursor-pointer rounded px-1.5 py-0.5 text-xs font-medium text-[#cba6f7] hover:bg-[#313244]"
        >
          Join
        </button>
      )}
    </CommandItem>
  );
}
