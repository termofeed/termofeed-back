package ws_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/ws"
)

func TestHub_ConnectAndDisconnectCallbacks(t *testing.T) {
	hub := ws.NewHub()

	var connectedCounts []int
	var disconnectedCounts []int

	hub.SetCallbacks(
		func(code string, count int) {
			connectedCounts = append(connectedCounts, count)
		},
		func(code string, count int) {
			disconnectedCounts = append(disconnectedCounts, count)
		},
	)

	c1 := ws.NewClientConn(hub, nil, "abc-defg-hij", "p1")
	c2 := ws.NewClientConn(hub, nil, "abc-defg-hij", "p2")

	hub.Register(c1)
	hub.Register(c2)

	assert.Equal(t, 2, hub.GetConnectedCount("abc-defg-hij"))
	assert.Equal(t, []int{1, 2}, connectedCounts)

	hub.Unregister(c1)
	assert.Equal(t, 1, hub.GetConnectedCount("abc-defg-hij"))
	assert.Equal(t, []int{1}, disconnectedCounts)

	hub.Unregister(c2)
	assert.Equal(t, 0, hub.GetConnectedCount("abc-defg-hij"))
	assert.Equal(t, []int{1, 0}, disconnectedCounts)
}

func TestHub_Broadcast(t *testing.T) {
	hub := ws.NewHub()
	c1 := ws.NewClientConn(hub, nil, "abc-defg-hij", "p1")
	hub.Register(c1)

	event := ws.ReactionCreatedEvent{
		Type:  ws.EventReactionCreated,
		Emoji: "😂",
		At:    time.Now().UTC(),
	}

	err := hub.Broadcast("abc-defg-hij", event)
	assert.NoError(t, err)

	// Another room should receive nothing
	err = hub.Broadcast("other-room", event)
	assert.NoError(t, err)
}

func TestEvents_Serialization(t *testing.T) {
	event := ws.ReactionCreatedEvent{
		Type:  ws.EventReactionCreated,
		Emoji: "👏",
		At:    time.Date(2026, 11, 18, 21, 0, 3, 0, time.UTC),
	}

	bytes, err := json.Marshal(event)
	assert.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(bytes, &parsed)
	assert.NoError(t, err)

	assert.Equal(t, "reaction.created", parsed["type"])
	assert.Equal(t, "👏", parsed["emoji"])
	assert.Equal(t, "2026-11-18T21:00:03Z", parsed["at"])
}
