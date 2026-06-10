// ---------------------------------------------------------------------------
// Binary frame codec for Constell WebSocket protocol.
//
// Wire format: [4 bytes big-endian payload length][protobuf payload]
//
// Client → Server: ClientMessage (protobuf)
// Server → Client: ServerEvent  (protobuf)
// ---------------------------------------------------------------------------

import { create, toBinary, fromBinary, type MessageInitShape } from "@bufbuild/protobuf";
import {
  ClientMessageSchema,
  ClientMessageType,
  SendDMRequestSchema,
  SendChannelMessageRequestSchema,
  SubscribeChannelRequestSchema,
  UnsubscribeChannelRequestSchema,
  ServerEventSchema,
  type ClientMessage,
  type ServerEvent,
} from "./protobuf/gateway/v1/gateway_pb.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Size of the frame header (4-byte big-endian length prefix). */
export const FRAME_HEADER_SIZE = 4;

/**
 * Generate a unique request ID for correlating client requests with server
 * ACK responses. Uses crypto.randomUUID when available, falling back to a
 * timestamp-based ID.
 */
export function generateRequestId(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback for environments without crypto.randomUUID
  return `${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}

// ---------------------------------------------------------------------------
// Client message construction
// ---------------------------------------------------------------------------

/** Options for constructing a ClientMessage via createClientMessage. */
export interface ClientMessageOptions {
  type: ClientMessageType;
  requestId?: string;
  sendDmRequest?: MessageInitShape<typeof SendDMRequestSchema>;
  sendChannelMessageRequest?: MessageInitShape<typeof SendChannelMessageRequestSchema>;
  subscribeChannelRequest?: MessageInitShape<typeof SubscribeChannelRequestSchema>;
  unsubscribeChannelRequest?: MessageInitShape<typeof UnsubscribeChannelRequestSchema>;
}

/**
 * Construct a ClientMessage protobuf message with the correct nested payload.
 *
 * @example
 * ```ts
 * const msg = createClientMessage({
 *   type: ClientMessageType.HEARTBEAT,
 * });
 *
 * const msg = createClientMessage({
 *   type: ClientMessageType.SEND_DM,
 *   sendDmRequest: { receiverId: "user-123", content: "Hello!" },
 * });
 * ```
 */
export function createClientMessage(opts: ClientMessageOptions): ClientMessage {
  const requestId = opts.requestId ?? generateRequestId();

  return create(ClientMessageSchema, {
    type: opts.type,
    requestId,
    sendDmRequest: opts.sendDmRequest,
    sendChannelMessageRequest: opts.sendChannelMessageRequest,
    subscribeChannelRequest: opts.subscribeChannelRequest,
    unsubscribeChannelRequest: opts.unsubscribeChannelRequest,
  });
}

// ---------------------------------------------------------------------------
// Encoding (client → server)
// ---------------------------------------------------------------------------

/**
 * Encode a ClientMessage into a binary frame.
 *
 * Returns a Uint8Array with the layout:
 *   [4 bytes big-endian payload length][protobuf payload]
 */
export function encodeClientFrame(msg: ClientMessage): Uint8Array {
  const payload = toBinary(ClientMessageSchema, msg);
  const frame = new Uint8Array(FRAME_HEADER_SIZE + payload.length);

  // Write 4-byte big-endian length prefix
  const view = new DataView(frame.buffer, frame.byteOffset, frame.byteLength);
  view.setUint32(0, payload.length, false); // false = big-endian

  // Write protobuf payload
  frame.set(payload, FRAME_HEADER_SIZE);

  return frame;
}

// ---------------------------------------------------------------------------
// Decoding (server → client)
// ---------------------------------------------------------------------------

/**
 * Decode a ServerEvent from raw protobuf bytes.
 *
 * @param payload - The protobuf payload (without the length prefix header).
 */
export function decodeServerEvent(payload: Uint8Array): ServerEvent {
  return fromBinary(ServerEventSchema, payload);
}

/**
 * Read a single ServerEvent from an ArrayBuffer.
 *
 * The buffer may contain:
 * - A complete frame (header + payload), possibly with extra bytes after it.
 * - An incomplete frame (not enough bytes for header or payload).
 *
 * Returns `{ event, bytesRead }` when a complete frame was decoded, or
 * `null` when the buffer does not contain a complete frame.
 *
 * @param buffer - The raw data received from the WebSocket.
 */
export function readServerEvent(
  buffer: ArrayBuffer,
): { event: ServerEvent; bytesRead: number } | null {
  if (buffer.byteLength < FRAME_HEADER_SIZE) {
    return null;
  }

  const view = new DataView(buffer);
  const payloadLength = view.getUint32(0, false); // false = big-endian

  const totalFrameSize = FRAME_HEADER_SIZE + payloadLength;
  if (buffer.byteLength < totalFrameSize) {
    return null;
  }

  const payload = new Uint8Array(buffer, FRAME_HEADER_SIZE, payloadLength);
  const event = fromBinary(ServerEventSchema, payload);

  return { event, bytesRead: totalFrameSize };
}
