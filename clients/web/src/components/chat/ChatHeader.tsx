import { useParams } from 'react-router';
import { useEffect, useState } from 'react';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUIStore } from '@/stores/uiStore';
import { useConstellClient } from '@/hooks/useConstellClient';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import type { User } from '@constell/sdk-js';
import { Hash, Search, Users } from 'lucide-react';

/**
 * Top bar (48px) for the chat area.
 *
 * - Channel mode: shows "# channel-name" + topic + search/members toggle
 * - DM mode: shows peer avatar + username + online status
 */
export function ChatHeader() {
  const { communityId, channelId, peerId } = useParams();
  const client = useConstellClient();
  const channels = useCommunitiesStore((s) => s.channels);
  const toggleMemberList = useUIStore((s) => s.toggleMemberList);
  const showMemberList = useUIStore((s) => s.showMemberList);
  const onlineUsers = useUIStore((s) => s.onlineUsers);

  const [peerUser, setPeerUser] = useState<User | null>(null);

  // Resolve channel info
  const channelList = communityId ? (channels.get(communityId) ?? []) : [];
  const currentChannel = channelId
    ? channelList.find((c) => c.id === channelId)
    : undefined;

  // Fetch peer user info for DM mode
  useEffect(() => {
    if (!peerId) {
      setPeerUser(null);
      return;
    }
    let cancelled = false;
    client.getUser(peerId).then((user) => {
      if (!cancelled) setPeerUser(user);
    }).catch(() => {
      // User not found — keep null
    });
    return () => { cancelled = true; };
  }, [peerId, client]);

  const isDM = !!peerId;
  const isOnline = peerId ? onlineUsers.has(peerId) : false;

  return (
    <div className="flex h-12 shrink-0 items-center border-b border-[#313244] bg-[#1e1e2e] px-4">
      {/* Left section — channel or DM info */}
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {isDM ? (
          <>
            <Avatar size="sm">
              {peerUser?.avatarUrl ? (
                <AvatarImage src={peerUser.avatarUrl} alt={peerUser.nickname} />
              ) : (
                <AvatarFallback>
                  {(peerUser?.nickname ?? peerId ?? '?').charAt(0).toUpperCase()}
                </AvatarFallback>
              )}
            </Avatar>
            <div className="min-w-0">
              <span className="text-sm font-semibold text-[#cdd6f4]">
                {peerUser?.nickname ?? peerId}
              </span>
              <span className={`ml-2 text-xs ${isOnline ? 'text-[#a6e3a1]' : 'text-[#585b70]'}`}>
                {isOnline ? 'Online' : 'Offline'}
              </span>
            </div>
          </>
        ) : (
          <>
            <Hash className="h-4 w-4 shrink-0 text-[#585b70]" />
            <span className="truncate text-sm font-semibold text-[#cdd6f4]">
              {currentChannel?.name ?? 'Select a channel'}
            </span>
            {currentChannel?.topic && (
              <>
                <span className="mx-1 text-[#585b70]">|</span>
                <span className="truncate text-xs text-[#585b70]">
                  {currentChannel.topic}
                </span>
              </>
            )}
          </>
        )}
      </div>

      {/* Right section — action buttons */}
      <div className="flex shrink-0 items-center gap-1">
        {/** Search button — only in channel mode */}
        {!isDM && (
          <button
            className="flex h-8 w-8 items-center justify-center rounded text-[#585b70] transition-colors hover:bg-[#313244] hover:text-[#cdd6f4]"
            aria-label="Search"
          >
            <Search className="h-4 w-4" />
          </button>
        )}

        {/** Members toggle — only in channel mode */}
        {!isDM && (
          <button
            className={`flex h-8 w-8 items-center justify-center rounded transition-colors ${
              showMemberList
                ? 'bg-[#313244] text-[#cdd6f4]'
                : 'text-[#585b70] hover:bg-[#313244] hover:text-[#cdd6f4]'
            }`}
            onClick={toggleMemberList}
            aria-label="Toggle member list"
          >
            <Users className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  );
}
