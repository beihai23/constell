import { useNavigate, useParams, useLocation } from 'react-router';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { ScrollArea } from '@/components/ui/scroll-area';
import { DMList } from '@/components/dm/DMList';
import { cn } from '@/lib/utils';
import type { Channel } from '@constell/sdk-js';

/**
 * Middle column (240px). Displays channel list or DM list depending on
 * the current route.
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

  return (
    <div className="flex w-60 shrink-0 flex-col bg-[#181825]">
      {/* Header */}
      <div className="flex h-12 items-center px-4 shadow-sm">
        <h2 className="truncate text-sm font-semibold text-[#cdd6f4]">
          {isDMView ? 'Direct Messages' : community?.name ?? 'Select a Community'}
        </h2>
      </div>

      {/* Search placeholder */}
      <div className="px-2 py-2">
        <div className="h-7 rounded bg-[#11111b] px-2 flex items-center">
          <span className="text-xs text-[#585b70]">Search...</span>
        </div>
      </div>

      {/* Scrollable content */}
      <ScrollArea className="flex-1">
        {isDMView ? (
          <DMList />
        ) : communityId ? (
          <div className="px-2">
            <div className="mb-1 flex items-center px-1">
              <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                Text Channels
              </span>
            </div>
            {channelList.map((channel) => (
              <ChannelItem
                key={channel.id}
                channel={channel}
                selected={currentChannelId === channel.id}
                unread={channelUnreads.get(channel.id) ?? 0}
                onClick={() => navigate(`/${communityId}/${channel.id}`)}
              />
            ))}
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
