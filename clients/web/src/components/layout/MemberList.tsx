import { useEffect, useState, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useConstellClient } from '@/hooks/useConstellClient';
import { usePullPresence } from '@/hooks/usePullPresence';
import { useUIStore } from '@/stores/uiStore';
import { useAuthStore } from '@/stores/authStore';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { Skeleton } from '@/components/ui/skeleton';
import { EmptyState, ErrorState } from '@/components/ui/state';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import type { Member, User } from '@constell/sdk-js';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface MemberListProps {
  communityId: string;
}

type LoadState = 'loading' | 'ready' | 'error';

// ---------------------------------------------------------------------------
// MemberList — right column (240px), online/offline groups
// ---------------------------------------------------------------------------

export function MemberList({ communityId }: MemberListProps) {
  const client = useConstellClient();
  const navigate = useNavigate();
  const onlineUsers = useUIStore((s) => s.onlineUsers);
  const user = useAuthStore((s) => s.user);
  const community = useCommunitiesStore((s) => s.communities.get(communityId));
  const [members, setMembers] = useState<Member[]>([]);
  // Distinguish "loading" from "empty" (MEM-LIST-2) and surface fetch failure
  // with a retry (MEM-LIST-3) instead of swallowing.
  const [loadState, setLoadState] = useState<LoadState>('loading');
  // Profile dialog target (MEM-PROFILE-1): userId of the member being viewed.
  const [profileUserId, setProfileUserId] = useState<string | null>(null);

  // Only the community owner may kick (MEM-KICK-1 UI / MEM-KICK-2). The
  // service enforces this regardless; the UI just hides the affordance.
  const isOwner = !!user && community?.ownerId === user.id;

  const handleKick = async (uid: string) => {
    try {
      await client.kickMember(communityId, uid);
      setMembers((prev) => prev.filter((m) => m.userId !== uid));
      setProfileUserId(null);
      toast.success('Member removed');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to remove member');
    }
  };

  // Fetch members on mount or community change. Factored out so the ErrorState
  // retry can re-run it.
  const loadMembers = () => {
    setLoadState('loading');
    client
      .getMembers(communityId)
      .then((result) => {
        setMembers(result.items);
        setLoadState('ready');
      })
      .catch(() => setLoadState('error'));
  };

  useEffect(() => {
    loadMembers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [communityId, client]);

  // Pull presence for the members shown in this list (source of truth = Redis).
  // Push events keep it fresh in real time; this guarantees correctness on
  // first view and when members change.
  const memberIds = useMemo(() => members.map((m) => m.userId), [members]);
  usePullPresence(memberIds);

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

      {/* Body — loading / error / empty / list */}
      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {loadState === 'error' && members.length === 0 ? (
          <ErrorState title="Couldn't load members" retry={loadMembers} compact />
        ) : members.length === 0 && loadState === 'loading' ? (
          <div aria-busy="true" className="space-y-1 p-1">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-2 px-2 py-1">
                <Skeleton className="size-8 shrink-0 rounded-full" />
                <Skeleton className="h-3 w-24" />
              </div>
            ))}
          </div>
        ) : (
          <>
            {online.length > 0 && (
              <div className="mb-2">
                <div className="px-1 py-1">
                  <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                    ONLINE &mdash; {online.length}
                  </span>
                </div>
                {online.map((member) => (
                  <MemberRow
                    key={member.userId}
                    member={member}
                    online
                    onSelect={() => setProfileUserId(member.userId)}
                  />
                ))}
              </div>
            )}

            {offline.length > 0 && (
              <div>
                <div className="px-1 py-1">
                  <span className="text-xs font-semibold tracking-wide text-[#585b70] uppercase">
                    OFFLINE &mdash; {offline.length}
                  </span>
                </div>
                {offline.map((member) => (
                  <MemberRow
                    key={member.userId}
                    member={member}
                    online={false}
                    onSelect={() => setProfileUserId(member.userId)}
                  />
                ))}
              </div>
            )}

            {members.length === 0 && loadState === 'ready' && (
              <EmptyState title="No members found" compact />
            )}
          </>
        )}
      </div>

      <MemberProfileDialog
        userId={profileUserId}
        canKick={isOwner}
        onClose={() => setProfileUserId(null)}
        onSendDM={(uid) => {
          setProfileUserId(null);
          navigate(`/@me/${uid}`);
        }}
        onKick={handleKick}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// MemberRow
