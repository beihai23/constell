import { useNavigate, useLocation } from 'react-router';
import { useMessagesStore } from '@/stores/messagesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { cn } from '@/lib/utils';

/**
 * DM conversation list shown in the ChannelList sidebar when on /@me.
 *
 * For now this renders from dmMessages store keys. A future iteration will
 * have a dedicated dmConversations list from the SDK.
 */
export function DMList() {
  const navigate = useNavigate();
  const location = useLocation();
  const dmMessages = useMessagesStore((s) => s.dmMessages);
  const dmUnreads = useUnreadStore((s) => s.dmUnreads);
  const onlineUsers = useUIStore((s) => s.onlineUsers);

  // Build a list of DM peers from the messages store.
  // Each key in dmMessages is a peerId.
  const peerIds = Array.from(dmMessages.keys());

  // Derive last message for preview
  const conversations: { peerId: string; lastContent: string; unread: number; online: boolean }[] =
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

  // Also include unread-only peers not yet in dmMessages
  for (const [peerId, count] of dmUnreads) {
    if (!peerIds.includes(peerId)) {
      conversations.push({
        peerId,
        lastContent: '',
        unread: count,
        online: onlineUsers.has(peerId),
      });
    }
  }

  // Sort: unread first, then by last message time (most recent first)
  conversations.sort((a, b) => {
    if (a.unread > 0 && b.unread === 0) return -1;
    if (b.unread > 0 && a.unread === 0) return 1;
    return 0;
  });

  // Determine selected peer from URL /@me/:peerId
  const selectedPeerId = getPeerIdFromPath(location.pathname);

  if (conversations.length === 0) {
    return (
      <div className="flex items-center justify-center p-4">
        <p className="text-xs text-[#585b70]">No conversations yet</p>
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
          {/* Avatar with online indicator */}
          <div className="relative shrink-0">
            <Avatar size="sm">
              <AvatarFallback>
                {conv.peerId.charAt(0).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            {/* Online dot */}
            <span
              className={cn(
                'absolute right-0 bottom-0 h-2.5 w-2.5 rounded-full ring-2 ring-[#181825]',
                conv.online ? 'bg-[#a6e3a1]' : 'bg-[#585b70]',
              )}
            />
          </div>

          {/* Name + preview */}
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm">{conv.peerId}</p>
            {conv.lastContent && (
              <p className="truncate text-xs text-[#585b70]">{conv.lastContent}</p>
            )}
          </div>

          {/* Unread badge */}
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
