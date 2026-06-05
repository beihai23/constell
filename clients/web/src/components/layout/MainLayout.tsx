import { Outlet } from 'react-router';
import { CommunityRail } from './CommunityRail';
import { ChannelList } from './ChannelList';
import { useClientEvents } from '@/hooks/useClientEvents';

export function MainLayout() {
  // Bridge SDK events to Zustand stores (called once at top level)
  useClientEvents();

  return (
    <div className="flex h-screen bg-background text-foreground">
      <CommunityRail />
      <ChannelList />
      <div className="flex-1 flex flex-col min-w-0">
        <Outlet />
      </div>
    </div>
  );
}
