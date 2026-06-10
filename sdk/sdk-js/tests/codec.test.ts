import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
  ClientMessageType,
  ClientMessageSchema,
  ServerEventSchema,
  ServerEventType,
  SendDMRequestSchema,
  DMReceivedEventSchema,
} from "../src/protobuf/gateway/v1/gateway_pb.js";
import {
  createClientMessage,
  encodeClientFrame,
  decodeServerEvent,
  readServerEvent,
  generateRequestId,
  FRAME_HEADER_SIZE,
} from "../src/codec.js";

describe("codec", () => {
  // -------------------------------------------------------------------------
  // generateRequestId
  // -------------------------------------------------------------------------
  describe("generateRequestId", () => {
    it("returns a non-empty string", () => {
      const id = generateRequestId();
      expect(id).toBeTruthy();
      expect(typeof id).toBe("string");
    });

    it("returns unique values on successive calls", () => {
      const ids = new Set(Array.from({ length: 100 }, () => generateRequestId()));
      expect(ids.size).toBe(100);
    });
  });

  // -------------------------------------------------------------------------
  // encodeClientFrame — heartbeat
  // -------------------------------------------------------------------------
  describe("encodeClientFrame (heartbeat)", () => {
    it("prepends a 4-byte big-endian length prefix", () => {
      const msg = createClientMessage({
        type: ClientMessageType.HEARTBEAT,
        requestId: "test-hb-1",
      });

      const frame = encodeClientFrame(msg);

      // Frame must be at least header + some payload
      expect(frame.length).toBeGreaterThan(FRAME_HEADER_SIZE);

      // Read length prefix
      const view = new DataView(
        frame.buffer,
        frame.byteOffset,
        frame.byteLength,
      );
      const payloadLength = view.getUint32(0, false);
      expect(payloadLength).toBe(frame.length - FRAME_HEADER_SIZE);
    });

    it("produces a frame that round-trips through decode", () => {
      const msg = createClientMessage({
        type: ClientMessageType.HEARTBEAT,
        requestId: "test-hb-2",
      });

      const frame = encodeClientFrame(msg);

      // Extract payload (skip header)
      const payload = frame.slice(FRAME_HEADER_SIZE);

      // Re-encode the original message to binary and compare
      const expectedPayload = toBinary(ClientMessageSchema, msg);
      expect(payload).toEqual(expectedPayload);
    });
  });

  // -------------------------------------------------------------------------
  // encodeClientFrame — SEND_DM
  // -------------------------------------------------------------------------
  describe("encodeClientFrame (SEND_DM)", () => {
    it("encodes a SEND_DM message with correct structure", () => {
      const msg = createClientMessage({
        type: ClientMessageType.SEND_DM,
        requestId: "dm-req-1",
        sendDmRequest: {
          receiverId: "user-42",
          content: "Hello, world!",
          fileIds: ["file-1", "file-2"],
        },
      });

      const frame = encodeClientFrame(msg);

      // Header + payload
      const view = new DataView(
        frame.buffer,
        frame.byteOffset,
        frame.byteLength,
      );
      const payloadLength = view.getUint32(0, false);
      expect(payloadLength).toBe(frame.length - FRAME_HEADER_SIZE);

      // Decode the payload and verify fields
      const payload = frame.slice(FRAME_HEADER_SIZE);
      const decoded = fromBinary(ClientMessageSchema, payload);
      expect(decoded.type).toBe(ClientMessageType.SEND_DM);
      expect(decoded.requestId).toBe("dm-req-1");
      expect(decoded.sendDmRequest).toBeDefined();
      expect(decoded.sendDmRequest!.receiverId).toBe("user-42");
      expect(decoded.sendDmRequest!.content).toBe("Hello, world!");
      expect(decoded.sendDmRequest!.fileIds).toEqual(["file-1", "file-2"]);
    });
  });

  // -------------------------------------------------------------------------
  // decodeServerEvent
  // -------------------------------------------------------------------------
  describe("decodeServerEvent", () => {
    it("decodes a ServerEvent from protobuf bytes", () => {
      // Build a DM_RECEIVED server event
      const original = create(ServerEventSchema, {
        type: ServerEventType.DM_RECEIVED,
        requestId: "",
        dmReceivedEvent: create(DMReceivedEventSchema, {
          messageId: "msg-1",
          senderId: "user-10",
          senderNickname: "Alice",
          content: "Hey!",
          createdAt: BigInt(1700000000000),
          attachments: [],
        }),
      });

      const payload = toBinary(ServerEventSchema, original);
      const decoded = decodeServerEvent(payload);

      expect(decoded.type).toBe(ServerEventType.DM_RECEIVED);
      expect(decoded.dmReceivedEvent).toBeDefined();
      expect(decoded.dmReceivedEvent!.messageId).toBe("msg-1");
      expect(decoded.dmReceivedEvent!.senderId).toBe("user-10");
      expect(decoded.dmReceivedEvent!.senderNickname).toBe("Alice");
      expect(decoded.dmReceivedEvent!.content).toBe("Hey!");
    });
  });

  // -------------------------------------------------------------------------
  // readServerEvent
  // -------------------------------------------------------------------------
  describe("readServerEvent", () => {
    it("reads a complete frame from a buffer", () => {
      // Build a HEARTBEAT_ACK event
      const event = create(ServerEventSchema, {
        type: ServerEventType.HEARTBEAT_ACK,
        requestId: "hb-123",
      });

      const payload = toBinary(ServerEventSchema, event);

      // Build the full frame: [4-byte length][payload]
      const frame = new Uint8Array(FRAME_HEADER_SIZE + payload.length);
      const view = new DataView(frame.buffer);
      view.setUint32(0, payload.length, false);
      frame.set(payload, FRAME_HEADER_SIZE);

      const result = readServerEvent(frame.buffer);

      expect(result).not.toBeNull();
      expect(result!.event.type).toBe(ServerEventType.HEARTBEAT_ACK);
      expect(result!.event.requestId).toBe("hb-123");
      expect(result!.bytesRead).toBe(FRAME_HEADER_SIZE + payload.length);
    });

    it("returns null when buffer is shorter than the header", () => {
      const buffer = new ArrayBuffer(3); // only 3 bytes, need 4 for header
      expect(readServerEvent(buffer)).toBeNull();
    });

    it("returns null when buffer has header but incomplete payload", () => {
      // Header says 100 bytes payload, but we only provide 10
      const buffer = new ArrayBuffer(FRAME_HEADER_SIZE + 10);
      const view = new DataView(buffer);
      view.setUint32(0, 100, false); // claims 100-byte payload

      expect(readServerEvent(buffer)).toBeNull();
    });

    it("reads only the first frame and reports correct bytesRead", () => {
      const event = create(ServerEventSchema, {
        type: ServerEventType.ACK,
        requestId: "ack-1",
      });

      const payload = toBinary(ServerEventSchema, event);

      // Build buffer with one frame + some extra trailing bytes
      const totalLength = FRAME_HEADER_SIZE + payload.length + 5; // 5 extra bytes
      const buffer = new ArrayBuffer(totalLength);
      const frame = new Uint8Array(buffer);
      const view = new DataView(buffer);
      view.setUint32(0, payload.length, false);
      frame.set(payload, FRAME_HEADER_SIZE);

      const result = readServerEvent(buffer);

      expect(result).not.toBeNull();
      expect(result!.bytesRead).toBe(FRAME_HEADER_SIZE + payload.length);
      expect(result!.event.type).toBe(ServerEventType.ACK);
      expect(result!.event.requestId).toBe("ack-1");
    });
  });
});
