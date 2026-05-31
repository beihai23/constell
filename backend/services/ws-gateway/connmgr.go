package main

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnEntry holds metadata for a single WebSocket connection.
type ConnEntry struct {
	UserID             string
	Conn               *websocket.Conn
	ConnectedAt        time.Time
	SubscribedChannels map[string]bool
}

// ConnManager manages local WebSocket connections in a thread-safe map.
type ConnManager struct {
	mu    sync.RWMutex
	conns map[string]*ConnEntry
}

// NewConnManager creates a new ConnManager.
func NewConnManager() *ConnManager {
	return &ConnManager{
		conns: make(map[string]*ConnEntry),
	}
}

// Register adds a WebSocket connection for the given user ID.
// If a connection already exists for this user, it is replaced.
func (m *ConnManager) Register(userID string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.conns[userID] = &ConnEntry{
		UserID:             userID,
		Conn:               conn,
		ConnectedAt:        time.Now(),
		SubscribedChannels: make(map[string]bool),
	}
}

// Unregister removes a connection by user ID and closes the underlying WebSocket.
func (m *ConnManager) Unregister(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.conns[userID]; ok {
		entry.Conn.Close()
		delete(m.conns, userID)
	}
}

// Get returns the ConnEntry for a user ID, or false if not found.
func (m *ConnManager) Get(userID string) (*ConnEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.conns[userID]
	if !ok {
		return nil, false
	}
	return entry, true
}

// GetAll returns a shallow copy of the entire connections map.
func (m *ConnManager) GetAll() map[string]*ConnEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ConnEntry, len(m.conns))
	for k, v := range m.conns {
		result[k] = v
	}
	return result
}

// Count returns the number of active connections.
func (m *ConnManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.conns)
}

// AddSubscribedChannel adds a channel to the user's subscribed channels set.
func (m *ConnManager) AddSubscribedChannel(userID string, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.conns[userID]; ok {
		entry.SubscribedChannels[channelID] = true
	}
}

// RemoveSubscribedChannel removes a channel from the user's subscribed channels set.
func (m *ConnManager) RemoveSubscribedChannel(userID string, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.conns[userID]; ok {
		delete(entry.SubscribedChannels, channelID)
	}
}
