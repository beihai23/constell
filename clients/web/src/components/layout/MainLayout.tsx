import { useEffect, useCallback } from 'react';
import { Outlet } from 'react-router';
import { CommunityRail } from './CommunityRail';
import { ChannelList, searchInputRef } from './ChannelList';
import { ConnectionStatusBar } from './ConnectionStatusBar';
import { useClientEvents } from '@/hooks/useClientEvents';
import { useInitialData } from '@/hooks/useInitialData';
import { useMessageSync } from '@/hooks/useMessageSync';

export function MainLayout() {
  // Bridge SDK events to Zustand stores (called once at top level)
  useClientEvents();
  // Load initial data (communities, channels, unreads) after login
  useInitialData();
  // Backfill any missed messages (lost push / reconnect / visibility). Mounted
  // once alongside useClientEvents so it lives exactly at the authenticated-app root.
  useMessageSync();

  // ⌘K / Ctrl+K → focus the search input in ChannelList
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      searchInputRef.current?.focus();
    }
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
        <div className="flex-1 flex flex-col min-w-0">
          <Outlet />
        </div>
      </div>
    </div>
  );
}
