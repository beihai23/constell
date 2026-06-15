import { useEffect } from 'react';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';

/**
 * Clears auth state and lets AuthGuard redirect to /login when the SDK reports
 * the session is dead (refresh permanently rejected, or no token at all).
 *
 * Mounted at the app root (inside ClientProvider) so the listener is live
 * before any WS/auth activity begins and never unmounts while the app is open
 * — the dead-session signal can fire within milliseconds of boot, before an
 * auth-gated subtree would have mounted its own listener.
 */
export function useAuthGate() {
  const client = useConstellClient();
  const reset = useAuthStore((s) => s.reset);

  useEffect(() => {
    const onUnauthorized = () => reset();
    client.on('unauthorized', onUnauthorized);
    return () => {
      client.off('unauthorized', onUnauthorized);
    };
  }, [client, reset]);
}
