package main

import (
	"encoding/binary"
	"fmt"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const frameHeaderSize = 4

// EncodeFrame serializes a ServerEvent into a length-prefixed binary frame:
//
//	[4 bytes big-endian length][protobuf payload]
func EncodeFrame(msg *gatewayv1.ServerEvent) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal server event: %w", err)
	}

	frame := make([]byte, frameHeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[:frameHeaderSize], uint32(len(payload)))
	copy(frame[frameHeaderSize:], payload)

	return frame, nil
}

// DecodeFrame parses a length-prefixed binary frame into a ServerEvent.
func DecodeFrame(data []byte) (*gatewayv1.ServerEvent, error) {
	if len(data) < frameHeaderSize {
		return nil, fmt.Errorf("frame too short: %d bytes, need at least %d", len(data), frameHeaderSize)
	}

	length := binary.BigEndian.Uint32(data[:frameHeaderSize])
	payload := data[frameHeaderSize:]

	if len(payload) < int(length) {
		return nil, fmt.Errorf("payload length mismatch: header says %d, got %d bytes", length, len(payload))
	}

	msg := &gatewayv1.ServerEvent{}
	if err := proto.Unmarshal(payload[:length], msg); err != nil {
		return nil, fmt.Errorf("unmarshal server event: %w", err)
	}

	return msg, nil
}

// EncodeClientFrame serializes a ClientMessage into a length-prefixed binary frame.
func EncodeClientFrame(msg *gatewayv1.ClientMessage) ([]byte, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal client message: %w", err)
	}

	frame := make([]byte, frameHeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[:frameHeaderSize], uint32(len(payload)))
	copy(frame[frameHeaderSize:], payload)

	return frame, nil
}

// DecodeClientFrame parses a length-prefixed binary frame into a ClientMessage.
func DecodeClientFrame(data []byte) (*gatewayv1.ClientMessage, error) {
	if len(data) < frameHeaderSize {
		return nil, fmt.Errorf("frame too short: %d bytes, need at least %d", len(data), frameHeaderSize)
	}

	length := binary.BigEndian.Uint32(data[:frameHeaderSize])
	payload := data[frameHeaderSize:]

	if len(payload) < int(length) {
		return nil, fmt.Errorf("payload length mismatch: header says %d, got %d bytes", length, len(payload))
	}

	msg := &gatewayv1.ClientMessage{}
	if err := proto.Unmarshal(payload[:length], msg); err != nil {
		return nil, fmt.Errorf("unmarshal client message: %w", err)
	}

	return msg, nil
}

// WriteMessage writes a ServerEvent as a binary WebSocket message to the connection.
func WriteMessage(conn *websocket.Conn, msg *gatewayv1.ServerEvent) error {
	frame, err := EncodeFrame(msg)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("write websocket message: %w", err)
	}

	return nil
}

// ReadMessage reads a binary WebSocket message and decodes it into a ClientMessage.
func ReadMessage(conn *websocket.Conn) (*gatewayv1.ClientMessage, error) {
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read websocket message: %w", err)
	}

	if messageType != websocket.BinaryMessage {
		return nil, fmt.Errorf("expected binary message, got message type %d", messageType)
	}

	msg, err := DecodeClientFrame(data)
	if err != nil {
		return nil, fmt.Errorf("decode client frame: %w", err)
	}

	return msg, nil
}
