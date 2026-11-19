package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	maxMessageQueueSize = 128
	writeTimeout        = 5 * time.Second
)

type ClientConn struct {
	hub           *Hub
	conn          *websocket.Conn
	roomCode      string
	participantID string
	send          chan []byte
	closeOnce     sync.Once
	closed        chan struct{}
}

func NewClientConn(hub *Hub, conn *websocket.Conn, roomCode, participantID string) *ClientConn {
	return &ClientConn{
		hub:           hub,
		conn:          conn,
		roomCode:      roomCode,
		participantID: participantID,
		send:          make(chan []byte, maxMessageQueueSize),
		closed:        make(chan struct{}),
	}
}

func (c *ClientConn) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.conn.Close(websocket.StatusNormalClosure, "closing connection")
	})
}

func (c *ClientConn) Send(msg []byte) bool {
	select {
	case <-c.closed:
		return false
	default:
	}

	select {
	case c.send <- msg:
		return true
	default:
		// Queue is full: slow client, close connection
		slog.Warn("client send queue full, closing slow client",
			"room", c.roomCode,
			"participantID", c.participantID,
		)
		go c.Close()
		return false
	}
}

func (c *ClientConn) WritePump(ctx context.Context) {
	defer func() {
		c.hub.Unregister(c)
		c.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
					slog.Debug("websocket write error", "err", err)
				}
				return
			}
		}
	}
}

func (c *ClientConn) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.Unregister(c)
		c.Close()
	}()

	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				slog.Debug("websocket client disconnected", "err", err)
			}
			return
		}
		// Termofeed participants only receive events over WebSocket; incoming payloads are ignored.
	}
}

type Hub struct {
	mu           sync.RWMutex
	rooms        map[string]map[*ClientConn]struct{}
	onConnect    func(roomCode string, connectedCount int)
	onDisconnect func(roomCode string, connectedCount int)
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[*ClientConn]struct{}),
	}
}

func (h *Hub) SetCallbacks(onConnect, onDisconnect func(roomCode string, connectedCount int)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onConnect = onConnect
	h.onDisconnect = onDisconnect
}

func (h *Hub) Register(c *ClientConn) {
	h.mu.Lock()
	clients, ok := h.rooms[c.roomCode]
	if !ok {
		clients = make(map[*ClientConn]struct{})
		h.rooms[c.roomCode] = clients
	}
	clients[c] = struct{}{}
	count := len(clients)
	cb := h.onConnect
	h.mu.Unlock()

	if cb != nil {
		cb(c.roomCode, count)
	}
}

func (h *Hub) Unregister(c *ClientConn) {
	h.mu.Lock()
	clients, ok := h.rooms[c.roomCode]
	if !ok {
		h.mu.Unlock()
		return
	}
	if _, found := clients[c]; !found {
		h.mu.Unlock()
		return
	}

	delete(clients, c)
	count := len(clients)
	if count == 0 {
		delete(h.rooms, c.roomCode)
	}
	cb := h.onDisconnect
	h.mu.Unlock()

	if cb != nil {
		cb(c.roomCode, count)
	}
}

func (h *Hub) GetConnectedCount(roomCode string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.rooms[roomCode])
}

func (h *Hub) Broadcast(roomCode string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	clients := make([]*ClientConn, 0, len(h.rooms[roomCode]))
	for c := range h.rooms[roomCode] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.Send(data)
	}
	return nil
}

// BroadcastAndClose broadcasts a final event and then closes all connections for the room.
func (h *Hub) BroadcastAndClose(roomCode string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.Lock()
	clients := make([]*ClientConn, 0, len(h.rooms[roomCode]))
	for c := range h.rooms[roomCode] {
		clients = append(clients, c)
	}
	delete(h.rooms, roomCode)
	h.mu.Unlock()

	for _, c := range clients {
		c.Send(data)
		go func(conn *ClientConn) {
			// Allow brief moment for send pump to flush final message
			time.Sleep(100 * time.Millisecond)
			conn.Close()
		}(c)
	}

	return nil
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	var allClients []*ClientConn
	for _, clients := range h.rooms {
		for c := range clients {
			allClients = append(allClients, c)
		}
	}
	h.rooms = make(map[string]map[*ClientConn]struct{})
	h.mu.Unlock()

	for _, c := range allClients {
		c.Close()
	}
}
