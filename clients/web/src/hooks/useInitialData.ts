import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';

/**
 * Loads initial data (communities, channels, unread counts) once after login.
 * Safe to call from MainLayout — uses a ref guard to avoid re-fetching.
 */
export function useInitialData() {
  const client = useConstellClient();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const setCommunities = useCommunitiesStore((s) => s.setCommunities);
  const setChannels = useCommunitiesStore((s) => s.setChannels);
  const setUnreads = useUnreadStore((s) => s.setUnreads);
  const loaded = useRef(false);

  useEffect(() => {
    if (!isAuthenticated || loaded.current) return;
    loaded.current = true;

    (async () => {
      try {
        // Load communities
        const result = await client.listCommunities();
        setCommunities(result.items);

        // Load channels for each community in parallel
        await Promise.all(
          result.items.map(async (c) => {
            try {
              const channels = await client.getChannels(c.id);
              setChannels(c.id, channels);
            } catch {
              // Individual channel loading failure is non-fatal
            }
          }),
        );
      } catch {
        toast.error('Failed to load communities');
      }

      // Load unread counts
      try {
        const unreads = await client.getUnreadCounts();
        setUnreads(unreads);
      } catch {
        // Unread counts are non-critical
      }

      // DM conversations — endpoint may return 501 (not implemented)
      try {
        await client.getDMConversations();
      } catch {
        // Gracefully skip — DMList derives from received messages
      }
    })();
  }, [isAuthenticated, client, setCommunities, setChannels, setUnreads]);
}
