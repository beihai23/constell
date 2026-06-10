import { useNavigate, useLocation } from 'react-router';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useAuthStore } from '@/stores/authStore';
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar';
import { Separator } from '@/components/ui/separator';
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

      {/* Create community placeholder */}
      <RailButton
        selected={false}
        onClick={() => {
          /* TODO: open create community dialog */
        }}
        label="Add a Community"
      >
        <span className="text-xl text-[#cdd6f4]">+</span>
      </RailButton>

      {/* Spacer to push avatar to bottom */}
      <div className="flex-1" />

      {/* Current user avatar */}
      {user && (
        <button
          className="group relative flex items-center justify-center"
          onClick={() => {
            /* TODO: open user settings */
          }}
        >
          <div className="flex h-12 w-12 items-center justify-center overflow-hidden rounded-full transition-all duration-200 hover:rounded-2xl hover:bg-[#313244]">
            <Avatar size="lg">
              {user.avatarUrl ? (
                <AvatarImage src={user.avatarUrl} alt={user.nickname} />
              ) : (
                <AvatarFallback>{user.nickname.charAt(0).toUpperCase()}</AvatarFallback>
              )}
            </Avatar>
          </div>
        </button>
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
