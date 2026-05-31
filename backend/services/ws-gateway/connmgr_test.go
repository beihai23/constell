package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupTestWS(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	var serverConn *websocket.Conn
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
		}
	}))
	t.Cleanup(func() { server.Close() })

	wsURL := "ws:" + strings.TrimPrefix(server.URL, "http:")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { clientConn.Close() })

	time.Sleep(10 * time.Millisecond)

	return clientConn, serverConn
}

func TestConnManager_RegisterAndGet(t *testing.T) {
	_, conn := setupTestWS(t)
	defer conn.Close()

	mgr := NewConnManager()
	mgr.Register("user-1", conn)

	entry, ok := mgr.Get("user-1")
	if !ok {
		t.Fatal("expected to find user-1 in conn manager")
	}
	if entry.UserID != "user-1" {
		t.Fatalf("expected UserID 'user-1', got %q", entry.UserID)
	}
	if entry.Conn == nil {
		t.Fatal("expected non-nil Conn")
	}

	t.Logf("register+get OK: user=%s connected=%v", entry.UserID, entry.ConnectedAt)
}

func TestConnManager_Get_NotFound(t *testing.T) {
	mgr := NewConnManager()
	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find nonexistent user")
	}
	t.Log("correctly returned not-found for nonexistent user")
}

func TestConnManager_Unregister(t *testing.T) {
	_, conn := setupTestWS(t)

	mgr := NewConnManager()
	mgr.Register("user-2", conn)

	if _, ok := mgr.Get("user-2"); !ok {
		t.Fatal("expected user-2 to exist before unregister")
	}

	mgr.Unregister("user-2")

	if _, ok := mgr.Get("user-2"); ok {
		t.Fatal("expected user-2 to be gone after unregister")
	}

	t.Log("unregister OK")
}

func TestConnManager_Count(t *testing.T) {
	_, conn1 := setupTestWS(t)
	_, conn2 := setupTestWS(t)

	mgr := NewConnManager()

	if mgr.Count() != 0 {
		t.Fatalf("expected count 0, got %d", mgr.Count())
	}

	mgr.Register("user-a", conn1)
	if mgr.Count() != 1 {
		t.Fatalf("expected count 1, got %d", mgr.Count())
	}

	mgr.Register("user-b", conn2)
	if mgr.Count() != 2 {
		t.Fatalf("expected count 2, got %d", mgr.Count())
	}

	mgr.Unregister("user-a")
	if mgr.Count() != 1 {
		t.Fatalf("expected count 1 after unregister, got %d", mgr.Count())
	}

	t.Log("count tracking OK")
}

func TestConnManager_GetAll(t *testing.T) {
	_, conn1 := setupTestWS(t)
	_, conn2 := setupTestWS(t)

	mgr := NewConnManager()
	mgr.Register("user-x", conn1)
	mgr.Register("user-y", conn2)

	all := mgr.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	found := make(map[string]bool)
	for uid := range all {
		found[uid] = true
	}
	if !found["user-x"] || !found["user-y"] {
		t.Fatal("expected both user-x and user-y in GetAll")
	}

	t.Log("getall OK")
}

func TestConnManager_RegisterReplaces(t *testing.T) {
	_, conn1 := setupTestWS(t)
	_, conn2 := setupTestWS(t)

	mgr := NewConnManager()
	mgr.Register("user-r", conn1)
	mgr.Register("user-r", conn2)

	if mgr.Count() != 1 {
		t.Fatalf("expected count 1 after replace, got %d", mgr.Count())
	}

	entry, ok := mgr.Get("user-r")
	if !ok {
		t.Fatal("expected user-r to exist")
	}
	if entry.Conn != conn2 {
		t.Fatal("expected conn to be the second (replacement) connection")
	}

	t.Log("register-replace OK")
}

func TestConnManager_SubscribedChannels(t *testing.T) {
	_, conn := setupTestWS(t)
	defer conn.Close()

	mgr := NewConnManager()
	mgr.Register("user-ch", conn)

	entry, ok := mgr.Get("user-ch")
	if !ok {
		t.Fatal("expected user-ch to exist")
	}

	if len(entry.SubscribedChannels) != 0 {
		t.Fatalf("expected 0 channels, got %d", len(entry.SubscribedChannels))
	}

	mgr.AddSubscribedChannel("user-ch", "channel-1")
	mgr.AddSubscribedChannel("user-ch", "channel-2")

	entry, _ = mgr.Get("user-ch")
	if len(entry.SubscribedChannels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(entry.SubscribedChannels))
	}
	if !entry.SubscribedChannels["channel-1"] || !entry.SubscribedChannels["channel-2"] {
		t.Fatal("expected both channel-1 and channel-2 to be subscribed")
	}

	mgr.RemoveSubscribedChannel("user-ch", "channel-1")
	entry, _ = mgr.Get("user-ch")
	if len(entry.SubscribedChannels) != 1 {
		t.Fatalf("expected 1 channel after remove, got %d", len(entry.SubscribedChannels))
	}
	if entry.SubscribedChannels["channel-1"] {
		t.Fatal("expected channel-1 to be removed")
	}

	t.Log("subscribed channels OK")
}

func TestConnManager_ConcurrentAccess(t *testing.T) {
	mgr := NewConnManager()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, conn := setupTestWS(t)
			uid := "user-concurrent-" + string(rune('A'+idx%26))
			mgr.Register(uid, conn)
			mgr.Get(uid)
			mgr.GetAll()
			mgr.Count()
			mgr.Unregister(uid)
		}(i)
	}

	wg.Wait()

	if mgr.Count() != 0 {
		t.Fatalf("expected count 0 after concurrent ops, got %d", mgr.Count())
	}

	t.Log("concurrent access OK")
}
