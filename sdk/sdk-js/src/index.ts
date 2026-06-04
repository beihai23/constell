// Errors
export { ConstellError, AuthError, NetworkError } from "./errors.js";

// Types — enums
export {
  ChannelType,
  SearchType,
  ClientEventType,
  WSStatus,
} from "./types.js";

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
} from "./types.js";
