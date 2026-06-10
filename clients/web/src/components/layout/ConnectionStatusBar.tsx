import { useUIStore } from '@/stores/uiStore';

/**
 * Thin status bar shown at the top of the app when the WebSocket
 * connection is not in a healthy "CONNECTED" state.
 *
 * - CONNECTING: yellow bar, "Connecting..."
 * - DISCONNECTED / RECONNECTING: red bar, "Disconnected — reconnecting..."
 * - CONNECTED: hidden (returns null)
 */
export function ConnectionStatusBar() {
  const wsStatus = useUIStore((s) => s.wsStatus);

  if (wsStatus === 'CONNECTED') return null;

  const isConnecting = wsStatus === 'CONNECTING';

  return (
    <div
      className={`text-center text-sm py-1 shrink-0 ${
        isConnecting
          ? 'bg-[#f9e2af] text-[#1e1e2e]'
          : 'bg-[#f38ba8] text-[#1e1e2e]'
      }`}
    >
      {isConnecting
        ? 'Connecting...'
        : wsStatus === 'RECONNECTING'
          ? 'Reconnecting...'
          : 'Disconnected — attempting to reconnect...'}
    </div>
  );
}
