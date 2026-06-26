import { useEffect } from 'react';
import { useParams } from 'react-router';
import { toast } from 'sonner';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { ChatHeader } from './ChatHeader';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import { MemberList } from '@/components/layout/MemberList';

/**
 * Full channel chat view: header + messages + input + optional member list.
 * Subscribes/unsubscribes the channel via SDK on mount/unmount.
 */
export function ChannelView() {
  const { communityId, channelId } = useParams();
  const client = useConstellClient();
  const setChannels = useCommunitiesStore((s) => s.setChannels);
  const clearUnread = useUnreadStore((s) => s.clearUnread);
  const showMemberList = useUIStore((s) => s.showMemberList);
  const setActiveChannel = useUIStore((s) => s.setActiveChannel);

  // Load channels for the community if not already loaded
  useEffect(() => {
    if (!communityId) return;
    client.getChannels(communityId).then((channels) => {
      setChannels(communityId, channels);
    }).catch(() => {
      toast.error('Failed to load channels');
    });
  }, [communityId, client, setChannels]);

  // Subscribe to channel + clear unreads on mount; unsubscribe on unmount.
  // Also mark this channel as active so incoming messages don't tally unread
  // while the user is viewing it (UNREAD-1).
  useEffect(() => {
    if (!channelId) return;

    client.subscribeChannel(channelId);
    clearUnread('channel', channelId);
    setActiveChannel(channelId);

    return () => {
      client.unsubscribeChannel(channelId);
      setActiveChannel(null);
    };
  }, [channelId, client, clearUnread, setActiveChannel]);

  return (
    <div className="flex min-h-0 flex-1 min-w-0">
      <div className="flex min-h-0 flex-1 flex-col min-w-0">
        <ChatHeader />
        <MessageList />
        <ChatInput />
      </div>
      {showMemberList && communityId && (
        <MemberList communityId={communityId} />
      )}
    </div>
  );
}
