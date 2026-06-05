import { useEffect, useState, useMemo } from 'react';
import { useConstellClient } from '@/hooks/useConstellClient';
import { useUIStore } from '@/stores/uiStore';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import type { Member } from '@constell/sdk-js';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface MemberListProps {
  communityId: string;
}

// ---------------------------------------------------------------------------
// MemberList — right column (240px), online/offline groups
// ---------------------------------------------------------------------------

export function MemberList({ communityId }: MemberListProps) {
  const client = useConstellClient();
  const onlineUsers = useUIStore((s) => s.onlineUsers);
  const [members, setMembers] = useState<Member[]>([]);

  // Fetch members on mount or community change
  useEffect(() => {
    let cancelled = false;
    client.getMembers(communityId).then((result) => {
      if (!cancelled) setMembers(result.items);
    }).catch(() => {
      // Silently fail
    });
    return () => { cancelled = true; };
  }, [communityId, client]);

  // Split into online / offline groups
  const { online, offline } = useMemo(() => {
    const online: Member[] = [];
    const offline: Member[] = [];
    for (const m of members) {
      if (onlineUsers.has(m.userId)) {
        online.push(m);
      } else {
        offline.push(m);
      }
    }
    // Sort alphabetically within each group
    const sortByName = (a: Member, b: Member) =>
      (a.nickname || a.userId).localeCompare(b.nickname || b.userId);
    online.sort(sortByName);
    offline.sort(sortByName);
    return { online, offline };
  }, [members, onlineUsers]);

  return (
    <div className="flex w-60 shrink-0 flex-col bg-[#181825] border-l border-[#313244]">
      {/* Header */}
      <div className="flex h-12 shrink-0 items-center px-4">
        <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
          MEMBERS &mdash; {members.length}
        </span>
      </div>

      {/* Scrollable member list */}
      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {/* Online section */}
        {online.length > 0 && (
          <div className="mb-2">
            <div className="px-1 py-1">
              <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                ONLINE &mdash; {online.length}
              </span>
            </div>
            {online.map((member) => (
              <MemberRow key={member.userId} member={member} online />
            ))}
          </div>
        )}

        {/* Offline section */}
        {offline.length > 0 && (
          <div>
            <div className="px-1 py-1">
              <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                OFFLINE &mdash; {offline.length}
              </span>
            </div>
            {offline.map((member) => (
              <MemberRow key={member.userId} member={member} online={false} />
            ))}
          </div>
        )}

        {members.length === 0 && (
          <div className="flex items-center justify-center p-4">
            <p className="text-xs text-[#585b70]">No members found</p>
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// MemberRow
// ---------------------------------------------------------------------------

interface MemberRowProps {
  member: Member;
  online: boolean;
}

function MemberRow({ member, online }: MemberRowProps) {
  return (
    <div className="flex items-center gap-2 rounded px-2 py-1 hover:bg-[#1e1e2e] transition-colors">
      <div className="relative shrink-0">
        <div className="flex h-8 w-8 items-center justify-center overflow-hidden rounded-full bg-[#313244]">
          <span className="text-xs font-semibold text-[#cdd6f4]">
            {member.nickname.charAt(0).toUpperCase()}
          </span>
        </div>
        {/* Online indicator */}
        <span
          className={`absolute -right-0.5 -bottom-0.5 h-3 w-3 rounded-full ring-2 ring-[#181825] ${
            online ? 'bg-[#a6e3a1]' : 'bg-[#585b70]'
          }`}
        />
      </div>
      <span className={`truncate text-sm ${online ? 'text-[#cdd6f4]' : 'text-[#585b70]'}`}>
        {member.nickname || member.userId}
      </span>
    </div>
  );
}