// ---------------------------------------------------------------------------

interface MemberRowProps {
  member: Member;
  online: boolean;
  onSelect?: () => void;
}

function MemberRow({ member, online, onSelect }: MemberRowProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="flex w-full items-center gap-2 rounded px-2 py-1 text-left transition-colors hover:bg-[#1e1e2e]"
    >
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
    </button>
  );
}

// ---------------------------------------------------------------------------
// MemberProfileDialog — click a member → profile card (MEM-PROFILE-1)
// ---------------------------------------------------------------------------

function MemberProfileDialog({
  userId,
  canKick,
  onClose,
  onSendDM,
  onKick,
}: {
  userId: string | null;
  canKick: boolean;
  onClose: () => void;
  onSendDM: (userId: string) => void;
  onKick: (userId: string) => void;
}) {
  const client = useConstellClient();
  const online = useUIStore((s) => (userId ? s.onlineUsers.has(userId) : false));
  const [user, setUser] = useState<User | null>(null);
  // Two-step kick confirm so a single mis-click can't remove a member.
  const [confirmKick, setConfirmKick] = useState(false);

  useEffect(() => {
    if (!userId) {
      setUser(null);
      setConfirmKick(false);
      return;
    }
    let cancelled = false;
    client
      .getUser(userId)
      .then((u) => {
        if (!cancelled) setUser(u);
      })
      .catch(() => {
        if (!cancelled) setUser(null);
      });
    return () => {
      cancelled = true;
    };
  }, [userId, client]);

  return (
    <Dialog open={userId !== null} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="gap-4 rounded-xl border border-[#313244] bg-[#1e1e2e] p-5 text-[#cdd6f4] shadow-2xl sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-base font-semibold text-[#cdd6f4]">
            Member profile
          </DialogTitle>
          <DialogDescription className="sr-only">View this member&apos;s profile.</DialogDescription>
        </DialogHeader>
        {user ? (
          <div className="flex flex-col items-center gap-2 py-2">
            <Avatar size="lg">
              {user.avatarUrl ? (
                <AvatarImage src={user.avatarUrl} alt={user.nickname} />
              ) : (
                <AvatarFallback>{user.nickname.charAt(0).toUpperCase()}</AvatarFallback>
              )}
            </Avatar>
            <div className="text-center">
              <p className="text-sm font-semibold text-[#cdd6f4]">{user.nickname}</p>
              {user.email && <p className="text-xs text-[#585b70]">{user.email}</p>}
              <p className={`mt-1 text-xs ${online ? 'text-[#a6e3a1]' : 'text-[#585b70]'}`}>
                {online ? 'Online' : 'Offline'}
              </p>
            </div>
            <Button
              className="mt-2 w-full"
              onClick={() => onSendDM(user.id)}
            >
              Send DM
            </Button>
            {canKick && (
              confirmKick ? (
                <div className="mt-1 flex w-full gap-2">
                  <Button variant="ghost" className="flex-1" onClick={() => setConfirmKick(false)}>
                    Cancel
                  </Button>
                  <Button
                    variant="destructive"
                    className="flex-1"
                    onClick={() => onKick(user.id)}
                  >
                    Confirm kick
                  </Button>
                </div>
              ) : (
                <Button
                  variant="destructive"
                  className="mt-1 w-full"
                  onClick={() => setConfirmKick(true)}
                >
                  Kick
                </Button>
              )
            )}
          </div>
        ) : (
          <div className="py-4 text-center text-sm text-[#585b70]">Loading…</div>
        )}
      </DialogContent>
    </Dialog>
  );
}
