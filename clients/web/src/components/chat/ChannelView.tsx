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

  // Load channels for the community if not already loaded
  useEffect(() => {
    if (!communityId) return;
    client.getChannels(communityId).then((channels) => {
      setChannels(communityId, channels);
    }).catch(() => {
      toast.error('Failed to load channels');
    });
  }, [communityId, client, setChannels]);

  // Subscribe to channel + clear unreads on mount; unsubscribe on unmount
  useEffect(() => {
    if (!channelId) return;

    client.subscribeChannel(channelId);
    clearUnread('channel', channelId);

    return () => {
      client.unsubscribeChannel(channelId);
    };
  }, [channelId, client, clearUnread]);

  return (
    <div className="flex flex-1 min-w-0">
      <div className="flex flex-1 flex-col min-w-0">
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
