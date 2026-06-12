import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';
import { useCommunitiesStore } from '@/stores/communitiesStore';
import { useUnreadStore } from '@/stores/unreadStore';
import { useUIStore } from '@/stores/uiStore';

/**
 * Loads initial data (communities, channels, unread counts, presence) once after login.
 * Safe to call from MainLayout — uses a ref guard to avoid re-fetching.
 */
export function useInitialData() {
  const client = useConstellClient();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const setCommunities = useCommunitiesStore((s) => s.setCommunities);
  const setChannels = useCommunitiesStore((s) => s.setChannels);
  const setUnreads = useUnreadStore((s) => s.setUnreads);
  const setOnline = useUIStore((s) => s.setOnline);
  const loaded = useRef(false);

  useEffect(() => {
    if (!isAuthenticated || loaded.current) return;
    loaded.current = true;

    (async () => {
      // Collect all member user IDs across communities for presence lookup
      const memberIds = new Set<string>();

      try {
        // Load communities
        const result = await client.listCommunities();
        setCommunities(result.items);

        // Load channels + members for each community in parallel
        await Promise.all(
          result.items.map(async (c) => {
            try {
              const [channels, membersResult] = await Promise.all([
                client.getChannels(c.id),
                client.getMembers(c.id, { limit: 100 }),
              ]);
              setChannels(c.id, channels);
              for (const m of membersResult.items) {
                memberIds.add(m.userId);
              }
            } catch {
              // Individual loading failure is non-fatal
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

      // Pull online presence for all known members
      if (memberIds.size > 0) {
        try {
          const presence = await client.getPresence(Array.from(memberIds));
          for (const id of presence.online) {
            setOnline(id);
          }
        } catch {
          // Presence is non-critical — UI will update via push events
        }
      }

      // DM conversations — endpoint may return 501 (not implemented)
      try {
        await client.getDMConversations();
      } catch {
        // Gracefully skip — DMList derives from received messages
      }
    })();
  }, [isAuthenticated, client, setCommunities, setChannels, setUnreads, setOnline]);
}
