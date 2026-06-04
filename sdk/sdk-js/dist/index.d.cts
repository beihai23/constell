/**
 * Base error class for all Constell SDK errors.
 *
 * Every SDK-thrown error is a subclass of {@link ConstellError}, making it
 * easy to distinguish backend/SDK errors from unrelated runtime exceptions.
 */
declare class ConstellError extends Error {
    /** Machine-readable error code returned by the backend (e.g. "UNAUTHORIZED"). */
    readonly code: string;
    /** HTTP status code (0 when not applicable, e.g. WebSocket errors). */
    readonly statusCode: number;
    constructor(code: string, message: string, statusCode?: number);
}
/**
 * Thrown when an authentication or authorisation request fails
 * (invalid credentials, expired token, etc.).
 */
declare class AuthError extends ConstellError {
    constructor(message: string, statusCode?: number);
}
/**
 * Thrown when a network-level failure occurs (fetch error, WebSocket
 * disconnect, timeout, DNS failure, etc.).
 */
declare class NetworkError extends ConstellError {
    constructor(message: string);
}

/** Lightweight user summary embedded in other messages. */
interface UserBrief {
    id: string;
    nickname: string;
    avatarUrl: string;
}
/** File attached to a message. */
interface Attachment {
    id: string;
    fileId: string;
    filename: string;
    contentType: string;
    size: number;
    url: string;
    thumbnailUrl: string;
}
interface User {
    id: string;
    email: string;
    nickname: string;
    avatarUrl: string;
    statusMessage: string;
    createdAt: number;
    updatedAt: number;
}
/** A single direct message. */
interface DMMessage {
    id: string;
    conversationId: string;
    senderId: string;
    content: string;
    createdAt: number;
    attachments: Attachment[];
}
/** A DM conversation summary (for listing). */
interface DMConversation {
    id: string;
    peer: UserBrief;
    lastMessage?: DMMessage;
    unreadCount: number;
}
interface Community {
    id: string;
    name: string;
    description: string;
    iconUrl: string;
    ownerId: string;
    createdAt: number;
    updatedAt: number;
}
declare enum ChannelType {
    Unspecified = 0,
    Text = 1,
    Announcement = 2
}
interface Channel {
    id: string;
    communityId: string;
    name: string;
    topic: string;
    type: ChannelType;
    position: number;
    createdAt: number;
    updatedAt: number;
}
interface Member {
    communityId: string;
    userId: string;
    nickname: string;
    roleIds: string[];
    joinedAt: number;
}
/** A single channel message. */
interface ChannelMessage {
    id: string;
    channelId: string;
    authorId: string;
    content: string;
    createdAt: number;
    updatedAt: number;
    attachments: Attachment[];
}
interface FileInfo {
    id: string;
    filename: string;
    contentType: string;
    size: number;
    url: string;
    thumbnailUrl: string;
    createdAt: number;
}
declare enum SearchType {
    Unspecified = 0,
    Users = 1,
    Messages = 2,
    DMMessages = 3
}
interface UserSearchResult {
    id: string;
    nickname: string;
    avatarUrl: string;
    relevance: number;
}
interface MessageSearchResult {
    id: string;
    channelId: string;
    communityId: string;
    authorId: string;
    content: string;
    createdAt: number;
    relevance: number;
}
interface DMMessageSearchResult {
    id: string;
    conversationId: string;
    peerId: string;
    content: string;
    createdAt: number;
    relevance: number;
}
interface SearchResults {
    users: UserSearchResult[];
    messages: MessageSearchResult[];
    dmMessages: DMMessageSearchResult[];
}
interface UnreadDMConversation {
    conversationId: string;
    peerId: string;
    count: number;
}
interface UnreadChannel {
    channelId: string;
    communityId: string;
    count: number;
}
interface UnreadCounts {
    dmTotal: number;
    dmConversations: UnreadDMConversation[];
    channelTotal: number;
    channels: UnreadChannel[];
}
/** Event types pushed from server → client over WebSocket. */
declare enum ClientEventType {
    DmReceived = "DM_RECEIVED",
    ChannelMessageReceived = "CHANNEL_MESSAGE_RECEIVED",
    UserOnline = "USER_ONLINE",
    UserOffline = "USER_OFFLINE",
    Error = "ERROR",
    HeartbeatAck = "HEARTBEAT_ACK",
    Ack = "ACK",
    Notification = "NOTIFICATION"
}
/** Notification event pushed from the server. */
interface NotificationEvent {
    notificationType: string;
    sourceId: string;
    communityId: string;
    senderId: string;
    senderNickname: string;
    preview: string;
    createdAt: number;
}
/**
 * A temporary, optimistic message created client-side before the server
 * confirms it. The `tempId` is a client-generated UUID used to reconcile
 * the server response.
 */
interface TempMessage {
    tempId: string;
    content: string;
    createdAt: number;
}
/** WebSocket connection status. */
declare enum WSStatus {
    Disconnected = "DISCONNECTED",
    Connecting = "CONNECTING",
    Connected = "CONNECTED",
    Reconnecting = "RECONNECTING"
}
/** Configuration object for creating a {@link ConstellClient}. */
interface ClientConfig {
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
/** Options for paginated list requests. */
interface PageOptions {
    /** Maximum number of items to return. */
    limit?: number;
    /** Offset-based pagination (mutually exclusive with cursor). */
    offset?: number;
    /** Cursor-based pagination (mutually exclusive with offset). */
    cursor?: string;
}
/** Wrapper returned by paginated list methods. */
interface PageResult<T> {
    items: T[];
    hasMore: boolean;
    nextCursor?: string;
    totalCount?: number;
}

export { type Attachment, AuthError, type Channel, type ChannelMessage, ChannelType, type ClientConfig, ClientEventType, type Community, ConstellError, type DMConversation, type DMMessage, type DMMessageSearchResult, type FileInfo, type Member, type MessageSearchResult, NetworkError, type NotificationEvent, type PageOptions, type PageResult, type SearchResults, SearchType, type TempMessage, type UnreadChannel, type UnreadCounts, type UnreadDMConversation, type User, type UserBrief, type UserSearchResult, WSStatus };
