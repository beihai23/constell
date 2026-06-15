import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { WSManager } from "../src/ws-manager.js";
import type { WSBusEvents } from "../src/ws-manager.js";
import { EventBus } from "../src/event-bus.js";
import { WSStatus } from "../src/types.js";
import { FRAME_HEADER_SIZE } from "../src/codec.js";
import { ClientMessageType } from "../src/protobuf/gateway/v1/gateway_pb.js";
import { fromBinary } from "@bufbuild/protobuf";
import { ClientMessageSchema } from "../src/protobuf/gateway/v1/gateway_pb.js";

// ---------------------------------------------------------------------------
// MockWebSocket — simulates the browser WebSocket API for testing.
// ---------------------------------------------------------------------------

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  binaryType: BinaryType = "arraybuffer";
  readyState: number = MockWebSocket.CONNECTING;
  url: string;

  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;

  sentData: Array<Uint8Array | string> = [];

  constructor(url: string) {
    this.url = url;
  }

  /** Simulate the server opening the connection. */
  simulateOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    if (this.onopen) {
      this.onopen(new Event("open"));
    }
  }

  /** Simulate the server closing the connection. */
  simulateClose(code = 1000, reason = ""): void {
    this.readyState = MockWebSocket.CLOSED;
    if (this.onclose) {
      this.onclose(new CloseEvent("close", { code, reason }));
    }
  }

  /** Simulate the server sending a message. */
  simulateMessage(data: ArrayBuffer): void {
    if (this.onmessage) {
      this.onmessage(new MessageEvent("message", { data }));
    }
  }

  /** Simulate a connection error. */
  simulateError(): void {
    if (this.onerror) {
      this.onerror(new Event("error"));
    }
  }

  /** Record data sent by the client. */
  send(data: Uint8Array | string): void {
    this.sentData.push(data);
  }

  close(_code?: number, _reason?: string): void {
    this.readyState = MockWebSocket.CLOSED;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a fresh WSManager with MockWebSocket injected via factory. */
function createManager(): {
  manager: WSManager;
  bus: EventBus<WSBusEvents>;
  getLastMock: () => MockWebSocket | undefined;
} {
  const bus = new EventBus<WSBusEvents>();
  const capturedMocks: MockWebSocket[] = [];

  const factory = (url: string) => {
    const ws = new MockWebSocket(url);
    capturedMocks.push(ws);
    return ws as unknown as WebSocket;
  };

  const manager = new WSManager(
    "ws://localhost:8081/ws",
    async () => "test-jwt-token",
    bus,
    factory,
  );

  const getLastMock = () => capturedMocks[capturedMocks.length - 1];

  return { manager, bus, getLastMock };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("WSManager", () => {
  // ---------------------------------------------------------------------------
  // 1. Connect and emit 'connected' event
  // ---------------------------------------------------------------------------
  it("connects and emits 'connected' event", async () => {
    const { manager, bus, getLastMock } = createManager();
    const connectedHandler = vi.fn();
    bus.on("connected", connectedHandler);

    manager.connect();

    // doConnect is async (awaits getToken), so mock is created after microtask
    await vi.waitFor(() => {
      expect(getLastMock()).toBeDefined();
    });

    const ws = getLastMock()!;
    expect(manager.status).toBe(WSStatus.Connecting);

    ws.simulateOpen();

    expect(manager.status).toBe(WSStatus.Connected);
    expect(connectedHandler).toHaveBeenCalledOnce();

    manager.disconnect();
  });

  // ---------------------------------------------------------------------------
  // 2. Send heartbeat binary frame (verify 4-byte prefix)
  // ---------------------------------------------------------------------------
  it("sends heartbeat binary frame with 4-byte prefix", async () => {
    const { manager, getLastMock } = createManager();

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());

    const ws = getLastMock()!;
    ws.simulateOpen();

    // Clear any resubscribe sends
    ws.sentData = [];

    manager.sendHeartbeat("hb-test-1");

    expect(ws.sentData.length).toBe(1);
    const sent = ws.sentData[0] as Uint8Array;

    // Must start with a 4-byte big-endian length prefix
    expect(sent.length).toBeGreaterThan(FRAME_HEADER_SIZE);
    const view = new DataView(sent.buffer, sent.byteOffset, sent.byteLength);
    const payloadLen = view.getUint32(0, false);
    expect(payloadLen).toBe(sent.length - FRAME_HEADER_SIZE);

    // Decode the payload and verify it is a HEARTBEAT message
    const payload = sent.slice(FRAME_HEADER_SIZE);
    const decoded = fromBinary(ClientMessageSchema, payload);
    expect(decoded.type).toBe(ClientMessageType.HEARTBEAT);
    expect(decoded.requestId).toBe("hb-test-1");

    manager.disconnect();
  });

  // ---------------------------------------------------------------------------
  // 3. Detect disconnect and emit 'disconnected'
  // ---------------------------------------------------------------------------
  it("detects disconnect and emits 'disconnected'", async () => {
    const { manager, bus, getLastMock } = createManager();
    const disconnectedHandler = vi.fn();
    bus.on("disconnected", disconnectedHandler);

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());

    const ws = getLastMock()!;
    ws.simulateOpen();

    expect(manager.status).toBe(WSStatus.Connected);

    // Simulate unexpected server close
    ws.simulateClose(1006, "abnormal");

    expect(disconnectedHandler).toHaveBeenCalledOnce();
    expect(manager.status).toBe(WSStatus.Reconnecting);
  });

  // ---------------------------------------------------------------------------
  // 4. disconnect() closes connection (no reconnect)
  // ---------------------------------------------------------------------------
  it("disconnect() closes connection and does not reconnect", async () => {
    const { manager, bus, getLastMock } = createManager();
    const disconnectedHandler = vi.fn();
    bus.on("disconnected", disconnectedHandler);

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());

    const ws = getLastMock()!;
    ws.simulateOpen();

    expect(manager.status).toBe(WSStatus.Connected);

    manager.disconnect();

    expect(manager.status).toBe(WSStatus.Disconnected);
    // disconnected event should NOT fire on intentional disconnect
    expect(disconnectedHandler).not.toHaveBeenCalled();
  });

  // ---------------------------------------------------------------------------
  // 5. Forward ArrayBuffer messages via 'message' event
  // ---------------------------------------------------------------------------
  it("forwards ArrayBuffer messages via 'message' event", async () => {
    const { manager, bus, getLastMock } = createManager();
    const messageHandler = vi.fn();
    bus.on("message", messageHandler);

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());

    const ws = getLastMock()!;
    ws.simulateOpen();

    const testData = new ArrayBuffer(8);
    new Uint8Array(testData).set([1, 2, 3, 4, 5, 6, 7, 8]);

    ws.simulateMessage(testData);

    expect(messageHandler).toHaveBeenCalledOnce();
    expect(messageHandler).toHaveBeenCalledWith(testData);

    manager.disconnect();
  });

  // ---------------------------------------------------------------------------
  // 6. Reconnect triggers on unexpected close
  // ---------------------------------------------------------------------------
  it("reconnects on unexpected close", async () => {
    const { manager, bus, getLastMock } = createManager();
    const connectedHandler = vi.fn();
    bus.on("connected", connectedHandler);

    // Use fake timers to control the reconnect delay
    vi.useFakeTimers();

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());

    const ws1 = getLastMock()!;
    ws1.simulateOpen();
    expect(connectedHandler).toHaveBeenCalledTimes(1);

    // Simulate unexpected close — should schedule reconnect
    ws1.simulateClose(1006);

    expect(manager.status).toBe(WSStatus.Reconnecting);

    // Advance timers past the first reconnect delay (base = 1000ms)
    vi.advanceTimersByTime(2000);

    // Let the async doConnect complete
    await vi.waitFor(() => expect(getLastMock()).not.toBe(ws1));

    const ws2 = getLastMock()!;
    expect(ws2).not.toBe(ws1);

    // Simulate the server opening the reconnected socket
    ws2.simulateOpen();

    expect(connectedHandler).toHaveBeenCalledTimes(2);
    expect(manager.status).toBe(WSStatus.Connected);

    vi.useRealTimers();
    manager.disconnect();
  });

  // ---------------------------------------------------------------------------
  // 7. connect() while Reconnecting reconnects immediately (no backoff wait)
  // ---------------------------------------------------------------------------
  it("connect() while Reconnecting reconnects immediately instead of waiting for backoff", async () => {
    const { manager, bus, getLastMock } = createManager();
    const connectedHandler = vi.fn();
    bus.on("connected", connectedHandler);

    vi.useFakeTimers();

    manager.connect();
    await vi.waitFor(() => expect(getLastMock()).toBeDefined());
    const ws1 = getLastMock()!;
    ws1.simulateOpen();
    expect(connectedHandler).toHaveBeenCalledTimes(1);

    // Unexpected close → a backoff reconnect is scheduled.
    ws1.simulateClose(1006);
    expect(manager.status).toBe(WSStatus.Reconnecting);

    // A fresh connect() — as happens after login/register stores a valid
    // token — must reconnect NOW, not wait for the backoff timer. doConnect
    // flips status to Connecting synchronously (before its first await), so
    // this assertion needs no timer advance.
    manager.connect();
    expect(manager.status).toBe(WSStatus.Connecting);

    // The token fetch resolves and a new socket is opened.
    await vi.waitFor(() => expect(getLastMock()).not.toBe(ws1));
    const ws2 = getLastMock()!;
    ws2.simulateOpen();
    expect(manager.status).toBe(WSStatus.Connected);
    expect(connectedHandler).toHaveBeenCalledTimes(2);

    vi.useRealTimers();
    manager.disconnect();
  });
});
