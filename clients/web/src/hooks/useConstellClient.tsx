import { createContext, useContext, useRef } from 'react';
import type { ReactNode } from 'react';
import { ConstellClient } from '@constell/sdk-js';
import { createClient } from '@/lib/client';
import { useAuthGate } from './useAuthGate';

const ClientContext = createContext<ConstellClient | null>(null);

/** Mounts the root-level unauthorized listener (must be inside the Provider). */
function AuthGate() {
  useAuthGate();
  return null;
}

export function ClientProvider({ children }: { children: ReactNode }) {
  const clientRef = useRef<ConstellClient | null>(null);
  if (!clientRef.current) {
    clientRef.current = createClient();
  }
  return (
    <ClientContext.Provider value={clientRef.current}>
      <AuthGate />
      {children}
    </ClientContext.Provider>
  );
}

export function useConstellClient(): ConstellClient {
  const client = useContext(ClientContext);
  if (!client) throw new Error('useConstellClient must be used within ClientProvider');
  return client;
}
