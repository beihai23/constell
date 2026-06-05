import { useEffect, type ReactNode } from 'react';
import { Navigate } from 'react-router';
import { useAuth } from '@/hooks/useAuth';

export function AuthGuard({ children }: { children: ReactNode }) {
  const { isAuthenticated, loading, initAuth } = useAuth();

  useEffect(() => {
    initAuth();
  }, [initAuth]);

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}
