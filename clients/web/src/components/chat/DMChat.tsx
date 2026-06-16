import { useEffect } from 'react';
import { useParams } from 'react-router';
import { useUnreadStore } from '@/stores/unreadStore';
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

  // Mark DM as read on mount
  useEffect(() => {
    if (!peerId) return;
    clearUnread('dm', peerId);
    // Also call server-side mark-read if the conversation ID is known
    // The unread store uses peerId as key, but markDMRead expects conversationId.
    // For now, clearing local unread is sufficient.
  }, [peerId, clearUnread]);

  return (
    <div className="flex flex-1 flex-col min-w-0">
      <ChatHeader />
      <MessageList />
      <ChatInput />
    </div>
  );
}
