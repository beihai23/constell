/**
 * ConstellClient — unified API that wires together AuthManager, RESTClient,
 * WSManager, and EventBus into a single entry point for consumers.
 *
 * Handles:
 * - Authentication (login, register, logout, restore from storage)
 * - REST API calls (communities, channels, users, files, search, notify)
 * - WebSocket messaging (send DM / channel message, subscribe / unsubscribe)
 * - Server event routing (DM received, channel message, presence, notifications)
 * - ACK / pending-request tracking with timeout
 */

import { AuthManager, type Storage } from "./auth.js";
import { RESTClient } from "./rest-client.js";
import { WSManager, type WSBusEvents, type WebSocketFactory } from "./ws-manager.js";
import { EventBus } from "./event-bus.js";
import {
  createClientMessage,
  encodeClientFrame,
  readServerEvent,
  generateRequestId,
} from "./codec.js";
import {
  ClientMessageType,
  ServerEventType,
} from "./protobuf/gateway/v1/gateway_pb.js";
import type {
  ClientConfig,
  WSStatus,
  User,
  DMMessage,
  DMConversation,
  ChannelMessage,
  Channel,
  Community,
  Member,
  FileInfo,
  SearchResults,
  UnreadCounts,
  NotificationEvent,
  Attachment,
  PageOptions,
  PageResult,
} from "./types.js";
import { ConstellError, NetworkError } from "./errors.js";

// ---------------------------------------------------------------------------
// Public event map — events consumers can subscribe to via client.on(...)
// ---------------------------------------------------------------------------

export interface ClientEvents {
  [key: string]: (...args: any[]) => void;
  /** A direct message was received. */
  dm_received: (msg: DMMessage) => void;
  /** A channel message was received. */
  channel_message: (msg: ChannelMessage) => void;
  /** A user came online. */
  user_online: (data: { userId: string }) => void;
  /** A user went offline. */
  user_offline: (data: { userId: string }) => void;
  /** A push notification from the server. */
  notification: (event: NotificationEvent) => void;
  /** WebSocket connected (or reconnected). */
  connected: () => void;
  /** WebSocket disconnected (unintentional). */
  disconnected: () => void;
}

// ---------------------------------------------------------------------------
// Pending request (for ACK / ERROR correlation)
// ---------------------------------------------------------------------------

