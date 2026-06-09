import { ConstellClient } from '@constell/sdk-js';

export function createClient(): ConstellClient {
  return new ConstellClient({
    apiUrl: import.meta.env.VITE_API_URL || '',
    wsUrl: import.meta.env.VITE_WS_URL || `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`,
  });
}
