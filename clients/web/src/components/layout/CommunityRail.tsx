import { useState, useRef, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useAuthStore } from '@/stores/authStore';
import { useAuth } from '@/hooks/useAuth';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import { Separator } from '@/components/ui/separator';
import { CreateCommunityDialog } from '@/components/communities/CreateCommunityDialog';
import { cn } from '@/lib/utils';

/**
 * Left column icon rail (72px). Discord-style vertical strip showing:
 * - DM/Home button
 * - Joined community icons (with unread badges)
 * - Create community button (placeholder)
 * - Current user avatar at bottom
 */
export function CommunityRail() {
  const navigate = useNavigate();
  const location = useLocation();
  const communities = useCommunitiesStore((s) => s.communities);
  const channels = useCommunitiesStore((s) => s.channels);
  const channelUnreads = useUnreadStore((s) => s.channelUnreads);
  const user = useAuthStore((s) => s.user);

  const isDMView = location.pathname.startsWith('/@me');
  const currentCommunityId = getCommunityIdFromPath(location.pathname);

  const [createOpen, setCreateOpen] = useState(false);

  // Compute per-community unread totals by cross-referencing
  // channels Map (communityId -> Channel[]) with channelUnreads (channelId -> count)
  const communityUnreadTotals = new Map<string, number>();
  for (const [communityId, channelList] of channels) {
    let total = 0;
    for (const ch of channelList) {
      total += channelUnreads.get(ch.id) ?? 0;
    }
    communityUnreadTotals.set(communityId, total);
  }
  // Also ensure communities with no channels yet still appear
  for (const id of communities.keys()) {
    if (!communityUnreadTotals.has(id)) {
      communityUnreadTotals.set(id, 0);
    }
  }

  const communityList = Array.from(communities.values());

  return (
    <div className="flex w-[72px] shrink-0 flex-col items-center gap-2 bg-[#11111b] py-3">
      {/* DM / Home button */}
      <RailButton
        selected={isDMView}
        onClick={() => navigate('/@me')}
        label="Direct Messages"
      >
        <span className="text-xl text-[#7c3aed]">&#128172;</span>
      </RailButton>

      <Separator className="w-8 bg-[#313244]" />

      {/* Community icons */}
      {communityList.map((community) => {
        const selected = currentCommunityId === community.id;
        const unread = communityUnreadTotals.get(community.id) ?? 0;

        return (
          <RailButton
            key={community.id}
            selected={selected}
            onClick={() => navigate(`/${community.id}`)}
            label={community.name}
            badge={unread > 0 ? unread : undefined}
          >
            <span className="text-sm font-semibold text-[#cdd6f4]">
              {community.name.charAt(0).toUpperCase()}
            </span>
          </RailButton>
        );
      })}

      {/* Create community */}
      <RailButton
        selected={false}
        onClick={() => setCreateOpen(true)}
        label="Add a Community"
      >
        <span className="text-xl text-[#cdd6f4]">+</span>
      </RailButton>

      {/* Spacer to push avatar to bottom */}
      <div className="flex-1" />

      {/* Current user avatar + menu */}
      {user && <UserMenu />}

      {/* Create-community dialog (controlled; rendered at end so it portals
          above the rail without affecting flex layout) */}
      <CreateCommunityDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// UserMenu — avatar button that opens a floating menu with profile + logout
// ---------------------------------------------------------------------------

function UserMenu() {
  const user = useAuthStore((s) => s.user);
  const { logout } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  if (!user) return null;

  const handleLogout = () => {
    setOpen(false);
    logout();
    navigate('/login', { replace: true });
  };

  return (
    <div className="relative" ref={menuRef}>
      <button
        className="group relative flex items-center justify-center"
        onClick={() => setOpen(!open)}
        aria-label="User menu"
      >
        <div className={cn(
          'flex h-12 w-12 items-center justify-center overflow-hidden transition-all duration-200',
          open ? 'rounded-2xl bg-[#313244]' : 'rounded-full hover:rounded-2xl hover:bg-[#313244]',
        )}>
          <Avatar size="lg">
            {user.avatarUrl ? (
              <AvatarImage src={user.avatarUrl} alt={user.nickname} />
            ) : (
              <AvatarFallback>{user.nickname.charAt(0).toUpperCase()}</AvatarFallback>
            )}
          </Avatar>
        </div>
      </button>

      {open && (
        <div className="absolute bottom-14 left-16 z-50 w-56 rounded-xl border border-[#313244] bg-[#1e1e2e] p-2 shadow-xl">
          {/* User info */}
          <div className="px-2 py-2">
            <p className="text-sm font-semibold text-[#cdd6f4]">{user.nickname}</p>
            <p className="text-xs text-[#585b70]">{user.email}</p>
          </div>
          <Separator className="my-1 bg-[#313244]" />
          <button
            onClick={handleLogout}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-[#f38ba8] hover:bg-[#313244]"
          >
            Logout
          </button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Extract communityId from a path like /communityId/channelId */
function getCommunityIdFromPath(pathname: string): string | null {
  const segments = pathname.split('/').filter(Boolean);
  if (segments.length === 0) return null;
  const first = segments[0];
  // /@me is not a community
  if (first === '@me') return null;
  return first;
}

// ---------------------------------------------------------------------------
// RailButton component
// ---------------------------------------------------------------------------

interface RailButtonProps {
  selected: boolean;
  onClick: () => void;
  label: string;
  /** If set, shows an unread badge with this count. */
  badge?: number;
  children: React.ReactNode;
}

function RailButton({ selected, onClick, label, badge, children }: RailButtonProps) {
  return (
    <button
      className="group relative flex items-center justify-center"
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      {/* Pill indicator for selected item */}
      <div
        className={cn(
          'absolute left-0 w-1 rounded-r-full bg-white transition-all duration-200',
          selected ? 'h-10' : 'h-0 group-hover:h-5',
        )}
      />

      <div
        className={cn(
          'flex h-12 w-12 items-center justify-center transition-all duration-200',
          selected
            ? 'rounded-[16px] bg-[#313244]'
            : 'rounded-full hover:rounded-2xl hover:bg-[#313244]',
        )}
      >
        {children}
      </div>

      {/* Unread badge */}
      {badge !== undefined && badge > 0 && (
        <span className="absolute right-0.5 top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-[#f38ba8] px-1 text-[10px] font-bold leading-none text-white">
          {badge > 99 ? '99+' : badge}
        </span>
      )}
    </button>
  );
}