interface PendingRequest {
  resolve: (value: unknown) => void;
  reject: (reason: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ACK_TIMEOUT_MS = 5_000;

// ---------------------------------------------------------------------------
// REST response shape helpers (backend returns snake_case JSON)
// ---------------------------------------------------------------------------

/** Map a snake_case REST attachment to camelCase SDK Attachment. */
function mapAttachment(a: Record<string, unknown>): Attachment {
  return {
    id: a.id as string,
    fileId: (a.file_id ?? a.fileId) as string,
    filename: a.filename as string,
    contentType: (a.content_type ?? a.contentType) as string,
    size: a.size as number,
    url: a.url as string,
    thumbnailUrl: (a.thumbnail_url ?? a.thumbnailUrl) as string,
  };
}

/** Map a snake_case REST DM message to camelCase SDK DMMessage. */
function mapDMMessage(m: Record<string, unknown>): DMMessage {
  return {
    id: m.id as string,
    conversationId: (m.conversation_id ?? m.conversationId) as string,
    senderId: (m.sender_id ?? m.senderId) as string,
    content: m.content as string,
    createdAt: (m.created_at ?? m.createdAt) as number,
    attachments: ((m.attachments ?? []) as Record<string, unknown>[]).map(mapAttachment),
  };
}

/** Map a snake_case REST channel message to camelCase SDK ChannelMessage. */
function mapChannelMessage(m: Record<string, unknown>): ChannelMessage {
  return {
    id: m.id as string,
    channelId: (m.channel_id ?? m.channelId) as string,
    authorId: (m.author_id ?? m.authorId) as string,
    content: m.content as string,
    createdAt: (m.created_at ?? m.createdAt) as number,
    updatedAt: (m.updated_at ?? m.updatedAt) as number,
    attachments: ((m.attachments ?? []) as Record<string, unknown>[]).map(mapAttachment),
  };
}

/** Map a snake_case REST channel to camelCase SDK Channel. */
function mapChannel(c: Record<string, unknown>): Channel {
  return {
    id: c.id as string,
    communityId: (c.community_id ?? c.communityId) as string,
    name: c.name as string,
    topic: (c.topic ?? "") as string,
    type: c.type as number,
    position: (c.position ?? 0) as number,
    createdAt: (c.created_at ?? c.createdAt) as number,
    updatedAt: (c.updated_at ?? c.updatedAt) as number,
  };
}

/** Map a snake_case REST community to camelCase SDK Community. */
function mapCommunity(c: Record<string, unknown>): Community {
  return {
    id: c.id as string,
    name: c.name as string,
    description: (c.description ?? "") as string,
    iconUrl: (c.icon_url ?? c.iconUrl ?? "") as string,
    ownerId: (c.owner_id ?? c.ownerId) as string,
    createdAt: (c.created_at ?? c.createdAt) as number,
    updatedAt: (c.updated_at ?? c.updatedAt) as number,
  };
}

/** Map a snake_case REST member to camelCase SDK Member. */
function mapMember(m: Record<string, unknown>): Member {
  return {
    communityId: (m.community_id ?? m.communityId) as string,
    userId: (m.user_id ?? m.userId) as string,
    nickname: m.nickname as string,
    roleIds: (m.role_ids ?? m.roleIds ?? []) as string[],
    joinedAt: (m.joined_at ?? m.joinedAt) as number,
  };
}

/** Map a snake_case REST DM conversation to camelCase SDK DMConversation. */
function mapDMConversation(c: Record<string, unknown>): DMConversation {
  return {
    id: c.id as string,
    peerId: (c.peer_id ?? c.peerId ?? "") as string,
    createdAt: (c.created_at ?? c.createdAt ?? 0) as number,
  };
}

/** Map a snake_case REST file info to camelCase SDK FileInfo. */
function mapFileInfo(f: Record<string, unknown>): FileInfo {
  return {
    id: f.id as string,
    filename: f.filename as string,
    contentType: (f.content_type ?? f.contentType) as string,
    size: f.size as number,
    url: f.url as string,
    thumbnailUrl: (f.thumbnail_url ?? f.thumbnailUrl ?? "") as string,
    createdAt: (f.created_at ?? f.createdAt) as number,
  };
}

/** Build query string from PageOptions. */
function buildPageQuery(opts?: PageOptions): string {
  if (!opts) return "";
  const params: string[] = [];
  if (opts.limit !== undefined) params.push(`limit=${opts.limit}`);
  if (opts.offset !== undefined) params.push(`offset=${opts.offset}`);
  if (opts.cursor !== undefined) params.push(`cursor=${encodeURIComponent(opts.cursor)}`);
  return params.length > 0 ? `?${params.join("&")}` : "";
}

/** Build a PageResult from a raw REST response. */
function buildPageResult<T>(
  raw: Record<string, unknown>,
  mapper: (item: Record<string, unknown>) => T,
): PageResult<T> {
  const items = ((raw.items ?? raw.messages ?? raw.conversations ?? raw.members ?? raw.channels ?? []) as Record<string, unknown>[]).map(mapper);
  return {
    items,
    hasMore: (raw.has_more ?? raw.hasMore ?? false) as boolean,
    nextCursor: (raw.next_cursor ?? raw.nextCursor) as string | undefined,
    totalCount: (raw.total_count ?? raw.totalCount) as number | undefined,
  };
}

// ---------------------------------------------------------------------------
// ConstellClient
// ---------------------------------------------------------------------------

export class ConstellClient {
  // Sub-managers (exposed read-only for advanced usage)
  readonly auth: AuthManager;
  readonly rest: RESTClient;
  readonly ws: WSManager;

  /** Public event bus for consumers. */
  private readonly bus: EventBus<ClientEvents>;

  /** Internal WS bus (receives raw frames from WSManager). */
  private readonly wsBus: EventBus<WSBusEvents>;

  /** Pending requests awaiting ACK / ERROR from the server. */
  private readonly pendingRequests = new Map<string, PendingRequest>();

  constructor(config: ClientConfig, storage?: Storage, wsFactory?: WebSocketFactory) {
    // Create the public event bus
    this.bus = new EventBus<ClientEvents>();

    // Create internal WS bus
    this.wsBus = new EventBus<WSBusEvents>();

    // Create sub-managers
    this.auth = new AuthManager(config.apiUrl, storage);
    this.rest = new RESTClient(this.auth, config.apiUrl);
    this.ws = new WSManager(
      config.wsUrl,
      () => this.auth.getValidToken(),
      this.wsBus,
      wsFactory,
    );

    // Wire WS events → client event routing
    this.wsBus.on("message", (data: ArrayBuffer) => {
      this.handleWSMessage(data);
    });

    this.wsBus.on("connected", () => {
      this.bus.emit("connected");
    });

    this.wsBus.on("disconnected", () => {
      this.bus.emit("disconnected");
    });
  }

  // -------------------------------------------------------------------------
  // Connection
  // -------------------------------------------------------------------------

  /** Open the WebSocket connection. No-op if already connected. */
  connect(): void {
    this.ws.connect();
  }

  /** Close the WebSocket connection (intentional — no reconnect). */
  disconnect(): void {
    this.ws.disconnect();
  }

  /** Current WebSocket connection status. */
  get status(): WSStatus {
    return this.ws.status;
  }

  // -------------------------------------------------------------------------
  // Auth
  // -------------------------------------------------------------------------

  /**
   * Log in with email and password, then open the WebSocket connection.
   * Returns the authenticated user.
   */
  async login(email: string, password: string): Promise<User> {
    const minimal = await this.auth.login(email, password);
    this.connect();
    // The auth response only carries user_id + tokens; fetch the full profile
    // (nickname, email, avatar) so consumers don't see an empty user.
    return this.enrichUser(minimal);
  }

  /**
   * Register a new account, then open the WebSocket connection.
   * Returns the newly created user.
   */
  async register(username: string, email: string, password: string): Promise<User> {
    const minimal = await this.auth.register(username, email, password);
    this.connect();
    return this.enrichUser(minimal);
  }

  /**
   * Fetch the full profile for a minimal (token-derived) user. Falls back to
   * the given user if the lookup fails, so a flaky profile fetch never blocks
   * an otherwise-successful login.
   */
  private async enrichUser(user: User): Promise<User> {
    if (!user?.id) return user;
    try {
      return await this.getUser(user.id);
    } catch {
      return user;
    }
  }

  /** Disconnect and clear stored tokens. */
  logout(): void {
    this.disconnect();
    this.auth.logout();
  }

  /**
   * Attempt to restore the authenticated user from stored tokens.
   * Returns the User if tokens are valid, or null. The returned user is
   * derived from the JWT and may lack a nickname; call `refreshProfile` to
   * fetch the full profile asynchronously.
   */
  initFromStorage(): User | null {
    return this.auth.initFromStorage();
  }

  /** Fetch the full profile for the current user and return it. */
  async refreshProfile(): Promise<User | null> {
    const minimal = this.auth.initFromStorage();
    if (!minimal?.id) return null;
    return this.enrichUser(minimal);
  }

  // -------------------------------------------------------------------------
  // DM methods
  // -------------------------------------------------------------------------

  /**
   * Send a direct message via WebSocket.
   * Returns a promise that resolves when the server ACKs (or rejects on timeout / error).
   */
  sendDM(receiverId: string, content: string, fileIds?: string[]): Promise<unknown> {
    const requestId = generateRequestId();
    const msg = createClientMessage({
      type: ClientMessageType.SEND_DM,
      requestId,
      sendDmRequest: {
        receiverId,
        content,
        fileIds: fileIds ?? [],
      },
    });
    return this.sendWithAck(requestId, encodeClientFrame(msg));
  }

  /** Fetch DM history with a peer (paginated, REST). */
  async getDMHistory(peerId: string, opts?: PageOptions): Promise<PageResult<DMMessage>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/dm/history/${peerId}${query}`,
    );
    return buildPageResult(raw, mapDMMessage);
  }

  /** Fetch DM conversations list (paginated, REST). */
  async getDMConversations(opts?: PageOptions): Promise<PageResult<DMConversation>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/dm/conversations${query}`,
    );
    return buildPageResult(raw, mapDMConversation);
  }

