import { useEffect, useCallback, useState } from 'react';
import { Outlet } from 'react-router';
import { CommunityRail } from './CommunityRail';
import { ChannelList } from './ChannelList';
import { ConnectionStatusBar } from './ConnectionStatusBar';
import { SearchDialog } from '@/components/search/SearchDialog';
import { useClientEvents } from '@/hooks/useClientEvents';
import { useInitialData } from '@/hooks/useInitialData';
import { useMessageSync } from '@/hooks/useMessageSync';

export function MainLayout() {
  const [searchOpen, setSearchOpen] = useState(false);

  // Bridge SDK events to Zustand stores (called once at top level)
  useClientEvents();
  // Load initial data (communities, channels, unreads) after login
  useInitialData();
  // Backfill any missed messages (lost push / reconnect / visibility). Mounted
  // once alongside useClientEvents so it lives exactly at the authenticated-app root.
  useMessageSync();

  // ⌘K / Ctrl+K → open the global search palette (SearchDialog). The sidebar's
  // inline input is now scoped to filtering the current column only.
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      setSearchOpen((v) => !v);
    }
  }, []);

  // ChatHeader's search button dispatches 'open-search' to open the palette
  // without a keyboard shortcut.
  useEffect(() => {
    const opener = () => setSearchOpen(true);
    window.addEventListener('open-search', opener);
    return () => window.removeEventListener('open-search', opener);
  }, []);

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
        <div className="flex flex-1 min-w-0 flex-col min-h-0">
          <Outlet />
        </div>
      </div>

      {/* ⌘K global search palette */}
      <SearchDialog open={searchOpen} onOpenChange={setSearchOpen} />
    </div>
  );
}
