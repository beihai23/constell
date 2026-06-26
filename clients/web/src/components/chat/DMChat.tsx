import { useEffect } from 'react';
import { useParams } from 'react-router';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { ChatHeader } from './ChatHeader';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';

/**
 * DM chat view: header + messages + input.
 * No member list in DM mode. Marks DM as read on mount.
 */
export function DMChat() {
  const { peerId } = useParams();
  const clearUnread = useUnreadStore((s) => s.clearUnread);
  const setActivePeer = useUIStore((s) => s.setActivePeer);

  // Mark DM as read on mount; mark peer as active so incoming DMs don't tally
  // unread while the user is viewing the conversation (UNREAD-1).
  useEffect(() => {
    if (!peerId) return;
    clearUnread('dm', peerId);
    setActivePeer(peerId);
    // Also call server-side mark-read if the conversation ID is known
    // The unread store uses peerId as key, but markDMRead expects conversationId.
    // For now, clearing local unread is sufficient.
    return () => setActivePeer(null);
  }, [peerId, clearUnread, setActivePeer]);

  return (
    <div className="flex min-h-0 flex-1 flex-col min-w-0">
      <ChatHeader />
      <MessageList />
      <ChatInput />
    </div>
  );
}
