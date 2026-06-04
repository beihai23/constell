"use strict";
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var index_exports = {};
__export(index_exports, {
  AuthError: () => AuthError,
  ChannelType: () => ChannelType,
  ClientEventType: () => ClientEventType,
  ConstellError: () => ConstellError,
  NetworkError: () => NetworkError,
  SearchType: () => SearchType,
  WSStatus: () => WSStatus
});
module.exports = __toCommonJS(index_exports);

// src/errors.ts
var ConstellError = class extends Error {
  /** Machine-readable error code returned by the backend (e.g. "UNAUTHORIZED"). */
  code;
  /** HTTP status code (0 when not applicable, e.g. WebSocket errors). */
  statusCode;
  constructor(code, message, statusCode = 0) {
    super(message);
    this.name = "ConstellError";
    this.code = code;
    this.statusCode = statusCode;
  }
};
var AuthError = class extends ConstellError {
  constructor(message, statusCode = 401) {
    super("AUTH_ERROR", message, statusCode);
    this.name = "AuthError";
  }
};
var NetworkError = class extends ConstellError {
  constructor(message) {
    super("NETWORK_ERROR", message, 0);
    this.name = "NetworkError";
  }
};

// src/types.ts
var ChannelType = /* @__PURE__ */ ((ChannelType2) => {
  ChannelType2[ChannelType2["Unspecified"] = 0] = "Unspecified";
  ChannelType2[ChannelType2["Text"] = 1] = "Text";
  ChannelType2[ChannelType2["Announcement"] = 2] = "Announcement";
  return ChannelType2;
})(ChannelType || {});
var SearchType = /* @__PURE__ */ ((SearchType2) => {
  SearchType2[SearchType2["Unspecified"] = 0] = "Unspecified";
  SearchType2[SearchType2["Users"] = 1] = "Users";
  SearchType2[SearchType2["Messages"] = 2] = "Messages";
  SearchType2[SearchType2["DMMessages"] = 3] = "DMMessages";
  return SearchType2;
})(SearchType || {});
var ClientEventType = /* @__PURE__ */ ((ClientEventType2) => {
  ClientEventType2["DmReceived"] = "DM_RECEIVED";
  ClientEventType2["ChannelMessageReceived"] = "CHANNEL_MESSAGE_RECEIVED";
  ClientEventType2["UserOnline"] = "USER_ONLINE";
  ClientEventType2["UserOffline"] = "USER_OFFLINE";
  ClientEventType2["Error"] = "ERROR";
  ClientEventType2["HeartbeatAck"] = "HEARTBEAT_ACK";
  ClientEventType2["Ack"] = "ACK";
  ClientEventType2["Notification"] = "NOTIFICATION";
  return ClientEventType2;
})(ClientEventType || {});
var WSStatus = /* @__PURE__ */ ((WSStatus2) => {
  WSStatus2["Disconnected"] = "DISCONNECTED";
  WSStatus2["Connecting"] = "CONNECTING";
  WSStatus2["Connected"] = "CONNECTED";
  WSStatus2["Reconnecting"] = "RECONNECTING";
  return WSStatus2;
})(WSStatus || {});
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  AuthError,
  ChannelType,
  ClientEventType,
  ConstellError,
  NetworkError,
  SearchType,
  WSStatus
});
//# sourceMappingURL=index.cjs.map