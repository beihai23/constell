import { useEffect, useCallback } from 'react';
import { Outlet } from 'react-router';
import { CommunityRail } from './CommunityRail';
import { ChannelList } from './ChannelList';
import { ConnectionStatusBar } from './ConnectionStatusBar';
import { SearchDialog } from '@/components/search/SearchDialog';
import { useClientEvents } from '@/hooks/useClientEvents';
import { useInitialData } from '@/hooks/useInitialData';
import { useUIStore } from '@/stores/uiStore';

export function MainLayout() {
  // Bridge SDK events to Zustand stores (called once at top level)
  useClientEvents();
  // Load initial data (communities, channels, unreads) after login
  useInitialData();

  const showSearch = useUIStore((s) => s.showSearch);
  const setShowSearch = useUIStore((s) => s.setShowSearch);

  // Global Cmd+K / Ctrl+K keyboard shortcut
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setShowSearch(!showSearch);
      }
    },
    [showSearch, setShowSearch],
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      {/* Connection status bar (visible when WS not connected) */}
      <ConnectionStatusBar />

      {/* Main three-column layout */}
      <div className="flex flex-1 min-h-0">
        <CommunityRail />
        <ChannelList />
        <div className="flex-1 flex flex-col min-w-0">
          <Outlet />
        </div>
      </div>

      {/* Search dialog (Cmd+K) */}
      <SearchDialog open={showSearch} onOpenChange={setShowSearch} />
    </div>
  );
}
