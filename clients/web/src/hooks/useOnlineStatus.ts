import { useUIStore } from '@/stores/uiStore';

/**
 * Returns true if the given user is currently online.
 */
export function useOnlineStatus(userId: string): boolean {
  return useUIStore((s) => s.onlineUsers.has(userId));
}
