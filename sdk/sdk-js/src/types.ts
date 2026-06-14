// ---------------------------------------------------------------------------
// Domain types — mirrors of the protobuf message definitions.
// All timestamps are Unix epoch milliseconds (int64 → number).
// ---------------------------------------------------------------------------

// ---- Common (common/v1) ---------------------------------------------------

/** Lightweight user summary embedded in other messages. */
export interface UserBrief {
  id: string;
  nickname: string;
  avatarUrl: string;
}

/** File attached to a message. */
export interface Attachment {
  id: string;
  fileId: string;
  filename: string;
  contentType: string;
  size: number;
  url: string;
  thumbnailUrl: string;
}

// ---- User (user/v1) -------------------------------------------------------

export interface User {
  id: string;
  email: string;
  nickname: string;
  avatarUrl: string;
  statusMessage: string;
  createdAt: number;
  updatedAt: number;
}

/** A single direct message. */
export interface DMMessage {
  id: string;
  conversationId: string;
  senderId: string;
  content: string;
  createdAt: number;
  attachments: Attachment[];
  /** Monotonically increasing per-conversation sequence number (0 if unknown). */
  seq: number;
}

/** A DM conversation summary (for listing). */
export interface DMConversation {
  id: string;
  peerId: string;
  createdAt: number;
}

// ---- Community (community/v1) ---------------------------------------------

export interface Community {
  id: string;
  name: string;
  description: string;
  iconUrl: string;
  ownerId: string;
  createdAt: number;
  updatedAt: number;
}

export enum ChannelType {
  Unspecified = 0,
  Text = 1,
  Announcement = 2,
}

export interface Channel {
  id: string;
  communityId: string;
  name: string;
  topic: string;
  type: ChannelType;
  position: number;
  createdAt: number;
  updatedAt: number;
}

export interface Member {
  communityId: string;
  userId: string;
  nickname: string;
  roleIds: string[];
  joinedAt: number;
}

/** A single channel message. */
export interface ChannelMessage {
  id: string;
  channelId: string;
  authorId: string;
  content: string;
  createdAt: number;
  updatedAt: number;
  attachments: Attachment[];
  /** Monotonically increasing per-channel sequence number (0 if unknown). */
  seq: number;
}

// ---- File (file/v1) -------------------------------------------------------

export interface FileInfo {
  id: string;
  filename: string;
  contentType: string;
  size: number;
  url: string;
  thumbnailUrl: string;
  createdAt: number;
}

// ---- Search (search/v1) ---------------------------------------------------

export enum SearchType {
  Unspecified = 0,
  Users = 1,
  Messages = 2,
  DMMessages = 3,
}

export interface UserSearchResult {
  id: string;
  nickname: string;
  avatarUrl: string;
  relevance: number;
}

export interface MessageSearchResult {
  id: string;
  channelId: string;
  communityId: string;
  authorId: string;
  content: string;
  createdAt: number;
  relevance: number;
}

export interface DMMessageSearchResult {
  id: string;
  conversationId: string;
  peerId: string;
  content: string;
  createdAt: number;
  relevance: number;
}

export interface SearchResults {
  users: UserSearchResult[];
  messages: MessageSearchResult[];
  dmMessages: DMMessageSearchResult[];
}

// ---- Notify (notify/v1) ---------------------------------------------------

export interface UnreadDMConversation {
  conversationId: string;
  peerId: string;
  count: number;
}

export interface UnreadChannel {
  channelId: string;
  communityId: string;
  count: number;
}

export interface UnreadCounts {
  dmTotal: number;
  dmConversations: UnreadDMConversation[];
  channelTotal: number;
  channels: UnreadChannel[];
}

// ---- Gateway / WebSocket (gateway/v1) -------------------------------------

/** Event types pushed from server → client over WebSocket. */
export enum ClientEventType {
  DmReceived = "DM_RECEIVED",
  ChannelMessageReceived = "CHANNEL_MESSAGE_RECEIVED",
  UserOnline = "USER_ONLINE",
  UserOffline = "USER_OFFLINE",
  Error = "ERROR",
  HeartbeatAck = "HEARTBEAT_ACK",
  Ack = "ACK",
  Notification = "NOTIFICATION",
}

/** Notification event pushed from the server. */
export interface NotificationEvent {
  notificationType: string;
  sourceId: string;
  communityId: string;
  senderId: string;
  senderNickname: string;
  preview: string;
  createdAt: number;
}

// ---- SDK-internal types ---------------------------------------------------

/**
 * A temporary, optimistic message created client-side before the server
 * confirms it. The `tempId` is a client-generated UUID used to reconcile
 * the server response.
 */
export interface TempMessage {
  tempId: string;
  content: string;
  createdAt: number;
}

/** WebSocket connection status. */
export enum WSStatus {
  Disconnected = "DISCONNECTED",
  Connecting = "CONNECTING",
  Connected = "CONNECTED",
  Reconnecting = "RECONNECTING",
}

/** Configuration object for creating a {@link ConstellClient}. */
export interface ClientConfig {
  /** Base URL of the API Gateway (e.g. "http://localhost:8080"). */
  apiUrl: string;
  /** WebSocket URL of the WS Gateway (e.g. "ws://localhost:8081/ws"). */
  wsUrl: string;
  /** Automatically reconnect WebSocket on disconnect (default true). */
  autoReconnect?: boolean;
  /** Maximum reconnection attempts before giving up (default Infinity). */
  maxReconnectAttempts?: number;
  /** Heartbeat interval in milliseconds (default 30 000). */
  heartbeatInterval?: number;
}

// ---- Pagination ------------------------------------------------------------

/** Options for paginated list requests. */
export interface PageOptions {
  /** Maximum number of items to return. */
  limit?: number;
  /** Offset-based pagination (mutually exclusive with cursor). */
  offset?: number;
  /** Cursor-based pagination (mutually exclusive with offset). */
  cursor?: string;
  /** If set, request messages with seq > sinceSeq (backfill). */
  sinceSeq?: number;
}

/** Wrapper returned by paginated list methods. */
export interface PageResult<T> {
  items: T[];
  hasMore: boolean;
  nextCursor?: string;
  totalCount?: number;
}
