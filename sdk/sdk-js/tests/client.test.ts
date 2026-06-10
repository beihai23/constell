import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ConstellClient, type ClientEvents } from "../src/client.js";
import { AuthManager, type Storage } from "../src/auth.js";
import { EventBus } from "../src/event-bus.js";
import { WSStatus } from "../src/types.js";
import { ConstellError, NetworkError } from "../src/errors.js";
import {
  ClientMessageType,
  ServerEventType,
} from "../src/protobuf/gateway/v1/gateway_pb.js";
import {
  createClientMessage,
  encodeClientFrame,
  FRAME_HEADER_SIZE,
} from "../src/codec.js";
import { fromBinary } from "@bufbuild/protobuf";
import { ClientMessageSchema } from "../src/protobuf/gateway/v1/gateway_pb.js";

// ---------------------------------------------------------------------------
// MockWebSocket — reusable from ws-manager.test.ts pattern
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

  simulateOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    if (this.onopen) this.onopen(new Event("open"));
  }

  simulateClose(code = 1000, reason = ""): void {
    this.readyState = MockWebSocket.CLOSED;
    if (this.onclose) this.onclose(new CloseEvent("close", { code, reason }));
  }

  simulateMessage(data: ArrayBuffer): void {
    if (this.onmessage) {
      this.onmessage(new MessageEvent("message", { data }));
    }
  }

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

function createStorage(): Storage {
  const map = new Map<string, string>();
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => { map.set(key, value); },
    removeItem: (key: string) => { map.delete(key); },
  };
}

function fakeJWT(payload: Record<string, unknown>): string {
  const json = JSON.stringify(payload);
  const b64 = btoa(json)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `eyJhbGciOiJIUzI1NiJ9.${b64}.fakesignature`;
}

const API_URL = "http://localhost:8080";
const WS_URL = "ws://localhost:8081/ws";

function createTestClient(): {
  client: ConstellClient;
  storage: Storage;
  getLastMock: () => MockWebSocket | undefined;
} {
  const storage = createStorage();
  // Pre-store a valid JWT
  const token = fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 3600 });
  storage.setItem("constell_access_token", token);
  storage.setItem("constell_refresh_token", fakeJWT({ sub: "u1", exp: Date.now() / 1000 + 86400 }));

  const capturedMocks: MockWebSocket[] = [];
  const factory = (url: string) => {
    const ws = new MockWebSocket(url);
    capturedMocks.push(ws);
    return ws as unknown as WebSocket;
  };

  const client = new ConstellClient(
    { apiUrl: API_URL, wsUrl: WS_URL },
    storage,
    factory as any,
  );

  const getLastMock = () => capturedMocks[capturedMocks.length - 1];

  return { client, storage, getLastMock };
}

/** Connect the WS and return the mock. */
async function connectWS(client: ConstellClient, getLastMock: () => MockWebSocket | undefined): Promise<MockWebSocket> {
  client.connect();
  await vi.waitFor(() => expect(getLastMock()).toBeDefined());
  const ws = getLastMock()!;
  ws.simulateOpen();
  return ws;
}

/** Encode a ServerEvent-like protobuf message into a binary frame.
 *  We use the ServerEventSchema to create real protobuf frames for testing. */
import { create, toBinary } from "@bufbuild/protobuf";
import { ServerEventSchema } from "../src/protobuf/gateway/v1/gateway_pb.js";

