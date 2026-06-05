import { useCallback } from 'react';
import { useConstellClient } from './useConstellClient';
import { useAuthStore } from '@/stores/authStore';

export function useAuth() {
  const client = useConstellClient();
  const { user, isAuthenticated, loading, setUser, setLoading, reset } = useAuthStore();

  const login = useCallback(async (email: string, password: string) => {
    setLoading(true);
    try {
      const u = await client.login(email, password);
      setUser(u);
    } catch {
      reset();
      throw new Error('Login failed');
    }
  }, [client, setUser, setLoading, reset]);

  const register = useCallback(async (username: string, email: string, password: string) => {
    setLoading(true);
    try {
      const u = await client.register(username, email, password);
      setUser(u);
    } catch {
      reset();
      throw new Error('Registration failed');
    }
  }, [client, setUser, setLoading, reset]);

  const logout = useCallback(() => {
    client.logout();
    reset();
  }, [client, reset]);

  const initAuth = useCallback(() => {
    setLoading(true);
    const u = client.initFromStorage();
    if (u) {
      setUser(u);
      client.connect();
    } else {
      setUser(null);
    }
  }, [client, setUser, setLoading]);

  return { user, isAuthenticated, loading, login, register, logout, initAuth };
}
