// ConstellClient — unified SDK entry point
export { ConstellClient } from "./client.js";
export type { ClientEvents } from "./client.js";

// Errors
export { ConstellError, AuthError, NetworkError } from "./errors.js";

// Auth — JWT token lifecycle management
export { AuthManager } from "./auth.js";
export type { Storage } from "./auth.js";

// RESTClient — authenticated HTTP API calls
export { RESTClient } from "./rest-client.js";

// EventBus — typed event emitter
export { EventBus } from "./event-bus.js";
export type { EventHandler, EventMap } from "./event-bus.js";

// Codec — binary frame encode/decode for WebSocket protocol
export {
  createClientMessage,
  encodeClientFrame,
  decodeServerEvent,
  readServerEvent,
  generateRequestId,
  FRAME_HEADER_SIZE,
} from "./codec.js";
export type { ClientMessageOptions } from "./codec.js";

// WSManager — WebSocket connection management with heartbeat and reconnect
export { WSManager } from "./ws-manager.js";
export type { WSBusEvents, WebSocketFactory } from "./ws-manager.js";

// Types — enums
export {
  ChannelType,
  SearchType,
  ClientEventType,
  WSStatus,
} from "./types.js";

// Protobuf-generated enums
export {
  ClientMessageType,
  ServerEventType,
} from "./protobuf/gateway/v1/gateway_pb.js";

// Protobuf-generated types (type-only)
export type {
  ClientMessage,
  ServerEvent,
  SendDMRequest,
  SendChannelMessageRequest,
  SubscribeChannelRequest,
  UnsubscribeChannelRequest,
  DMReceivedEvent,
  ChannelMessageReceivedEvent,
  UserOnlineEvent,
  UserOfflineEvent,
  ErrorEvent,
  AckEvent,
  NotificationEvent as GatewayNotificationEvent,
} from "./protobuf/gateway/v1/gateway_pb.js";

// Types — interfaces (re-exported as type-only)
export type {
  // Common
  UserBrief,
  Attachment,
  // User
  User,
  DMMessage,
  DMConversation,
  // Community
  Community,
  Channel,
  Member,
  ChannelMessage,
  // File
  FileInfo,
  // Search
  UserSearchResult,
  MessageSearchResult,
  DMMessageSearchResult,
  CommunitySearchResult,
  SearchResults,
  // Notify
  UnreadDMConversation,
  UnreadChannel,
  UnreadCounts,
  // Gateway / WS
  NotificationEvent,
  // SDK-internal
  TempMessage,
  ClientConfig,
  PageOptions,
  PageResult,
  MessageAck,
} from "./types.js";