  // -------------------------------------------------------------------------
  // Channel message methods
  // -------------------------------------------------------------------------

  /**
   * Send a channel message via WebSocket.
   * Returns a promise that resolves on ACK, rejects on timeout / error.
   */
  sendChannelMessage(channelId: string, content: string, fileIds?: string[]): Promise<unknown> {
    const requestId = generateRequestId();
    const msg = createClientMessage({
      type: ClientMessageType.SEND_CHANNEL_MESSAGE,
      requestId,
      sendChannelMessageRequest: {
        channelId,
        content,
        fileIds: fileIds ?? [],
      },
    });
    return this.sendWithAck(requestId, encodeClientFrame(msg));
  }

  /** Fetch channel message history (paginated, REST). */
  async getChannelHistory(channelId: string, opts?: PageOptions): Promise<PageResult<ChannelMessage>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/channels/${channelId}/messages${query}`,
    );
    return buildPageResult(raw, mapChannelMessage);
  }

  /** Subscribe to real-time events for a channel (fire-and-forget via WS). */
  subscribeChannel(channelId: string): void {
    const msg = createClientMessage({
      type: ClientMessageType.SUBSCRIBE_CHANNEL,
      subscribeChannelRequest: { channelId },
    });
    this.ws.send(encodeClientFrame(msg));
    // Track for resubscription on reconnect
    const current = this.ws.getSubscribedChannels();
    if (!current.includes(channelId)) {
      this.ws.setSubscribedChannels([...current, channelId]);
    }
  }

  /** Unsubscribe from a channel's real-time events (fire-and-forget via WS). */
  unsubscribeChannel(channelId: string): void {
    const msg = createClientMessage({
      type: ClientMessageType.UNSUBSCRIBE_CHANNEL,
      unsubscribeChannelRequest: { channelId },
    });
    this.ws.send(encodeClientFrame(msg));
    // Remove from tracked channels
    const current = this.ws.getSubscribedChannels();
    this.ws.setSubscribedChannels(current.filter((id) => id !== channelId));
  }

  // -------------------------------------------------------------------------
  // Community methods (REST)
  // -------------------------------------------------------------------------

  /** List all communities the user belongs to. */
  async listCommunities(opts?: PageOptions): Promise<PageResult<Community>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/communities${query}`,
    );
    return buildPageResult(raw, mapCommunity);
  }

  /** Create a new community. */
  async createCommunity(name: string, description?: string): Promise<Community> {
    const raw = await this.rest.post<Record<string, unknown>>("/api/v1/communities", {
      name,
      description,
    });
    return mapCommunity(raw);
  }

  /** Get channels for a community. */
  async getChannels(communityId: string): Promise<Channel[]> {
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/communities/${communityId}/channels`,
    );
    const channels = (raw.channels ?? raw.items ?? []) as Record<string, unknown>[];
    return channels.map(mapChannel);
  }

  /** Get members of a community (paginated). */
  async getMembers(communityId: string, opts?: PageOptions): Promise<PageResult<Member>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/communities/${communityId}/members${query}`,
    );
    return buildPageResult(raw, mapMember);
  }

  /** Add a member to a community. */
  async addMember(communityId: string, userId: string): Promise<Member> {
    const raw = await this.rest.post<Record<string, unknown>>(
      `/api/v1/communities/${communityId}/members`,
      { user_id: userId },
    );
    return mapMember(raw);
  }

  // -------------------------------------------------------------------------
  // User methods (REST)
  // -------------------------------------------------------------------------

  /** Get a user's profile. */
  async getUser(userId: string): Promise<User> {
    const raw = await this.rest.get<Record<string, unknown>>(`/api/v1/users/${userId}`);
    return {
      id: raw.id as string,
      email: (raw.email ?? "") as string,
      nickname: raw.nickname as string,
      avatarUrl: (raw.avatar_url ?? raw.avatarUrl ?? "") as string,
      statusMessage: (raw.status_message ?? raw.statusMessage ?? "") as string,
      createdAt: (raw.created_at ?? raw.createdAt) as number,
      updatedAt: (raw.updated_at ?? raw.updatedAt) as number,
    };
  }

  /** List the current user's friends. */
  async listFriends(opts?: PageOptions): Promise<PageResult<User>> {
    const query = buildPageQuery(opts);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/users/me/friends${query}`,
    );
    return buildPageResult(raw, (u) => ({
      id: u.id as string,
      email: (u.email ?? "") as string,
      nickname: u.nickname as string,
      avatarUrl: (u.avatar_url ?? u.avatarUrl ?? "") as string,
      statusMessage: (u.status_message ?? u.statusMessage ?? "") as string,
      createdAt: (u.created_at ?? u.createdAt) as number,
      updatedAt: (u.updated_at ?? u.updatedAt) as number,
    }));
  }

  // -------------------------------------------------------------------------
  // Presence (REST)
  // -------------------------------------------------------------------------

  /** Get online/offline status for a list of user IDs. */
  async getPresence(userIds: string[]): Promise<{ online: string[]; offline: string[] }> {
    const ids = userIds.join(",");
    return this.rest.get(`/api/v1/users/presence?ids=${encodeURIComponent(ids)}`);
  }

  // -------------------------------------------------------------------------
  // File methods (REST)
  // -------------------------------------------------------------------------

  /** Upload a file via multipart form-data. */
  async uploadFile(
    data: Uint8Array,
    filename: string,
    contentType: string,
  ): Promise<FileInfo> {
    const formData = new FormData();
    formData.append(
      "file",
      new Blob([data], { type: contentType }),
      filename,
    );
    const raw = await this.rest.upload<Record<string, unknown>>(
      "/api/v1/files/upload",
      formData,
    );
    return mapFileInfo(raw);
  }

  /** Get a file's download URL. */
  async getFileURL(fileId: string): Promise<{ url: string }> {
    return this.rest.get<{ url: string }>(`/api/v1/files/${fileId}/url`);
  }

  // -------------------------------------------------------------------------
  // Search (REST)
  // -------------------------------------------------------------------------

  /** Search across users, messages, and DM messages. */
  async search(query: string, opts?: { limit?: number }): Promise<SearchResults> {
    const params: string[] = [`q=${encodeURIComponent(query)}`];
    if (opts?.limit !== undefined) params.push(`limit=${opts.limit}`);
    const raw = await this.rest.get<Record<string, unknown>>(
      `/api/v1/search?${params.join("&")}`,
    );

    return {
      users: ((raw.users ?? []) as Record<string, unknown>[]).map((u) => ({
        id: u.id as string,
        nickname: u.nickname as string,
        avatarUrl: (u.avatar_url ?? u.avatarUrl ?? "") as string,
        relevance: u.relevance as number,
      })),
      messages: ((raw.messages ?? []) as Record<string, unknown>[]).map((m) => ({
        id: m.id as string,
        channelId: (m.channel_id ?? m.channelId) as string,
        communityId: (m.community_id ?? m.communityId) as string,
        authorId: (m.author_id ?? m.authorId) as string,
        content: m.content as string,
        createdAt: (m.created_at ?? m.createdAt) as number,
        relevance: m.relevance as number,
      })),
      dmMessages: ((raw.dm_messages ?? raw.dmMessages ?? []) as Record<string, unknown>[]).map((m) => ({
        id: m.id as string,
        conversationId: (m.conversation_id ?? m.conversationId) as string,
        peerId: (m.peer_id ?? m.peerId) as string,
        content: m.content as string,
        createdAt: (m.created_at ?? m.createdAt) as number,
        relevance: m.relevance as number,
      })),
    };
  }

  // -------------------------------------------------------------------------
  // Notify (REST)
  // -------------------------------------------------------------------------

  /** Get unread notification counts. */
  async getUnreadCounts(): Promise<UnreadCounts> {
    const raw = await this.rest.get<Record<string, unknown>>("/api/v1/notify/unread");
    return {
      dmTotal: (raw.dm_total ?? raw.dmTotal ?? 0) as number,
      dmConversations: ((raw.dm_conversations ?? raw.dmConversations ?? []) as Record<string, unknown>[]).map((c) => ({
        conversationId: (c.conversation_id ?? c.conversationId) as string,
        peerId: (c.peer_id ?? c.peerId) as string,
        count: c.count as number,
      })),
      channelTotal: (raw.channel_total ?? raw.channelTotal ?? 0) as number,
      channels: ((raw.channels ?? []) as Record<string, unknown>[]).map((c) => ({
        channelId: (c.channel_id ?? c.channelId) as string,
        communityId: (c.community_id ?? c.communityId) as string,
        count: c.count as number,
      })),
    };
  }

  /** Mark all DMs in a conversation as read. */
  markDMRead(conversationId: string): Promise<unknown> {
    return this.rest.post(`/api/v1/notify/dm/${conversationId}/read`);
  }

  /** Mark all messages in a channel as read. */
  markChannelRead(channelId: string): Promise<unknown> {
    return this.rest.post(`/api/v1/notify/channel/${channelId}/read`);
  }

  // -------------------------------------------------------------------------
  // Event subscription (delegates to bus)
  // -------------------------------------------------------------------------

  /** Subscribe to a client event. */
  on<K extends keyof ClientEvents>(event: K, handler: ClientEvents[K]): void {
    this.bus.on(event, handler);
  }

  /** Unsubscribe from a client event. */
  off<K extends keyof ClientEvents>(event: K, handler: ClientEvents[K]): void {
    this.bus.off(event, handler);
  }

  /** Remove all event listeners. */
  removeAllListeners(): void {
    this.bus.removeAllListeners();
  }

  // -------------------------------------------------------------------------
  // Internal — WS message handling
  // -------------------------------------------------------------------------

  /**
   * Decode a binary WS frame and route the ServerEvent to the correct
   * typed event on the public bus, or resolve/reject a pending request.
   */
  private handleWSMessage(data: ArrayBuffer): void {
    const result = readServerEvent(data);
    if (!result) return;

    const { event } = result;

    switch (event.type) {
      case ServerEventType.DM_RECEIVED: {
        const dm = event.dmReceivedEvent;
        if (!dm) break;
        this.bus.emit("dm_received", {
          id: dm.messageId,
          conversationId: "", // DMReceivedEvent doesn't carry conversationId; consumer can derive
          senderId: dm.senderId,
          content: dm.content,
          createdAt: Number(dm.createdAt),
          attachments: dm.attachments.map((a) => ({
            id: a.id,
            fileId: a.fileId,
            filename: a.filename,
            contentType: a.contentType,
            size: Number(a.size),
            url: a.url,
            thumbnailUrl: a.thumbnailUrl,
          })),
        });
        break;
      }

      case ServerEventType.CHANNEL_MESSAGE_RECEIVED: {
        const cm = event.channelMessageEvent;
        if (!cm) break;
        this.bus.emit("channel_message", {
          id: cm.messageId,
          channelId: cm.channelId,
          authorId: cm.senderId,
          content: cm.content,
          createdAt: Number(cm.createdAt),
          updatedAt: Number(cm.createdAt),
          attachments: cm.attachments.map((a) => ({
            id: a.id,
            fileId: a.fileId,
            filename: a.filename,
            contentType: a.contentType,
            size: Number(a.size),
            url: a.url,
            thumbnailUrl: a.thumbnailUrl,
          })),
        });
        break;
      }

      case ServerEventType.USER_ONLINE: {
        const uo = event.userOnlineEvent;
        if (!uo) break;
        this.bus.emit("user_online", { userId: uo.userId });
        break;
      }

      case ServerEventType.USER_OFFLINE: {
        const uoff = event.userOfflineEvent;
        if (!uoff) break;
        this.bus.emit("user_offline", { userId: uoff.userId });
        break;
      }

      case ServerEventType.NOTIFICATION: {
        const n = event.notificationEvent;
        if (!n) break;
        this.bus.emit("notification", {
          notificationType: n.notificationType,
          sourceId: n.sourceId,
          communityId: n.communityId,
          senderId: n.senderId,
          senderNickname: n.senderNickname,
          preview: n.preview,
          createdAt: Number(n.createdAt),
        });
        break;
      }

      case ServerEventType.ACK: {
        // Resolve the matching pending request
        const pending = this.pendingRequests.get(event.requestId);
        if (pending) {
          clearTimeout(pending.timer);
          this.pendingRequests.delete(event.requestId);
          pending.resolve(undefined);
        }
        break;
      }

      case ServerEventType.ERROR: {
        // Reject the matching pending request
        const err = event.errorEvent;
        const pending = this.pendingRequests.get(event.requestId);
        if (pending) {
          clearTimeout(pending.timer);
          this.pendingRequests.delete(event.requestId);
          pending.reject(
            new ConstellError(
              err?.code ?? "WS_ERROR",
              err?.message ?? "Unknown WebSocket error",
            ),
          );
        }
        break;
      }

      // HEARTBEAT_ACK, UNSPECIFIED — no action needed
      default:
        break;
    }
  }

  // -------------------------------------------------------------------------
  // Internal — send with ACK tracking
  // -------------------------------------------------------------------------

  /**
   * Send a binary frame and return a promise that:
   * - resolves when the server sends an ACK with matching requestId
   * - rejects on timeout (5s) or when an ERROR event arrives
   */
  private sendWithAck(requestId: string, frame: Uint8Array): Promise<unknown> {
    return new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pendingRequests.delete(requestId);
        reject(new NetworkError(`Request timed out (id=${requestId})`));
      }, ACK_TIMEOUT_MS);

      this.pendingRequests.set(requestId, { resolve, reject, timer });

      this.ws.send(frame);
    });
  }
}
