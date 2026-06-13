import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useUsersStore } from '@/stores/usersStore';

/**
 * Lazily resolves a user ID to a profile via the SDK. Result is cached in
 * the usersStore so repeated lookups for the same ID are free. Returns
 * `undefined` while loading or if the user can't be found.
 */
export function useResolvedUser(userId: string | null | undefined) {
  const client = useConstellClient();
  const user = useUsersStore((s) => (userId ? s.users.get(userId) : undefined));
  const loading = useUsersStore((s) => (userId ? s.loading.has(userId) : false));

  useEffect(() => {
    if (!userId || user || loading) return;
    useUsersStore.getState().markLoading(userId);
    client
      .getUser(userId)
      .then((u) => useUsersStore.getState().setUser(u))
      .catch(() => {
        // Resolve failed — drop loading flag so we don't retry forever
        useUsersStore.setState((s) => {
          const l = new Set(s.loading);
          l.delete(userId);
          return { loading: l };
        });
      });
  }, [client, userId, user, loading]);

  return user;
}