function encodeServerEventFrame(eventInit: Parameters<typeof create<typeof ServerEventSchema>>[1]): ArrayBuffer {
  const event = create(ServerEventSchema, eventInit);
  const payload = toBinary(ServerEventSchema, event);
  const frame = new Uint8Array(FRAME_HEADER_SIZE + payload.length);
  const view = new DataView(frame.buffer, frame.byteOffset, frame.byteLength);
  view.setUint32(0, payload.length, false);
  frame.set(payload, FRAME_HEADER_SIZE);
  return frame.buffer as ArrayBuffer;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ConstellClient", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Connection
  // ---------------------------------------------------------------------------
  describe("connection", () => {
    it("connect() opens WebSocket connection", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);
      expect(client.status).toBe(WSStatus.Connected);
      client.disconnect();
    });

    it("disconnect() closes WebSocket connection", async () => {
      const { client, getLastMock } = createTestClient();
      await connectWS(client, getLastMock);
      client.disconnect();
      expect(client.status).toBe(WSStatus.Disconnected);
    });

    it("status reflects WS connection state", () => {
      const { client } = createTestClient();
      expect(client.status).toBe(WSStatus.Disconnected);
    });
  });

  // ---------------------------------------------------------------------------
  // Auth
  // ---------------------------------------------------------------------------
  describe("auth", () => {
    it("login() calls auth.login and then connects", async () => {
      const { client, storage, getLastMock } = createTestClient();

      const mockUser = { id: "u1", email: "a@b.com", nickname: "A", avatarUrl: "", statusMessage: "", createdAt: 0, updatedAt: 0 };
      vi.spyOn(client.auth, "login").mockResolvedValueOnce(mockUser);

      const user = await client.login("a@b.com", "password");

      expect(user).toEqual(mockUser);
      expect(client.auth.login).toHaveBeenCalledWith("a@b.com", "password");

      // Should have called connect()
      await vi.waitFor(() => expect(getLastMock()).toBeDefined());
      client.disconnect();
    });

    it("register() calls auth.register and then connects", async () => {
      const { client, getLastMock } = createTestClient();

      const mockUser = { id: "u2", email: "b@c.com", nickname: "B", avatarUrl: "", statusMessage: "", createdAt: 0, updatedAt: 0 };
      vi.spyOn(client.auth, "register").mockResolvedValueOnce(mockUser);

      const user = await client.register("b", "b@c.com", "password");

      expect(user).toEqual(mockUser);
      expect(client.auth.register).toHaveBeenCalledWith("b", "b@c.com", "password");
      await vi.waitFor(() => expect(getLastMock()).toBeDefined());
      client.disconnect();
    });

    it("logout() disconnects and clears tokens", async () => {
      const { client, getLastMock } = createTestClient();
      await connectWS(client, getLastMock);

      vi.spyOn(client.auth, "logout");

      client.logout();

      expect(client.status).toBe(WSStatus.Disconnected);
      expect(client.auth.logout).toHaveBeenCalled();
    });

    it("initFromStorage() delegates to auth", () => {
      const { client, storage } = createTestClient();
      const user = client.initFromStorage();
      // The fake JWT has sub: "u1"
      expect(user).not.toBeNull();
      expect(user!.id).toBe("u1");
    });
  });

  // ---------------------------------------------------------------------------
  // WS event routing
  // ---------------------------------------------------------------------------
  describe("WS event routing", () => {
    it("emits dm_received when DM_RECEIVED event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("dm_received", handler);

      const frame = encodeServerEventFrame({
        type: ServerEventType.DM_RECEIVED,
        requestId: "",
        dmReceivedEvent: {
          messageId: "msg1",
          senderId: "user2",
          senderNickname: "Alice",
          content: "Hello!",
          createdAt: BigInt(1700000000000),
          attachments: [],
        },
      });

      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith({
        id: "msg1",
        conversationId: "",
        senderId: "user2",
        content: "Hello!",
        createdAt: 1700000000000,
        attachments: [],
      });

      client.disconnect();
    });

    it("emits channel_message when CHANNEL_MESSAGE_RECEIVED event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("channel_message", handler);

      const frame = encodeServerEventFrame({
        type: ServerEventType.CHANNEL_MESSAGE_RECEIVED,
        requestId: "",
        channelMessageEvent: {
          messageId: "msg2",
          channelId: "ch1",
          senderId: "user3",
          senderNickname: "Bob",
          content: "Channel hello!",
          createdAt: BigInt(1700000001000),
          attachments: [],
        },
      });

      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith({
        id: "msg2",
        channelId: "ch1",
        authorId: "user3",
        content: "Channel hello!",
        createdAt: 1700000001000,
        updatedAt: 1700000001000,
        attachments: [],
      });

      client.disconnect();
    });

    it("emits user_online when USER_ONLINE event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("user_online", handler);

      const frame = encodeServerEventFrame({
        type: ServerEventType.USER_ONLINE,
        requestId: "",
        userOnlineEvent: { userId: "user4" },
      });

      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith({ userId: "user4" });

      client.disconnect();
    });

    it("emits user_offline when USER_OFFLINE event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("user_offline", handler);

      const frame = encodeServerEventFrame({
        type: ServerEventType.USER_OFFLINE,
        requestId: "",
        userOfflineEvent: { userId: "user5" },
      });

      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith({ userId: "user5" });

      client.disconnect();
    });

    it("emits notification when NOTIFICATION event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("notification", handler);

      const frame = encodeServerEventFrame({
        type: ServerEventType.NOTIFICATION,
        requestId: "",
        notificationEvent: {
          notificationType: "DM",
          sourceId: "src1",
          communityId: "comm1",
          senderId: "user6",
          senderNickname: "Charlie",
          preview: "You have a new message",
          createdAt: BigInt(1700000002000),
        },
      });

      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith({
        notificationType: "DM",
        sourceId: "src1",
        communityId: "comm1",
        senderId: "user6",
        senderNickname: "Charlie",
        preview: "You have a new message",
        createdAt: 1700000002000,
      });

      client.disconnect();
    });

    it("emits connected/disconnected events from WS bus", async () => {
      const { client, getLastMock } = createTestClient();

      const connectedHandler = vi.fn();
      const disconnectedHandler = vi.fn();
      client.on("connected", connectedHandler);
      client.on("disconnected", disconnectedHandler);

      const ws = await connectWS(client, getLastMock);

      expect(connectedHandler).toHaveBeenCalledOnce();

      // Unintentional close
      ws.simulateClose(1006, "abnormal");

      expect(disconnectedHandler).toHaveBeenCalledOnce();

      client.disconnect();
    });
  });

  // ---------------------------------------------------------------------------
  // ACK / pending request tracking
  // ---------------------------------------------------------------------------
  describe("ACK tracking", () => {
    it("sendDM resolves when ACK arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);
      ws.sentData = [];

      const sendPromise = client.sendDM("user2", "Hello!");

      // Should have sent exactly 1 frame
      expect(ws.sentData.length).toBe(1);
      const sent = ws.sentData[0] as Uint8Array;

      // Decode to find requestId
      const payload = sent.slice(FRAME_HEADER_SIZE);
      const decoded = fromBinary(ClientMessageSchema, payload);
      expect(decoded.type).toBe(ClientMessageType.SEND_DM);

      // Simulate ACK from server
      const ackFrame = encodeServerEventFrame({
        type: ServerEventType.ACK,
        requestId: decoded.requestId,
      });
      ws.simulateMessage(ackFrame);

      // Should resolve without error
      await expect(sendPromise).resolves.toBeUndefined();

      client.disconnect();
    });

    it("sendDM rejects on timeout", async () => {
      vi.useFakeTimers();

      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      const sendPromise = client.sendDM("user2", "Hello!");

      // Advance past the 5s timeout
      vi.advanceTimersByTime(6_000);

      await expect(sendPromise).rejects.toThrow("Request timed out");

      vi.useRealTimers();
      client.disconnect();
    });

    it("sendChannelMessage rejects when ERROR event arrives", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);
      ws.sentData = [];

      const sendPromise = client.sendChannelMessage("ch1", "Hello!");

      // Decode to find requestId
      const sent = ws.sentData[0] as Uint8Array;
      const payload = sent.slice(FRAME_HEADER_SIZE);
      const decoded = fromBinary(ClientMessageSchema, payload);
      expect(decoded.type).toBe(ClientMessageType.SEND_CHANNEL_MESSAGE);

      // Simulate ERROR from server
      const errorFrame = encodeServerEventFrame({
        type: ServerEventType.ERROR,
        requestId: decoded.requestId,
        errorEvent: {
          code: "FORBIDDEN",
          message: "Not allowed to post here",
        },
      });
      ws.simulateMessage(errorFrame);

      await expect(sendPromise).rejects.toThrow("Not allowed to post here");

      client.disconnect();
    });
  });

  // ---------------------------------------------------------------------------
  // Channel subscribe / unsubscribe
  // ---------------------------------------------------------------------------
  describe("channel subscribe/unsubscribe", () => {
    it("subscribeChannel sends SUBSCRIBE_CHANNEL message", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);
      ws.sentData = [];

      client.subscribeChannel("ch1");

      expect(ws.sentData.length).toBe(1);
      const sent = ws.sentData[0] as Uint8Array;
      const payload = sent.slice(FRAME_HEADER_SIZE);
      const decoded = fromBinary(ClientMessageSchema, payload);
      expect(decoded.type).toBe(ClientMessageType.SUBSCRIBE_CHANNEL);
      expect(decoded.subscribeChannelRequest?.channelId).toBe("ch1");

      // Should track for resubscription
      expect(client.ws.getSubscribedChannels()).toContain("ch1");

      client.disconnect();
    });

    it("unsubscribeChannel sends UNSUBSCRIBE_CHANNEL message", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);
      ws.sentData = [];

      // Subscribe first
      client.subscribeChannel("ch1");
      expect(client.ws.getSubscribedChannels()).toContain("ch1");

      // Now unsubscribe
      client.unsubscribeChannel("ch1");

      const sentFrames = ws.sentData as Uint8Array[];
      // Last frame should be unsubscribe
      const lastPayload = sentFrames[sentFrames.length - 1].slice(FRAME_HEADER_SIZE);
      const decoded = fromBinary(ClientMessageSchema, lastPayload);
      expect(decoded.type).toBe(ClientMessageType.UNSUBSCRIBE_CHANNEL);

      expect(client.ws.getSubscribedChannels()).not.toContain("ch1");

      client.disconnect();
    });

    it("does not duplicate channel in tracking on double subscribe", async () => {
      const { client, getLastMock } = createTestClient();
      const ws = await connectWS(client, getLastMock);

      client.subscribeChannel("ch1");
      client.subscribeChannel("ch1");

      expect(client.ws.getSubscribedChannels().filter((id) => id === "ch1").length).toBe(1);

      client.disconnect();
    });
  });

  // ---------------------------------------------------------------------------
  // REST delegation
  // ---------------------------------------------------------------------------
  describe("REST methods", () => {
    let client: ConstellClient;
    let storage: Storage;

    beforeEach(() => {
      const env = createTestClient();
      client = env.client;
      storage = env.storage;
    });

    afterEach(() => {
      client.disconnect();
    });

    // --- DM History ---
    it("getDMHistory calls REST with correct path and maps response", async () => {
      const restResponse = {
        items: [
          {
            id: "m1",
            conversation_id: "conv1",
            sender_id: "u2",
            content: "Hi",
            created_at: 1700000000000,
            attachments: [],
          },
        ],
        has_more: false,
        next_cursor: "",
        total_count: 1,
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getDMHistory("u2", { limit: 20 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/dm/history/u2?limit=20");
      expect(result.items[0]).toEqual({
        id: "m1",
        conversationId: "conv1",
        senderId: "u2",
        content: "Hi",
        createdAt: 1700000000000,
        attachments: [],
      });
      expect(result.hasMore).toBe(false);
    });

    // --- DM Conversations ---
    it("getDMConversations calls REST and maps response", async () => {
      const restResponse = {
        items: [
          {
            id: "conv1",
            peer: { id: "u2", nickname: "Alice", avatar_url: "http://avatar" },
            last_message: {
              id: "m1",
              conversation_id: "conv1",
              sender_id: "u2",
              content: "Hey",
              created_at: 1700000000000,
              attachments: [],
            },
            unread_count: 3,
          },
        ],
        has_more: true,
        next_cursor: "cursor123",
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getDMConversations({ cursor: "abc" });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/dm/conversations?cursor=abc");
      expect(result.items[0].peer.nickname).toBe("Alice");
      expect(result.items[0].unreadCount).toBe(3);
      expect(result.hasMore).toBe(true);
      expect(result.nextCursor).toBe("cursor123");
    });

    // --- Channel History ---
    it("getChannelHistory calls REST with correct path and maps response", async () => {
      const restResponse = {
        items: [
          {
            id: "m1",
            channel_id: "ch1",
            author_id: "u1",
            content: "Hello channel",
            created_at: 1700000000000,
            updated_at: 1700000000000,
            attachments: [],
          },
        ],
        has_more: false,
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getChannelHistory("ch1", { limit: 50 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/channels/ch1/messages?limit=50");
      expect(result.items[0]).toEqual({
        id: "m1",
        channelId: "ch1",
        authorId: "u1",
        content: "Hello channel",
        createdAt: 1700000000000,
        updatedAt: 1700000000000,
        attachments: [],
      });
    });

    // --- Communities ---
    it("listCommunities maps response", async () => {
      const restResponse = [
        { id: "s1", name: "Community 1", description: "Desc", icon_url: "", owner_id: "u1", created_at: 0, updated_at: 0 },
      ];

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.listCommunities();

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/servers");
      expect(result[0]).toEqual({
        id: "s1",
        name: "Community 1",
        description: "Desc",
        iconUrl: "",
        ownerId: "u1",
        createdAt: 0,
        updatedAt: 0,
      });
    });

    it("createCommunity sends POST", async () => {
      const restResponse = { id: "s2", name: "New", description: "New desc", icon_url: "", owner_id: "u1", created_at: 0, updated_at: 0 };

      vi.spyOn(client.rest, "post").mockResolvedValueOnce(restResponse);

      const result = await client.createCommunity("New", "New desc");

      expect(client.rest.post).toHaveBeenCalledWith("/api/v1/servers", { name: "New", description: "New desc" });
      expect(result.name).toBe("New");
    });

    it("getChannels maps response", async () => {
      const restResponse = [
        { id: "ch1", community_id: "s1", name: "general", topic: "", type: 1, position: 0, created_at: 0, updated_at: 0 },
      ];

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getChannels("s1");

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/servers/s1/channels");
      expect(result[0].name).toBe("general");
      expect(result[0].communityId).toBe("s1");
    });

    it("getMembers maps paginated response", async () => {
      const restResponse = {
        items: [
          { community_id: "s1", user_id: "u2", nickname: "Alice", role_ids: ["r1"], joined_at: 0 },
        ],
        has_more: false,
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getMembers("s1", { limit: 10 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/servers/s1/members?limit=10");
      expect(result.items[0].userId).toBe("u2");
    });

    it("addMember sends POST with user_id", async () => {
      const restResponse = { community_id: "s1", user_id: "u3", nickname: "NewMember", role_ids: [], joined_at: 0 };

      vi.spyOn(client.rest, "post").mockResolvedValueOnce(restResponse);

      const result = await client.addMember("s1", "u3");

      expect(client.rest.post).toHaveBeenCalledWith("/api/v1/servers/s1/members", { user_id: "u3" });
      expect(result.userId).toBe("u3");
    });

    // --- Users ---
    it("getUser maps response", async () => {
      const restResponse = { id: "u2", email: "a@b.com", nickname: "Alice", avatar_url: "", status_message: "Hi", created_at: 0, updated_at: 0 };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getUser("u2");

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/users/u2");
      expect(result.nickname).toBe("Alice");
      expect(result.statusMessage).toBe("Hi");
    });

    it("listFriends maps paginated response", async () => {
      const restResponse = {
        items: [
          { id: "u2", email: "a@b.com", nickname: "Bob", avatar_url: "", status_message: "", created_at: 0, updated_at: 0 },
        ],
        has_more: false,
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.listFriends({ limit: 20 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/users/me/friends?limit=20");
      expect(result.items[0].nickname).toBe("Bob");
    });

    // --- Files ---
    it("uploadFile sends FormData and maps response", async () => {
      const restResponse = { id: "f1", filename: "test.txt", content_type: "text/plain", size: 100, url: "http://minio/file", thumbnail_url: "", created_at: 0 };

      vi.spyOn(client.rest, "upload").mockResolvedValueOnce(restResponse);

      const data = new Uint8Array([1, 2, 3]);
      const result = await client.uploadFile(data, "test.txt", "text/plain");

      expect(client.rest.upload).toHaveBeenCalledWith("/api/v1/files/upload", expect.any(FormData));
      expect(result.id).toBe("f1");
      expect(result.contentType).toBe("text/plain");
    });

    it("getFileURL calls correct endpoint", async () => {
      vi.spyOn(client.rest, "get").mockResolvedValueOnce({ url: "http://minio/file" });

      const result = await client.getFileURL("f1");

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/files/f1/url");
      expect(result.url).toBe("http://minio/file");
    });

    // --- Search ---
    it("search calls correct endpoint and maps response", async () => {
      const restResponse = {
        users: [{ id: "u2", nickname: "Alice", avatar_url: "", relevance: 0.9 }],
        messages: [{ id: "m1", channel_id: "ch1", community_id: "s1", author_id: "u1", content: "hello", created_at: 0, relevance: 0.8 }],
        dm_messages: [{ id: "dm1", conversation_id: "conv1", peer_id: "u2", content: "hi", created_at: 0, relevance: 0.7 }],
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.search("hello", { limit: 10 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/search?q=hello&limit=10");
      expect(result.users[0].nickname).toBe("Alice");
      expect(result.messages[0].channelId).toBe("ch1");
      expect(result.dmMessages[0].peerId).toBe("u2");
    });

    // --- Notify ---
    it("getUnreadCounts maps response", async () => {
      const restResponse = {
        dm_total: 5,
        dm_conversations: [{ conversation_id: "conv1", peer_id: "u2", count: 3 }],
        channel_total: 2,
        channels: [{ channel_id: "ch1", community_id: "s1", count: 2 }],
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.getUnreadCounts();

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/notify/unread");
      expect(result.dmTotal).toBe(5);
      expect(result.dmConversations[0].conversationId).toBe("conv1");
      expect(result.channelTotal).toBe(2);
      expect(result.channels[0].channelId).toBe("ch1");
    });

    it("markDMRead sends POST", async () => {
      vi.spyOn(client.rest, "post").mockResolvedValueOnce(undefined);

      await client.markDMRead("conv1");

      expect(client.rest.post).toHaveBeenCalledWith("/api/v1/notify/dm/conv1/read");
    });

    it("markChannelRead sends POST", async () => {
      vi.spyOn(client.rest, "post").mockResolvedValueOnce(undefined);

      await client.markChannelRead("ch1");

      expect(client.rest.post).toHaveBeenCalledWith("/api/v1/notify/channel/ch1/read");
    });
  });

  // ---------------------------------------------------------------------------
  // Event subscription
  // ---------------------------------------------------------------------------
  describe("event subscription", () => {
    it("on/off delegates to bus", async () => {
      const { client, getLastMock } = createTestClient();
      await connectWS(client, getLastMock);

      const handler = vi.fn();
      client.on("dm_received", handler);

      // Emit a DM event
      const frame = encodeServerEventFrame({
        type: ServerEventType.DM_RECEIVED,
        requestId: "",
        dmReceivedEvent: {
          messageId: "m1",
          senderId: "u2",
          senderNickname: "Alice",
          content: "Hi",
          createdAt: BigInt(0),
          attachments: [],
        },
      });
      const ws = getLastMock()!;
      ws.simulateMessage(frame);

      expect(handler).toHaveBeenCalledOnce();

      // Unsubscribe
      client.off("dm_received", handler);

      // Send another event
      ws.simulateMessage(frame);
      expect(handler).toHaveBeenCalledOnce(); // should NOT have been called again

      client.disconnect();
    });

    it("removeAllListeners clears all handlers", async () => {
      const { client, getLastMock } = createTestClient();
      await connectWS(client, getLastMock);

      const handler1 = vi.fn();
      const handler2 = vi.fn();
      client.on("dm_received", handler1);
      client.on("user_online", handler2);

      client.removeAllListeners();

      // Emit events — neither handler should fire
      const ws = getLastMock()!;

      ws.simulateMessage(encodeServerEventFrame({
        type: ServerEventType.DM_RECEIVED,
        requestId: "",
        dmReceivedEvent: {
          messageId: "m1", senderId: "u2", senderNickname: "A", content: "", createdAt: BigInt(0), attachments: [],
        },
      }));

      ws.simulateMessage(encodeServerEventFrame({
        type: ServerEventType.USER_ONLINE,
        requestId: "",
        userOnlineEvent: { userId: "u2" },
      }));

      expect(handler1).not.toHaveBeenCalled();
      expect(handler2).not.toHaveBeenCalled();

      client.disconnect();
    });
  });

  // ---------------------------------------------------------------------------
  // Sub-managers are accessible
  // ---------------------------------------------------------------------------
  describe("sub-manager access", () => {
    it("exposes auth, rest, ws as readonly properties", () => {
      const { client } = createTestClient();
      expect(client.auth).toBeInstanceOf(AuthManager);
      expect(client.rest).toBeDefined();
      expect(client.ws).toBeDefined();
    });
  });
});
