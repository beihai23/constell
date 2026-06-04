// ---------------------------------------------------------------------------
// WSManager — WebSocket connection manager with heartbeat and reconnect.
// ---------------------------------------------------------------------------

import { EventBus } from "./event-bus.js";
import { WSStatus } from "./types.js";
import {
  encodeClientFrame,
  createClientMessage,
  generateRequestId,
} from "./codec.js";
import { ClientMessageType } from "./protobuf/gateway/v1/gateway_pb.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Events emitted by WSManager on the internal bus. */
export interface WSBusEvents {
  connected: () => void;
  disconnected: () => void;
  message: (data: ArrayBuffer) => void;
}

// ---------------------------------------------------------------------------
// Reconnect parameters
// ---------------------------------------------------------------------------

const BASE_DELAY_MS = 1000;
const MAX_DELAY_MS = 30000;
const HEARTBEAT_INTERVAL_MS = 30000;
const JITTER_FACTOR = 0.2; // ±20%

// ---------------------------------------------------------------------------
// WSManager
// ---------------------------------------------------------------------------

/** Factory type for creating WebSocket instances (injectable for testing). */
export type WebSocketFactory = (url: string) => WebSocket;

export class WSManager {
  private _status: WSStatus = WSStatus.Disconnected;
  private ws: WebSocket | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private intentionalClose = false;
  private subscribedChannels: string[] = [];
  private createWebSocket: WebSocketFactory;

  constructor(
    private url: string,
    private getToken: () => Promise<string>,
    private bus: EventBus<WSBusEvents>,
    wsFactory?: WebSocketFactory,
  ) {
    this.createWebSocket = wsFactory ?? ((u: string) => new WebSocket(u));
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  /** Current connection status. */
  get status(): WSStatus {
    return this._status;
  }

  /** Open a WebSocket connection. No-op if already connected/connecting. */
  connect(): void {
    if (
      this._status === WSStatus.Connecting ||
      this._status === WSStatus.Connected ||
      this._status === WSStatus.Reconnecting
    ) {
      return;
    }
    this.intentionalClose = false;
    this.doConnect();
  }

  /** Close the connection intentionally (no reconnect). */
  disconnect(): void {
    this.intentionalClose = true;
    this.cleanup();

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    this.setStatus(WSStatus.Disconnected);
  }

  /** Send raw binary data over the WebSocket. */
  send(data: Uint8Array): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(data);
    }
  }

  /** Send a heartbeat binary frame. */
  sendHeartbeat(requestId?: string): void {
    const id = requestId ?? generateRequestId();
    const msg = createClientMessage({
      type: ClientMessageType.HEARTBEAT,
      requestId: id,
    });
    const frame = encodeClientFrame(msg);
    this.send(frame);
  }

  /** Store channels to re-subscribe on reconnect. */
  setSubscribedChannels(channels: string[]): void {
    this.subscribedChannels = channels;
  }

  /** Get the currently tracked channels. */
  getSubscribedChannels(): string[] {
    return this.subscribedChannels;
  }

  // -------------------------------------------------------------------------
  // Internal
  // -------------------------------------------------------------------------

  private setStatus(status: WSStatus): void {
    this._status = status;
  }

  private async doConnect(): Promise<void> {
    this.setStatus(WSStatus.Connecting);

    let token: string;
    try {
      token = await this.getToken();
    } catch {
      // If token retrieval fails, schedule a reconnect.
      this.scheduleReconnect();
      return;
    }

    const url = `${this.url}?token=${encodeURIComponent(token)}`;
    const ws = this.createWebSocket(url);
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      this.ws = ws;
      this.setStatus(WSStatus.Connected);
      this.reconnectAttempt = 0;
      this.startHeartbeat();
      this.resubscribeChannels();
      this.bus.emit("connected");
    };

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        this.bus.emit("message", event.data);
      }
    };

    ws.onclose = () => {
      this.clearHeartbeat();
      this.nullifyHandlers(ws);

      if (this.ws === ws) {
        this.ws = null;
      }

      if (!this.intentionalClose) {
        this.bus.emit("disconnected");
        this.scheduleReconnect();
      }
    };

    ws.onerror = () => {
      // onclose fires after onerror, so reconnect logic lives in onclose.
    };
  }

  // -------------------------------------------------------------------------
  // Heartbeat
  // -------------------------------------------------------------------------

  private startHeartbeat(): void {
    this.clearHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      this.sendHeartbeat();
    }, HEARTBEAT_INTERVAL_MS);
  }

  private clearHeartbeat(): void {
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  // -------------------------------------------------------------------------
  // Reconnect
  // -------------------------------------------------------------------------

  private scheduleReconnect(): void {
    if (this.intentionalClose) return;

    this.setStatus(WSStatus.Reconnecting);

    const delay = this.computeDelay(this.reconnectAttempt);
    this.reconnectAttempt++;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.doConnect();
    }, delay);
  }

  private computeDelay(attempt: number): number {
    const base = Math.min(BASE_DELAY_MS * Math.pow(2, attempt), MAX_DELAY_MS);
    const jitter = base * JITTER_FACTOR;
    return base - jitter + Math.random() * jitter * 2;
  }

  // -------------------------------------------------------------------------
  // Cleanup
  // -------------------------------------------------------------------------

  private cleanup(): void {
    this.clearHeartbeat();

    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  /** Null out handlers on a WebSocket to prevent leaks. */
  private nullifyHandlers(ws: WebSocket): void {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.onerror = null;
  }

  // -------------------------------------------------------------------------
  // Resubscribe
  // -------------------------------------------------------------------------

  /** After connecting, re-send subscribe messages for tracked channels. */
  private resubscribeChannels(): void {
    for (const channelId of this.subscribedChannels) {
      const msg = createClientMessage({
        type: ClientMessageType.SUBSCRIBE_CHANNEL,
        subscribeChannelRequest: { channelId },
      });
      const frame = encodeClientFrame(msg);
      this.send(frame);
    }
  }
}
