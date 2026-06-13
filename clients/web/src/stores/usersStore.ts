import { create } from 'zustand';
import type { User } from '@constell/sdk-js';

/**
 * Cache of resolved user profiles, keyed by user ID. The hook
 * `useResolvedUser` lazily fetches any unknown ID via the SDK.
 */
interface UsersState {
  users: Map<string, User>;
  loading: Set<string>;
  setUser: (user: User) => void;
  markLoading: (id: string) => void;
}

export const useUsersStore = create<UsersState>((set) => ({
  users: new Map(),
  loading: new Set(),
  setUser: (user) =>
    set((state) => {
      const users = new Map(state.users);
      users.set(user.id, user);
      const loading = new Set(state.loading);
      loading.delete(user.id);
      return { users, loading };
    }),
  markLoading: (id) =>
    set((state) => {
      if (state.users.has(id) || state.loading.has(id)) return state;
      const loading = new Set(state.loading);
      loading.add(id);
      return { loading };
    }),
}));
